#Requires -Version 7
# S08 development helper: prints the 78-cell matrix of a tsgk-qualification-result/r1 and
# fails (exit 1) only on the kit gate: completeness, run cohort and eligibility, every
# cell's kit mechanism and cross-platform comparison, the executed extra-role rows
# (mechanism_gate). Grammar requirement failures and uncovered obligations are matrix
# results for disposition: they keep the support claim BLOCKED and do not fail the job.
param(
  [Parameter(Mandatory)][string]$Result,
  [Parameter(Mandatory)][int]$QualifyExit
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
if ($QualifyExit -notin @(0, 1, 3)) { Write-Output "qualify ended without a complete result (exit $QualifyExit)"; exit 1 }
if (-not (Test-Path -LiteralPath $Result -PathType Leaf)) { Write-Output 'qualification result missing'; exit 1 }
$r = Get-Content -LiteralPath $Result -Raw | ConvertFrom-Json
if ($r.result_schema -ne 'tsgk-qualification-result/r1') { Write-Output 'not a qualification result'; exit 1 }
foreach ($line in $r.explanation) { Write-Output $line }
$platforms = @($r.cells | ForEach-Object { $_.platform } | Select-Object -Unique)
Write-Output ("{0,-15} {1}" -f 'route', ($platforms -join ' '))
foreach ($route in @($r.cells | ForEach-Object { $_.route } | Select-Object -Unique)) {
  $row = foreach ($p in $platforms) {
    $c = @($r.cells | Where-Object { $_.route -eq $route -and $_.platform -eq $p })[0]
    '{0}({1}/{2}/{3})' -f $c.status, $c.mechanism, $c.requirement, $c.comparison
  }
  Write-Output ("{0,-15} {1}" -f $route, ($row -join ' '))
}
foreach ($x in $r.extra_roles) { Write-Output ("role {0} {1} {2}" -f $x.id, $x.platform, $x.status) }
foreach ($f in $r.findings) { Write-Output ("FINDING {0} {1}" -f $f.code, $f.path) }
foreach ($c in $r.cells) {
  foreach ($f in $c.set.findings) { Write-Output ("CELL {0}@{1} {2} {3}" -f $c.route, $c.platform, $f.code, $f.path) }
}
foreach ($c in $r.comparisons) {
  foreach ($d in $c.differences) { Write-Output ("DIFFERENCE {0} {1}" -f $c.set, $d) }
}
Write-Output ("assessment={0} completeness={1} mechanism_gate={2} support_claim={3}" -f $r.assessment, $r.completeness, $r.mechanism_gate, $r.support_claim)
if ($r.mechanism_gate -ne 'PASS') { exit 1 }
exit 0
