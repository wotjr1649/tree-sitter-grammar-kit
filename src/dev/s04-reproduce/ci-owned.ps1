#Requires -Version 7
# S04 hosted check: acquire the registered tree-sitter 0.27.0 asset for this host, verify
# its release digest before any launch, and run `tsgk reproduce` (JSON mode, no JS runtime)
# on the owned fixture src/testdata/reproduce with the registered reference outputs.
# Everything is written under -Destination (outside the checkout).
param(
  [Parameter(Mandatory)][string]$Destination,
  [Parameter(Mandatory)][string]$Platform,   # windows/amd64 | linux/amd64 | darwin/arm64
  [string]$CgroupParent = ''
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if (Test-Path -LiteralPath $Destination) { throw 'destination exists' }
New-Item -ItemType Directory -Path $Destination | Out-Null

$registry = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/reproduction-routes.json') -Raw | ConvertFrom-Json
$asset = $registry.generator.release_assets.$Platform
if (-not $asset) { throw "no registered generator asset for $Platform" }
$url = "https://github.com/tree-sitter/tree-sitter/releases/download/v$($registry.generator.version)/$($asset.asset)"
$gz = Join-Path $Destination $asset.asset
Invoke-WebRequest -Uri $url -OutFile $gz -MaximumRedirection 5 -TimeoutSec 120
$size = (Get-Item -LiteralPath $gz).Length
$sum = (Get-FileHash -LiteralPath $gz -Algorithm SHA256).Hash.ToLowerInvariant()
if ($size -ne $asset.bytes -or $sum -ne $asset.sha256) { throw "asset digest mismatch: $size $sum" }

# Decompress (bounded) only after the digest matched.
$exe = Join-Path $Destination ('tree-sitter' + $(if ($IsWindows) { '.exe' } else { '' }))
$in = [IO.File]::OpenRead($gz)
try {
  $z = [IO.Compression.GZipStream]::new($in, [IO.Compression.CompressionMode]::Decompress)
  $out = [IO.File]::Open($exe, [IO.FileMode]::CreateNew)
  try {
    $buf = [byte[]]::new(1MB); $total = 0
    while (($n = $z.Read($buf, 0, $buf.Length)) -gt 0) {
      $total += $n
      if ($total -gt 64MB) { throw 'decompressed tool over 64 MiB' }
      $out.Write($buf, 0, $n)
    }
  } finally { $out.Dispose(); $z.Dispose() }
} finally { $in.Dispose() }
if (-not $IsWindows) { chmod 0755 $exe; if ($LASTEXITCODE -ne 0) { throw 'chmod failed' } }
$exeInfo = [ordered]@{ name = 'tree-sitter'; version = $registry.generator.version
  sha256 = (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLowerInvariant(); bytes = (Get-Item -LiteralPath $exe).Length }
Write-Output ("generator asset={0} asset_sha256={1} executable_sha256={2} bytes={3}" -f $asset.asset, $sum, $exeInfo.sha256, $exeInfo.bytes)

$fixture = Join-Path $repo 'src/testdata/reproduce'
$expected = Get-Content -LiteralPath (Join-Path $fixture 'expected.json') -Raw | ConvertFrom-Json
$grammar = Join-Path $fixture 'src/grammar.json'
$profile = [ordered]@{
  schema = 'tsgk-reproduce/r1'; id = 's04-owned-json-r1'; route = 'owned-fixture'; mode = 'json'
  generator = $exeInfo; abi = 15; optimize = $true; grammar = 'src/grammar.json'
  inputs = @([ordered]@{ path = 'src/grammar.json'; role = 'grammar_json'
      sha256 = (Get-FileHash -LiteralPath $grammar -Algorithm SHA256).Hash.ToLowerInvariant(); bytes = (Get-Item -LiteralPath $grammar).Length })
  outputs = @($expected.outputs)
  limits = [ordered]@{ wall_seconds = 120; output_bytes = 8388608; storage_bytes = 67108864; memory_bytes = 1073741824
    input_files = 4; input_bytes = 1048576; file_bytes = 1048576 }
}
$profilePath = Join-Path $Destination 'profile.json'
$profile | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $profilePath -Encoding utf8NoBOM
$work = Join-Path $Destination 'work'
New-Item -ItemType Directory -Path $work | Out-Null
$cli = Join-Path $Destination ('tsgk' + $(if ($IsWindows) { '.exe' } else { '' }))
go build -o $cli ./src/cmd/tsgk
if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' }
$cliArgs = @('reproduce', '--root', $fixture, '--profile', $profilePath, '--out', (Join-Path $Destination 'result'), '--work', $work,
  '--tool', "tree-sitter=$exe", '--allow', 'EXEC_GENERATOR')
if ($CgroupParent) { $cliArgs += @('--cgroup-parent', $CgroupParent) }
$line = & $cli @cliArgs
$code = $LASTEXITCODE
Write-Output $line
$rep = $line | ConvertFrom-Json
Write-Output ("reproduce exit={0} status={1} assessment={2} claims={3} backend={4} memory={5}" -f $code, $rep.execution_status, $rep.assessment,
  ($rep.claims | ConvertTo-Json -Compress), $rep.capabilities.backend, $rep.capabilities.memory)
if ($code -ne 0 -or $rep.assessment -ne 'PASS' -or $rep.claims.json_regeneration -ne 'PASS' -or $rep.claims.js_reproduction -ne 'NOT_CLAIMED') {
  throw 'owned fixture reproduction did not pass'
}
