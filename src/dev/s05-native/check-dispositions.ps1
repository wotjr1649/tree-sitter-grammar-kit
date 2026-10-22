# #217: fails when a run-routes summary shows a registered case's (or SVC segment's)
# incremental route BLOCKED, which is undisposed: the remaining error-tree BLOCKED originals
# are dispositioned outside the registered corpus in src/testdata/native/reuse-holdouts.json.
# A summary that cannot show it (another schema, requirement_results missing, null or not an
# array, an entry without a claim) also fails. run-routes.ps1 keeps recording the BLOCKED as
# a requirement result; this check adds the disposition gate on top of it.
param([Parameter(Mandatory)][string]$Summary)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$s = Get-Content -Raw -LiteralPath $Summary | ConvertFrom-Json
if ($s -isnot [pscustomobject] -or -not $s.PSObject.Properties['schema'] -or $s.schema -ne 'tsgk-s05-route-run/r1') { throw "not a tsgk-s05-route-run/r1 summary" }
$results = $s.PSObject.Properties['requirement_results']
if (-not $results -or $results.Value -isnot [array]) { throw "requirement_results missing or not an array" }
foreach ($r in $results.Value) {
  if ($r -isnot [pscustomobject] -or -not $r.PSObject.Properties['claim'] -or $r.claim -isnot [string]) { throw "requirement result without a claim" }
}
$undisposed = @($results.Value | Where-Object { $_.claim -eq 'incremental_route' })
if ($undisposed.Count -ne 0) { throw "undisposed route BLOCKED (#217): $(@($undisposed | ForEach-Object { $_.case }) -join ', ')" }
Write-Output ("dispositions: requirement_results={0} undisposed_route_blocked=0" -f $results.Value.Count)
