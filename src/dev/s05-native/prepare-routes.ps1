#Requires -Version 7
# S05 development helper: prepares the native build inputs of the registered routes
# (src/contracts/native-routes.json) and the pinned runtime under -Destination.
# Upstream files come from the pinned commit archive (codeload) or, locally, from an
# S01-bound source tree; adopted routes apply the registered literal patch chain, check
# every patched file against its adoption hash and regenerate parser.c with `tsgk
# reproduce` (tree-sitter 0.27.0 + Node 24.21.0, registered digests) unless
# -ReuseGenerated points at already verified outputs. Every file is hash-checked.
param(
  [Parameter(Mandatory)][string]$Destination,
  [Parameter(Mandatory)][string]$Platform,        # windows/amd64 | linux/amd64 | darwin/arm64
  [string]$LocalSources = '',                     # S01 layout: <dir>/<any>/<repo>-<commit>/...
  [string]$LocalRuntime = '',                     # directory holding LICENSE and lib/
  [string]$ReuseGenerated = '',                   # <dir>/<route>/workspace-a/out (local only)
  [string]$TreeSitterExe = '',                    # local verified copy (hash-checked)
  [string]$NodeExe = '',                          # local verified copy (hash-checked)
  [string]$CgroupParent = '',
  [string[]]$Routes = @()
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if (Test-Path -LiteralPath $Destination) { throw 'destination exists' }
New-Item -ItemType Directory -Path $Destination | Out-Null
$Destination = (Resolve-Path $Destination).Path
$registry = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/native-routes.json') -Raw | ConvertFrom-Json
$repro = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/reproduction-routes.json') -Raw | ConvertFrom-Json
$runtimeManifest = Get-Content -LiteralPath (Join-Path $repo 'src/drivers/native-c/runtime-manifest.json') -Raw | ConvertFrom-Json
$stats = [ordered]@{ downloads = @(); downloaded_bytes = 0; regenerated = @(); reused = @(); routes = @() }

function Get-Sha([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Assert-File([string]$Path, [string]$Sha, [long]$Bytes, [string]$What) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "$What missing: $Path" }
  $len = (Get-Item -LiteralPath $Path).Length
  $sum = Get-Sha $Path
  if ($sum -ne $Sha -or $len -ne $Bytes) { throw "$What identity mismatch: $Path $len $sum" }
}
function Get-Download([string]$Url, [string]$Out) {
  Invoke-WebRequest -Uri $Url -OutFile $Out -MaximumRedirection 5 -TimeoutSec 120
  $len = (Get-Item -LiteralPath $Out).Length
  $script:stats.downloads += [ordered]@{ url = $Url; bytes = $len; sha256 = (Get-Sha $Out) }
  $script:stats.downloaded_bytes += $len
}
function Expand-Tar([string]$Archive, [string]$Dir, [string[]]$Members = @()) {
  New-Item -ItemType Directory -Force -Path $Dir | Out-Null
  $tar = if ($IsWindows) { Join-Path $env:SystemRoot 'System32/tar.exe' } else { '/usr/bin/tar' }
  & $tar -xf $Archive -C $Dir @Members
  if ($LASTEXITCODE -ne 0) { throw "tar failed: $Archive" }
}
function Copy-Into([string]$From, [string]$To) {
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $To) | Out-Null
  Copy-Item -LiteralPath $From -Destination $To
}

# ---- repository sources ----
$sourceDirs = @{}
function Get-RepoDir($r) {
  $key = "$($r.repository)@$($r.commit)"
  if ($sourceDirs.ContainsKey($key)) { return $sourceDirs[$key] }
  $name = ($r.repository -split '/')[1] + '-' + $r.commit
  if ($LocalSources) {
    $hit = Get-ChildItem -LiteralPath $LocalSources -Directory | ForEach-Object { Join-Path $_.FullName $name } | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    if (-not $hit) { throw "local source missing: $key" }
  } else {
    $archive = Join-Path $Destination ("archives/" + ($r.repository -replace '/', '--') + "--$($r.commit).tgz")
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $archive) | Out-Null
    Get-Download $r.archive.url $archive
    if ($r.archive.PSObject.Properties['sha256']) { Assert-File $archive $r.archive.sha256 $r.archive.bytes 'archive' }
    $dir = Join-Path $Destination ("src/" + ($r.repository -replace '/', '--'))
    Expand-Tar $archive $dir
    $hit = Join-Path $dir $name
  }
  $sourceDirs[$key] = $hit
  return $hit
}

function Get-PatchedText([string]$Base, $Chain, [string]$Target) {
  $text = [IO.File]::ReadAllText($Base, [Text.UTF8Encoding]::new($false, $true))
  foreach ($step in $Chain) {
    if ($step.target -ne $Target) { continue }
    $doc = Get-Content -LiteralPath (Join-Path $repo $step.subject) -Raw | ConvertFrom-Json
    $node = $doc
    foreach ($seg in ($step.pointer.Trim('/') -split '/')) { $node = if ($seg -match '^\d+$') { $node[[int]$seg] } else { $node.$seg } }
    foreach ($op in $node.($step.field)) {
      $before = if ($step.field -eq 'replacements') { $op.old } else { $op.before }
      $after = if ($step.field -eq 'replacements') { $op.new } else { $op.after }
      $want = if ($step.field -eq 'replacements') { 1 } else { [int]$op.occurrences }
      $count = 0; $at = 0
      while (($at = $text.IndexOf($before, $at, [StringComparison]::Ordinal)) -ge 0) { $count++; $at += [Math]::Max(1, $before.Length) }
      if ($count -ne $want) { throw "patch $($step.subject)$($step.pointer) on ${Target}: $count occurrences, want $want" }
      $text = $text.Replace($before, $after, [StringComparison]::Ordinal)
    }
  }
  return $text
}

# ---- tools for regeneration ----
$toolDir = Join-Path $Destination 'tools'
function Get-Tools {
  New-Item -ItemType Directory -Force -Path $toolDir | Out-Null
  $gen = $repro.generator
  $ts = Join-Path $toolDir ('tree-sitter' + $(if ($IsWindows) { '.exe' } else { '' }))
  if ($TreeSitterExe) { Copy-Item -LiteralPath $TreeSitterExe -Destination $ts }
  else {
    $asset = $gen.release_assets.$Platform
    $gz = Join-Path $toolDir $asset.asset
    Get-Download "https://github.com/tree-sitter/tree-sitter/releases/download/v$($gen.version)/$($asset.asset)" $gz
    Assert-File $gz $asset.sha256 $asset.bytes 'tree-sitter asset'
    $in = [IO.File]::OpenRead($gz); $z = [IO.Compression.GZipStream]::new($in, [IO.Compression.CompressionMode]::Decompress)
    $out = [IO.File]::Open($ts, [IO.FileMode]::CreateNew)
    try { $z.CopyTo($out) } finally { $out.Dispose(); $z.Dispose(); $in.Dispose() }
    if (-not $IsWindows) { chmod 0755 $ts; if ($LASTEXITCODE -ne 0) { throw 'chmod' } }
  }
  if ($gen.executables.PSObject.Properties[$Platform]) { $e = $gen.executables.$Platform; Assert-File $ts $e.sha256 $e.bytes 'tree-sitter executable' }
  $js = $repro.js_runtime
  $node = Join-Path $toolDir ('node' + $(if ($IsWindows) { '.exe' } else { '' }))
  if ($NodeExe) { Copy-Item -LiteralPath $NodeExe -Destination $node; if ($Platform -eq 'windows/amd64') { $a = $js.release_assets.$Platform; if ((Get-Sha $node) -ne $a.sha256) { throw 'node identity mismatch' } } }
  else {
    $a = $js.release_assets.$Platform
    $file = Join-Path $toolDir (Split-Path -Leaf $a.asset)
    Get-Download "https://nodejs.org/dist/v$($js.version)/$($a.asset)" $file
    if ((Get-Sha $file) -ne $a.sha256) { throw 'node asset digest mismatch' }
    if ($Platform -eq 'windows/amd64') { Move-Item -LiteralPath $file -Destination $node }
    else {
      $stem = (Split-Path -Leaf $a.asset) -replace '\.tar\.(xz|gz)$', ''
      Expand-Tar $file $toolDir @("$stem/bin/node")
      Move-Item -LiteralPath (Join-Path $toolDir "$stem/bin/node") -Destination $node
    }
  }
  return @{ ts = $ts; node = $node }
}

function Get-NpmFile([string]$Rel) {
  # node_modules/<package>/<file> from the registered npm tarball (integrity sha512)
  $parts = $Rel -split '/'
  $pkg = $repro.npm_dependencies | Where-Object { $_.package -eq $parts[1] } | Select-Object -First 1
  $dir = Join-Path $Destination "npm/$($pkg.package)"
  if (-not (Test-Path -LiteralPath $dir)) {
    $tgz = Join-Path $Destination "npm/$($pkg.package)-$($pkg.version).tgz"
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $tgz) | Out-Null
    Get-Download "https://registry.npmjs.org/$($pkg.package)/-/$($pkg.package)-$($pkg.version).tgz" $tgz
    $want = ($pkg.integrity -replace '^sha512-', '')
    $got = [Convert]::ToBase64String([Security.Cryptography.SHA512]::HashData([IO.File]::ReadAllBytes($tgz)))
    if ($got -ne $want) { throw "npm integrity mismatch: $($pkg.package)" }
    Expand-Tar $tgz $dir
  }
  return Join-Path $dir ("package/" + (($parts | Select-Object -Skip 2) -join '/'))
}

$cli = Join-Path $Destination ('tsgk' + $(if ($IsWindows) { '.exe' } else { '' }))
$tools = $null

# ---- runtime ----
$runtime = Join-Path $Destination 'runtime'
if ($LocalRuntime) { $rtSrc = $LocalRuntime }
else {
  $archive = Join-Path $Destination 'archives/tree-sitter-runtime.tgz'
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $archive) | Out-Null
  Get-Download $runtimeManifest.url $archive
  Expand-Tar $archive (Join-Path $Destination 'src/runtime')
  $rtSrc = Join-Path $Destination "src/runtime/tree-sitter-$($runtimeManifest.commit)"
}
foreach ($f in $runtimeManifest.files) {
  $to = Join-Path $runtime $f.path
  Copy-Into (Join-Path $rtSrc $f.path) $to
  Assert-File $to $f.sha256 $f.bytes 'runtime file'
}

foreach ($r in $registry.routes) {
  if ($Routes.Count -and $Routes -notcontains $r.route) { continue }
  $src = Get-RepoDir $r
  $root = Join-Path $Destination "routes/$($r.route)"
  $prefix = if ($r.grammar_dir -eq '.') { '' } else { "$($r.grammar_dir)/" }
  $patched = @{}
  if ($r.PSObject.Properties['regeneration'] -and $r.regeneration) {
    foreach ($pf in $r.regeneration.patched_files) {
      $text = Get-PatchedText (Join-Path $src $pf.path) $r.regeneration.patch_chain $pf.path
      $tmp = Join-Path $Destination "patched/$($r.route)/$($pf.path)"
      New-Item -ItemType Directory -Force -Path (Split-Path -Parent $tmp) | Out-Null
      [IO.File]::WriteAllText($tmp, $text, [Text.UTF8Encoding]::new($false))
      Assert-File $tmp $pf.sha256 $pf.bytes 'patched file'
      $patched[$pf.path] = $tmp
    }
    $outDir = $null
    if ($ReuseGenerated) {
      $outDir = Join-Path $ReuseGenerated "$($r.route)/workspace-a/out"
      $stats.reused += $r.route
    } else {
      if (-not $tools) {
        $tools = Get-Tools
        Push-Location $repo
        try { go build -o $cli ./src/cmd/tsgk; if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' } } finally { Pop-Location }
      }
      $subject = Join-Path $Destination "subjects/$($r.route)"
      $inputs = @()
      foreach ($in in $r.regeneration.inputs) {
        $from = if ($patched.ContainsKey($in.path)) { $patched[$in.path] } elseif ($in.path.StartsWith('node_modules/')) { Get-NpmFile $in.path } else { Join-Path $src $in.path }
        $to = Join-Path $subject $in.path
        Copy-Into $from $to
        Assert-File $to $in.sha256 $in.bytes 'regeneration input'
        $inputs += $in
      }
      $tsId = [ordered]@{ name = 'tree-sitter'; version = $repro.generator.version; sha256 = (Get-Sha $tools.ts); bytes = (Get-Item -LiteralPath $tools.ts).Length }
      $nodeId = [ordered]@{ name = 'node'; version = $repro.js_runtime.version; sha256 = (Get-Sha $tools.node); bytes = (Get-Item -LiteralPath $tools.node).Length }
      $outputs = @($r.regeneration.outputs | ForEach-Object { [ordered]@{ path = $_.path; reference = 'PRESENT'; sha256 = $_.sha256; bytes = $_.bytes } })
      $profile = [ordered]@{ schema = 'tsgk-reproduce/r1'; id = "s05-$($r.route)-regen"; route = $r.route; mode = 'js'; generator = $tsId; js_runtime = $nodeId
        abi = $r.regeneration.abi; optimize = $r.regeneration.optimize; grammar = $r.regeneration.grammar; inputs = $inputs; outputs = $outputs
        limits = [ordered]@{ wall_seconds = 300; output_bytes = 8388608; storage_bytes = 536870912; memory_bytes = $r.regeneration.memory_bytes; input_files = 64; input_bytes = 67108864; file_bytes = 16777216 } }
      $pf = Join-Path $Destination "profiles/$($r.route)-regen.json"
      New-Item -ItemType Directory -Force -Path (Split-Path -Parent $pf) | Out-Null
      $profile | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $pf -Encoding utf8NoBOM
      $work = Join-Path $Destination "repro-work/$($r.route)"
      New-Item -ItemType Directory -Force -Path $work | Out-Null
      New-Item -ItemType Directory -Force -Path (Join-Path $Destination 'repro') | Out-Null
      $rout = Join-Path $Destination "repro/$($r.route)"
      $cliArgs = @('reproduce', '--root', $subject, '--profile', $pf, '--out', $rout, '--work', $work, '--tool', "tree-sitter=$($tools.ts)", '--tool', "node=$($tools.node)", '--allow', 'EXEC_GENERATOR')
      if ($CgroupParent) { $cliArgs += @('--cgroup-parent', $CgroupParent) }
      $line = & $cli @cliArgs
      $code = $LASTEXITCODE
      $rep = $line | ConvertFrom-Json
      $stats.regenerated += [ordered]@{ route = $r.route; exit = $code; assessment = $rep.assessment; claims = $rep.claims; wall_ms = @($rep.runs | ForEach-Object { $_.process.wall_ms }) }
      Write-Output ("regenerate {0}: exit={1} assessment={2} claims={3}" -f $r.route, $code, $rep.assessment, ($rep.claims | ConvertTo-Json -Compress))
      if ($code -ne 0 -or $rep.assessment -ne 'PASS') { throw "regeneration of $($r.route) did not reproduce the registered outputs" }
      $outDir = Join-Path $rout 'workspace-a/out'
    }
    foreach ($o in $r.regeneration.outputs) {
      if ($o.path -in @('grammar.json', 'node-types.json')) { continue }
      $to = Join-Path $root ($prefix + 'src/' + $o.path)
      Copy-Into (Join-Path $outDir $o.path) $to
      Assert-File $to $o.sha256 $o.bytes 'generated output'
    }
  }
  foreach ($f in $r.files) {
    if ($f.origin -eq 'generated') { continue }
    $from = if ($f.origin -eq 'patched') { $patched[$f.path] } else { Join-Path $src $f.path }
    $to = Join-Path $root $f.path
    Copy-Into $from $to
    Assert-File $to $f.sha256 $f.bytes "$($r.route) file"
  }
  $stats.routes += $r.route
  Write-Output "prepared $($r.route): $($r.files.Count) files"
}
foreach ($d in @('archives', 'src', 'subjects', 'repro-work', 'npm', 'patched')) {
  $p = Join-Path $Destination $d
  if (Test-Path -LiteralPath $p) { Remove-Item -LiteralPath $p -Recurse -Force }
}
$stats | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $Destination 'prepared.json') -Encoding utf8NoBOM
Write-Output ("prepared routes={0} downloaded_bytes={1} regenerated={2} reused={3}" -f $stats.routes.Count, $stats.downloaded_bytes, $stats.regenerated.Count, $stats.reused.Count)
