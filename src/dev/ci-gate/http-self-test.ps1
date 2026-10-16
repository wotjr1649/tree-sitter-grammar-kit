#Requires -Version 7
param([Parameter(Mandatory)][string]$Destination)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'http.ps1')
Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Net.Http;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
public sealed class CIGateTestHandler : HttpMessageHandler {
  public readonly Queue<HttpResponseMessage> Responses = new();
  public readonly List<string> Credentials = new();
  protected override HttpResponseMessage Send(HttpRequestMessage r, CancellationToken c) {
    c.ThrowIfCancellationRequested();
    Credentials.Add(r.Headers.Authorization == null ? "" : r.Headers.Authorization.ToString());
    return Responses.Dequeue();
  }
  protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage r, CancellationToken c) => Task.FromResult(Send(r,c));
}
public sealed class CIGateUnknownLength : ByteArrayContent {
  public CIGateUnknownLength(byte[] bytes) : base(bytes) {}
  protected override bool TryComputeLength(out long n) { n=0; return false; }
}
public sealed class CIGateCancelStream : MemoryStream {
  private readonly CancellationTokenSource cancel;
  public CIGateCancelStream(CancellationTokenSource c) : base(new byte[16]) { cancel=c; }
  public override Task<int> ReadAsync(byte[] b,int o,int n,CancellationToken c) {
    int count=base.Read(b,o,Math.Min(n,1)); cancel.Cancel(); return Task.FromResult(count);
  }
}
public sealed class CIGateCancelContent : HttpContent {
  private readonly CancellationTokenSource cancel;
  public CIGateCancelContent(CancellationTokenSource c) {cancel=c;}
  protected override bool TryComputeLength(out long n) {n=0;return false;}
  protected override Stream CreateContentReadStream(CancellationToken c) => new CIGateCancelStream(cancel);
  protected override Task SerializeToStreamAsync(Stream s,System.Net.TransportContext c) => throw new InvalidOperationException("stream override required");
}
'@
function Response([int]$Code, [string]$Body, [string]$Location = '') {
  $r = [Net.Http.HttpResponseMessage]::new([Net.HttpStatusCode]$Code)
  $r.Content = [Net.Http.ByteArrayContent]::new([Text.Encoding]::UTF8.GetBytes($Body))
  if ($Location) { $r.Headers.Location = [uri]$Location }
  return $r
}
$cases = @('redirect', 'http-redirect', 'external-redirect', 'user-info', 'redirect-limit', 'http-failure', 'oversize', 'actual-oversize', 'partial', 'cancelled', 'mid-cancel', 'overwrite')
foreach ($case in $cases) {
  $handler = [CIGateTestHandler]::new()
  $client = [Net.Http.HttpClient]::new($handler)
  $deadline = [Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(10))
  $out = Join-Path $Destination "$case.json"
  try {
    switch ($case) {
      'redirect' { $handler.Responses.Enqueue((Response 302 '' 'https://storage.blob.core.windows.net/test?signature=UNIT_ONLY')); $handler.Responses.Enqueue((Response 200 'ok')) }
      'http-redirect' { $handler.Responses.Enqueue((Response 302 '' 'http://example.invalid/test?token=UNIT_ONLY')) }
      'external-redirect' { $handler.Responses.Enqueue((Response 302 '' 'https://api.github.com.evil.invalid/UNIT_ONLY')) }
      'user-info' { $handler.Responses.Enqueue((Response 302 '' 'https://UNIT_ONLY@api.github.com/test')) }
      'redirect-limit' { for ($i=0; $i -lt 4; $i++) { $handler.Responses.Enqueue((Response 302 '' 'https://storage.blob.core.windows.net/test?signature=UNIT_ONLY')) } }
      'http-failure' { $handler.Responses.Enqueue((Response 503 'private response UNIT_ONLY')) }
      'oversize' { $handler.Responses.Enqueue((Response 200 '12345')) }
      'actual-oversize' { $r = Response 200 ''; $r.Content = [CIGateUnknownLength]::new([Text.Encoding]::UTF8.GetBytes('12345')); $handler.Responses.Enqueue($r) }
      'partial' { $r = Response 200 'ok'; $r.Content.Headers.ContentLength = 4; $handler.Responses.Enqueue($r) }
      'cancelled' { $deadline.Cancel(); $handler.Responses.Enqueue((Response 200 'ok')) }
      'mid-cancel' { $r = Response 200 ''; $r.Content = [CIGateCancelContent]::new($deadline); $handler.Responses.Enqueue($r) }
      'overwrite' { [IO.File]::WriteAllText($out, 'preserve'); $handler.Responses.Enqueue((Response 200 'ok')) }
    }
    $caught = $null
    try { Get-CIGateFile -Client $client -Uri 'https://api.github.com/artifact' -Out $out -Limit 4 -Cancellation $deadline.Token -Token 'UNIT_TEST_ONLY' } catch { $caught = $_.Exception.Message }
    if ($case -eq 'redirect') {
      if ($caught -or [IO.File]::ReadAllText($out) -ne 'ok' -or $handler.Credentials.Count -ne 2 -or $handler.Credentials[0] -ne 'Bearer UNIT_TEST_ONLY' -or $handler.Credentials[1] -ne '') { throw 'redirect credential control failed' }
    } else {
      if (!$caught -or $caught -ne 'GitHub download failed' -or $caught.Contains('UNIT_ONLY')) { throw 'failure/redaction control failed' }
      if ($case -in @('http-redirect', 'external-redirect', 'user-info') -and $handler.Credentials.Count -ne 1) { throw 'unsafe redirect followed' }
      if ($case -eq 'overwrite' -and [IO.File]::ReadAllText($out) -ne 'preserve') { throw 'existing output overwritten' }
      if ($case -eq 'mid-cancel' -and (!$deadline.IsCancellationRequested -or (Get-Item -LiteralPath $out).Length -ne 1)) { throw 'copy was not interrupted after its first read' }
      if ($case -eq 'actual-oversize' -and (Get-Item -LiteralPath $out).Length -gt 4) { throw 'received bytes exceeded output bound' }
    }
  } finally {
    $deadline.Dispose()
    $client.Dispose()
  }
}
Write-Output "PASS HTTP transport controls=$($cases.Count)"
