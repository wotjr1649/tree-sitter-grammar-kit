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
$remedySubject='42396d74938e6938d38aa9adc1ea84fbe05a708fa220ca09074dc1bb56d671f4'
$r2Subject='a72b87c3dfe6561855749b64cce03bdaa5d7c231948f42dfa6ef41ce84a7747e'
foreach($stage in @('patch-r1','sql-pg-r1','patch-r2','sql-pg-r2')){
    $binding=if($stage.EndsWith('r2')){$r2Subject}else{$remedySubject}
    $selected=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $binding
    if($selected.acquisition_limit_bytes -ne $(if($stage -ceq 'sql-pg-r2'){1610612736L}else{1073741824L})){throw 'Stage cap leaked into another route'}
    $wrong=if($stage.EndsWith('r2')){$remedySubject}else{$r2Subject}
    $rejected=$false;try{$null=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $wrong}catch{$rejected=$true}
    if(-not $rejected){throw 'Old/new remedy subject cross-binding accepted'}
}
foreach($stage in @('patch-r1','sql-pg-r1')){$null=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $remedySubject}
foreach($vector in @(@{RemedyStage='patch-r1';RemedySubject=''},@{RemedyStage='patch-r1';RemedySubject=$subject},@{RemedyStage='unknown';RemedySubject=$remedySubject},@{RemedyStage='';RemedySubject=$remedySubject})){
    $rejected=$false;try{$null=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject @vector}catch{$rejected=$true}
    if(-not $rejected){throw 'Unbound remedy stage/subject accepted'}
}
$rejected=$false;try{$null=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile bookworm-r1 -RemedyStage patch-r1 -RemedySubject $remedySubject}catch{$rejected=$true}
if(-not $rejected){throw 'Legacy image authorized remedy work'}
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
. (Join-Path $PSScriptRoot 'remedy.ps1')
$remedySubjects=ReadRemedySubjects
foreach($stage in @('patch-r1','sql-pg-r1')){
    $rows=NewRemedyRows $stage $originalInputs.cases $remedySubjects.cases.cases
    if($rows.Count -ne $(if($stage -ceq 'patch-r1'){85}else{46}) -or @($rows|Where-Object {$_.case.edit}).Count -ne $(if($stage -ceq 'patch-r1'){16}else{3})){throw 'Remedy rows/edit budget mismatch'}
    if($stage -ceq 'patch-r1' -and @($rows|Where-Object {$_.producer.EndsWith('-original') -and $_.id -cnotlike 'P05-REMEDY-*'}).Count){throw 'Identical failing baseline scheduled again'}
}
$r2Rows=NewRemedyRows 'patch-r2' $originalInputs.cases $remedySubjects.cases.cases
if($r2Rows.Count -ne 39 -or @($r2Rows|Where-Object {$_.case.edit}).Count -ne 7 -or @($r2Rows|Where-Object {$_.route -ceq 'csharp'}).Count -ne 27 -or @($r2Rows|Where-Object {$_.route -ceq 'typescript'}).Count -ne 4 -or @($r2Rows|Where-Object {$_.route -ceq 'tsx'}).Count -ne 8 -or @($r2Rows|Where-Object {$_.producer -cnotlike '*-candidate-r2'}).Count){throw 'Exact r2 producer budget mismatch'}
$sqlRows=NewRemedyRows 'sql-pg-r2' $originalInputs.cases $remedySubjects.cases.cases
if($sqlRows.Count -ne 46 -or @($sqlRows|Where-Object {$_.case.edit}).Count -ne 3){throw 'SQLPG row/edit identity changed'}
$altered=$originalInputs.cases|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable
@($altered|Where-Object route -CEQ 'csharp')[0].expected.facts='altered expectation'
$rejected=$false;try{$null=NewRemedyRows 'patch-r2' $altered $remedySubjects.cases.cases}catch{$rejected=$true}
if(-not $rejected){throw 'Changed exact r2 expectation accepted'}
$rejected=$false;try{$null=NewRemedyRows 'unknown' $originalInputs.cases $remedySubjects.cases.cases}catch{$rejected=$true}
if(-not $rejected){throw 'Unknown remedy stage scheduled'}
foreach($case in $remedySubjects.cases.cases){
    $bytes=[Text.Encoding]::UTF8.GetBytes($case.input_utf8)
    AssertRemedyObject $bytes.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()) @{bytes=$case.input_bytes;sha256=$case.input_sha256}
    if($case.edit -and ($case.edit.start_byte -ge $case.edit.old_end_byte -or $case.edit.old_end_byte -gt $bytes.Length)){throw 'New edit outside original source'}
}
$fixture='before';$changed='after'
$pin=@{original_bytes=6;original_sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($fixture))).ToLowerInvariant();operations=@(@{before=$fixture;after=$changed;occurrences=1});proposed_result_bytes=5;proposed_result_sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($changed))).ToLowerInvariant()}
if((ApplyLiteralPatch $fixture $pin) -cne $changed){throw 'Owned literal patch failed'}
foreach($kind in @('source','count','output')){
    $bad=$pin.Clone();$bad.operations=@(@{before=$fixture;after=$changed;occurrences=$(if($kind -ceq 'count'){2}else{1})})
    if($kind -ceq 'output'){$bad.proposed_result_sha256='0'*64}
    $rejected=$false;try{$null=ApplyLiteralPatch $(if($kind -ceq 'source'){'Before'}else{$fixture}) $bad}catch{$rejected=$true}
    if(-not $rejected){throw 'Changed source/count/output patch accepted'}
}
foreach($helper in @('CheckElfClosure','CheckOwnedControl','ReadGrammarName','CheckPriorEvidenceSubjects','FileIdentity','CheckInertImports','CheckJsInputs','CaseLedger')){
    $definition=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $helper},$true))
    if($definition.Count -ne 1){throw 'Control helper identity mismatch'}
    . ([scriptblock]::Create($definition[0].Extent.Text))
}
$swiftSource='e798585e0b27886fce7fc540b3e246073bd6bedd2d7d18c63c5832b6a148db2b'
CheckInertImports $swiftSource '50974:import"'
foreach($vector in @(@($swiftSource,'50975:import"'),@($swiftSource,'50974:import";50980:import('),@('unreviewed','50974:import"'))){
    $rejected=$false;try{CheckInertImports $vector[0] $vector[1]}catch{$rejected=$true}
    if(-not $rejected){throw 'Unreviewed Swift import occurrence accepted'}
}
foreach($binding in @(@('ea0bed0718ca8db1f69c826a41776bff98488afc451c57b2994529dcb6f4b593',"10284:import';18414:import';21263:import("),@('21559c1095e398e31e22d601af1388b107009a9dde888daf748c17878186f846',"10301:import';18431:import';21280:import("),@('bcff6a77ef53571245cdccf1799c62229548fca75f503ea258040df086f03512','51547:import"'))){
    CheckInertImports $binding[0] $binding[1]
    $rejected=$false;try{CheckInertImports $binding[0] ($binding[1]+';1:import(')}catch{$rejected=$true}
    if(-not $rejected){throw 'Extra import in approved candidate accepted'}
}
$priorTsql='5774bbd37ae4a3eebf4228fa61e9600a10f6c822317e394d950d9f7caa63f458'
$priorCsharp='98590999770a8681c3c3347dc61b3aae81dd30e3b26629a66e49b4c706d3be79'
$priorTsPg='7b47cfe79823f84575cc981578f8c9024ff7f6821edb3b852656a5cc58c43bb9'
CheckPriorEvidenceSubjects '' '' 'bookworm-r1'
CheckPriorEvidenceSubjects $priorTsql $priorCsharp 'trixie-r1'
CheckPriorEvidenceSubjects $priorTsql $priorCsharp 'trixie-r1' $priorTsPg
foreach($vector in @(@('trixie-r1','unknown'),@('bookworm-r1',$priorTsPg))){
    $rejected=$false;try{CheckPriorEvidenceSubjects $priorTsql $priorCsharp $vector[0] $vector[1]}catch{$rejected=$true}
    if(-not $rejected){throw 'Mismatched TS/PG stage observation accepted'}
}
foreach($vector in @(@('unknown','','trixie-r1'),@($priorTsql,'','bookworm-r1'),@('','unknown','trixie-r1'),@('',$priorCsharp,'bookworm-r1'))){
    $rejected=$false;try{CheckPriorEvidenceSubjects $vector[0] $vector[1] $vector[2]}catch{$rejected=$true}
    if(-not $rejected){throw 'Mismatched prior observation/image accepted'}
}
& {
    param($caseInputs,$tsql,$csharp)
    $inputs=$caseInputs;$TsqlPriorEvidenceSubject=$tsql;$CsharpPriorEvidenceSubject=$csharp
    $script:commands=[Collections.Generic.List[object]]::new()
    $ledger=CaseLedger
    $prior=@($ledger|Where-Object prior_evidence_subject)
    if($ledger.Count -ne 111 -or $prior.Count -ne 76 -or @($ledger|Where-Object state -CEQ 'NOT_RUN').Count -ne 35 -or @($prior|Where-Object {$null -ne $_.exit_code -or $null -ne $_.raw_stdout}).Count -or @($prior|Where-Object edit_registered).Count -ne 4){throw 'Prior observations lost, relabelled current, or altered case scope'}
    if(@($prior|Where-Object {$_.state -cne 'NOT_REEXECUTED_PRIOR_OBSERVED_RESULTS_RETAINED' -or ($_.route -ceq 'tsql' -and $_.prior_evidence_subject -cne $tsql) -or ($_.route -ceq 'csharp' -and $_.prior_evidence_subject -cne $csharp) -or $_.route -cnotin @('tsql','csharp')}).Count){throw 'Prior route/subject/state mapping changed'}
} $originalInputs $priorTsql $priorCsharp
& {
    param($caseInputs,$tsql,$csharp,$tsPg)
    $inputs=$caseInputs;$TsqlPriorEvidenceSubject=$tsql;$CsharpPriorEvidenceSubject=$csharp;$TsPgStageEvidenceSubject=$tsPg
    $script:commands=[Collections.Generic.List[object]]::new()
    $ledger=CaseLedger;$prior=@($ledger|Where-Object prior_evidence_subject)
    $blocked=@($prior|Where-Object state -CEQ 'NOT_REEXECUTED_PRIOR_STAGE_BLOCKER_RETAINED')
    if($ledger.Count -ne 111 -or $prior.Count -ne 108 -or $blocked.Count -ne 16 -or @($ledger|Where-Object state -CEQ 'NOT_RUN').Count -ne 3 -or @($prior|Where-Object {$null -ne $_.exit_code -or $null -ne $_.raw_stdout}).Count -or @($prior|Where-Object edit_registered).Count -ne 10){throw 'Prior executed rows or unrun stage blockers lost or promoted'}
    foreach($row in $prior){
        $subject=if($row.route -ceq 'tsql'){$tsql}elseif($row.route -ceq 'csharp'){$csharp}else{$tsPg}
        $state=if($row.route -ceq 'postgresql-sql'){'NOT_REEXECUTED_PRIOR_STAGE_BLOCKER_RETAINED'}else{'NOT_REEXECUTED_PRIOR_OBSERVED_RESULTS_RETAINED'}
        if($row.route -cnotin @('tsql','csharp','typescript','tsx','postgresql-sql') -or $row.prior_evidence_subject -cne $subject -or $row.state -cne $state){throw 'Prior route/subject/stage state mapping changed'}
    }
} $originalInputs $priorTsql $priorCsharp $priorTsPg
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
& {
    param($caseRoot)
    $root=Join-Path $caseRoot 'js-closure';$sourceRoot=Join-Path $root 'source'
    [void][IO.Directory]::CreateDirectory($sourceRoot)
    $entry=Join-Path $sourceRoot 'grammar.js';$dependency=Join-Path $sourceRoot 'dependency.js'
    [IO.File]::WriteAllText($dependency,'module.exports = {};',[Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText($entry,"const grammar = require('./dependency'); module.exports = grammar;",[Text.UTF8Encoding]::new($false))
    $closure=CheckJsInputs $entry $sourceRoot
    if($closure.Count -ne 2){throw 'Literal JS closure incomplete'}
    foreach($invalid in @("import('unregistered');","import 'unregistered';","const grammar = require(name);","const load = require;","eval('unregistered');","// foo: import('x').y.z;","const token = 'import';")){
        [IO.File]::WriteAllText($entry,$invalid,[Text.UTF8Encoding]::new($false))
        $rejected=$false;try{$null=CheckJsInputs $entry $sourceRoot}catch{$rejected=$true}
        if(-not $rejected){throw 'Unregistered loader or unreviewed import occurrence accepted'}
    }
    [IO.File]::WriteAllText($entry,"require('../outside');",[Text.UTF8Encoding]::new($false))
    $outside=Join-Path $root 'outside.js';[IO.File]::WriteAllText($outside,'module.exports = {};',[Text.UTF8Encoding]::new($false))
    $rejected=$false;try{$null=CheckJsInputs $entry $sourceRoot}catch{$rejected=$true}
    if(-not $rejected){throw 'JS source-root escape accepted'}
} $root
& {
    param($caseRoot)
    $root=Join-Path $caseRoot 'remedy-closure';$sourceRoot=Join-Path $root 'source';$parserRoot=Join-Path $root 'generated'
    [void][IO.Directory]::CreateDirectory($sourceRoot);[void][IO.Directory]::CreateDirectory($parserRoot)
    $script:verifiedSource=@{}
    $entry=Join-Path $sourceRoot 'grammar.js';$dependency=Join-Path $sourceRoot 'dependency.js'
    [IO.File]::WriteAllText($dependency,'export default {};',[Text.UTF8Encoding]::new($false))
    function PinFixture([string]$path){$identity=FileIdentity $path;$script:verifiedSource[$identity.path]=$identity.sha256}
    PinFixture $dependency
    [IO.File]::WriteAllText($entry,"import rules from './dependency.js';`nexport default rules;",[Text.UTF8Encoding]::new($false));PinFixture $entry
    if((CheckSqlJsInputs $entry $sourceRoot).Count -ne 2){throw 'Owned literal ESM closure incomplete'}
    foreach($source in @("import rules from 'unregistered';","import('./dependency.js');","const load = require;","import rules from '../outside.js';")){
        [IO.File]::WriteAllText($entry,$source,[Text.UTF8Encoding]::new($false));PinFixture $entry
        $rejected=$false;try{$null=CheckSqlJsInputs $entry $sourceRoot}catch{$rejected=$true}
        if(-not $rejected){throw 'Unresolved, dynamic, package or escaped SQL import accepted'}
    }
    $parser=Join-Path $parserRoot 'parser.c';$header=Join-Path $parserRoot 'header.h'
    [IO.File]::WriteAllText($parser,'#include "header.h"',[Text.UTF8Encoding]::new($false));[IO.File]::WriteAllText($header,'/* owned header */',[Text.UTF8Encoding]::new($false))
    if((CheckRemedyQuotedIncludes @($parser) $parserRoot).Count -ne 2){throw 'Owned quoted includes incomplete'}
    foreach($name in @('missing.h','../outside.h')){
        [IO.File]::WriteAllText($parser,('#include "'+$name+'"'),[Text.UTF8Encoding]::new($false))
        if($name.StartsWith('..')){[IO.File]::WriteAllText((Join-Path $root 'outside.h'),'/* unpinned */',[Text.UTF8Encoding]::new($false))}
        $rejected=$false;try{$null=CheckRemedyQuotedIncludes @($parser) $parserRoot}catch{$rejected=$true}
        if(-not $rejected){throw 'Missing or unpinned quoted include accepted'}
    }
    [void][IO.Directory]::CreateDirectory((Join-Path $root 'records'))
    $script:remedyRows=NewRemedyRows 'patch-r1' $originalInputs.cases $remedySubjects.cases.cases
    $script:failed=$false
    $recordDefinition=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Record'},$true))
    . ([scriptblock]::Create($recordDefinition[0].Extent.Text))
    [IO.File]::WriteAllText($parser,'#include "missing.h"',[Text.UTF8Encoding]::new($false))
    $blocked=TryRemedyPrecheck 'csharp-original' 'build' {CheckRemedyQuotedIncludes @($parser) $parserRoot}
    $own=@($script:remedyRows|Where-Object producer -CEQ 'csharp-original');$others=@($script:remedyRows|Where-Object producer -CNE 'csharp-original')
    if($blocked.passed -or -not $script:failed -or @($own|Where-Object dependency -CNE 'PRECHECK_FAILED_BUILD').Count -or @($others|Where-Object dependency -CNE 'GENERATION_BUILD_AND_SAFETY_PREREQUISITES').Count){throw 'Producer closure failure stopped or relabelled unrelated work'}
    [IO.File]::WriteAllText($parser,'#include "header.h"',[Text.UTF8Encoding]::new($false))
    $passed=TryRemedyPrecheck 'csharp-candidate' 'build' {CheckRemedyQuotedIncludes @($parser) $parserRoot}
    if(-not $passed.passed -or $passed.value.Count -ne 2){throw 'Independent producer closure did not continue'}
    foreach($failure in @('Quoted include bytes changed','Project header bytes changed','Container child cleanup unverified','Native operation budget exceeded','Input is not a regular file')){
        $rejected=$false;try{$null=TryRemedyPrecheck 'swift-candidate' 'build' {throw $failure}}catch{$rejected=$true}
        if(-not $rejected){throw 'Shared integrity, cleanup or budget failure was swallowed'}
    }
    $script:remedyRows=$null;$script:failed=$false
} $root
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
$supervisorBurst=& {
    param($sourceAst,$caseRoot)
    $root=Join-Path $caseRoot 'supervisor-burst'
    foreach($directory in @('records','raw','home')){[void][IO.Directory]::CreateDirectory((Join-Path $root $directory))}
    foreach($helper in @('Run','Record','Require','NetworkBudgetReceipt','CompleteAcquisition')){
        $definition=@($sourceAst.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $helper},$true))
        if($definition.Count -ne 1){throw 'Supervisor helper identity mismatch'}
        . ([scriptblock]::Create($definition[0].Extent.Text))
    }
    $script:commands=[Collections.Generic.List[object]]::new();$script:wall=[Diagnostics.Stopwatch]::StartNew()
    $script:imageSize=0L;$script:acquiring=$false;$runnerRoot=$root;$script:acquisitionLimit=1073741824L
    $source=Join-Path $root 'burst.go';$binary=Join-Path $root $(if($IsWindows){'burst.exe'}else{'burst'})
    [IO.File]::WriteAllText($source,('package main; import ("os"; "time"); func main() { if len(os.Args) > 1 { time.Sleep(3 * time.Second); return }; b := make([]byte, 65536); for i := 0; i < 128; i++ { n, e := os.Stdout.Write(b); if e != nil || n != len(b) { os.Exit(1) } } }'+"`n"),[Text.UTF8Encoding]::new($false))
    $build=[Diagnostics.ProcessStartInfo]::new();$build.FileName=Application go;$build.UseShellExecute=$false;$build.CreateNoWindow=$true;$build.WorkingDirectory=$root
    foreach($argument in @('build','-o',$binary,$source)){$build.ArgumentList.Add($argument)}
    $build.Environment.Clear()
    foreach($key in @('PATH','SystemRoot','WINDIR','COMSPEC','PATHEXT')){if([Environment]::GetEnvironmentVariable($key)){$build.Environment[$key]=[Environment]::GetEnvironmentVariable($key)}}
    foreach($key in @('HOME','USERPROFILE','TMPDIR','TEMP','TMP','GOCACHE','GOTMPDIR')){$build.Environment[$key]=$root}
    foreach($setting in @{CGO_ENABLED='0';GOENV='off';GOWORK='off';GOTOOLCHAIN='local';GOPROXY='off';GOSUMDB='off'}.GetEnumerator()){$build.Environment[$setting.Key]=$setting.Value}
    $compiler=[Diagnostics.Process]::new();$compiler.StartInfo=$build
    try{if(-not $compiler.Start()){throw 'Owned Go fixture build start failed'};if(-not $compiler.WaitForExit(30000)){$compiler.Kill($true);if(-not $compiler.WaitForExit(2000)){throw 'Owned fixture compiler cleanup unknown'};throw 'Owned fixture build timeout'};if($compiler.ExitCode -ne 0){throw 'Owned fixture build failed'}}finally{$compiler.Dispose()}
    $result=Run 'burst' @() 2 8388608 -executable $binary
    Require $result
    $expected=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([byte[]]::new(8388608))).ToLowerInvariant()
    if($result.observed_bytes -ne 8388608 -or $result.stored_stdout_bytes -ne 8388608 -or $result.stored_stderr_bytes -ne 0 -or $result.stdout_sha256 -cne $expected -or -not $result.host_cleanup_verified){throw 'Bounded supervisor burst lost bytes or cleanup'}
    $limited=Run 'burst-output-limit' @() 2 131072 -executable $binary
    if($limited.termination -cne 'OUTPUT_LIMIT' -or $limited.stored_stdout_bytes+$limited.stored_stderr_bytes -gt 131072 -or -not $limited.host_cleanup_verified){throw 'Output limit/cleanup negative failed'}
    $timeout=Run 'burst-timeout' @('wait') 1 1048576 -executable $binary
    if($timeout.termination -cne 'TIMEOUT' -or -not $timeout.host_cleanup_verified){throw 'Timeout/cleanup negative failed'}
    $script:acquiring=$true;$script:networkStart=100L
    function NetworkReceived {return 1073741941L}
    $networkLimited=Run 'burst-download-limit' @('wait') 1 1048576 -executable $binary
    if($networkLimited.termination -cne 'DOWNLOAD_LIMIT' -or -not $networkLimited.host_cleanup_verified -or $networkLimited.acquisition_network.state -cne 'OBSERVED' -or $networkLimited.acquisition_network.received_counter_bytes -ne 1073741841L){throw 'Download-limit counter/cleanup receipt lost'}
    $script:acquisitionLimit=1610612736L
    $sqlAllowed=Run 'burst-sqlpg-counter' @() 2 8388608 -executable $binary
    Require $sqlAllowed
    if($sqlAllowed.acquisition_network.limit_bytes -ne 1610612736L -or $sqlAllowed.acquisition_network.received_counter_bytes -ne 1073741841L -or -not $sqlAllowed.host_cleanup_verified){throw 'SQLPG supervisor used another stage cap'}
    $script:acquisitionLimit=1073741824L
    function NetworkReceived {throw 'Owned counter-unavailable negative'}
    if((NetworkBudgetReceipt).state -cne 'NOT_VERIFIED'){throw 'Unavailable counter fabricated an observation'}
    function NetworkReceived {return 99L}
    if((NetworkBudgetReceipt).state -cne 'NOT_VERIFIED'){throw 'Regressed counter fabricated an observation'}
    $rejected=$false;try{CompleteAcquisition}catch{$rejected=$true}
    if(-not $rejected -or -not $script:acquiring -or (Test-Path -LiteralPath (Join-Path $root 'records/download-budget.json'))){throw 'Regressed final counter released acquisition gate'}
    function NetworkReceived {return 1073741941L}
    $rejected=$false;try{CompleteAcquisition}catch{$rejected=$true}
    if(-not $rejected -or -not $script:acquiring){throw 'Exceeded final counter released acquisition gate'}
    [void][IO.Directory]::CreateDirectory((Join-Path $root 'acquisition/records'))
    [IO.File]::WriteAllText((Join-Path $root 'acquisition/records/acquisition.json'),'{"download_bytes":0}',[Text.UTF8Encoding]::new($false))
    $toolchain=@{compressed_bytes=0}
    function NetworkReceived {return 117L}
    CompleteAcquisition
    $final=Get-Content -LiteralPath (Join-Path $root 'records/download-budget.json') -Raw|ConvertFrom-Json
    if($script:acquiring -or $final.received_network_upper_bound_bytes -ne 17 -or $final.final_snapshot.state -cne 'OBSERVED'){throw 'Verified final counter transition/receipt failed'}
    $savedRoot=$root
    foreach($cap in @(1073741824L,1610612736L)){
        $script:acquisitionLimit=$cap
        foreach($delta in @(-1L,0L,1L)){
            $root=Join-Path $savedRoot ('cap-'+$cap+'-'+($delta+1))
            foreach($dir in @('records','acquisition/records')){[void][IO.Directory]::CreateDirectory((Join-Path $root $dir))}
            [IO.File]::WriteAllText((Join-Path $root 'acquisition/records/acquisition.json'),'{"download_bytes":0}',[Text.UTF8Encoding]::new($false))
            $script:acquiring=$true;$script:networkStart=100L
            function NetworkReceived {return 100L+$script:acquisitionLimit+$delta}
            $rejected=$false;try{CompleteAcquisition}catch{$rejected=$true}
            if($rejected -ne ($delta -gt 0) -or $script:acquiring -ne ($delta -gt 0)){throw 'Stage cap boundary/transition failed'}
            if($delta -le 0){$r=Get-Content -Raw (Join-Path $root 'records/download-budget.json')|ConvertFrom-Json;if($r.limit_bytes -ne $cap -or $r.final_snapshot.limit_bytes -ne $cap){throw 'Stage cap receipt mismatch'}}
        }
    }
    $root=$savedRoot;$script:acquisitionLimit=1073741824L;$script:acquiring=$false
    return @{result='PASS';bytes=$result.stored_stdout_bytes;seconds_limit=2;wall_seconds=$result.wall_seconds;sha256=$expected;owned_processes=5;fixture='OWNED_GO_STDOUT_ONLY';owned_build_processes=1;compiler_sha256=(Get-FileHash -LiteralPath $build.FileName).Hash.ToLowerInvariant();source_sha256=(Get-FileHash -LiteralPath $source).Hash.ToLowerInvariant();binary_sha256=(Get-FileHash -LiteralPath $binary).Hash.ToLowerInvariant();build_seconds_limit=30;output_limit_negative='PASS';timeout_negative='PASS';download_limit_receipt_negative='PASS_CONTROLLED_COUNTER_NOT_HOST_NETWORK_PROOF';counter_unavailable_or_regressed='PASS';both_stage_cap_boundaries='PASS';sqlpg_polling_cap='PASS_CONTROLLED_COUNTER_NOT_HOST_NETWORK_PROOF';upstream_native=$false}
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
@{result='PASS';checks=@('exact remedy subject/stage/image mismatches rejected','patch stage85/edit16 and SQL-PG46/edit3; no identical original baseline scheduled','literal original/count/output digest mismatches rejected; 34 inputs exact bytes','ESM dynamic/package/root escape and missing/unpinned quoted includes rejected','duplicate PATH applications choose first exact path','missing application rejected','separate acquisition, capture and image subjects required','empty, old B, mismatched and unknown profiles rejected','original image identity preserved; new image bound to separate subject','case-distinct grammar keys accepted and exact name retained; invalid name rejected','ELF arch/loader/library/RUNPATH mismatches rejected','owned structure/edit checker rejects wrong fields, unequal trees and missing negative error','frozen init exact identity; live/stopped/child/PID mismatch/forged command rejected','NUL/CRLF tar recovery preserves bytes; deleted/replaced bytes rejected','executable archive mode mismatch rejected','real Freeze/StopContainer receipt names stay unique with owned responses','actual supervisor recovers exact8MiB owned burst within2seconds and verifies cleanup','failed acquisition retains observed counter; unavailable/regressed counter remains NOT_VERIFIED');unix_mode_roundtrip=$(if($IsWindows){'NOT_APPLICABLE'}else{'PASS'});supervisor_burst=$supervisorBurst;fixture_processes_executed=5;native_fixture_processes_executed=5;upstream_native_invocations=0;run_sha256=(Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'run.ps1')).Hash.ToLowerInvariant();approval_sha256=(Get-FileHash -LiteralPath $approval).Hash.ToLowerInvariant()}|ConvertTo-Json -Depth 5|Set-Content -LiteralPath (Join-Path $root 'self-test.json') -Encoding utf8NoBOM
Write-Output 'P05 self-check PASS; owned Go supervisor fixture build1/execute5, upstream native0'
