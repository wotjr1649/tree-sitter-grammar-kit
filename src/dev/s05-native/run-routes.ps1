#Requires -Version 7
# S05/S06 development helper: runs `tsgk incremental` for every registered route on this
# host with the prepared inputs (prepare-routes.ps1), the registered cases
# (src/testdata/native/routes, gaps, n461) and, with -Large, the synthetic
# real-world-source-r3 fixtures (src/contracts/native-large-fixtures.json; windows/amd64
# only, NOT_APPLICABLE elsewhere). With -Oracle (S06) it also records every route through
# `tsgk oracle record` (tsgk-native/r2): the same cases plus the route's query case
# (src/testdata/native/queries), the fact query pack (src/contracts/fact-query-pack.json)
# for its routes, the dynamic SQL fixtures, and with -Large the native-query-large cases.
# Writes one result directory per run and summary.json under -Destination. Exit 1 (the kit
# axis) when a build is refused or fails, a result has an error finding, a case does not
# complete, a record set does not verify, an incremental equality claim is FAIL or BLOCKED,
# an incremental route claim is FAIL, or a query equality, fact reproduction or dynamic SQL
# claim is FAIL or BLOCKED. Requirement-axis results do not fail the run: language
# expectation failures, query expectation claims (judged by `tsgk qualify` as Q) and a
# route BLOCKED on an error tree are recorded in requirement_results, API claims (the
# runtime node API disagreeing with its cursor) in api_findings, for disposition.
param(
  [Parameter(Mandatory)][string]$Prepared,
  [Parameter(Mandatory)][string]$Destination,
  [Parameter(Mandatory)][string]$Compiler,
  [Parameter(Mandatory)][string]$Platform,
  [string]$CgroupParent = '',
  [string[]]$Routes = @(),
  [switch]$Large,
  [switch]$Oracle
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if (Test-Path -LiteralPath $Destination) { throw 'destination exists' }
New-Item -ItemType Directory -Path $Destination | Out-Null
$Destination = (Resolve-Path $Destination).Path
$Prepared = (Resolve-Path $Prepared).Path
$registry = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/native-routes.json') -Raw | ConvertFrom-Json
$facts = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/fact-mapping.json') -Raw | ConvertFrom-Json
$utf8 = [Text.UTF8Encoding]::new($false, $true)
$packFile = Join-Path $repo 'src/contracts/fact-query-pack.json'
$pack = Get-Content -LiteralPath $packFile -Raw | ConvertFrom-Json
$r3Reason = 'NET461 workload is Windows-hosted (WinForms/.NET Framework 4.6.1)'

function Get-Sha([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Get-ShaBytes([byte[]]$Data) { [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($Data)).ToLowerInvariant() }
function Find-Bytes([byte[]]$Hay, [byte[]]$Needle, [int]$From = 0) {
  if ($Needle.Length -eq 0) { return $Hay.Length }
  # Latin-1 maps every byte to one char, so an ordinal string search is a byte search.
  $l1 = [Text.Encoding]::Latin1
  return $l1.GetString($Hay).IndexOf($l1.GetString($Needle), $From, [StringComparison]::Ordinal)
}

# Converts a case-file anchor {type, text, occurrence} to the profile anchor: the byte range
# of the occurrence-th (1-based, default 1) start offset of the text's UTF-8 bytes in the
# step's source; every offset counts, so occurrences may overlap. Only the members type and
# text (strings) and occurrence (absent or an integer, never null) are accepted, by exact
# name: a misspelt member would otherwise fall back to occurrence 1. The inventory generator
# (src/internal/foundation) converts the same way and TestAnchorConversion compares both.
function Resolve-Anchor([byte[]]$Source, $Anchor, [string]$Case) {
  $names = @($Anchor.PSObject.Properties | ForEach-Object { $_.Name })
  $unknown = @($names | Where-Object { $_ -cnotin @('type', 'text', 'occurrence') })
  if ($unknown.Count) { throw "${Case}: anchor member $($unknown -join ',') is not type, text or occurrence" }
  if ($names -cnotcontains 'type' -or $names -cnotcontains 'text' -or $Anchor.type -isnot [string] -or $Anchor.text -isnot [string]) {
    throw "${Case}: anchor type and text must be strings"
  }
  $n = 1
  if ($names -ccontains 'occurrence') {
    $n = $Anchor.occurrence
    if ($null -eq $n -or ($n -isnot [int] -and $n -isnot [long])) { throw "${Case}: anchor $($Anchor.type) occurrence is not an integer" }
  }
  $text = $utf8.GetBytes([string]$Anchor.text)
  if ($n -lt 1 -or $text.Length -eq 0 -or $Anchor.type -eq '') { throw "${Case}: anchor $($Anchor.type) has an empty type or text, or occurrence $n" }
  $at = -1
  $from = 0
  for ($k = 0; $k -lt $n; $k++) {
    $at = Find-Bytes $Source $text $from
    if ($at -lt 0) { throw "${Case}: anchor $($Anchor.type) occurrence $n not found" }
    $from = $at + 1
  }
  return [ordered]@{ type = [string]$Anchor.type; start_byte = $at; end_byte = $at + $text.Length }
}

$cli = Join-Path $Destination ('tsgk' + $(if ($IsWindows) { '.exe' } else { '' }))
Push-Location $repo
try { go build -o $cli ./src/cmd/tsgk; if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' } } finally { Pop-Location }
$ccLines = @(& $Compiler --version) # whole output: a cut pipeline would leave $LASTEXITCODE stale
if ($LASTEXITCODE -ne 0) { throw 'compiler --version failed' }
$ccVersion = $ccLines | Select-Object -First 1
# A linked compiler (Ubuntu /usr/bin/gcc -> gcc-13) is identified by the file it resolves
# to, as tsgk does: Get-FileHash follows the link but Length would be the link's own size.
$ccItem = Get-Item -LiteralPath $Compiler
if ($ccItem.LinkTarget) { $ccItem = $ccItem.ResolveLinkTarget($true) }
$compilerId = [ordered]@{ name = 'cc'; version = 'host'; sha256 = (Get-Sha $ccItem.FullName); bytes = $ccItem.Length }
$work = Join-Path $Destination 'work'
New-Item -ItemType Directory -Path $work | Out-Null
$summary = [ordered]@{ schema = 'tsgk-s05-route-run/r1'; platform = $Platform; compiler = [ordered]@{ path = $Compiler; version_line = $ccVersion; sha256 = $compilerId.sha256; bytes = $compilerId.bytes }
  started_at = (Get-Date).ToUniversalTime().ToString('o'); routes = @(); large = @(); oracle = @(); oracle_large = @(); api_findings = @(); requirement_results = @(); failures = @() }

function Get-Declarations([string]$Route) {
  $m = $facts.routes | Where-Object { $_.route -eq $Route } | Select-Object -First 1
  if (-not $m) { return $null }
  $items = @($m.facts | Where-Object { $_.PSObject.Properties['node'] -and $_.fact -in @('type_declaration', 'member_declaration', 'create_object') } |
      ForEach-Object { [ordered]@{ fact = $_.fact; node = $_.node; name = $_.name } })
  if (-not $items.Count) { return $null }
  return [ordered]@{ mapping = $facts.revision; items = $items }
}

function New-Root($r) {
  $root = Join-Path $Destination "roots/$($r.route)"
  foreach ($f in $r.files) {
    $to = Join-Path $root $f.path
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $to) | Out-Null
    Copy-Item -LiteralPath (Join-Path $Prepared "routes/$($r.route)/$($f.path)") -Destination $to
    if ((Get-Sha $to) -ne $f.sha256) { throw "prepared file mismatch: $($r.route) $($f.path)" }
  }
  return $root
}

function Invoke-Profile($r, [string]$Root, [string]$Id, [string]$Operation, [string]$Output, $Cases, $Decls, [string]$Format = '') {
  # the profile requires byte-ascending paths
  $sorted = [Collections.Generic.List[object]]::new()
  foreach ($f in $r.files) { $sorted.Add([ordered]@{ path = $f.path; role = $f.role; sha256 = $f.sha256; bytes = $f.bytes }) }
  $sorted.Sort([Comparison[object]] { param($a, $b) [string]::CompareOrdinal($a.path, $b.path) })
  $profile = [ordered]@{ schema = 'tsgk-incremental/r2'; id = $Id; route = $r.route; operation = $Operation; symbol = $r.symbol; encoding = 'UTF-8'; output = $Output
    compiler = $compilerId; grammar = @($sorted); declarations = $Decls; cases = @($Cases) }
  if ($Format) { $profile.format = $Format }
  $pf = Join-Path $Destination "profiles/$Id.json"
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $pf) | Out-Null
  $profile | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $pf -Encoding utf8NoBOM
  $out = Join-Path $Destination "results/$Id"
  New-Item -ItemType Directory -Force -Path (Join-Path $Destination 'results') | Out-Null
  $cliArgs = @('incremental', '--root', $Root, '--profile', $pf, '--runtime', (Join-Path $Prepared 'runtime'), '--tool', "cc=$Compiler", '--work', $work, '--out', $out,
    '--allow', 'BUILD_NATIVE', '--allow', 'EXEC_NATIVE')
  if ($CgroupParent) { $cliArgs += @('--cgroup-parent', $CgroupParent) }
  $line = & $cli @cliArgs
  $code = $LASTEXITCODE
  $res = $line | ConvertFrom-Json
  return @{ code = $code; res = $res }
}

# Writes a case file's sources under the root and converts find/replace edits to bytes and
# text anchors to the byte ranges of their step's source (Resolve-Anchor).
function Convert-Cases([string]$File, [string]$Root) {
  $doc = Get-Content -LiteralPath $File -Raw | ConvertFrom-Json
  $out = @()
  foreach ($c in $doc.cases) {
    $bytes = $utf8.GetBytes($c.source_utf8)
    $rel = "cases/$($c.id).txt"
    $path = Join-Path $Root $rel
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $path) | Out-Null
    [IO.File]::WriteAllBytes($path, $bytes)
    $cur = $bytes
    $versions = [Collections.Generic.List[byte[]]]::new()
    $versions.Add($cur)
    $edits = @()
    foreach ($e in $c.edits) {
      $find = $utf8.GetBytes($e.find); $repl = $utf8.GetBytes($e.replace)
      $at = Find-Bytes $cur $find
      if ($at -lt 0) { throw "$($c.id): edit target not found" }
      $edits += [ordered]@{ start_byte = $at; old_end_byte = $at + $find.Length; new_end_byte = $at + $repl.Length
        old = [Convert]::ToBase64String($find); new = [Convert]::ToBase64String($repl) }
      $next = [byte[]]::new($cur.Length - $find.Length + $repl.Length)
      [Array]::Copy($cur, 0, $next, 0, $at)
      [Array]::Copy($repl, 0, $next, $at, $repl.Length)
      [Array]::Copy($cur, $at + $find.Length, $next, $at + $repl.Length, $cur.Length - $at - $find.Length)
      $cur = $next
      $versions.Add($cur)
    }
    $expect = @($c.expect | ForEach-Object {
        $x = [ordered]@{ step = $_.step; syntax = $_.syntax; contains = @($_.contains); declarations = '' }
        if ($_.PSObject.Properties['anchors'] -and @($_.anchors).Count) {
          $src = $versions[[int]$_.step]
          $x.anchors = @(foreach ($a in $_.anchors) { Resolve-Anchor $src $a $c.id })
        }
        $x })
    $out += [ordered]@{ id = $c.id; input = [ordered]@{ path = $rel; role = 'case'; sha256 = (Get-ShaBytes $bytes); bytes = $bytes.Length }; edits = $edits; points = @(); expect = $expect }
  }
  return $out
}

# Requirement-axis results (judged by `tsgk qualify`, not by this job): a query
# expectation claim that is not PASS, with its failures, and an incremental route BLOCKED
# because the grammar left an error in the tree. Recorded for disposition only.
function Add-RequirementResult([string]$Label, $Case, $QueryFailures) {
  $qe = $(if ($Case.PSObject.Properties['oracle_claims'] -and $Case.oracle_claims) { $Case.oracle_claims.query_expectations } else { $null })
  if ($qe -in @('FAIL', 'BLOCKED')) {
    $script:summary.requirement_results += [ordered]@{ case = "$Label/$($Case.id)"; claim = 'query_expectations'; result = $qe; code = $Case.code; failures = @($QueryFailures) }
  }
  if ($Case.claims.incremental_route -eq 'BLOCKED') {
    $script:summary.requirement_results += [ordered]@{ case = "$Label/$($Case.id)"; claim = 'incremental_route'; result = 'BLOCKED'; code = $Case.code; failures = @() }
  }
}

function Add-Result([string]$Label, $Run) {
  $res = $Run.res
  $entry = [ordered]@{ route = $Label; exit = $Run.code; execution_status = $res.execution_status; assessment = $res.assessment; build = $null; cases = @() }
  if ($res.build) {
    $entry.build = [ordered]@{ identity = $res.build.identity; executable_sha256 = $res.build.executable_sha256; compiler_version = $res.build.compiler_version
      steps = @($res.build.steps | ForEach-Object { [ordered]@{ name = $_.name; status = $_.result.status; exit = $_.result.exit_code; wall_ms = $_.result.wall_ms } }) }
  }
  foreach ($f in @($res.findings)) { if ($f -and $f.severity -eq 'error') { $script:summary.failures += "${Label}: $($f.code)" } }
  foreach ($c in @($res.cases)) {
    $fails = @($c.expectations | Where-Object { $_.result -ne 'PASS' } | ForEach-Object { "step $($_.step): $($_.result) $($_.detail)" })
    $entry.cases += [ordered]@{ id = $c.id; execution_status = $c.execution_status; assessment = $c.assessment; code = $c.code; claims = $c.claims; expectation_failures = $fails
      steps = @($c.steps | ForEach-Object { [ordered]@{ step = $_.step; has_error = $(if ($_.incremental) { $_.incremental.has_error } else { $null }); digest = $(if ($_.incremental) { $_.incremental.digest } else { $null }); equal = $(if ($_.comparison) { $_.comparison.equal } else { $null }); reused = $(if ($_.route) { $_.route.reused_nodes } else { $null })
            svc_coverage = $(if ($_.PSObject.Properties['composite']) { $_.composite.coverage } else { $null }) } }) }
    if ($c.execution_status -ne 'COMPLETED') { $script:summary.failures += "$Label/$($c.id): $($c.execution_status) $($c.code)" }
    if ($c.claims.incremental_equality -in @('FAIL', 'BLOCKED') -or $c.claims.incremental_route -eq 'FAIL') { $script:summary.failures += "$Label/$($c.id): incremental $($c.claims.incremental_equality)/$($c.claims.incremental_route) $($c.code)" }
    Add-RequirementResult $Label $c @()
  }
  if ($res.execution_status -ne 'COMPLETED' -and -not @($res.cases).Count) { $script:summary.failures += "${Label}: $($res.execution_status) build or refusal" }
  $script:summary.routes += $entry
  Write-Output ("route {0}: exit={1} status={2} assessment={3} cases={4}" -f $Label, $Run.code, $res.execution_status, $res.assessment, @($res.cases).Count)
}

# S06: an oracle profile is the incremental profile plus queries, the pack binding and the
# API switch; every case carries its query and dynamic SQL expectations.
function Invoke-Oracle($r, [string]$Root, [string]$Id, [string]$Operation, [string]$Output, $Cases, $Decls, $Queries, [bool]$UsePack, [bool]$Api, [string]$Format = '') {
  $sorted = [Collections.Generic.List[object]]::new()
  foreach ($f in $r.files) { $sorted.Add([ordered]@{ path = $f.path; role = $f.role; sha256 = $f.sha256; bytes = $f.bytes }) }
  $sorted.Sort([Comparison[object]] { param($a, $b) [string]::CompareOrdinal($a.path, $b.path) })
  $factPack = $null
  if ($UsePack) { $factPack = [ordered]@{ revision = $pack.revision; sha256 = (Get-Sha $packFile); route = $r.route } }
  $profile = [ordered]@{ schema = 'tsgk-oracle/r2'; id = $Id; route = $r.route; operation = $Operation; symbol = $r.symbol; encoding = 'UTF-8'; output = $Output
    compiler = $compilerId; grammar = @($sorted); declarations = $Decls; cases = @($Cases); queries = @($Queries); fact_pack = $factPack; api = $Api }
  if ($Format) { $profile.format = $Format }
  $pf = Join-Path $Destination "profiles/$Id.json"
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $pf) | Out-Null
  $profile | ConvertTo-Json -Depth 16 | Set-Content -LiteralPath $pf -Encoding utf8NoBOM
  $out = Join-Path $Destination "records/$Id"
  New-Item -ItemType Directory -Force -Path (Join-Path $Destination 'records') | Out-Null
  $cliArgs = @('oracle', 'record', '--root', $Root, '--profile', $pf, '--runtime', (Join-Path $Prepared 'runtime'), '--tool', "cc=$Compiler", '--work', $work, '--out', $out,
    '--allow', 'BUILD_NATIVE', '--allow', 'EXEC_NATIVE')
  if ($UsePack) { $cliArgs += @('--fact-pack', $packFile) }
  if ($CgroupParent) { $cliArgs += @('--cgroup-parent', $CgroupParent) }
  $line = & $cli @cliArgs
  $code = $LASTEXITCODE
  return @{ code = $code; res = ($line | ConvertFrom-Json); out = $out }
}

# Adds the empty S06 expectations to converted S05 cases.
function Add-OracleFields($Cases) {
  foreach ($c in $Cases) { $c.query_expect = @(); $c.dynamic_sql_expect = $null }
  return $Cases
}

# The route's query case: the registered source case with its edits and S05 expectations,
# plus the query expectations; returns @{ queries; cases }.
function Convert-QueryCases($r, [string]$Root) {
  $file = Join-Path $repo "src/testdata/native/queries/$($r.route).json"
  if (-not (Test-Path -LiteralPath $file)) { return @{ queries = @(); cases = @() } }
  $doc = Get-Content -LiteralPath $file -Raw | ConvertFrom-Json
  $sources = (Get-Content -LiteralPath (Join-Path $repo "src/testdata/native/routes/$($r.route).json") -Raw | ConvertFrom-Json).cases
  $cases = @()
  foreach ($q in $doc.cases) {
    $src = $sources | Where-Object { $_.id -eq $q.source_case }
    if (-not $src) { throw "$($q.id): source case $($q.source_case) not found" }
    $tmp = Join-Path $Destination "tmp-$($q.id).json"
    $copy = $src | Select-Object *
    $copy.id = $q.id
    [ordered]@{ cases = @($copy) } | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $tmp -Encoding utf8NoBOM
    $c = @(Convert-Cases $tmp $Root)[0]
    Remove-Item -LiteralPath $tmp
    $c.query_expect = @($q.query_expect | ForEach-Object { [ordered]@{ query = $_.query; step = $_.step; status = $_.status; code = $_.code
          captures = $(if ($null -eq $_.captures) { $null } else { ,@($_.captures | ForEach-Object { [ordered]@{ name = $_.name; type = $_.type; text = $_.text } }) }); error = $_.error } })
    $c.dynamic_sql_expect = $null
    $cases += $c
  }
  return @{ queries = @($doc.queries | ForEach-Object { [ordered]@{ id = $_.id; source = $_.source } }); cases = $cases }
}

# The dynamic SQL fixtures of a route with their registered facts and known-miss ranges.
function Get-DynamicCases([string]$Route, [string]$Root) {
  $spec = Get-Content -LiteralPath (Join-Path $repo 'src/testdata/native/dynamic-sql/expected.json') -Raw | ConvertFrom-Json
  $out = @()
  foreach ($f in $spec.files | Where-Object { $_.route -eq $Route }) {
    $from = Join-Path $repo "src/testdata/native/dynamic-sql/$($f.path)"
    if ((Get-Sha $from) -ne $f.sha256) { throw "dynamic SQL fixture identity: $($f.path)" }
    $rel = "cases/dynamic-$($f.path)"
    New-Item -ItemType Directory -Force -Path (Join-Path $Root 'cases') | Out-Null
    Copy-Item -LiteralPath $from -Destination (Join-Path $Root $rel)
    $out += [ordered]@{ id = "dynamic-sql-$Route"; input = [ordered]@{ path = $rel; role = 'case'; sha256 = $f.sha256; bytes = $f.bytes }; edits = @(); points = @(); expect = @()
      query_expect = @(); dynamic_sql_expect = [ordered]@{ facts = @($f.facts); known_misses = @($f.non_facts | Where-Object { $_.what -eq 'KNOWN_MISS' } | ForEach-Object { [ordered]@{ start_byte = $_.start_byte; end_byte = $_.end_byte } }) } }
  }
  return $out
}

function Get-PackQueries([string]$Route, [string]$Only = '') {
  $pr = $pack.routes | Where-Object { $_.route -eq $Route }
  if (-not $pr) { return @() }
  return @($pr.queries | Where-Object { -not $Only -or $_.facts -eq $Only } | ForEach-Object { [ordered]@{ id = $_.id; source = $_.source } })
}

function Add-OracleResult([string]$Label, $Run) {
  $res = $Run.res
  $entry = [ordered]@{ route = $Label; exit = $Run.code; execution_status = $res.execution_status; assessment = $res.assessment
    set_valid = $(if ($res.set) { $res.set.valid } else { $null }); build = $(if ($res.build) { $res.build.identity } else { $null }); cases = @() }
  foreach ($f in @($res.findings)) { if ($f -and $f.severity -eq 'error') { $script:summary.failures += "oracle ${Label}: $($f.code)" } }
  if (-not $res.set -or -not $res.set.valid) { $script:summary.failures += "oracle ${Label}: record set not verified" }
  $i = 0
  foreach ($c in @($res.cases)) {
    $record = Join-Path $Run.out ('records/{0:D5}-{1}.json' -f $i, $c.id)
    $i++
    $detail = $null
    if (Test-Path -LiteralPath $record) { $detail = Get-Content -LiteralPath $record -Raw | ConvertFrom-Json }
    $qfails = @()
    if ($detail -and $detail.PSObject.Properties['query_expectations']) { $qfails = @($detail.query_expectations | Where-Object { $_.result -ne 'PASS' } | ForEach-Object { "$($_.query) step $($_.step): $($_.result) $($_.detail)" }) }
    $facts = $null
    if ($detail -and $detail.PSObject.Properties['facts'] -and $detail.facts) {
      $facts = [ordered]@{ difference = $(if ($detail.facts.PSObject.Properties['first_difference']) { $detail.facts.first_difference } else { '' })
        declarations = $(if ($detail.facts.declarations) { [ordered]@{ items = @($detail.facts.declarations.items).Count; reproduces = $detail.facts.declarations.reproduces_s05 } } else { $null })
        dynamic_sql = $(if ($detail.facts.dynamic_sql) { [ordered]@{ items = @($detail.facts.dynamic_sql.items).Count; known_misses = @($detail.facts.dynamic_sql.known_misses).Count } } else { $null }) }
    }
    $entry.cases += [ordered]@{ id = $c.id; execution_status = $c.execution_status; assessment = $c.assessment; code = $c.code; claims = $c.claims; oracle = $c.oracle_claims
      query_expectation_failures = $qfails; facts = $facts }
    if ($c.execution_status -ne 'COMPLETED') { $script:summary.failures += "oracle $Label/$($c.id): $($c.execution_status) $($c.code)" }
    if ($c.claims.incremental_equality -in @('FAIL', 'BLOCKED') -or $c.claims.incremental_route -eq 'FAIL') { $script:summary.failures += "oracle $Label/$($c.id): incremental $($c.claims.incremental_equality)/$($c.claims.incremental_route)" }
    Add-RequirementResult "oracle $Label" $c $qfails
    if ($c.oracle_claims) {
      foreach ($k in @('query_equality', 'fact_reproduction', 'dynamic_sql')) {
        if ($c.oracle_claims.$k -in @('FAIL', 'BLOCKED')) { $script:summary.failures += "oracle $Label/$($c.id): $k $($c.oracle_claims.$k)" }
      }
      # an API claim FAIL is the runtime's node API disagreeing with its cursor (recorded with
      # the first difference for disposition); the comparator is guarded by the owned tests
      if ($c.oracle_claims.api -in @('FAIL', 'BLOCKED') -and $detail) {
        $first = @($detail.steps | ForEach-Object { $_.incremental, $_.fresh } | Where-Object { $_ -and $_.PSObject.Properties['api'] -and $_.api -and -not $_.api.consistent } | Select-Object -First 1)
        $script:summary.api_findings += [ordered]@{ case = "$Label/$($c.id)"; claim = $c.oracle_claims.api; first_difference = $(if ($first.Count) { $first[0].api.first_difference } else { $null }) }
      }
    }
  }
  if ($res.execution_status -ne 'COMPLETED' -and -not @($res.cases).Count) { $script:summary.failures += "oracle ${Label}: $($res.execution_status) build or refusal" }
  $script:summary.oracle += $entry
  Write-Output ("oracle {0}: exit={1} status={2} assessment={3} cases={4} set={5}" -f $Label, $Run.code, $res.execution_status, $res.assessment, @($res.cases).Count, $entry.set_valid)
}

foreach ($r in $registry.routes) {
  if ($Routes.Count -and $Routes -notcontains $r.route) { continue }
  $root = New-Root $r
  $cases = @()
  foreach ($kind in @('routes', 'gaps', 'n461')) {
    $file = Join-Path $repo "src/testdata/native/$kind/$($r.route).json"
    if (Test-Path -LiteralPath $file) { $cases += Convert-Cases $file $root }
  }
  if (-not $cases.Count) { $summary.failures += "$($r.route): no registered cases"; continue }
  Add-Result $r.route (Invoke-Profile $r $root "s05-$($r.route)" 'native-parse-edit' 'tree' $cases (Get-Declarations $r.route))
  if ($Oracle) {
    $qc = Convert-QueryCases $r $root
    if (-not @($qc.cases).Count) { $summary.failures += "oracle $($r.route): no registered query case" }
    $oc = @(Add-OracleFields $cases) + @($qc.cases) + @(Get-DynamicCases $r.route $root)
    $queries = @($qc.queries) + @(Get-PackQueries $r.route)
    $usePack = [bool]($pack.routes | Where-Object { $_.route -eq $r.route })
    Add-OracleResult $r.route (Invoke-Oracle $r $root "s06-$($r.route)" 'native-query' 'tree' $oc (Get-Declarations $r.route) $queries $usePack $true)
  }
}

# SVC-SERVICEHOST-r1 composite cases: the C# route with the format set.
$svcFile = Join-Path $repo 'src/testdata/native/n461/svc.json'
if ((Test-Path -LiteralPath $svcFile) -and (-not $Routes.Count -or $Routes -contains 'csharp')) {
  $r = $registry.routes | Where-Object { $_.route -eq 'csharp' }
  $root = New-Root $r
  $root2 = Join-Path $Destination 'roots/csharp-svc'
  Move-Item -LiteralPath $root -Destination $root2
  $svcCases = Convert-Cases $svcFile $root2
  Add-Result 'csharp-svc' (Invoke-Profile $r $root2 's05-csharp-svc' 'native-parse-edit' 'tree' $svcCases $null 'SVC-SERVICEHOST-r1')
  if ($Oracle) {
    Add-OracleResult 'csharp-svc' (Invoke-Oracle $r $root2 's06-csharp-svc' 'native-query' 'tree' (Add-OracleFields $svcCases) $null (Get-PackQueries 'csharp') $false $true 'SVC-SERVICEHOST-r1')
  }
}

if ($Large -and -not $IsWindows) {
  # C1-REAL-WORLD-SOURCE-WINDOWS-R3: the NET461 large-file profile is qualified on
  # windows/amd64 only; elsewhere it is neither run nor measured.
  $spec = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/native-large-fixtures.json') -Raw | ConvertFrom-Json
  foreach ($fx in $spec.fixtures) {
    $summary.large += [ordered]@{ id = $fx.id; execution_status = 'NOT_RUN'; assessment = 'NOT_APPLICABLE'; code = 'OPERATION_PLATFORM_SCOPE'; reason = $r3Reason }
    if ($Oracle) { $summary.oracle_large += [ordered]@{ id = $fx.id; execution_status = 'NOT_RUN'; assessment = 'NOT_APPLICABLE'; code = 'OPERATION_PLATFORM_SCOPE'; reason = $r3Reason } }
  }
  Write-Output "large: NOT_APPLICABLE on $Platform ($r3Reason)"
} elseif ($Large) {
  $spec = Get-Content -LiteralPath (Join-Path $repo 'src/contracts/native-large-fixtures.json') -Raw | ConvertFrom-Json
  $r = $registry.routes | Where-Object { $_.route -eq $spec.route }
  $root = New-Root $r
  New-Item -ItemType Directory -Force -Path (Join-Path $root 'cases') | Out-Null
  $cases = @()
  foreach ($fx in $spec.fixtures) {
    $sb = [Text.StringBuilder]::new()
    [void]$sb.Append($fx.header)
    for ($i = 0; $i -lt $fx.repeat; $i++) { [void]$sb.Append($fx.block.Replace('{N}', [string]$i)) }
    [void]$sb.Append($fx.footer)
    $bytes = $utf8.GetBytes($sb.ToString())
    $sha = Get-ShaBytes $bytes
    if ($bytes.Length -ne $fx.bytes -or $sha -ne $fx.sha256) { throw "large fixture $($fx.id) identity $($bytes.Length) $sha" }
    $rel = "cases/$($fx.id).cs"
    [IO.File]::WriteAllBytes((Join-Path $root $rel), $bytes)
    $cases += [ordered]@{ id = $fx.id; input = [ordered]@{ path = $rel; role = 'case'; sha256 = $sha; bytes = $bytes.Length }; edits = @()
      points = @($fx.points | ForEach-Object { [ordered]@{ id = $_.id; byte = $_.byte } }); expect = @($fx.expect | ForEach-Object { [ordered]@{ step = 0; syntax = $_.syntax; contains = @(); declarations = $_.declarations } }) }
  }
  $run = Invoke-Profile $r $root 's05-large' 'real-world-source-r3' 'auto' $cases (Get-Declarations $r.route)
  if (-not @($run.res.cases).Count) { $summary.failures += "large: $($run.res.execution_status) build or refusal" }
  foreach ($c in @($run.res.cases)) {
    $t = if (@($c.steps).Count) { $c.steps[0].incremental } else { $null }
    $summary.large += [ordered]@{ id = $c.id; execution_status = $c.execution_status; assessment = $c.assessment; code = $c.code
      form = $(if ($t) { $t.form } else { $null }); parse_ms = $(if ($t) { $t.parse_ms } else { $null }); descendant_count = $(if ($t) { $t.descendant_count } else { $null })
      process_wall_ms = $(if ($c.process) { $c.process.wall_ms } else { $null }); memory_peak = $(if ($c.process) { $c.process.memory.peak_bytes } else { $null })
      errors_total = $(if ($t -and $t.PSObject.Properties['summary']) { $t.summary.errors.total } else { $null }); declarations = $(if ($t -and $t.PSObject.Properties['summary']) { $t.summary.declarations.assessment } else { $null }) }
    Write-Output ("large {0}: {1} {2} form={3} parse_ms={4}" -f $c.id, $c.execution_status, $c.assessment, $(if ($t) { $t.form } else { '' }), $(if ($t) { $t.parse_ms } else { '' }))
  }
  if ($Oracle) {
    # S06-A15/A17: the fact pack on the large fixtures through native-query-large reproduces
    # the S05 declaration items; one input over a limit (every node captured) ends typed
    # RESOURCE_LIMIT.
    $ocases = @(Add-OracleFields $cases)
    # the declaration query alone: the dynamic SQL query's structural candidates (every
    # binary expression of the synthetic blocks) would exceed the output limit by design
    $orun = Invoke-Oracle $r $root 's06-large' 'native-query-large' 'auto' $ocases (Get-Declarations $r.route) (Get-PackQueries 'csharp' 'declarations') $true $false
    $over = @(Add-OracleFields @($cases | Where-Object { $_.id -eq 'cs-large-8m-errors' } | ForEach-Object {
          $x = [ordered]@{}
          foreach ($k in $_.Keys) { $x[$k] = $_[$k] }
          $x.id = 'cs-large-8m-over-captures'
          $x }))
    $orun2 = Invoke-Oracle $r $root 's06-large-over' 'native-query-large' 'auto' $over (Get-Declarations $r.route) @([ordered]@{ id = 'all.nodes'; source = "_ @node`n" }) $false $false
    foreach ($pair in @(@($orun, $false), @($orun2, $true))) {
      $run, $isOver = $pair
      if (-not @($run.res.cases).Count) { $summary.failures += "oracle large: $($run.res.execution_status) build or refusal" }
      $i = 0
      foreach ($c in @($run.res.cases)) {
        $rec = Join-Path $run.out ('records/{0:D5}-{1}.json' -f $i, $c.id)
        $i++
        $d = if (Test-Path -LiteralPath $rec) { Get-Content -LiteralPath $rec -Raw | ConvertFrom-Json } else { $null }
        $repro = if ($d -and $d.PSObject.Properties['facts'] -and $d.facts -and $d.facts.declarations) { $d.facts.declarations.reproduces_s05 } else { $null }
        $summary.oracle_large += [ordered]@{ id = $c.id; execution_status = $c.execution_status; assessment = $c.assessment; code = $c.code; oracle = $c.oracle_claims; declarations_reproduced = $repro
          memory_peak = $(if ($d -and $d.process) { $d.process.memory.peak_bytes } else { $null }); process_wall_ms = $(if ($d -and $d.process) { $d.process.wall_ms } else { $null }) }
        if ($isOver -and $c.execution_status -ne 'RESOURCE_LIMIT') { $summary.failures += "oracle large $($c.id): over-limit input ended $($c.execution_status)" }
        if (-not $isOver -and ($c.execution_status -ne 'COMPLETED' -or $repro -ne $true)) { $summary.failures += "oracle large $($c.id): $($c.execution_status) $($c.code) reproduces=$repro" }
        Write-Output ("oracle large {0}: {1} {2} {3} reproduces={4}" -f $c.id, $c.execution_status, $c.assessment, $c.code, $repro)
      }
    }
  }
}
$summary.finished_at = (Get-Date).ToUniversalTime().ToString('o')
$summary | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath (Join-Path $Destination 'summary.json') -Encoding utf8NoBOM
if (Test-Path -LiteralPath (Join-Path $Destination 'roots')) { Remove-Item -LiteralPath (Join-Path $Destination 'roots') -Recurse -Force }
Write-Output ("routes={0} failures={1}" -f $summary.routes.Count, $summary.failures.Count)
foreach ($f in $summary.failures) { Write-Output "FAILURE $f" }
# Exit explicitly: otherwise the caller's $LASTEXITCODE is the last tsgk exit (3 for the
# expected 32 MiB RESOURCE_LIMIT) and a clean run reads as failed.
if ($summary.failures.Count) { exit 1 }
exit 0
