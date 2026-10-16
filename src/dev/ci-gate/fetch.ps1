#Requires -Version 7
param(
  [Parameter(Mandatory)][string]$Gate,
  [Parameter(Mandatory)][string]$Expected,
  [Parameter(Mandatory)][string]$Work
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'http.ps1')
. (Join-Path $PSScriptRoot 'margin-fetch.ps1')
if ($env:GITHUB_REPOSITORY -cne 'wotjr1649/tree-sitter-grammar-kit' -or $env:GITHUB_RUN_ID -cnotmatch '^[1-9][0-9]{0,19}$' -or $env:GITHUB_RUN_ATTEMPT -cnotmatch '^[1-9][0-9]{0,19}$') { throw 'workflow identity invalid' }
$metadata = Join-Path $Work 'metadata'
$planPath = Join-Path $Work 'plan.json'
$destination = Join-Path $Work 'artifacts'
if ((Test-Path -LiteralPath $metadata) -or (Test-Path -LiteralPath $planPath) -or (Test-Path -LiteralPath $destination)) { throw 'fetch needs new outputs' }
New-Item -ItemType Directory -Path $metadata | Out-Null
$token = [Environment]::GetEnvironmentVariable('GH_TOKEN')
# Go subprocesses never receive a credential; only the fixed HTTP transport uses it.
[Environment]::SetEnvironmentVariable('GH_TOKEN', $null)
$handler = [Net.Http.HttpClientHandler]::new()
$handler.AllowAutoRedirect = $false
$client = [Net.Http.HttpClient]::new($handler)
$client.Timeout = [TimeSpan]::FromSeconds(120)
$deadline = [Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(365))
try {
  $endpoint = "https://api.github.com/repos/wotjr1649/tree-sitter-grammar-kit/actions/runs/$env:GITHUB_RUN_ID"
  Get-CIGateFile -Client $client -Uri $endpoint -Out (Join-Path $metadata 'run.json') -Limit 1048576 -Cancellation $deadline.Token -Token $token
  for ($page = 1; $page -le 10; $page++) {
    $listingPath = Join-Path $metadata "listing-$page.json"
    Get-CIGateFile -Client $client -Uri "$endpoint/artifacts?per_page=100&page=$page" -Out $listingPath -Limit 1048576 -Cancellation $deadline.Token -Token $token
    # This only bounds pagination; Go validates the original JSON before selecting IDs.
    try { $listing = [IO.File]::ReadAllText($listingPath) | ConvertFrom-Json -AsHashtable } catch { throw 'artifact pagination JSON invalid' }
    if ($listing -isnot [Collections.IDictionary]) { throw 'artifact pagination object invalid' }
    $total = $listing['total_count']
    if (($total -isnot [long] -and $total -isnot [int]) -or $total -lt 0 -or $total -gt 1000) { throw 'artifact pagination invalid' }
    if ($page * 100 -ge $total) { break }
  }
  $deadline.Token.ThrowIfCancellationRequested()
  & $Gate plan -expected $Expected -metadata $metadata -out $planPath
  if ($LASTEXITCODE -ne 0) { throw 'artifact plan failed' }
  $plan = [IO.File]::ReadAllText($planPath) | ConvertFrom-Json -AsHashtable
  New-Item -ItemType Directory -Path $destination | Out-Null
  foreach ($artifact in $plan['artifacts']) {
    $id = [string]$artifact['id']
    $name = [string]$artifact['name']
    $size = $artifact['size_in_bytes']
    if ($id -cnotmatch '^[1-9][0-9]{0,19}$' -or $name -cnotmatch '^(s05-routes-(windows|linux|darwin)|s08-qualification|large-api)-[1-9][0-9]{0,19}-[1-9][0-9]{0,19}$' -or $size -le 0 -or $size -gt 104857600) { throw 'validated plan changed' }
    Get-CIGateFile -Client $client -Uri "https://api.github.com/repos/wotjr1649/tree-sitter-grammar-kit/actions/artifacts/$id/zip" -Out (Join-Path $destination "$name.zip") -Limit $size -Cancellation $deadline.Token -Token $token
  }
  & $Gate unpack -expected $Expected -plan $planPath -artifacts $destination
  if ($LASTEXITCODE -ne 0) { throw 'artifact digest/extraction failed' }
  $marginDeadline = [Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(120))
  try {
    Get-CIMarginEvidence -Client $client -Cancellation $marginDeadline.Token -Token $token -Gate $Gate -Expected $Expected -Work $Work
  } finally { $marginDeadline.Dispose() }
} finally {
  $deadline.Dispose()
  $client.Dispose()
  $token = $null
}
