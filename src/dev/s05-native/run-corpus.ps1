#Requires -Version 7
# S05 development helper (local Windows only): runs the private corpus NET461-PHASE2-LOCAL-r1
# through `tsgk corpus` (encoding steps 1-6 with the pinned index-euc-kr) and then, per
# route, `tsgk incremental` with operation private-corpus-local (batch frames of at most
# 500 files / 256 MiB, 60 s per file, record output). Never send its outputs anywhere:
# records with paths stay under -Destination (an untracked local artifacts directory);
# summary.json carries counts and judgements only. The corpus is read in place, never copied.
param(
  [Parameter(Mandatory)][string]$CorpusRoot,
  [Parameter(Mandatory)][string]$Prepared,
  [Parameter(Mandatory)][string]$Destination,
  [Parameter(Mandatory)][string]$Compiler,
  [int]$WallSeconds = 3600
)
$ErrorActionPreference = 'Stop'
# No strict mode: the results carry optional (omitted) JSON members.
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$start = Get-Date
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if (Test-Path -LiteralPath $Destination) { throw 'destination exists' }
New-Item -ItemType Directory -Path $Destination | Out-Null
$Destination = (Resolve-Path $Destination).Path
$Prepared = (Resolve-Path $Prepared).Path
$CorpusRoot = (Resolve-Path $CorpusRoot).Path
$registry = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/native-routes.json') -Raw | ConvertFrom-Json
$facts = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/fact-mapping.json') -Raw | ConvertFrom-Json

Push-Location $repo
try {
  $commit = (git rev-parse HEAD).Trim()
  $dirty = @(git status --porcelain=v1 --untracked-files=all)
} finally { Pop-Location }
$cli = Join-Path $Destination 'tsgk.exe'
Push-Location $repo
try { go build -o $cli ./src/cmd/tsgk; if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' } } finally { Pop-Location }
$ccSha = (Get-FileHash -LiteralPath $Compiler -Algorithm SHA256).Hash.ToLowerInvariant()
# A linked compiler (Ubuntu /usr/bin/gcc -> gcc-13) is identified by the file it resolves
# to, as tsgk does: Get-FileHash follows the link but Length would be the link's own size.
$ccItem = Get-Item -LiteralPath $Compiler
if ($ccItem.LinkTarget) { $ccItem = $ccItem.ResolveLinkTarget($true) }
$compilerId = [ordered]@{ name = 'cc'; version = 'host'; sha256 = $ccSha; bytes = $ccItem.Length }
$ccLines = @(& $Compiler --version) # whole output: a cut pipeline would leave $LASTEXITCODE stale
if ($LASTEXITCODE -ne 0) { throw 'compiler --version failed' }
$ccVersion = $ccLines | Select-Object -First 1

# 1. inventory (S01 operation, cp949 profile) with the S05 table-completed encodings
$inventory = Join-Path $Destination 'inventory.json'
& $cli corpus --root $CorpusRoot --out $inventory | Out-Null
if ($LASTEXITCODE -ne 0) { throw "corpus inventory exit $LASTEXITCODE" }
$inv = Get-Content -LiteralPath $inventory -Raw | ConvertFrom-Json

function Test-Portable([string]$p) {
  if ($p -match '[\\:]' -or $p -match '[\x00-\x1f\x7f]') { return $false }
  foreach ($seg in $p -split '/') {
    if (-not $seg -or $seg -eq '.' -or $seg -eq '..' -or $seg.TrimEnd(' ', '.') -ne $seg) { return $false }
    if ($seg -match '^(?i)(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$') { return $false }
  }
  return $true
}

$records = [Collections.Generic.List[object]]::new()
$groups = @{ csharp = @(); tsql = @(); xml = @(); svc = @() }
$i = 0
foreach ($rec in $inv.records) {
  $i++
  if (-not $rec.route -or $rec.state -eq 'PRESENCE_ONLY') { continue }
  $id = 'f{0:d6}' -f $i
  $entry = [ordered]@{ id = $id; path = $rec.path; route = $rec.route; size = $rec.size; generated = $rec.generated; encoding = $rec.encoding
    execution_status = 'NOT_RUN'; assessment = 'NOT_ASSESSED'; code = 'NOT_RUN'; has_error = $null; descendant_count = $null; digest = $null; errors_total = $null; form = $null; svc_coverage = $null }
  if ($rec.state -ne 'COMPLETED') { $entry.assessment = 'BLOCKED'; $entry.code = "INVENTORY_$($rec.state)" }
  elseif (-not $rec.encoding -or $rec.encoding.assessment -ne 'PASS') { $entry.assessment = 'BLOCKED'; $entry.code = $(if ($rec.encoding) { $rec.encoding.code } else { 'ENCODING_ABSENT' }) }
  elseif (-not (Test-Portable $rec.path)) { $entry.assessment = 'BLOCKED'; $entry.code = 'PATH_NOT_PORTABLE' }
  else {
    $groups[$rec.route] += [ordered]@{ id = $id; encoding = $rec.encoding.encoding; input = [ordered]@{ path = $rec.path; role = 'case'; sha256 = $rec.sha256; bytes = $rec.size }; edits = @(); points = @(); expect = @() }
  }
  $records.Add($entry)
}
$byId = @{}
foreach ($e in $records) { $byId[$e.id] = $e }

function Get-Declarations([string]$Route) {
  $m = $facts.routes | Where-Object { $_.route -eq $Route } | Select-Object -First 1
  if (-not $m) { return $null }
  $items = @($m.facts | Where-Object { $_.PSObject.Properties['node'] -and $_.fact -in @('type_declaration', 'member_declaration', 'create_object') } |
      ForEach-Object { [ordered]@{ fact = $_.fact; node = $_.node; name = $_.name } })
  return [ordered]@{ mapping = $facts.revision; items = $items }
}

$work = Join-Path $Destination 'work'
New-Item -ItemType Directory -Path $work | Out-Null
$runs = @()
foreach ($g in @('csharp', 'tsql', 'xml', 'svc')) {
  $cases = $groups[$g]
  if (-not $cases.Count) { continue }
  $remaining = [int]($WallSeconds - ((Get-Date) - $start).TotalSeconds)
  if ($remaining -le 0) { $runs += [ordered]@{ group = $g; skipped = 'RUN_WALL_EXHAUSTED' }; continue }
  $route = if ($g -eq 'svc') { 'csharp' } else { $g }
  $r = $registry.routes | Where-Object { $_.route -eq $route }
  $sorted = [Collections.Generic.List[object]]::new()
  foreach ($f in $r.files) { $sorted.Add([ordered]@{ path = $f.path; role = $f.role; sha256 = $f.sha256; bytes = $f.bytes }) }
  $sorted.Sort([Comparison[object]] { param($a, $b) [string]::CompareOrdinal($a.path, $b.path) })
  $profile = [ordered]@{ schema = 'tsgk-incremental/r1'; id = "s05-corpus-$g"; route = $route; operation = 'private-corpus-local'; symbol = $r.symbol
    encoding = 'UTF-8'; output = 'record'; compiler = $compilerId; grammar = @($sorted); declarations = (Get-Declarations $route); cases = @($cases) }
  if ($g -eq 'svc') { $profile.format = 'SVC-SERVICEHOST-r1' }
  $pf = Join-Path $Destination "profile-$g.json"
  $profile | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $pf -Encoding utf8NoBOM
  $out = Join-Path $Destination "result-$g"
  $t0 = Get-Date
  $line = & $cli incremental --root $CorpusRoot --grammar-root (Join-Path $Prepared "routes/$route") --profile $pf --runtime (Join-Path $Prepared 'runtime') `
    --tool "cc=$Compiler" --work $work --out $out --allow BUILD_NATIVE --allow EXEC_NATIVE --run-wall $remaining
  $code = $LASTEXITCODE
  $res = $line | ConvertFrom-Json
  foreach ($c in @($res.cases)) {
    $e = $byId[$c.id]
    $e.execution_status = $c.execution_status; $e.assessment = $c.assessment; $e.code = $c.code
    if (@($c.steps).Count) {
      $s = $c.steps[0]
      # incremental is null when no tree was parsed (SVC observation only)
      if ($s.incremental) { $e.has_error = $s.incremental.has_error; $e.descendant_count = $s.incremental.descendant_count; $e.digest = $s.incremental.digest; $e.form = $s.incremental.form
        if ($s.incremental.PSObject.Properties['summary'] -and $s.incremental.summary) { $e.errors_total = $s.incremental.summary.errors.total } }
      if ($s.PSObject.Properties['composite'] -and $s.composite) { $e.svc_coverage = $s.composite.coverage }
    }
  }
  $runs += [ordered]@{ group = $g; exit = $code; execution_status = $res.execution_status; assessment = $res.assessment; cases = @($res.cases).Count; batches = @($res.batches).Count
    build_identity = $(if ($res.build) { $res.build.identity } else { $null }); executable_sha256 = $(if ($res.build) { $res.build.executable_sha256 } else { $null })
    wall_seconds = [int]((Get-Date) - $t0).TotalSeconds; fatal_batches = @($res.batches | Where-Object { $_.fatal }).Count }
  Write-Output ("corpus {0}: exit={1} status={2} cases={3} batches={4}" -f $g, $code, $res.execution_status, @($res.cases).Count, @($res.batches).Count)
}

# local per-file records (paths: private, untracked)
$records | ForEach-Object { $_ | ConvertTo-Json -Compress -Depth 6 } | Set-Content -LiteralPath (Join-Path $Destination 'records.jsonl') -Encoding utf8NoBOM

# public-safe summary: counts and judgements only
$count = { param($sel) $h = [ordered]@{}; foreach ($e in $records) { $k = & $sel $e; if ($null -eq $k) { $k = 'null' }; $h[[string]$k] = 1 + [int]($h[[string]$k]) }; $h }
$summary = [ordered]@{ schema = 'tsgk-s05-private-corpus-run/r1'; corpus = 'NET461-PHASE2-LOCAL-r1'; operation = 'private-corpus-local'
  candidate_commit = $commit; clean_tree = ($dirty.Count -eq 0); dirty_entries = $dirty.Count
  host = [ordered]@{ os = [Environment]::OSVersion.VersionString; arch = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString(); processors = [Environment]::ProcessorCount
    memory_bytes = [GC]::GetGCMemoryInfo().TotalAvailableMemoryBytes }
  tools = [ordered]@{ compiler_sha256 = $ccSha; compiler_version = $ccVersion; runtime_commit = (Get-Content -LiteralPath (Join-Path $repo 'src/drivers/native-c/runtime-manifest.json') -Raw | ConvertFrom-Json).commit }
  inventory = [ordered]@{ records = @($inv.records).Count; routed_non_presence = $records.Count; presence_only = @($inv.records | Where-Object { $_.state -eq 'PRESENCE_ONLY' }).Count
    unrouted = @($inv.records | Where-Object { -not $_.route }).Count; encoding_codes = $inv.summary.encoding_codes }
  by_route_status = & $count { param($e) "$($e.route):$($e.execution_status):$($e.assessment)" }
  has_error = & $count { param($e) if ($e.execution_status -eq 'COMPLETED') { "$($e.route):$(if ($null -eq $e.has_error) { 'null' } else { $e.has_error })" } }
  codes = & $count { param($e) if ($e.code) { "$($e.route):$($e.code)" } }
  forms = & $count { param($e) if ($e.form) { "$($e.route):$($e.form)" } }
  not_run = @($records | Where-Object { $_.execution_status -eq 'NOT_RUN' -and $_.assessment -ne 'BLOCKED' }).Count
  runs = $runs; wall_seconds = [int]((Get-Date) - $start).TotalSeconds }
$summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $Destination 'summary.json') -Encoding utf8NoBOM
Write-Output ("corpus done: files={0} not_run={1} wall={2}s clean_tree={3}" -f $records.Count, $summary.not_run, $summary.wall_seconds, $summary.clean_tree)
