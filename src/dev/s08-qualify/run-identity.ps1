#Requires -Version 7
# S08 development helper: writes the host's tsgk-run-identity/r1 document into a route run
# directory (run-routes.ps1 -Destination) from the named GitHub Actions run variables, the
# actual Go host and the checkout. Only the listed variables are read; nothing else from
# the environment is recorded. -HeadSHA is the pull request head (the checkout on push).
param(
  [Parameter(Mandatory)][string]$Destination,
  [string]$HeadSHA = ''
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (-not (Test-Path -LiteralPath $Destination -PathType Container)) { throw 'route run directory missing' }
$out = Join-Path $Destination 'run-identity.json'
if (Test-Path -LiteralPath $out) { throw 'run identity exists' }
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
Push-Location $repo
try {
  $checkout = @(git rev-parse HEAD)
  if ($LASTEXITCODE -ne 0) { throw 'checkout identity failed' }
  $goEnv = @(go env GOOS GOARCH GOVERSION)
  if ($LASTEXITCODE -ne 0 -or $goEnv.Count -ne 3) { throw 'go env failed' }
} finally { Pop-Location }
foreach ($n in @('GITHUB_REPOSITORY', 'GITHUB_WORKFLOW', 'GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT', 'GITHUB_EVENT_NAME', 'GITHUB_SHA', 'GITHUB_JOB', 'RUNNER_OS', 'RUNNER_ARCH')) {
  if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($n))) { throw "run variable missing: $n" }
}
$head = if ($HeadSHA) { $HeadSHA } else { $env:GITHUB_SHA }
$image = '{0}/{1}' -f [Environment]::GetEnvironmentVariable('ImageOS'), [Environment]::GetEnvironmentVariable('ImageVersion')
$id = [ordered]@{
  schema = 'tsgk-run-identity/r1'; repository = $env:GITHUB_REPOSITORY; workflow = $env:GITHUB_WORKFLOW; run_id = $env:GITHUB_RUN_ID; run_attempt = $env:GITHUB_RUN_ATTEMPT
  event = $env:GITHUB_EVENT_NAME; sha = $env:GITHUB_SHA; head_sha = $head; checkout = $checkout[0].Trim(); job = $env:GITHUB_JOB; runner_os = $env:RUNNER_OS
  runner_arch = $env:RUNNER_ARCH; image = $image; goos = $goEnv[0].Trim(); goarch = $goEnv[1].Trim(); go_version = $goEnv[2].Trim(); evidence_mode = 'NEW_RUN'
}
$id | ConvertTo-Json | Set-Content -LiteralPath $out -Encoding utf8NoBOM
Write-Output ("run identity: run={0} attempt={1} event={2} checkout={3} host={4}/{5}" -f $id.run_id, $id.run_attempt, $id.event, $id.checkout, $id.goos, $id.goarch)
exit 0
