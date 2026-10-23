#Requires -Version 7
param([Parameter(Mandatory)][string]$Destination)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$clock = [Diagnostics.Stopwatch]::StartNew()
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
  public readonly List<string> Hosts = new();
  protected override HttpResponseMessage Send(HttpRequestMessage r, CancellationToken c) {
    c.ThrowIfCancellationRequested();
    Credentials.Add(r.Headers.Authorization == null ? "" : r.Headers.Authorization.ToString());
    Hosts.Add(r.RequestUri.Host);
    var next = Responses.Dequeue();
    if (next == null) throw new HttpRequestException("transport reset UNIT_ONLY");
    return next;
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
Write-Output "Add-Type ms=$($clock.ElapsedMilliseconds)"
function Response([int]$Code, [string]$Body, [string]$Location = '') {
  $r = [Net.Http.HttpResponseMessage]::new([Net.HttpStatusCode]$Code)
  $r.Content = [Net.Http.ByteArrayContent]::new([Text.Encoding]::UTF8.GetBytes($Body))
  if ($Location) { $r.Headers.Location = [uri]$Location }
  return $r
}
function Partial { $r = Response 200 'ok'; $r.Content.Headers.ContentLength = 4; return $r }
$retried = @('transient-then-ok', 'transport-then-ok', 'partial-then-ok', 'redirect-retry')
$cases = @('redirect', 'http-redirect', 'external-redirect', 'user-info', 'redirect-limit', 'http-failure', 'client-error', 'oversize', 'actual-oversize', 'partial', 'cancelled', 'mid-cancel', 'overwrite') + $retried
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
      'http-failure' { for ($i=0; $i -lt 3; $i++) { $handler.Responses.Enqueue((Response 503 'private response UNIT_ONLY')) } }
      'client-error' { $handler.Responses.Enqueue((Response 404 'private response UNIT_ONLY')) }
      'transient-then-ok' { $handler.Responses.Enqueue((Response 502 '')); $handler.Responses.Enqueue((Response 200 'ok')) }
      'transport-then-ok' { $handler.Responses.Enqueue($null); $handler.Responses.Enqueue((Response 200 'ok')) }
      'partial-then-ok' { $handler.Responses.Enqueue((Partial)); $handler.Responses.Enqueue((Response 200 'ok')) }
      'redirect-retry' { foreach ($r in @((Response 302 '' 'https://storage.blob.core.windows.net/test?signature=UNIT_ONLY'), (Response 503 ''), (Response 302 '' 'https://storage.blob.core.windows.net/test?signature=UNIT_ONLY'), (Response 200 'ok'))) { $handler.Responses.Enqueue($r) } }
      'oversize' { $handler.Responses.Enqueue((Response 200 '12345')) }
      'actual-oversize' { $r = Response 200 ''; $r.Content = [CIGateUnknownLength]::new([Text.Encoding]::UTF8.GetBytes('12345')); $handler.Responses.Enqueue($r) }
      'partial' { for ($i=0; $i -lt 3; $i++) { $handler.Responses.Enqueue((Partial)) } }
      'cancelled' { $deadline.Cancel(); $handler.Responses.Enqueue((Response 200 'ok')) }
      'mid-cancel' { $r = Response 200 ''; $r.Content = [CIGateCancelContent]::new($deadline); $handler.Responses.Enqueue($r) }
      'overwrite' { [IO.File]::WriteAllText($out, 'preserve'); $handler.Responses.Enqueue((Response 200 'ok')) }
    }
    $caught = $null
    try { Get-CIGateFile -Client $client -Uri 'https://api.github.com/artifact' -Out $out -Limit 4 -Cancellation $deadline.Token -Token 'UNIT_TEST_ONLY' -RetryDelay ([TimeSpan]::FromMilliseconds(1)) } catch { $caught = $_.Exception.Message }
    $sent = ($handler.Hosts | ForEach-Object { $_ }) -join ','
    $credentials = ($handler.Credentials | ForEach-Object { if ($_) { 'token' } else { '-' } }) -join ','
    if ($case -eq 'redirect') {
      if ($caught -or [IO.File]::ReadAllText($out) -ne 'ok' -or $handler.Credentials.Count -ne 2 -or $handler.Credentials[0] -ne 'Bearer UNIT_TEST_ONLY' -or $handler.Credentials[1] -ne '') { throw 'redirect credential control failed' }
    } elseif ($case -in $retried) {
      # every try restarts at the GitHub endpoint; the credential never reaches the redirect target
      $want = if ($case -eq 'redirect-retry') { 'api.github.com,storage.blob.core.windows.net,api.github.com,storage.blob.core.windows.net|token,-,token,-' } else { 'api.github.com,api.github.com|token,token' }
      if ($caught -or [IO.File]::ReadAllText($out) -ne 'ok' -or "$sent|$credentials" -ne $want -or $handler.Responses.Count -ne 0) { throw "transient retry control failed: $case" }
    } else {
      if (!$caught -or $caught -ne 'GitHub download failed' -or $caught.Contains('UNIT_ONLY')) { throw 'failure/redaction control failed' }
      if ($case -in @('http-redirect', 'external-redirect', 'user-info', 'client-error') -and $handler.Credentials.Count -ne 1) { throw 'unsafe redirect followed or permanent failure retried' }
      if ($case -in @('http-failure', 'partial') -and ($handler.Credentials.Count -ne 3 -or $credentials -ne 'token,token,token')) { throw 'transient failure not bounded to three tries' }
      if ($case -eq 'overwrite' -and $handler.Credentials.Count -ne 0) { throw 'request sent for an existing output' }
      if ($case -eq 'overwrite' -and [IO.File]::ReadAllText($out) -ne 'preserve') { throw 'existing output overwritten' }
      if ($case -eq 'mid-cancel' -and (!$deadline.IsCancellationRequested -or (Get-Item -LiteralPath $out).Length -ne 1)) { throw 'copy was not interrupted after its first read' }
      if ($case -eq 'actual-oversize' -and (Get-Item -LiteralPath $out).Length -gt 4) { throw 'received bytes exceeded output bound' }
    }
  } finally {
    $deadline.Dispose()
    $client.Dispose()
  }
}
Write-Output "PASS HTTP transport controls=$($cases.Count) total ms=$($clock.ElapsedMilliseconds)"
