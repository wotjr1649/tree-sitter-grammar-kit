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
$imageSubject='5dc3d89579acd801130549ba35c989055b8e548d4ba73e51cc365760d9c4ac09'
$legacy=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile bookworm-r1
$originalInputs=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'inputs.json') -Raw|ConvertFrom-Json
if($legacy.image -cne $originalInputs.image -or $legacy.compressed_bytes -ne $originalInputs.image_compressed_bytes){throw 'Original image identity changed'}
$candidate=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject
if($candidate.image -cne 'node@sha256:98ad2493de85738f55c11fe22e8586caf1fd917b7a8075c57ab9c55116e06492' -or $candidate.compressed_bytes -ne 440298459){throw 'Proposed image identity mismatch'}
foreach($vector in @(@{ImageProfile='trixie-r1';ImageSubject=''},@{ImageProfile='trixie-r1';ImageSubject=$subject},@{ImageProfile='bookworm-r1';ImageSubject=$imageSubject},@{ImageProfile='unknown';ImageSubject=$imageSubject},@{ImageProfile='';ImageSubject=$imageSubject})){
    $rejected=$false;try{$null=& $approval -Profile pinned-tsql-r1 -Subject $subject @vector}catch{$rejected=$true}
    if(-not $rejected){throw 'Unbound image profile/approval was accepted'}
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
foreach($helper in @('CheckElfClosure','CheckOwnedControl','ReadGrammarName')){
    $definition=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $helper},$true))
    if($definition.Count -ne 1){throw 'Control helper identity mismatch'}
    . ([scriptblock]::Create($definition[0].Extent.Text))
}
$elf="Class: ELF64`nMachine: Advanced Micro Devices X86-64`n[Requesting program interpreter: /lib64/ld-linux-x86-64.so.2]`n(NEEDED) Shared library: [libc.so.6]"
CheckElfClosure $elf
foreach($invalid in @($elf.Replace('ELF64','ELF32'),$elf.Replace('X86-64','AArch64'),$elf.Replace('/lib64/ld-linux-x86-64.so.2','/unapproved/loader'),$elf.Replace('libc.so.6','unapproved.so'),($elf+"`n(RUNPATH) /unapproved"))){
    $rejected=$false;try{CheckElfClosure $invalid}catch{$rejected=$true};if(-not $rejected){throw 'Unsafe ELF closure was accepted'}
}
$controlTree=@{kind='source_file';children=@(@{kind='declaration';children=@(@{kind='identifier';field='name';start=5;end=9},@{kind='identifier';field='body';start=12;end=17})})}
$control=@(foreach($stage in @('original','damaged-incremental','damaged-fresh','restored-incremental','restored-fresh')){@{stage=$stage;has_error=$stage.StartsWith('damaged');tree=$controlTree}})+@(@{comparison='damaged';equal=$true},@{comparison='restored';equal=$true})
CheckOwnedControl $control
$controlTree.children[0].children[1].field='wrong'
$rejected=$false;try{CheckOwnedControl $control}catch{$rejected=$true};if(-not $rejected){throw 'Wrong owned field was accepted'}
$controlTree.children[0].children[1].field='body';$control[-1].equal=$false
$rejected=$false;try{CheckOwnedControl $control}catch{$rejected=$true};if(-not $rejected){throw 'Unequal edit trees were accepted'}
$control[-1].equal=$true;$control[1].has_error=$false
$rejected=$false;try{CheckOwnedControl $control}catch{$rejected=$true};if(-not $rejected){throw 'Missing owned negative error was accepted'}
$control[1].has_error=$true;$control[-1].comparison='damaged'
$rejected=$false;try{CheckOwnedControl $control}catch{$rejected=$true};if(-not $rejected){throw 'Missing restored comparison was accepted'}
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
$grammarPath=Join-Path $root 'grammar.json'
[IO.File]::WriteAllText($grammarPath,'{"name":"TSQL","rules":{"AS":{"type":"STRING","value":"AS"},"as":{"type":"STRING","value":"as"}}}',[Text.UTF8Encoding]::new($false))
if((ReadGrammarName $grammarPath) -cne 'TSQL'){throw 'Case-distinct grammar keys/name were not preserved'}
[IO.File]::WriteAllText($grammarPath,'{"name":"invalid-name","rules":{}}',[Text.UTF8Encoding]::new($false))
$rejected=$false;try{[void](ReadGrammarName $grammarPath)}catch{$rejected=$true}
if(-not $rejected){throw 'Invalid grammar name was accepted'}
Add-Type -AssemblyName System.Formats.Tar
foreach($helper in @('UnpackResult','CheckRecoveredProof')){
    $definition=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $helper},$true))
    if($definition.Count -ne 1){throw 'Recovery helper identity mismatch'}
    . ([scriptblock]::Create($definition[0].Extent.Text))
}
$proofBytes=[Convert]::FromHexString('707265736572766564000D0A')
$archive=Join-Path $root 'binary-proof.tar'
$stream=[IO.File]::Open($archive,[IO.FileMode]::CreateNew)
$writer=[Formats.Tar.TarWriter]::new($stream)
$entry=[Formats.Tar.UstarTarEntry]::new([Formats.Tar.TarEntryType]::RegularFile,'./proof')
$entry.DataStream=[IO.MemoryStream]::new($proofBytes)
try{$writer.WriteEntry($entry)}finally{$entry.DataStream.Dispose();$writer.Dispose();$stream.Dispose()}
$recovered=Join-Path $root 'recovered'
UnpackResult $archive $recovered 1048576
CheckRecoveredProof (Join-Path $recovered 'proof')
foreach($altered in @('7072657365727665640D0A','707265736572766564000A','707265736572766564000D0B')){
    [IO.File]::WriteAllBytes((Join-Path $recovered 'proof'),[Convert]::FromHexString($altered))
    $rejected=$false;try{CheckRecoveredProof (Join-Path $recovered 'proof')}catch{$rejected=$true}
    if(-not $rejected){throw 'Changed NUL/CRLF proof was accepted'}
}
foreach($mode in @(493,420)){
    $archive=Join-Path $root "mode-$mode.tar"
    $stream=[IO.File]::Open($archive,[IO.FileMode]::CreateNew);$writer=[Formats.Tar.TarWriter]::new($stream)
    $entry=[Formats.Tar.UstarTarEntry]::new([Formats.Tar.TarEntryType]::RegularFile,'./probe')
    $entry.Mode=[IO.UnixFileMode]$mode;$entry.DataStream=[IO.MemoryStream]::new($proofBytes)
    try{$writer.WriteEntry($entry)}finally{$entry.DataStream.Dispose();$writer.Dispose();$stream.Dispose()}
    $modeRoot=Join-Path $root "mode-$mode"
    if($mode -eq 493){
        if(-not $IsWindows){
            UnpackResult $archive $modeRoot 1048576 -Executable
            CheckRecoveredProof (Join-Path $modeRoot 'probe')
            if([int][IO.File]::GetUnixFileMode((Join-Path $modeRoot 'probe')) -ne 493){throw 'Allowed executable mode changed'}
        }
    }else{
        $rejected=$false;try{UnpackResult $archive $modeRoot 1048576 -Executable}catch{$rejected=$true}
        if(-not $rejected){throw 'Unexpected executable mode accepted'}
    }
}
& {
    param($sourceAst,$caseRoot)
    $root=Join-Path $caseRoot 'command-lifecycle'
    [void][IO.Directory]::CreateDirectory((Join-Path $root 'records'))
    foreach($helper in @('Record','Require','ConfirmFrozenState','Freeze','Snapshot','StopContainer')){
        $definition=@($sourceAst.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $helper},$true))
        if($definition.Count -ne 1){throw 'Lifecycle helper identity mismatch'}
        . ([scriptblock]::Create($definition[0].Extent.Text))
    }
    $script:containers=[Collections.Generic.List[string]]::new();$script:containers.Add('owned')
    $script:captureToolVerified=$true;$script:captures=0
    # Exercise real receipt naming and lifecycle calls with owned Docker responses.
    function Run([string]$label,[string[]]$argv,[int]$seconds,[long]$limit=8388608,[switch]$Cleanup){
        Record ('command-'+$label) @{argv=$argv;seconds=$seconds;simulation=$true}
        $text=if($argv[0] -eq 'top'){"PID COMMAND`n3253 /bin/sleep infinity"}
            elseif($label.EndsWith('-stopped')){'false 0'}
            elseif($argv.Count -gt 2 -and $argv[2].Contains('.State.Pid')){'true true 3253'}
            else{'true true'}
        return @{label=$label;exit_code=0;termination='EXITED';text=$text}
    }
    function TextOutput($result){return $result.text}
    [void](Snapshot 'owned' 'operation' 1048576)
    StopContainer 'owned' 'operation'
    if($script:containers.Count){throw 'Lifecycle receipt fixture incomplete'}
} $ast $root
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
@{result='PASS';checks=@('duplicate PATH applications choose first exact path','missing application rejected','separate acquisition, capture and image subjects required','empty, old B, mismatched and unknown profiles rejected','original image identity preserved; new image bound to separate subject','case-distinct grammar keys accepted and exact name retained; invalid name rejected','ELF arch/loader/library/RUNPATH mismatches rejected','owned structure/edit checker rejects wrong fields, unequal trees and missing negative error','frozen init exact identity; live/stopped/child/PID mismatch/forged command rejected','NUL/CRLF tar recovery preserves bytes; deleted/replaced bytes rejected','executable archive mode mismatch rejected','real Freeze/StopContainer receipt names stay unique with owned responses');unix_mode_roundtrip=$(if($IsWindows){'NOT_APPLICABLE'}else{'PASS'});fixture_processes_executed=0;run_sha256=(Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'run.ps1')).Hash.ToLowerInvariant();approval_sha256=(Get-FileHash -LiteralPath $approval).Hash.ToLowerInvariant()}|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $root 'self-test.json') -Encoding utf8NoBOM
Write-Output 'P05 application discovery and acquisition approval self-check PASS; fixture execution 0'
