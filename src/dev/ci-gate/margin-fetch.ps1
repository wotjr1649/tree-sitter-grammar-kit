#Requires -Version 7
# Dot-sourced only by fetch.ps1: Go children have no token in their environment.
function Get-CIMarginEvidence {
  param($Client, $Cancellation, [string]$Token, [string]$Gate, [string]$Expected, [string]$Work)
  $root = Join-Path $Work 'margin'
  $metadata = Join-Path $root 'metadata'
  New-Item -ItemType Directory -Path $metadata | Out-Null
  $endpoint = 'https://api.github.com/repos/wotjr1649/tree-sitter-grammar-kit/actions'
  # The jobs API can trail the finished producers this gate needs: fetch a new snapshot while it
  # still shows a measured job or step running; the margin check itself stays unchanged.
  $jobs = Join-Path $metadata 'jobs.json'
  for ($try = 1; ; $try++) {
    Get-CIGateFile -Client $Client -Uri "$endpoint/runs/$env:GITHUB_RUN_ID/attempts/$env:GITHUB_RUN_ATTEMPT/jobs?per_page=100" -Out $jobs -Limit 1048576 -Cancellation $Cancellation -Token $Token
    & $Gate margin-settled -expected $Expected -history $root
    if ($LASTEXITCODE -eq 0 -or ($LASTEXITCODE -eq 3 -and $try -ge 4)) { break }
    if ($LASTEXITCODE -ne 3) { throw 'margin job settlement check failed' }
    Remove-Item -LiteralPath $jobs
    # inside the existing 120 s collection deadline; a longer API outage still fails below
    if ($Cancellation.WaitHandle.WaitOne([TimeSpan]::FromSeconds(15))) { throw 'margin settlement deadline' }
  }
  $cutoff = [DateTime]::UtcNow
  @{ cutoff = $cutoff.ToString('yyyy-MM-ddTHH:mm:ssZ') } | ConvertTo-Json -Compress | Set-Content -LiteralPath (Join-Path $metadata 'cutoff.json') -Encoding utf8NoBOM
  $window = [Uri]::EscapeDataString($cutoff.AddDays(-30).ToString('yyyy-MM-ddTHH:mm:ssZ') + '..' + $cutoff.ToString('yyyy-MM-ddTHH:mm:ssZ'))
  for ($page = 1; $page -le 20; $page++) {
    $path = Join-Path $metadata "runs-$page.json"
    Get-CIGateFile -Client $Client -Uri "$endpoint/workflows/foundation.yml/runs?branch=main&exclude_pull_requests=true&created=$window&per_page=50&page=$page" -Out $path -Limit 1048576 -Cancellation $Cancellation -Token $Token
    try { $listing = [IO.File]::ReadAllText($path) | ConvertFrom-Json -AsHashtable } catch { throw 'margin run pagination JSON invalid' }
    if ($listing -isnot [Collections.IDictionary] -or ($listing.total_count -isnot [long] -and $listing.total_count -isnot [int]) -or $listing.total_count -lt 0 -or $listing.total_count -gt 1000) { throw 'HISTORY_WINDOW_INCOMPLETE' }
    if ($page * 50 -ge $listing.total_count) { break }
  }
  $discover = Join-Path $root 'runs.json'
  & $Gate margin-discover -expected $Expected -metadata (Join-Path $Work 'metadata') -history $metadata -out $discover
  if ($LASTEXITCODE -ne 0) { throw 'margin history discovery failed' }
  $runs = @([IO.File]::ReadAllText($discover) | ConvertFrom-Json -AsHashtable)
  if ($runs.Count -gt 60) { throw 'margin history run limit' }
  foreach ($run in $runs) {
    $id = [string]$run.id
    if ($id -cnotmatch '^[1-9][0-9]{0,19}$') { throw 'margin history ID invalid' }
    Get-CIGateFile -Client $Client -Uri "$endpoint/runs/$id/artifacts?per_page=100&page=1" -Out (Join-Path $metadata "artifacts-$id.json") -Limit 1048576 -Cancellation $Cancellation -Token $Token
  }
  $planPath = Join-Path $root 'plan.json'
  & $Gate margin-plan -expected $Expected -metadata (Join-Path $Work 'metadata') -history $metadata -out $planPath
  if ($LASTEXITCODE -ne 0) { throw 'margin history plan failed' }
  $plan = [IO.File]::ReadAllText($planPath) | ConvertFrom-Json -AsHashtable
  $artifacts = @($plan.current) + @($plan.history | ForEach-Object { $_.artifact })
  if ($artifacts.Count -lt 9 -or $artifacts.Count -gt 69) { throw 'margin artifact count invalid' }
  $zips = Join-Path $root 'zips'
  New-Item -ItemType Directory -Path $zips | Out-Null
  foreach ($artifact in $artifacts) {
    $id = [string]$artifact.id
    $name = [string]$artifact.name
    $size = $artifact.size_in_bytes
    if ($id -cnotmatch '^[1-9][0-9]{0,19}$' -or $name -cnotmatch '^ci-(margin|config-(foundation|native-prepare|native-routes|native-routes-macos|large-api|qualification)-(windows|linux|darwin))-[1-9][0-9]{0,19}-[1-9][0-9]{0,19}$' -or $size -le 0 -or $size -gt 262144) { throw 'margin download plan changed' }
    Get-CIGateFile -Client $Client -Uri "$endpoint/artifacts/$id/zip" -Out (Join-Path $zips "$name.zip") -Limit $size -Cancellation $Cancellation -Token $Token
  }
}
