#Requires -Version 7
# S05 development helper: runs `tsgk incremental` for every registered route on this host
# with the prepared inputs (prepare-routes.ps1), the registered cases
# (src/testdata/native/routes, src/testdata/native/gaps) and, with -Large, the synthetic
# real-world-source-r2 fixtures (src/contracts/native-large-fixtures.json). Writes one
# result directory per route and summary.json under -Destination. Exit 1 when a build
# fails, a case does not complete, or an incremental equality/route claim fails;
# expectation failures are grammar findings for disposition and do not fail the run.
param(
  [Parameter(Mandatory)][string]$Prepared,
  [Parameter(Mandatory)][string]$Destination,
  [Parameter(Mandatory)][string]$Compiler,
  [Parameter(Mandatory)][string]$Platform,
  [string]$CgroupParent = '',
  [string[]]$Routes = @(),
  [switch]$Large
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

function Get-Sha([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Get-ShaBytes([byte[]]$Data) { [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($Data)).ToLowerInvariant() }
function Find-Bytes([byte[]]$Hay, [byte[]]$Needle) {
  if ($Needle.Length -eq 0) { return $Hay.Length }
  # Latin-1 maps every byte to one char, so an ordinal string search is a byte search.
  $l1 = [Text.Encoding]::Latin1
  return $l1.GetString($Hay).IndexOf($l1.GetString($Needle), [StringComparison]::Ordinal)
}

$cli = Join-Path $Destination ('tsgk' + $(if ($IsWindows) { '.exe' } else { '' }))
Push-Location $repo
try { go build -o $cli ./src/cmd/tsgk; if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' } } finally { Pop-Location }
$ccVersion = (& $Compiler --version | Select-Object -First 1)
if ($LASTEXITCODE -ne 0) { throw 'compiler --version failed' }
$compilerId = [ordered]@{ name = 'cc'; version = 'host'; sha256 = (Get-Sha $Compiler); bytes = (Get-Item -LiteralPath $Compiler).Length }
$work = Join-Path $Destination 'work'
New-Item -ItemType Directory -Path $work | Out-Null
$summary = [ordered]@{ schema = 'tsgk-s05-route-run/r1'; platform = $Platform; compiler = [ordered]@{ path = $Compiler; version_line = $ccVersion; sha256 = $compilerId.sha256; bytes = $compilerId.bytes }
  started_at = (Get-Date).ToUniversalTime().ToString('o'); routes = @(); large = @(); failures = @() }

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

function Invoke-Profile($r, [string]$Root, [string]$Id, [string]$Operation, [string]$Output, $Cases, $Decls) {
  # the profile requires byte-ascending paths
  $sorted = [Collections.Generic.List[object]]::new()
  foreach ($f in $r.files) { $sorted.Add([ordered]@{ path = $f.path; role = $f.role; sha256 = $f.sha256; bytes = $f.bytes }) }
  $sorted.Sort([Comparison[object]] { param($a, $b) [string]::CompareOrdinal($a.path, $b.path) })
  $profile = [ordered]@{ schema = 'tsgk-incremental/r1'; id = $Id; route = $r.route; operation = $Operation; symbol = $r.symbol; encoding = 'UTF-8'; output = $Output
    compiler = $compilerId; grammar = @($sorted); declarations = $Decls; cases = @($Cases) }
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

foreach ($r in $registry.routes) {
  if ($Routes.Count -and $Routes -notcontains $r.route) { continue }
  $root = New-Root $r
  $cases = @()
  foreach ($kind in @('routes', 'gaps')) {
    $file = Join-Path $repo "src/testdata/native/$kind/$($r.route).json"
    if (-not (Test-Path -LiteralPath $file)) { continue }
    $doc = Get-Content -LiteralPath $file -Raw | ConvertFrom-Json
    foreach ($c in $doc.cases) {
      $bytes = $utf8.GetBytes($c.source_utf8)
      $rel = "cases/$($c.id).txt"
      $path = Join-Path $root $rel
      New-Item -ItemType Directory -Force -Path (Split-Path -Parent $path) | Out-Null
      [IO.File]::WriteAllBytes($path, $bytes)
      $cur = $bytes
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
      }
      $expect = @($c.expect | ForEach-Object { [ordered]@{ step = $_.step; syntax = $_.syntax; contains = @($_.contains); declarations = '' } })
      $cases += [ordered]@{ id = $c.id; input = [ordered]@{ path = $rel; role = 'case'; sha256 = (Get-ShaBytes $bytes); bytes = $bytes.Length }; edits = $edits; points = @(); expect = $expect }
    }
  }
  if (-not $cases.Count) { $summary.failures += "$($r.route): no registered cases"; continue }
  $run = Invoke-Profile $r $root "s05-$($r.route)" 'native-parse-edit' 'tree' $cases (Get-Declarations $r.route)
  $res = $run.res
  $entry = [ordered]@{ route = $r.route; exit = $run.code; execution_status = $res.execution_status; assessment = $res.assessment; build = $null; cases = @() }
  if ($res.build) {
    $entry.build = [ordered]@{ identity = $res.build.identity; executable_sha256 = $res.build.executable_sha256; compiler_version = $res.build.compiler_version
      steps = @($res.build.steps | ForEach-Object { [ordered]@{ name = $_.name; status = $_.result.status; exit = $_.result.exit_code; wall_ms = $_.result.wall_ms } }) }
  }
  foreach ($f in @($res.findings)) { if ($f -and $f.severity -eq 'error') { $summary.failures += "$($r.route): $($f.code)" } }
  foreach ($c in @($res.cases)) {
    $fails = @($c.expectations | Where-Object { $_.result -ne 'PASS' } | ForEach-Object { "step $($_.step): $($_.result) $($_.detail)" })
    $entry.cases += [ordered]@{ id = $c.id; execution_status = $c.execution_status; assessment = $c.assessment; code = $c.code; claims = $c.claims; expectation_failures = $fails
      steps = @($c.steps | ForEach-Object { [ordered]@{ step = $_.step; has_error = $_.incremental.has_error; digest = $_.incremental.digest; equal = $(if ($_.comparison) { $_.comparison.equal } else { $null }); reused = $(if ($_.route) { $_.route.reused_nodes } else { $null }) } }) }
    if ($c.execution_status -ne 'COMPLETED') { $summary.failures += "$($r.route)/$($c.id): $($c.execution_status) $($c.code)" }
    if ($c.claims.incremental_equality -eq 'FAIL' -or $c.claims.incremental_route -eq 'FAIL') { $summary.failures += "$($r.route)/$($c.id): incremental $($c.claims.incremental_equality)/$($c.claims.incremental_route) $($c.code)" }
  }
  if ($res.execution_status -ne 'COMPLETED' -and -not @($res.cases).Count) { $summary.failures += "$($r.route): $($res.execution_status) build or refusal" }
  $summary.routes += $entry
  Write-Output ("route {0}: exit={1} status={2} assessment={3} cases={4}" -f $r.route, $run.code, $res.execution_status, $res.assessment, @($res.cases).Count)
}

if ($Large) {
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
  $run = Invoke-Profile $r $root 's05-large' 'real-world-source-r2' 'auto' $cases (Get-Declarations $r.route)
  foreach ($c in @($run.res.cases)) {
    $t = if (@($c.steps).Count) { $c.steps[0].incremental } else { $null }
    $summary.large += [ordered]@{ id = $c.id; execution_status = $c.execution_status; assessment = $c.assessment; code = $c.code
      form = $(if ($t) { $t.form } else { $null }); parse_ms = $(if ($t) { $t.parse_ms } else { $null }); descendant_count = $(if ($t) { $t.descendant_count } else { $null })
      process_wall_ms = $(if ($c.process) { $c.process.wall_ms } else { $null }); memory_peak = $(if ($c.process) { $c.process.memory.peak_bytes } else { $null })
      errors_total = $(if ($t -and $t.summary) { $t.summary.errors.total } else { $null }); declarations = $(if ($t -and $t.summary) { $t.summary.declarations.assessment } else { $null }) }
    Write-Output ("large {0}: {1} {2} form={3} parse_ms={4}" -f $c.id, $c.execution_status, $c.assessment, $(if ($t) { $t.form } else { '' }), $(if ($t) { $t.parse_ms } else { '' }))
  }
}
$summary.finished_at = (Get-Date).ToUniversalTime().ToString('o')
$summary | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath (Join-Path $Destination 'summary.json') -Encoding utf8NoBOM
Remove-Item -LiteralPath (Join-Path $Destination 'roots') -Recurse -Force
Write-Output ("routes={0} failures={1}" -f $summary.routes.Count, $summary.failures.Count)
foreach ($f in $summary.failures) { Write-Output "FAILURE $f" }
if ($summary.failures.Count) { exit 1 }
