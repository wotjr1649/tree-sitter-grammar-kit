#Requires -Version 7
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$dest = Join-Path $env:RUNNER_TEMP "ci-config-$env:GITHUB_JOB-$env:RUNNER_OS-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"
if (Test-Path -LiteralPath $dest) { throw 'margin configuration output exists' }
New-Item -ItemType Directory -Path $dest | Out-Null
& (Join-Path $PSScriptRoot '../s08-qualify/run-identity.ps1') -Destination $dest -HeadSHA $env:PR_HEAD_SHA
if ($LASTEXITCODE -ne 0) { throw 'margin configuration identity failed' }
$cc = ''
if ($env:GITHUB_JOB -cin @('foundation', 'native-routes', 'large-api')) {
  if ($env:TSGK_NATIVE_CC) { $cc = $env:TSGK_NATIVE_CC }
  else { $cc = & (Join-Path $PSScriptRoot '../s05-native/select-compiler.ps1') }
  if (!$cc -or !(Test-Path -LiteralPath $cc -PathType Leaf)) { throw 'selected margin compiler missing' }
}
$taskConfigArgs = @('run', './src/dev/ci-gate', 'config', '-expected', (Join-Path $dest 'run-identity.json'), '-out', (Join-Path $dest 'config.json'))
if ($cc) { $taskConfigArgs += @('-compiler', $cc) }
go @taskConfigArgs
if ($LASTEXITCODE -ne 0) { throw 'margin configuration capture failed' }
