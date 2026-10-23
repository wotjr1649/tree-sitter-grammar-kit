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
  public CancellationTokenSource CancelOnSend;
  public int CancelDelay;
  protected override HttpResponseMessage Send(HttpRequestMessage r, CancellationToken c) {
    c.ThrowIfCancellationRequested();
    Credentials.Add(r.Headers.Authorization == null ? "" : r.Headers.Authorization.ToString());
    Hosts.Add(r.RequestUri.Host);
    var next = Responses.Dequeue();
    if (CancelOnSend != null) CancelOnSend.CancelAfter(CancelDelay);
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
$cases = @('redirect', 'http-redirect', 'external-redirect', 'user-info', 'redirect-limit', 'http-failure', 'client-error', 'oversize', 'actual-oversize', 'partial', 'cancelled', 'mid-cancel', 'overwrite', 'output-unwritable', 'backoff-cancel') + $retried
foreach ($case in $cases) {
  $handler = [CIGateTestHandler]::new()
  $client = [Net.Http.HttpClient]::new($handler)
  $deadline = [Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(10))
  $out = Join-Path $Destination "$case.json"
  $delay = [TimeSpan]::FromMilliseconds(1)
  try {
    switch ($case) {
      'redirect' { $handler.Responses.Enqueue((Response 302 '' 'https://storage.blob.core.windows.net/test?signature=UNIT_ONLY')); $handler.Responses.Enqueue((Response 200 'ok')) }
      'http-redirect' { $handler.Responses.Enqueue((Response 302 '' 'http://example.invalid/test?token=UNIT_ONLY')) }
      'external-redirect' { $handler.Responses.Enqueue((Response 302 '' 'https://api.github.com.evil.invalid/UNIT_ONLY')) }
      'user-info' { $handler.Responses.Enqueue((Response 302 '' 'https://UNIT_ONLY@api.github.com/test')) }
      'redirect-limit' { for ($i=0; $i -lt 4; $i++) { $handler.Responses.Enqueue((Response 302 '' 'https://storage.blob.core.windows.net/test?signature=UNIT_ONLY')) } }
      'http-failure' { for ($i=0; $i -lt 3; $i++) { $handler.Responses.Enqueue((Response 503 'private response UNIT_ONLY')) } }
      'client-error' { $handler.Responses.Enqueue((Response 404 'private response UNIT_ONLY')) }
      'output-unwritable' { $out = Join-Path $Destination 'absent-directory/output.json'; $handler.Responses.Enqueue((Response 200 'ok')) }
      # the deadline ends inside the 30 s back-off (or, on a stalled host, just before it)
      'backoff-cancel' { $delay = [TimeSpan]::FromSeconds(30); $handler.CancelOnSend = $deadline; $handler.CancelDelay = 100; $handler.Responses.Enqueue((Response 503 '')) }
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
    $started = $clock.ElapsedMilliseconds
    try { Get-CIGateFile -Client $client -Uri 'https://api.github.com/artifact' -Out $out -Limit 4 -Cancellation $deadline.Token -Token 'UNIT_TEST_ONLY' -RetryDelay $delay } catch { $caught = $_.Exception.Message }
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
      # deterministic failures and cancellation are never tried again
      $once = @{ 'oversize' = 1; 'actual-oversize' = 1; 'redirect-limit' = 4; 'cancelled' = 0; 'mid-cancel' = 1; 'output-unwritable' = 1; 'backoff-cancel' = 1 }
      if ($once.ContainsKey($case) -and $handler.Credentials.Count -ne $once[$case]) { throw "non-transient failure retried: $case" }
      if ($case -eq 'backoff-cancel' -and $clock.ElapsedMilliseconds - $started -gt 10000) { throw 'retry back-off ignored the deadline' }
      if ($case -eq 'overwrite' -and [IO.File]::ReadAllText($out) -ne 'preserve') { throw 'existing output overwritten' }
      if ($case -eq 'mid-cancel' -and (!$deadline.IsCancellationRequested -or (Get-Item -LiteralPath $out).Length -ne 1)) { throw 'copy was not interrupted after its first read' }
      if ($case -eq 'actual-oversize' -and (Get-Item -LiteralPath $out).Length -gt 4) { throw 'received bytes exceeded output bound' }
    }
  } finally {
    $deadline.Dispose()
    $client.Dispose()
  }
}
# #275 listing loop: a fake offline helper returns scripted exit codes and records how many
# metadata files it saw, so stale listing pages and the try bound are both observable.
function Listing([int]$Total) { Response 200 ('{"total_count":' + $Total + ',"artifacts":[]}') }
$plans = [ordered]@{
  'lag-then-ok' = @{ codes = @(3, 0); responses = @((Response 200 '{}'), (Listing 150), (Listing 150), (Response 200 '{}'), (Listing 50)); seen = '3,2'; error = '' }
  'lag-exhausted' = @{ codes = @(3, 3, 3); responses = @((Response 200 '{}'), (Listing 1), (Response 200 '{}'), (Listing 1), (Response 200 '{}'), (Listing 1)); seen = '2,2,2'; error = 'artifact plan failed' }
  'earlier-exhausted' = @{ codes = @(4, 4, 4); responses = @((Response 200 '{}'), (Listing 1), (Response 200 '{}'), (Listing 1), (Response 200 '{}'), (Listing 1)); seen = '2,2,2'; error = 'partial re-run: a required artifact exists only for an earlier attempt; re-run all jobs' }
  'defect' = @{ codes = @(1); responses = @((Response 200 '{}'), (Listing 1)); seen = '2'; error = 'artifact plan failed' }
  'listing-cancel' = @{ codes = @(3); responses = @((Response 200 '{}'), (Listing 1)); seen = '2'; error = 'artifact listing deadline' }
}
foreach ($name in $plans.Keys) {
  $p = $plans[$name]
  $dir = Join-Path $Destination "plan-$name"
  $metadata = Join-Path $dir 'metadata'
  New-Item -ItemType Directory -Path $metadata | Out-Null
  Set-Content -LiteralPath (Join-Path $dir 'codes.txt') -Value $p.codes
  $gate = Join-Path $dir 'gate.ps1'
  Set-Content -LiteralPath $gate -Value @'
$dir = Split-Path -Parent $MyInvocation.MyCommand.Path
$calls = @(Get-Content -LiteralPath (Join-Path $dir 'calls.txt') -ErrorAction SilentlyContinue).Count
Add-Content -LiteralPath (Join-Path $dir 'calls.txt') -Value (Get-ChildItem -LiteralPath $args[4] -File).Count
if ($global:CIGateListingCancel) { $global:CIGateListingCancel.Cancel() }
exit [int]@(Get-Content -LiteralPath (Join-Path $dir 'codes.txt'))[$calls]
'@
  $handler = [CIGateTestHandler]::new()
  foreach ($r in $p.responses) { $handler.Responses.Enqueue($r) }
  $client = [Net.Http.HttpClient]::new($handler)
  $deadline = [Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(10))
  $wait = [TimeSpan]::FromMilliseconds(1)
  # the deadline ends while the helper reports lag, so the wait must observe it
  $global:CIGateListingCancel = if ($name -eq 'listing-cancel') { $wait = [TimeSpan]::FromSeconds(30); $deadline } else { $null }
  try {
    $caught = ''
    try { Get-CIGatePlan -Client $client -Endpoint 'https://api.github.com/run' -Gate $gate -Expected 'unused' -Metadata $metadata -PlanPath (Join-Path $dir 'plan.json') -Cancellation $deadline.Token -Token 'UNIT_TEST_ONLY' -ListingDelay $wait } catch { $caught = $_.Exception.Message }
    $seen = @(Get-Content -LiteralPath (Join-Path $dir 'calls.txt')) -join ','
    if ($caught -ne $p.error -or $seen -ne $p.seen -or $handler.Responses.Count -ne 0) { throw "listing loop control failed: $name ($caught; saw $seen; $($handler.Responses.Count) responses left)" }
  } finally {
    $deadline.Dispose()
    $client.Dispose()
  }
}
Write-Output "PASS HTTP transport controls=$($cases.Count) listing controls=$($plans.Count) total ms=$($clock.ElapsedMilliseconds)"
