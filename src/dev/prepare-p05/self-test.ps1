param([Parameter(Mandatory)][string]$Destination)
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion.Major -ne 7){throw 'PowerShell 7 required'}
$approval=Join-Path $PSScriptRoot 'approval.ps1'
$subject='9e07792474be6b96406cba915c30c90696a42299ffa9dd8ac60324b3b7a69367'
& $approval -Profile archive-r1
& $approval -Profile pinned-tsql-r1 -Subject $subject
foreach($vector in @(@{Profile='pinned-tsql-r1';Subject=''},@{Profile='pinned-tsql-r1';Subject='b28d726cf456472b8d37117818930500188f730aafb18bcb6b636214d34ec752'},@{Profile='archive-r1';Subject=$subject},@{Profile='unknown';Subject=$subject})){
    $rejected=$false;try{& $approval @vector}catch{$rejected=$true}
    if(-not $rejected){throw 'Unbound acquisition profile/approval was accepted'}
}
& $approval -Profile pinned-tsql-r1 -Subject $subject -Execution -ExecutionSubject '1cdf1711088ebaba3347ce617ddfc733b0e4323a401a6c9efe50c37cf1deb42e'
foreach($invalidSubject in @('',$subject)){
    $rejected=$false;try{& $approval -Profile pinned-tsql-r1 -Subject $subject -Execution -ExecutionSubject $invalidSubject}catch{$rejected=$true}
    if(-not $rejected){throw 'Old/missing approval authorized new capture'}
}
$root=[IO.Path]::GetFullPath($Destination)
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$prefixes=@([IO.Path]::GetFullPath((Join-Path $repoRoot '.work/campaign-01-prepare-03')))
if($env:RUNNER_TEMP){$prefixes += [IO.Path]::GetFullPath($env:RUNNER_TEMP)}
if(-not @($prefixes|Where-Object {$root.StartsWith($_.TrimEnd('/','\')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::Ordinal) }).Count -or (Test-Path -LiteralPath $root)){throw 'Fresh task test destination required'}
$ancestor=Get-Item -LiteralPath ([IO.Path]::GetDirectoryName($root)) -Force
while($ancestor){if($ancestor.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Reparse test ancestor'};$ancestor=$ancestor.Parent}
$errors=$null;$tokens=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'run.ps1'),[ref]$tokens,[ref]$errors)
if($errors){throw 'Harness parse failed'}
function RejectReadOnlyAssignments($candidate){
    $protected=@(Get-Variable|Where-Object {$_.Options -band ([Management.Automation.ScopedItemOptions]::ReadOnly -bor [Management.Automation.ScopedItemOptions]::Constant)}|ForEach-Object Name)
    foreach($assignment in $candidate.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left -is [Management.Automation.Language.VariableExpressionAst]},$true)){
        if(($assignment.Left.VariablePath.UserPath -split ':')[-1] -in $protected){throw 'Read-only PowerShell variable assignment'}
    }
}
RejectReadOnlyAssignments $ast
$rejected=$false;try{RejectReadOnlyAssignments ([scriptblock]::Create('$PID=1').Ast)}catch{$rejected=$true}
if(-not $rejected){throw 'Read-only assignment negative case failed'}
$function=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Application'},$true))
if($function.Count -ne 1){throw 'Application helper identity mismatch'}
. ([scriptblock]::Create($function[0].Extent.Text))
$freezeFunction=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'ConfirmFrozenState'},$true))
if($freezeFunction.Count -ne 1){throw 'Freeze helper identity mismatch'}
. ([scriptblock]::Create($freezeFunction[0].Extent.Text))
$validTop="PID COMMAND`n3253 /bin/sleep infinity`n"
if((ConfirmFrozenState 'true true 3253' $validTop) -ne 3253){throw 'Frozen init positive case failed'}
foreach($vector in @(@('true false 3253',$validTop),@('false true 3253',$validTop),@('true true 1',$validTop),@('true true 3253',"PID COMMAND`n3253 /bin/sleep infinity`n3254 child"),@('true true 3253',"PID COMMAND`n3253 forged /bin/sleep infinity"),@('true true 3253',"PID COMMAND`n1 /bin/sleep infinity"))){
    $rejected=$false;try{[void](ConfirmFrozenState $vector[0] $vector[1])}catch{$rejected=$true}
    if(-not $rejected){throw 'Unsafe/nonquiescent container was accepted'}
}
$name=if($IsWindows){'p05-tool-check.cmd'}else{'p05-tool-check'}
$directories=@((Join-Path $root 'first'),(Join-Path $root 'second'))
foreach($directory in $directories){
    [void][IO.Directory]::CreateDirectory($directory)
    $path=Join-Path $directory $name
    [IO.File]::WriteAllText($path,"# owned discovery-only fixture`n",[Text.UTF8Encoding]::new($false))
    if(-not $IsWindows){[IO.File]::SetUnixFileMode($path,[IO.UnixFileMode]493)}
}
$savedPath=$env:PATH
try {
    $env:PATH=$directories -join [IO.Path]::PathSeparator
    $all=@(Get-Command -Name $name -CommandType Application -All)
    if($all.Count -ne 2){throw 'Duplicate application baseline missing'}
    $selected=Application $name
    if($selected -isnot [string] -or $selected -cne (Join-Path $directories[0] $name)){throw 'Application precedence/path selection failed'}
    $rejected=$false;try{[void](Application 'p05-tool-not-present')}catch{$rejected=$true}
    if(-not $rejected){throw 'Missing application was accepted'}
} finally {$env:PATH=$savedPath}
@{result='PASS';checks=@('duplicate PATH applications choose first exact path','missing application rejected','separate acquisition and capture subjects required','empty, old B, mismatched and unknown profiles rejected','frozen init exact identity; live/stopped/child/PID mismatch/forged command rejected');fixture_processes_executed=0;run_sha256=(Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'run.ps1')).Hash.ToLowerInvariant();approval_sha256=(Get-FileHash -LiteralPath $approval).Hash.ToLowerInvariant()}|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $root 'self-test.json') -Encoding utf8NoBOM
Write-Output 'P05 application discovery and acquisition approval self-check PASS; fixture execution 0'
