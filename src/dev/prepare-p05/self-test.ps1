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
$exactSubject='dba0d5409f845fdcd1c2a0f373bac5cf90edf3b21033d5d060f59ced06dcd9ef'
foreach($stage in @('patch-r1','sql-pg-r1','patch-r2','sql-pg-r2','sql-only-r2')){
    $binding=if($stage.EndsWith('r2')){$r2Subject}else{$remedySubject}
    $selected=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $binding
    if($selected.acquisition_limit_bytes -ne $(if($stage -ceq 'sql-pg-r2'){1610612736L}else{1073741824L})){throw 'Stage cap leaked into another route'}
    $wrong=if($stage.EndsWith('r2')){$remedySubject}else{$r2Subject}
    $rejected=$false;try{$null=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $wrong}catch{$rejected=$true}
    if(-not $rejected){throw 'Old/new remedy subject cross-binding accepted'}
}
foreach($stage in @('csharp-r3','pg-legacy-r1','mssql-evaluate-r1')){
    $selected=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $exactSubject
    if($selected.acquisition_limit_bytes -ne $(if($stage -ceq 'pg-legacy-r1'){1610612736L}else{1073741824L})){throw 'Exact stage cap cross-binding'}
    foreach($wrong in @('',$subject,$remedySubject,$r2Subject)){
        $rejected=$false;try{$null=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $wrong}catch{$rejected=$true}
        if(-not $rejected){throw 'Earlier subject authorized new exact effects'}
    }
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
$followupSubject='a68d0717a75b6b769f3ea44eef4591c49bfedfc14f578abfc65657c43bffd1f2'
foreach($stage in @('csharp-r4','pg-legacy-g6-r1','mssql-patch-r1')){
    $selected=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $followupSubject
    $limits=ExactRemedyLimits $stage
    if($selected.acquisition_limit_bytes -ne $limits.source_image_counter_bytes){throw 'Followup stage counter mismatch'}
    foreach($wrong in @('',$subject,$remedySubject,$r2Subject,$exactSubject)){
        $rejected=$false;try{$null=& $approval -Profile pinned-tsql-r1 -Subject $subject -ImageProfile trixie-r1 -ImageSubject $imageSubject -RemedyStage $stage -RemedySubject $wrong}catch{$rejected=$true}
        if(-not $rejected){throw 'Prior subject authorized followup effect'}
    }
    $rows=NewRemedyRows $stage $originalInputs.cases $remedySubjects.cases.cases
    if($rows.Count -ne $limits.X -or @($rows|Where-Object {$_.case.edit}).Count -ne $limits.producer_edit -or @($rows|Where-Object {$_.producer -clike '*baseline*'}).Count){throw 'Followup rows/edit or baseline reuse mismatch'}
    $rejected=$false;try{$null=AssertExactAcquisitionLimit $stage $(if($limits.source_image_counter_bytes -eq 1073741824){1610612736L}else{1073741824L})}catch{$rejected=$true};if(-not $rejected){throw 'Followup cross-stage counter accepted'}
}
foreach($case in $remedySubjects['followup-r2'].new_cases){
    AssertFollowupCase $case
    foreach($failure in @('empty','count','hash','window')){
        $bad=$case|ConvertTo-Json -Depth 30|ConvertFrom-Json -AsHashtable
        switch($failure){'empty'{$bad.input_utf8=''};'count'{$bad.input_bytes++};'hash'{$bad.input_sha256='0'*64};'window'{$bad.expected.error_window=@{start=-1;end=1}}}
        $rejected=$false;try{AssertFollowupCase $bad}catch{$rejected=$true};if(-not $rejected){throw 'Invalid followup payload accepted'}
    }
}
foreach($stage in @('csharp-r4','pg-legacy-g6-r1','mssql-patch-r1','pg-legacy-r1','unknown')){
    foreach($kind in @('generation','build','execution','preflight','diagnostic')){
        foreach($owned in @($false,$true)){
            $actual=RemedyMemoryBytes $stage $kind 'generate-postgresql-legacy-candidate-r1-g6' $owned
            $expected=if($stage -ceq 'pg-legacy-g6-r1' -and $kind -ceq 'generation' -and -not $owned){6442450944L}else{4294967296L}
            if($actual -ne $expected -or (RemedyMemoryBytes $stage $kind 'other-operation' $owned) -ne 4294967296){throw 'PG generation-only memory leaked'}
        }
    }
}
AssertGenerationCapacity 8589934592 @(@{max='max';current=0},@{max='17179869184';current=8589934592L})
foreach($vector in @(@{available=8589934591L;parents=@(@{max='max';current=0})},@{available=8589934592L;parents=@()},@{available=8589934592L;parents=@(@{max='UNKNOWN';current=0})},@{available=8589934592L;parents=@(@{max='8589934592';current=1})},@{available=8589934592L;parents=@(@{max='max';current=-1})})){
    $rejected=$false;try{AssertGenerationCapacity $vector.available $vector.parents}catch{$rejected=$true};if(-not $rejected){throw 'Unknown or insufficient generation host capacity accepted'}
}
AssertMemoryEnvelope 6442450944 6442450944 6442450944 '6442450944'
$hierarchyRoot=@{path='/sys/fs/cgroup';mount_root='/';max='NOT_APPLICABLE_HIERARCHY_ROOT';current=$null;memory_controller_available=$true;max_file_present=$false;current_file_present=$false;primary_source='https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#memory-interface-files'}
AssertGenerationCapacity 8589934592 @($hierarchyRoot)
foreach($key in @('path','mount_root','memory_controller_available','max_file_present','current_file_present','current','primary_source')){
    $bad=$hierarchyRoot.Clone();$bad[$key]=switch($key){'path'{'/sys/fs/cgroup/child'};'mount_root'{'/hidden'};'memory_controller_available'{$false};'current'{0};default{$true}}
    $rejected=$false;try{AssertGenerationCapacity 8589934592 @($bad)}catch{$rejected=$true};if(-not $rejected){throw 'Unverified hierarchy root accepted'}
}
$bad=$hierarchyRoot.Clone();$bad.Remove('primary_source')
$rejected=$false;try{AssertGenerationCapacity 8589934592 @($bad)}catch{$rejected=$true};if(-not $rejected){throw 'Missing hierarchy root primary source accepted'}
$bad=$hierarchyRoot.Clone();$bad.primary_source='https://www.kernel.org/invalid'
$rejected=$false;try{AssertGenerationCapacity 8589934592 @($bad)}catch{$rejected=$true};if(-not $rejected){throw 'Wrong hierarchy root primary source accepted'}
$before="low 0`nhigh 0`nmax 0`noom 0`noom_kill 0`noom_group_kill 0`n"
$pressure=$before.Replace('max 0','max 1');$oom=$pressure.Replace('oom 0','oom 1').Replace('oom_kill 0','oom_kill 1')
if((MemoryEventDelta $before $pressure).state -cne 'NO_OOM_OBSERVED' -or (MemoryEventDelta $before $oom).state -cne 'OOM_OBSERVED'){throw 'Pressure/OOM delta classification failed'}
foreach($pair in @(@($oom,$before),@($before,$before.Replace('oom_kill 0', 'oom_kill UNKNOWN')),@($before,($before+"oom 0`n")),@($before,$before.Replace("oom 0`n",'')))){
    $rejected=$false;try{$null=MemoryEventDelta $pair[0] $pair[1]}catch{$rejected=$true};if(-not $rejected){throw 'Unknown/regressed/duplicate/missing memory counter accepted'}
}
$groupOom=$before.Replace('oom_group_kill 0','oom_group_kill 1')
if((MemoryEventDelta $before $groupOom).state -cne 'OOM_OBSERVED'){throw 'Group-only OOM delta classification failed'}
foreach($pair in @(@($before.Replace("oom_group_kill 0`n",''),$before.Replace("oom_group_kill 0`n",'')),@($before,$before.Replace('oom_group_kill 0','oom_group_kill UNKNOWN')),@($groupOom,$before))){
    $rejected=$false;try{$null=MemoryEventDelta $pair[0] $pair[1]}catch{$rejected=$true};if(-not $rejected){throw 'Missing/unknown/regressed group OOM counter accepted'}
}
foreach($vector in @(@(6442450944L,4294967296L,6442450944L,'6442450944'),@(6442450944L,6442450944L,4294967296L,'6442450944'),@(6442450944L,6442450944L,6442450944L,'4294967296'),@(6442450944L,6442450944L,6442450944L,'max'),@(6442450944L,6442450944L,6442450944L,''))){
    $rejected=$false;try{AssertMemoryEnvelope $vector[0] $vector[1] $vector[2] $vector[3]}catch{$rejected=$true};if(-not $rejected){throw 'Inspect/cgroup memory mismatch accepted'}
}
foreach($stage in @('patch-r1','sql-pg-r1')){
    $rows=NewRemedyRows $stage $originalInputs.cases $remedySubjects.cases.cases
    if($rows.Count -ne $(if($stage -ceq 'patch-r1'){85}else{46}) -or @($rows|Where-Object {$_.case.edit}).Count -ne $(if($stage -ceq 'patch-r1'){16}else{3})){throw 'Remedy rows/edit budget mismatch'}
    if($stage -ceq 'patch-r1' -and @($rows|Where-Object {$_.producer.EndsWith('-original') -and $_.id -cnotlike 'P05-REMEDY-*'}).Count){throw 'Identical failing baseline scheduled again'}
}
$r2Rows=NewRemedyRows 'patch-r2' $originalInputs.cases $remedySubjects.cases.cases
if($r2Rows.Count -ne 39 -or @($r2Rows|Where-Object {$_.case.edit}).Count -ne 7 -or @($r2Rows|Where-Object {$_.route -ceq 'csharp'}).Count -ne 27 -or @($r2Rows|Where-Object {$_.route -ceq 'typescript'}).Count -ne 4 -or @($r2Rows|Where-Object {$_.route -ceq 'tsx'}).Count -ne 8 -or @($r2Rows|Where-Object {$_.producer -cnotlike '*-candidate-r2'}).Count){throw 'Exact r2 producer budget mismatch'}
$sqlRows=NewRemedyRows 'sql-pg-r2' $originalInputs.cases $remedySubjects.cases.cases
if($sqlRows.Count -ne 46 -or @($sqlRows|Where-Object {$_.case.edit}).Count -ne 3){throw 'SQLPG row/edit identity changed'}
$remainingSqlRows=NewRemedyRows 'sql-only-r2' $originalInputs.cases $remedySubjects.cases.cases
if($remainingSqlRows.Count -ne 30 -or @($remainingSqlRows|Where-Object {$_.case.edit}).Count -ne 1 -or -not (EqualRemedyData $remainingSqlRows @($sqlRows|Where-Object producer -CEQ 'derek-sql-candidate'))){throw 'Remaining SQL rows differ from the approved B subset'}
$altered=$originalInputs.cases|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable
@($altered|Where-Object route -CEQ 'csharp')[0].expected.facts='altered expectation'
$rejected=$false;try{$null=NewRemedyRows 'csharp-r4' $altered $remedySubjects.cases.cases}catch{$rejected=$true};if(-not $rejected){throw 'Changed followup expectation accepted'}
$rejected=$false;try{$null=NewRemedyRows 'patch-r2' $altered $remedySubjects.cases.cases}catch{$rejected=$true}
if(-not $rejected){throw 'Changed exact r2 expectation accepted'}
foreach($stage in @('csharp-r3','pg-legacy-r1','mssql-evaluate-r1')){
    $rows=NewRemedyRows $stage $originalInputs.cases $remedySubjects.cases.cases
    $limits=ExactRemedyLimits $stage
    $binding=AssertExactAcquisitionLimit $stage $limits.source_image_counter_bytes
    if(-not $binding.match -or $binding.actual_limit_bytes -ne $binding.proposal_limit_bytes){throw 'Exact counter binding missing'}
    $rejected=$false;try{$null=AssertExactAcquisitionLimit $stage $(if($limits.source_image_counter_bytes -eq 1073741824L){1610612736L}else{1073741824L})}catch{$rejected=$true}
    if(-not $rejected){throw 'Cross-stage counter accepted'}
    if($rows.Count -ne $limits.X -or @($rows|Where-Object {$_.case.edit}).Count -ne $limits.producer_edit){throw 'Exact three-effect rows/edit budget mismatch'}
    if($stage -ceq 'csharp-r3'){$rejected=$false;try{$null=NewRemedyRows $stage $altered $remedySubjects.cases.cases}catch{$rejected=$true};if(-not $rejected){throw 'Changed r3 expectation accepted'}}
    if($stage -ceq 'mssql-evaluate-r1'){
        foreach($row in $rows){$prior=@($remainingSqlRows|Where-Object id -CEQ $row.id)[0];$priorCase=$prior.case|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable;if(-not (EqualRemedyData $row.case $priorCase)){throw 'MSSQL trial changed frozen SQL requirement'}}
    }
}
foreach($case in $remedySubjects['exact-r1'].new_pg_cases){
    $bytes=[Text.Encoding]::UTF8.GetBytes($case.input_utf8)
    AssertRemedyObject $bytes.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()) @{bytes=$case.input_bytes;sha256=$case.input_sha256}
}
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
& {
    param($caseRoot)
    $root=Join-Path $caseRoot 'exact-input-controls';$sourceRoot=Join-Path $root 'source'
    [void][IO.Directory]::CreateDirectory($sourceRoot)
    $source=Join-Path $sourceRoot 'grammar.json';[IO.File]::WriteAllText($source,'{"name":"owned","rules":{"source":{"type":"STRING","value":"x"}}}',[Text.UTF8Encoding]::new($false))
    $id=FileIdentity $source;$script:verifiedSource=@{};$script:verifiedSource[$id.path]=$id.sha256
    if((CheckRemedyJson $source).Count -ne 1){throw 'Owned JSON generation closure missing'}
    [IO.File]::AppendAllText($source,' ')
    $rejected=$false;try{$null=CheckRemedyJson $source}catch{$rejected=$true};if(-not $rejected){throw 'Changed JSON bytes accepted'}
    $sourceRoot=Join-Path $root 'patch-source';[void][IO.Directory]::CreateDirectory($sourceRoot)
    $source=Join-Path $sourceRoot 'fixture.txt';[IO.File]::WriteAllText($source,'before',[Text.UTF8Encoding]::new($false))
    $id=FileIdentity $source;$script:verifiedSource[$id.path]=$id.sha256
    function Record([string]$name,$value){if($value.adopted){throw 'Owned patch trial adopted provider'}}
    $change=$pin.Clone();$change.base_file=@{bytes=6;sha256=$id.sha256}
    CopyExactRemedyCandidate $sourceRoot @{id='OWNED-EXACT';file='fixture.txt';change=$change}
    if([IO.File]::ReadAllText((Join-Path $root 'candidates/OWNED-EXACT/fixture.txt')) -cne 'after' -or [IO.File]::ReadAllText($source) -cne 'before'){throw 'Owned exact patch copy changed original or result'}
    $second=Join-Path $sourceRoot 'second.txt';[IO.File]::WriteAllText($second,'before',[Text.UTF8Encoding]::new($false));$secondId=FileIdentity $second;$script:verifiedSource[$secondId.path]=$secondId.sha256
    $firstChange=$change.Clone();$firstChange.target='fixture.txt';$secondChange=$change.Clone();$secondChange.target='second.txt'
    CopyExactRemedyCandidate $sourceRoot @{id='OWNED-MULTIFILE';files=@($firstChange,$secondChange)}
    foreach($name in @('fixture.txt','second.txt')){if([IO.File]::ReadAllText((Join-Path $root ('candidates/OWNED-MULTIFILE/'+$name))) -cne 'after' -or [IO.File]::ReadAllText((Join-Path $sourceRoot $name)) -cne 'before'){throw 'Owned multi-file patch failed or changed source'}}
    $missing=$change.Clone();$missing.target='missing.txt';$rejected=$false;try{CopyExactRemedyCandidate $sourceRoot @{id='OWNED-MISSING';files=@($firstChange,$missing)}}catch{$rejected=$true};if(-not $rejected){throw 'Missing multi-file patch target accepted'}
    $script:verifiedSource=@{}
} $root
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
$applicationName=if($IsWindows){'p05-tool-check.cmd'}else{'p05-tool-check'}
$directories=@((Join-Path $root 'first'),(Join-Path $root 'second'))
foreach($directory in $directories){
    [void][IO.Directory]::CreateDirectory($directory)
    $path=Join-Path $directory $applicationName
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
    [IO.File]::WriteAllText($dependency,'export function make_keyword(word) { return new RegExp(word); }; export default { keyword_function: () => make_keyword("function") };',[Text.UTF8Encoding]::new($false))
    function PinFixture([string]$path){$identity=FileIdentity $path;$script:verifiedSource[$identity.path]=$identity.sha256}
    PinFixture $dependency
    [IO.File]::WriteAllText($entry,"import rules from './dependency.js';`nexport default rules;",[Text.UTF8Encoding]::new($false));PinFixture $entry
    if((CheckSqlJsInputs $entry $sourceRoot).Count -ne 2){throw 'Owned literal ESM closure incomplete'}
    foreach($source in @("import rules from 'unregistered';","import('./dependency.js');","const load = require;","require('./dependency.js');","createRequire('unregistered');","eval('unregistered');","Function('return 1');","new Function('return 1');","const evaluate = Function;","import rules from '../outside.js';")){
        [IO.File]::WriteAllText($entry,$source,[Text.UTF8Encoding]::new($false));PinFixture $entry
        $rejected=$false;try{$null=CheckSqlJsInputs $entry $sourceRoot}catch{$rejected=$true}
        if(-not $rejected){throw 'Unresolved, dynamic, package or escaped SQL import accepted'}
    }
    [IO.File]::WriteAllText($entry,"import rules from './dependency.js';`nexport default rules;",[Text.UTF8Encoding]::new($false));PinFixture $entry
    [IO.File]::AppendAllText($dependency,' // changed after verification',[Text.UTF8Encoding]::new($false))
    $rejected=$false;try{$null=CheckSqlJsInputs $entry $sourceRoot}catch{if($_.Exception.Message -cne 'SQL dependency bytes changed'){throw};$rejected=$true}
    if(-not $rejected){throw 'Changed SQL dependency bytes accepted'}
    PinFixture $dependency;$script:verifiedSource.Remove((FileIdentity $dependency).path)
    $rejected=$false;try{$null=CheckSqlJsInputs $entry $sourceRoot}catch{if($_.Exception.Message -cne 'Unregistered SQL dependency'){throw};$rejected=$true}
    if(-not $rejected){throw 'Unregistered SQL dependency accepted'}
    $commentRoot=Join-Path $root 'reviewed-comment';$comment=Join-Path $commentRoot 'grammar/statements/create-function.js'
    [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($comment))
    $fixture=Join-Path $PSScriptRoot 'fixtures/mssql-create-function.js.txt'
    $originalComment=[IO.File]::ReadAllBytes($fixture)
    if($originalComment.Length -ne 2539 -or (Get-FileHash -LiteralPath $fixture).Hash.ToLowerInvariant() -cne 'f395d3e20195f86c3e3902f0ca05f32e0b6df242ecc744e54732ca71d8161b4a'){throw 'Reviewed prose fixture bytes changed'}
    [IO.File]::WriteAllBytes($comment,$originalComment);PinFixture $comment
    $helpers=Join-Path $commentRoot 'grammar/helpers.js'
    [IO.File]::WriteAllText($helpers,'export const owned = true;',[Text.UTF8Encoding]::new($false));PinFixture $helpers
    if((CheckSqlJsInputs $comment $commentRoot).Count -ne 2){throw 'Reviewed inert prose was rejected'}
    $originalText=[Text.Encoding]::UTF8.GetString($originalComment)
    foreach($changed in @($originalText.Replace('Functions require','Functions  require'),$originalText.Replace('allow it empty','allow it empty!'),($originalText+"`nconst load = require;"),($originalText+"`neval('unreviewed');"))){
        [IO.File]::WriteAllText($comment,$changed,[Text.UTF8Encoding]::new($false));PinFixture $comment
        $rejected=$false;try{$null=CheckSqlJsInputs $comment $commentRoot}catch{if($_.Exception.Message -cne 'Unreviewed SQL loader/evaluation'){throw};$rejected=$true}
        if(-not $rejected){throw 'Changed or additional loader occurrence accepted'}
    }
    $wrongPath=Join-Path $commentRoot 'grammar/statements/other.js'
    [IO.File]::WriteAllBytes($wrongPath,$originalComment);PinFixture $wrongPath
    $rejected=$false;try{$null=CheckSqlJsInputs $wrongPath $commentRoot}catch{if($_.Exception.Message -cne 'Unreviewed SQL loader/evaluation'){throw};$rejected=$true}
    if(-not $rejected){throw 'Reviewed prose exception escaped its exact path'}
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
$exportAst=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'collect.ps1'),[ref]$tokens,[ref]$errors)
if($errors){throw 'Collector parse failed'}
foreach($name in @('CheckCollectionTime','CollectionHash','ReadCollectedRecord','AssertFollowupEvidenceBinding','GeneratedArtifactProvenance','StoreCsharpSourceCopies','AssertLosslessCsharpSources','AssertLosslessCsharpManifest','ExpandedFailure')){
    $definition=@($exportAst.FindAll({param($n)$n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
    if($definition.Count -ne 1){throw 'Collector guard helper missing'};. ([scriptblock]::Create($definition[0].Extent.Text))
}
$script:collectionClock=[Diagnostics.Stopwatch]::StartNew()
& {
    param($caseRoot)
    $copyRoot=Join-Path $caseRoot 'owned-lossless-csharp'
    [void][IO.Directory]::CreateDirectory((Join-Path $copyRoot 'records'))
    $data=[byte[]]::new(131072);$data[0]=0;$data[1]=13;$data[2]=10;$data[-1]=255
    $digest=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($data)).ToLowerInvariant();$files=@()
    foreach($id in @('P05-CS-REMEDY-r1','P05-CSHARP-REMEDY-r2','P05-CSHARP-REMEDY-r3','P05-CSHARP-REMEDY-r4')){
        $relative='candidates/'+$id+'/src/parser.c';$path=Join-Path $copyRoot $relative
        [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($path));[IO.File]::WriteAllBytes($path,$data);$files+=Get-Item -LiteralPath $path
        $proof=@{proposal_id=$id;files=@(@{patched=$false;candidate=@{path=$relative;bytes=$data.Length;sha256=$digest}})}
        [IO.File]::WriteAllText((Join-Path $copyRoot ('records/candidate-'+$id.ToLowerInvariant()+'.json')),($proof|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
    }
    $originalFiles=$files;$rawPath=Join-Path $copyRoot 'raw/frozen.stdout';[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($rawPath));[IO.File]::WriteAllBytes($rawPath,[byte[]]@(0,13,10,255));$rawDigest=CollectionHash $rawPath;$files+=Get-Item -LiteralPath $rawPath
    $files=StoreCsharpSourceCopies $copyRoot $files;$match=AssertLosslessCsharpSources $copyRoot
    if($match.original_copy_count -ne 4 -or $match.unique_objects -ne 1 -or $match.original_identity_bytes -ne 4*$data.Length -or $match.unique_decoded_source_bytes -ne $data.Length -or $files.Count -ne 3 -or @($files|Where-Object FullName -CEQ $rawPath).Count -ne 1 -or (CollectionHash $rawPath) -cne $rawDigest){throw 'Lossless source deduplication/identity/raw retention mismatch'}
    foreach($file in $originalFiles){if((CollectionHash $file.FullName) -cne $digest){throw 'Original candidate source changed'}}
    $recordPath=Join-Path $copyRoot 'records/lossless-csharp-sources.json';$originalRecord=[IO.File]::ReadAllText($recordPath)
    foreach($failure in @('path','bytes','hash','gzip-path','gzip-bytes','gzip-hash','proof-path','retention','identity-mode','duplicate','object-count','deleted','null-copies')){
        $record=$originalRecord|ConvertFrom-Json -AsHashtable
        switch($failure){'path'{$record.copies[0].original_path='../outside'};'bytes'{$record.copies[0].original_bytes=$true};'hash'{$record.copies[0].original_sha256='0'*64};'gzip-path'{$record.copies[0].lossless_gzip_path='../outside'};'gzip-bytes'{$record.copies[0].gzip_bytes=$true};'gzip-hash'{$record.copies[0].gzip_sha256='0'*64};'proof-path'{$record.copies[0].copy_proof='../outside'};'retention'{$record.copies[0].original_retained_on_runner=$false};'identity-mode'{$record.copies[0].identity_is_uncompressed_bytes=$false};'duplicate'{$record.copies+=@($record.copies[0])};'object-count'{$record.unique_objects++};'deleted'{$record.original_files_deleted=$true};'null-copies'{$record.copies=$null}}
        [IO.File]::WriteAllText($recordPath,($record|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false));$rejected=$false;try{$null=AssertLosslessCsharpSources $copyRoot}catch{$rejected=$true};if(-not $rejected){throw 'Lossless source record drift accepted'}
    }
    for($missing=0;$missing -lt 4;$missing++){
        $record=$originalRecord|ConvertFrom-Json -AsHashtable;$record.copies=@(for($i=0;$i -lt 4;$i++){if($i -ne $missing){$record.copies[$i]}})
        [IO.File]::WriteAllText($recordPath,($record|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false));$rejected=$false;try{$null=AssertLosslessCsharpSources $copyRoot}catch{$rejected=$true};if(-not $rejected){throw 'Missing original identity accepted'}
        $subset=@(for($i=0;$i -lt 4;$i++){if($i -ne $missing){$originalFiles[$i]}})
        $rejected=$false;try{$null=StoreCsharpSourceCopies $copyRoot $subset}catch{$rejected=$_.Exception.Message -ceq 'Exact four candidate source paths required'};if(-not $rejected){throw 'Missing candidate enumeration accepted'}
    }
    $extraPath=Join-Path $copyRoot 'candidates/UNAPPROVED/src/parser.c';[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($extraPath));[IO.File]::WriteAllBytes($extraPath,$data)
    $rejected=$false;try{$null=StoreCsharpSourceCopies $copyRoot @($originalFiles+(Get-Item -LiteralPath $extraPath))}catch{$rejected=$_.Exception.Message -ceq 'Unexpected or duplicate candidate source path'};if(-not $rejected){throw 'Extra candidate source accepted'}
    [IO.File]::WriteAllText($recordPath,$originalRecord,[Text.UTF8Encoding]::new($false))
    $small=$originalRecord|ConvertFrom-Json -AsHashtable;$proofs=@{}
    foreach($copy in $small.copies){$copy.original_bytes--;$path=Join-Path $copyRoot $copy.copy_proof;$proofs[$path]=[IO.File]::ReadAllText($path);$proof=$proofs[$path]|ConvertFrom-Json -AsHashtable;$proof.files[0].candidate.bytes--;[IO.File]::WriteAllText($path,($proof|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))}
    [IO.File]::WriteAllText($recordPath,($small|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false));$rejected=$false;try{$null=AssertLosslessCsharpSources $copyRoot}catch{$rejected=$_.Exception.Message -ceq 'Lossless source expansion limit'};if(-not $rejected){throw 'Decoded source byte bound ignored'}
    foreach($path in $proofs.Keys){[IO.File]::WriteAllText($path,$proofs[$path],[Text.UTF8Encoding]::new($false))};[IO.File]::WriteAllText($recordPath,$originalRecord,[Text.UTF8Encoding]::new($false))
    $objectPath=Join-Path $copyRoot ('source-objects/'+$digest+'.c.gz');$encoded=[IO.File]::ReadAllBytes($objectPath)
    [IO.File]::WriteAllBytes($objectPath,[byte[]]@(0,13,10));$rejected=$false;try{$null=AssertLosslessCsharpSources $copyRoot}catch{$rejected=$true};if(-not $rejected){throw 'Corrupt source object accepted'}
    [IO.File]::WriteAllBytes($objectPath,$encoded);$rejected=$false;try{$null=StoreCsharpSourceCopies $copyRoot $originalFiles}catch{$rejected=$true};if(-not $rejected -or (CollectionHash $objectPath) -cne [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($encoded)).ToLowerInvariant()){throw 'Source object collision overwrite'}
    foreach($tail in @([byte[]]@(0,255),$encoded)){
        [IO.File]::WriteAllBytes($objectPath,[byte[]]@($encoded+$tail));$rejected=$false;try{$null=AssertLosslessCsharpSources $copyRoot}catch{$rejected=$_.Exception.Message -ceq 'Lossless source compressed bytes mismatch'};if(-not $rejected){throw 'Changed trailing bytes/member identity accepted'}
    }
    [IO.File]::WriteAllBytes($objectPath,$encoded)
    $manifest=@(foreach($file in $files){@{path=[IO.Path]::GetRelativePath($copyRoot,$file.FullName).Replace('\','/');bytes=$file.Length;sha256=(CollectionHash $file.FullName)}})
    $budget=AssertLosslessCsharpManifest $manifest $match 201326592
    if($budget.unique_decoded_source_bytes -ne $data.Length -or $budget.logical_copy_identity_bytes -ne 4*$data.Length -or $budget.duplicate_materialization){throw 'Source expanded accounting mismatch'}
    foreach($field in @('bytes','sha256')){
        $drift=$manifest|ConvertTo-Json -Depth 5|ConvertFrom-Json -AsHashtable;$member=@($drift|Where-Object {$_.path.StartsWith('source-objects/',[StringComparison]::Ordinal)})[0]
        if($field -ceq 'bytes'){$member.bytes++}else{$member.sha256='0'*64};$rejected=$false;try{$null=AssertLosslessCsharpManifest $drift $match 201326592}catch{$rejected=$_.Exception.Message -ceq 'Lossless source final manifest identity mismatch'};if(-not $rejected){throw 'Final manifest source drift accepted'}
    }
    & {
        param($manifest,$sourceProof,$collectorAst)
        $summary=@{counts=@{generation=0;build=0;execution=0};outcomes=@();cleanup_errors=@()};$exactLimits=@{local_expanded_max_bytes=201326592}
        $guard=@($collectorAst.FindAll({param($n)$n -is [Management.Automation.Language.TryStatementAst] -and $n.Body.Extent.Text.Contains('$sourceExpansion=AssertLosslessCsharpManifest',[StringComparison]::Ordinal)},$true))
        if($guard.Count -ne 1){throw 'Actual source manifest catch guard unavailable'}
        $records=[Collections.Generic.List[object]]::new();$rejected=$false
        try{& ([scriptblock]::Create($guard[0].Extent.Text))|ForEach-Object {$records.Add($_)}}catch{$rejected=$_.Exception.Message -ceq 'Lossless source final manifest identity mismatch'}
        if(-not $rejected -or $records.Count -ne 1 -or ($records[0]|ConvertFrom-Json).failure -cne 'LOSSLESS_SOURCE_IDENTITY_FAILURE'){throw 'Source identity failure misclassified as budget overflow'}
    } $drift $match $exportAst
    & {
        param($fallbackRoot,$collectorAst,$originalFiles,$copyRoot)
        [void][IO.Directory]::CreateDirectory((Join-Path $fallbackRoot 'records'));$files=@()
        foreach($file in $originalFiles){$relative=[IO.Path]::GetRelativePath($copyRoot,$file.FullName);$target=Join-Path $fallbackRoot $relative;[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target));[IO.File]::WriteAllBytes($target,[IO.File]::ReadAllBytes($file.FullName));$files+=Get-Item -LiteralPath $target}
        foreach($file in Get-ChildItem -LiteralPath (Join-Path $copyRoot 'records') -Filter 'candidate-*.json' -File){[IO.File]::WriteAllBytes((Join-Path $fallbackRoot ('records/'+$file.Name)),[IO.File]::ReadAllBytes($file.FullName))}
        $store=@($collectorAst.FindAll({param($n)$n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'StoreCsharpSourceCopies'},$true))[0]
        . ([scriptblock]::Create($store.Extent.Text.Replace('function StoreCsharpSourceCopies','function OriginalLosslessStore')))
        function StoreCsharpSourceCopies([string]$task,$files){$stored=OriginalLosslessStore $task $files;$object=@($stored|Where-Object Extension -CEQ '.gz')[0];[IO.File]::WriteAllBytes($object.FullName,[byte[]]@(0,13,10));return ,$stored}
        $guard=@($collectorAst.FindAll({param($n)$n -is [Management.Automation.Language.TryStatementAst] -and $n.Body.Extent.Text.Contains('$files=StoreCsharpSourceCopies $task $files',[StringComparison]::Ordinal)},$true))
        if($guard.Count -ne 1){throw 'Actual lossless fallback guard unavailable'}
        $task=$fallbackRoot;$originalSourceFiles=$files;$bindingFailure=$false;. ([scriptblock]::Create($guard[0].Extent.Text))
        if(-not $bindingFailure -or $sourceProof.result -cne 'NOT_VERIFIED' -or -not $sourceProof.original_failed_files_preserved -or $files.Count -ne 4){throw 'Post-Store corruption failed to restore original evidence selection'}
        foreach($file in $files){if($file.Extension -cne '.c' -or $file.Length -ne 131072){throw 'Post-Store fallback omitted original source bytes'}}
    } (Join-Path $caseRoot 'owned-lossless-csharp-fallback') $exportAst $originalFiles $copyRoot
    $largeRoot=Join-Path $caseRoot 'owned-lossless-csharp-stage-overflow';[void][IO.Directory]::CreateDirectory((Join-Path $largeRoot 'records'));$largeFiles=@();$index=0
    foreach($id in @('P05-CS-REMEDY-r1','P05-CSHARP-REMEDY-r2','P05-CSHARP-REMEDY-r3','P05-CSHARP-REMEDY-r4')){
        $relative='candidates/'+$id+'/src/parser.c';$path=Join-Path $largeRoot $relative;[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($path))
        $sink=[IO.File]::Open($path,[IO.FileMode]::CreateNew)
        try{CheckCollectionTime;$sink.SetLength(67108864);$sink.WriteByte([byte]$index);$index++}finally{$sink.Dispose()}
        $sha=CollectionHash $path
        $largeFiles+=Get-Item -LiteralPath $path;$proof=@{proposal_id=$id;files=@(@{patched=$false;candidate=@{path=$relative;bytes=67108864;sha256=$sha}})}
        [IO.File]::WriteAllText((Join-Path $largeRoot ('records/candidate-'+$id.ToLowerInvariant()+'.json')),($proof|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
    }
    $stored=StoreCsharpSourceCopies $largeRoot $largeFiles;$largeProof=AssertLosslessCsharpSources $largeRoot
    $largeManifest=@(foreach($file in $stored){@{path=[IO.Path]::GetRelativePath($largeRoot,$file.FullName).Replace('\','/');bytes=$file.Length;sha256=(CollectionHash $file.FullName)}})
    if($largeProof.unique_objects -ne 4 -or $largeProof.unique_decoded_source_bytes -ne 268435456 -or ($largeManifest|Measure-Object bytes -Sum).Sum -ge 201326592){throw 'Stage overflow fixture does not exercise decoded total'}
    $rejected=$false;try{$null=AssertLosslessCsharpManifest $largeManifest $largeProof 201326592}catch{$rejected=$_.Exception.Message -ceq 'Lossless source stage expanded reserve exceeded'};if(-not $rejected){throw 'Unique decoded stage overflow accepted'}
    $overflow=ExpandedFailure $largeManifest $largeProof 201326592 @{counts=@{generation=0;build=0;execution=0};outcomes=@();cleanup_errors=@()}
    if($overflow.unique_decoded_source_bytes -ne 268435456 -or $overflow.charged_expanded_bytes -le $overflow.expanded_cap -or $overflow.largest_members.Count -gt 12 -or $overflow.support_assessment -cne 'NOT_VERIFIED' -or [Text.Encoding]::UTF8.GetByteCount(($overflow|ConvertTo-Json -Depth 8 -Compress)) -gt 8388608){throw 'Decoded stage failure record lost'}
    & {
        param($hashDeadlineFile)
        $clock=[pscustomobject]@{checks=0};$clock|Add-Member -MemberType ScriptProperty -Name Elapsed -Value {$this.checks=$this.checks+1;if($this.checks -gt 1){[TimeSpan]::FromSeconds(121)}else{[TimeSpan]::Zero}}
        $script:collectionClock=$clock;$rejected=$false
        try{$null=CollectionHash $hashDeadlineFile}catch{$rejected=$_.Exception.Message -ceq 'Total evidence collection time limit'}
        if(-not $rejected -or $clock.checks -lt 2){throw 'Streaming hash deadline ignored'}
        $released=[IO.File]::Open($hashDeadlineFile,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::None);$released.Dispose()
        $script:collectionClock=[Diagnostics.Stopwatch]::StartNew()
    } $largeFiles[0].FullName
    $script:collectionClock=[pscustomobject]@{Elapsed=[TimeSpan]::FromSeconds(121)};$rejected=$false;try{$null=AssertLosslessCsharpSources $copyRoot}catch{$rejected=$true};if(-not $rejected){throw 'Lossless source total deadline ignored'}
    $script:collectionClock=[Diagnostics.Stopwatch]::StartNew()
    @{result='PASS';original_copies=4;unique_objects=1;original_bytes=4*$data.Length;compressed_bytes=$encoded.Length;NUL_CRLF_roundtrip='PASS';negative_controls=34;actual_stage_overflow_fixture_bytes=268435456;original_files_retained=$true;raw_unchanged=$true;post_store_corruption_fallback='PASS';upstream_native=$false}|ConvertTo-Json -Compress
} $root
$bindingRoot=Join-Path $root 'owned-followup-binding'
foreach($directory in @('records','raw','acquisition/records')){[void][IO.Directory]::CreateDirectory((Join-Path $bindingRoot $directory))}
function OwnedBindingRecord([string]$relative,$data){[IO.File]::WriteAllText((Join-Path $bindingRoot $relative),($data|ConvertTo-Json -Depth 40),[Text.UTF8Encoding]::new($false))}
foreach($stage in @('csharp-r4','pg-legacy-g6-r1','mssql-patch-r1')){
    $limits=ExactRemedyLimits $stage;$rows=NewRemedyRows $stage $originalInputs.cases $remedySubjects.cases.cases
    $ledger=@{remedy_stage=$stage;remedy_approval_subject=$followupSubject;source_inputs_sha256='f998fb4e73b022cfc7b50196d471a72a0b7bbb4aabce2996be5f1bf596a5a1e4';planned_stage_rows=$limits.X;planned_producer_edits=$limits.producer_edit;rows=$rows;owned_fixture_only=$true}
    $proof=@{remedy_stage=$stage;remedy_approval_subject=$followupSubject;acquisition_limit_bytes=$limits.source_image_counter_bytes;counts=@{generation=0;build=0;execution=0;preflight=0;diagnostic=0};owned_counts=@{generation=0;build=0;execution=0};capture_count=0;command_count=0;cleanup_errors=@();outcomes=@();verdict='FAILED';failure_type='OWNED_NOT_RUN_BOUNDARY_CONTROL';owned_fixture_only=$true}
    $acq=@{remedy_stage=$stage;remedy_approval_subject=$followupSubject;acquisition_limit_bytes=$limits.source_image_counter_bytes;download_bytes=0;http_requests=0;state='COMPLETED';owned_fixture_only=$true}
    $budget=@{limit_bytes=$limits.source_image_counter_bytes;source_http_bytes=0;received_network_upper_bound_bytes=0;final_snapshot=@{limit_bytes=$limits.source_image_counter_bytes;state='OBSERVED';start_counter_bytes=1;end_counter_bytes=1;received_counter_bytes=0};owned_fixture_only=$true}
    OwnedBindingRecord 'records/case-ledger.json' $ledger;OwnedBindingRecord 'acquisition/records/acquisition.json' $acq;OwnedBindingRecord 'records/download-budget.json' $budget
    $match=AssertFollowupEvidenceBinding $bindingRoot $stage $followupSubject $proof (Join-Path $PSScriptRoot 'inputs.json')
    if($match.result -cne 'MATCH' -or $match.observed_captured_native.execution -ne 0 -or $match.planned_producer_edits -ne $limits.producer_edit){throw 'Failed/NOT_RUN boundary control promoted execution'}
    foreach($failure in @('subject','counter','rows','expectation','edit','attempts','commands','cleanup','acquisition','download','snapshot','unknown','over-cap','regressed','source-bytes','completion','executed-row')){
        $pp=$proof|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable;$ll=$ledger|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable;$aa=$acq.Clone();$bb=$budget|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable
        switch($failure){'subject'{$pp.remedy_approval_subject='0'*64};'counter'{$pp.acquisition_limit_bytes++};'rows'{$ll.rows=@($ll.rows[1..($ll.rows.Count-1)])};'expectation'{$ll.rows[0].case.expected.facts='changed'};'edit'{$ll.planned_producer_edits++};'attempts'{$pp.counts.generation=$limits.G+1};'commands'{$pp.command_count=1};'cleanup'{$pp.cleanup_errors=@(@{owned='unverified'})};'acquisition'{$aa.acquisition_limit_bytes++};'download'{$bb.limit_bytes++};'snapshot'{$bb.final_snapshot.limit_bytes++};'unknown'{$bb.final_snapshot.state='NOT_VERIFIED'};'over-cap'{$bb.final_snapshot.end_counter_bytes=$limits.source_image_counter_bytes+2;$bb.final_snapshot.received_counter_bytes=$limits.source_image_counter_bytes+1;$bb.received_network_upper_bound_bytes=$limits.source_image_counter_bytes+1};'regressed'{$bb.final_snapshot.end_counter_bytes=0;$bb.final_snapshot.received_counter_bytes=-1};'source-bytes'{$aa.download_bytes=1};'completion'{$pp.verdict='BOUNDED_INPUTS_COMPLETED_REVIEW_REQUIRED'};'executed-row'{$ll.rows[0].state='EXITED'}}
        OwnedBindingRecord 'records/case-ledger.json' $ll;OwnedBindingRecord 'acquisition/records/acquisition.json' $aa;OwnedBindingRecord 'records/download-budget.json' $bb
        $rejected=$false;try{$null=AssertFollowupEvidenceBinding $bindingRoot $stage $followupSubject $pp (Join-Path $PSScriptRoot 'inputs.json')}catch{$rejected=$true};if(-not $rejected){throw ('Collector binding negative accepted: '+$failure)}
    }
}
$stage='pg-legacy-g6-r1';$limits=ExactRemedyLimits $stage;$rows=NewRemedyRows $stage $originalInputs.cases $remedySubjects.cases.cases
$ledger.remedy_stage=$stage;$ledger.planned_stage_rows=$limits.X;$ledger.planned_producer_edits=$limits.producer_edit;$ledger.rows=$rows
$proof.remedy_stage=$stage;$proof.acquisition_limit_bytes=$limits.source_image_counter_bytes;$proof.counts.generation=1;$proof.counts.diagnostic=3;$proof.command_count=5;$proof.verdict='REPRODUCED_FAILURES'
$acq.remedy_stage=$stage;$acq.acquisition_limit_bytes=$limits.source_image_counter_bytes;$budget.limit_bytes=$limits.source_image_counter_bytes;$budget.final_snapshot.limit_bytes=$limits.source_image_counter_bytes
OwnedBindingRecord 'records/case-ledger.json' $ledger;OwnedBindingRecord 'acquisition/records/acquisition.json' $acq;OwnedBindingRecord 'records/download-budget.json' $budget
$label='generate-'+$rows[0].producer;$container='1'*64
function OwnedCommand([string]$label,[string[]]$argv,[int]$exitCode,[string]$stdout){
    $out=[Text.Encoding]::UTF8.GetBytes($stdout);$err=[byte[]]::new(0)
    [IO.File]::WriteAllBytes((Join-Path $bindingRoot ('raw/'+$label+'.stdout')),$out);[IO.File]::WriteAllBytes((Join-Path $bindingRoot ('raw/'+$label+'.stderr')),$err)
    OwnedBindingRecord ('records/command-'+$label+'.json') @{label=$label;argv=$argv;termination='EXITED';exit_code=$exitCode;host_cleanup_verified=$true;seconds_limit=$(if($label -ceq ('generate-'+$rows[0].producer)){300}else{10});output_limit=8388608;stored_stdout_bytes=$out.Length;stored_stderr_bytes=0;stdout_sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($out)).ToLowerInvariant();stderr_sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($err)).ToLowerInvariant();owned_fixture_only=$true}
}
$memoryBefore="low 0`nhigh 0`nmax 0`noom 0`noom_kill 0`noom_group_kill 0`n";$memoryAfter=$memoryBefore.Replace('max 0','max 1').Replace('oom 0','oom 1').Replace('oom_kill 0','oom_kill 1')
OwnedCommand $label @('exec',$container,'owned-not-executed') 137 ''
foreach($when in @('before','after')){OwnedCommand ($label+'-memory-'+$when) @('exec',$container,'/bin/cat','/sys/fs/cgroup/memory.events') 0 $(if($when -ceq 'before'){$memoryBefore}else{$memoryAfter})}
OwnedCommand ($label+'-inspect') @('inspect',$container) 0 (@(@{Id=$container;HostConfig=@{Memory=6442450944L;MemorySwap=6442450944L}})|ConvertTo-Json -AsArray -Depth 4)
OwnedCommand ($label+'-memory-max') @('exec',$container,'/bin/cat','/sys/fs/cgroup/memory.max') 0 '6442450944'
$events=@{container=$container;native_command=$label;memory_limit_bytes=6442450944L;state='OOM_OBSERVED';counters=(MemoryEventDelta $memoryBefore $memoryAfter);original_exit_code=137;original_termination='EXITED';owned_fixture_only=$true}
foreach($when in @('before','after')){$relative='raw/'+$label+'-memory-'+$when+'.stdout';$events[$when+'_raw']=@{path=$relative;bytes=(Get-Item -LiteralPath (Join-Path $bindingRoot $relative)).Length;sha256=(CollectionHash (Join-Path $bindingRoot $relative))}}
OwnedBindingRecord ('records/'+$label+'-memory-events.json') $events
OwnedBindingRecord 'records/pg-generation-capacity.json' @{result='PASS';before_G=$true;generation_memory_bytes=6442450944L;MemAvailable_bytes=8589934592L;resolved_parents=@($hierarchyRoot);owned_fixture_only=$true;real_capacity='NOT_VERIFIED'}
$limit=@{container=$container;bytes=6442450944L;inspect_and_cgroup='MATCH';owned_fixture_only=$true};OwnedBindingRecord ('records/'+$label+'-memory-limit.json') $limit
$proof.outcomes=@(@{kind='generation';scope='REGISTERED_P05_REMEDY';termination='EXITED';exit_code=137;label=$label;result_directory=$null;memory_limit_bytes=6442450944L;resource_state='OOM_OBSERVED';memory_event_record=('records/'+$label+'-memory-events.json')})
$match=AssertFollowupEvidenceBinding $bindingRoot $stage $followupSubject $proof (Join-Path $PSScriptRoot 'inputs.json')
if($match.observed_captured_native.generation -ne 1 -or $match.observed_captured_native.execution -ne 0){throw 'Owned OOM control promoted B/X'}
foreach($failure in @('container','delta','limit','build-after-oom')){
    $ee=$events|ConvertTo-Json -Depth 12|ConvertFrom-Json -AsHashtable;$pp=$proof|ConvertTo-Json -Depth 12|ConvertFrom-Json -AsHashtable;$mm=$limit.Clone()
    switch($failure){'container'{$ee.container='2'*64};'delta'{$ee.counters.delta.oom=0};'limit'{$mm.bytes=4294967296L};'build-after-oom'{$pp.counts.build=1}}
    OwnedBindingRecord ('records/'+$label+'-memory-events.json') $ee;OwnedBindingRecord ('records/'+$label+'-memory-limit.json') $mm
    $rejected=$false;try{$null=AssertFollowupEvidenceBinding $bindingRoot $stage $followupSubject $pp (Join-Path $PSScriptRoot 'inputs.json')}catch{$rejected=$true};if(-not $rejected){throw ('Owned OOM evidence negative accepted: '+$failure)}
}
$script:collectionClock=[pscustomobject]@{Elapsed=[TimeSpan]::FromSeconds(121)}
$rejected=$false;try{$null=CollectionHash (Join-Path $bindingRoot 'records/case-ledger.json')}catch{$rejected=$_.Exception.Message -ceq 'Total evidence collection time limit'};if(-not $rejected){throw 'Collector total deadline ignored'}
$script:collectionClock=[Diagnostics.Stopwatch]::StartNew()
$exportRoot=Join-Path $root 'owned-export-provenance'
if($env:RUNNER_TEMP -and -not $IsWindows){
    $run=if($env:GITHUB_RUN_ID){$env:GITHUB_RUN_ID}else{'0'}
    if($run -cnotmatch '^[0-9]+$'){throw 'Owned export run identity invalid'}
    $exportPrefix=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('/')+'/'
    $exportRoot=[IO.Path]::GetFullPath((Join-Path $exportPrefix ('tsgk-p05-remedy-'+$run+'-'+[DateTime]::UtcNow.Ticks)))
    if(-not $exportRoot.StartsWith($exportPrefix,[StringComparison]::Ordinal) -or (Test-Path -LiteralPath $exportRoot)){throw 'Fresh owned export root required'}
    $ancestor=Get-Item -LiteralPath $exportPrefix -Force
    while($ancestor){if($ancestor.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Owned export reparse ancestor'};$ancestor=$ancestor.Parent}
}
if(Test-Path -LiteralPath $exportRoot){throw 'Owned export fixture exists'}
$createdExport=$false;$emptyExportRoot=$null
try {
    [void][IO.Directory]::CreateDirectory($exportRoot);$createdExport=$true
    foreach($d in @('records','raw','results/generate-owned-export/generated')){[void][IO.Directory]::CreateDirectory((Join-Path $exportRoot $d))}
    $bytes=[byte[]]@(0,13,10,67,10);$digest=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
    [IO.File]::WriteAllBytes((Join-Path $exportRoot 'results/generate-owned-export/generated/parser.c'),$bytes)
    $outcome=@{kind='generation';scope='REGISTERED_P05';termination='EXITED';exit_code=0;label='generate-owned-export';result_directory='generate-owned-export'}
    foreach($suffix in @('','-copy')){
        foreach($stream in @('stdout','stderr')){$streamBytes=[byte[]]::new(0);if($suffix -ceq '-copy' -and $stream -ceq 'stdout'){$streamBytes=$bytes};[IO.File]::WriteAllBytes((Join-Path $exportRoot ('raw/generate-owned-export'+$suffix+'.'+$stream)),$streamBytes)}
        $command=@{label=('generate-owned-export'+$suffix);termination='EXITED';exit_code=0;host_cleanup_verified=$true;owned_fixture_only=$true}
        foreach($stream in @('stdout','stderr')){$file=Get-Item (Join-Path $exportRoot ('raw/generate-owned-export'+$suffix+'.'+$stream));$command['stored_'+$stream+'_bytes']=$file.Length;$command[$stream+'_sha256']=(Get-FileHash $file.FullName).Hash.ToLowerInvariant()}
        $command|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $exportRoot ('records/command-generate-owned-export'+$suffix+'.json')) -Encoding utf8NoBOM
    }
    $parser=@{path='results/generate-owned-export/generated/parser.c';bytes=$bytes.Length;sha256=$digest}
    @{before_generation=$true;files=@($parser);owned_fixture_only=$true}|ConvertTo-Json -Depth 8|Set-Content -LiteralPath (Join-Path $exportRoot 'records/generate-owned-export-inputs.json') -Encoding utf8NoBOM
    $buildPath=Join-Path $exportRoot 'records/build-owned-export-inputs.json'
    $proof=GeneratedArtifactProvenance $exportRoot $outcome $bytes.Length $digest
    if($proof.build_inputs_state -cne 'NOT_RUN_INPUT_RECORD_ABSENT'){throw 'Unrun build was promoted'}
    foreach($wrong in @($true,$false)){
        $identity=$parser.Clone();if($wrong){$identity.sha256='0'*64}
        @{before_build=$true;files=@($identity);owned_fixture_only=$true}|ConvertTo-Json -Depth 8|Set-Content -LiteralPath $buildPath -Encoding utf8NoBOM
        $rejected=$false;try{$proof=GeneratedArtifactProvenance $exportRoot $outcome $bytes.Length $digest}catch{$rejected=$true}
        if($rejected -ne $wrong){throw 'Build/source provenance mismatch control failed'}
    }
    $rejected=$false;try{$null=GeneratedArtifactProvenance $exportRoot @{label='generate-missing';result_directory='generate-missing'} $bytes.Length $digest}catch{$rejected=$true}
    if(-not $rejected){throw 'Missing command provenance accepted'}
    $copyPath=Join-Path $exportRoot 'raw/generate-owned-export-copy.stdout'
    [IO.File]::WriteAllBytes($copyPath,[byte[]]@(1,13,10,67,10))
    $rejected=$false;try{$null=GeneratedArtifactProvenance $exportRoot $outcome $bytes.Length $digest}catch{$rejected=$true}
    if(-not $rejected){throw 'Changed original capture accepted'}
    [IO.File]::WriteAllBytes($copyPath,$bytes)
    if($env:RUNNER_TEMP -and -not $IsWindows){
        @{remedy_stage='csharp-r3';outcomes=@($outcome);owned_fixture_only=$true}|ConvertTo-Json -Depth 8|Set-Content -LiteralPath (Join-Path $exportRoot 'records/summary.json') -Encoding utf8NoBOM
        & (Join-Path $PSScriptRoot 'collect.ps1') -Root $exportRoot
        $record=@(Get-Content -LiteralPath (Join-Path $exportRoot 'records/generated-artifacts.json') -Raw|ConvertFrom-Json)
        if($record.Count -ne 1 -or $record[0].original_bytes -ne $bytes.Length -or $record[0].provenance.references.Count -ne 8){throw 'Owned C export receipt mismatch'}
        $input=[IO.File]::OpenRead((Join-Path $exportRoot $record[0].lossless_gzip_path));$gzip=[IO.Compression.GZipStream]::new($input,[IO.Compression.CompressionMode]::Decompress);$sink=[IO.MemoryStream]::new()
        try{$gzip.CopyTo($sink);if([Convert]::ToHexString($sink.ToArray()) -cne [Convert]::ToHexString($bytes)){throw 'C export changed original NUL/CRLF'}}finally{$sink.Dispose();$gzip.Dispose();$input.Dispose()}
        $emptyExportRoot=[IO.Path]::GetFullPath((Join-Path $exportPrefix ('tsgk-p05-remedy-'+$run+'-'+[DateTime]::UtcNow.Ticks)))
        if(Test-Path -LiteralPath $emptyExportRoot){$emptyExportRoot=$null;throw 'Fresh empty export root required'}
        [void][IO.Directory]::CreateDirectory((Join-Path $emptyExportRoot 'records'))
        @{remedy_stage='csharp-r3';outcomes=@(@{kind='generation';scope='REGISTERED_P05';termination='EXITED';exit_code=1;label='generate-owned-failed';result_directory='generate-owned-failed'});owned_fixture_only=$true}|ConvertTo-Json -Depth 8|Set-Content -LiteralPath (Join-Path $emptyExportRoot 'records/summary.json') -Encoding utf8NoBOM
        & (Join-Path $PSScriptRoot 'collect.ps1') -Root $emptyExportRoot
        if([IO.File]::ReadAllText((Join-Path $emptyExportRoot 'records/generated-artifacts.json')).Trim() -cne '[]' -or -not (Test-Path -LiteralPath (Join-Path $emptyExportRoot 'evidence.zip'))){throw 'Failed generation evidence was not packaged'}
    }
}finally{
    if($createdExport -and $env:RUNNER_TEMP -and -not $IsWindows){
        foreach($cleanupRoot in @($exportRoot,$emptyExportRoot)|Where-Object {$_ -and (Test-Path -LiteralPath $_)}){
            if([IO.Path]::GetFullPath($cleanupRoot) -cne $cleanupRoot -or -not $cleanupRoot.StartsWith($exportPrefix,[StringComparison]::Ordinal) -or (Get-Item -LiteralPath $cleanupRoot -Force).Attributes -band [IO.FileAttributes]::ReparsePoint -or @(Get-ChildItem -LiteralPath $cleanupRoot -Force -Recurse|Where-Object {$_.Attributes -band [IO.FileAttributes]::ReparsePoint}).Count){throw 'Owned export cleanup target rejected'}
            [IO.Directory]::Delete($cleanupRoot,$true)
            if(Test-Path -LiteralPath $cleanupRoot){throw 'Owned export cleanup not verified'}
        }
    }
}
$savedPath=$env:PATH
try {
    $env:PATH=$directories -join [IO.Path]::PathSeparator
    $all=@(Get-Command -Name $applicationName -CommandType Application -All)
    if($all.Count -ne 2){throw 'Duplicate application baseline missing'}
    $selected=Application $applicationName
    if($selected -isnot [string] -or $selected -cne (Join-Path $directories[0] $applicationName)){throw 'Application precedence/path selection failed'}
    $rejected=$false;try{[void](Application 'p05-tool-not-present')}catch{$rejected=$true}
    if(-not $rejected){throw 'Missing application was accepted'}
} finally {$env:PATH=$savedPath}
@{result='PASS';checks=@('exact remedy subject/stage/image mismatches rejected','patch-r1 rows85/edit16; patch-r2 C#27+TS4+TSX8=39/edit7; SQL-PG r1/r2 rows46/edit3; no identical original baseline scheduled','both stage caps: reservation/polling/completion and below/equal/above transitions; unavailable/negative/regressed counters rejected','literal original/count/output digest mismatches rejected; 34 inputs exact bytes','ESM dynamic/package/root escape and missing/unpinned quoted includes rejected','duplicate PATH applications choose first exact path','missing application rejected','separate acquisition, capture and image subjects required','empty, old B, mismatched and unknown profiles rejected','original image identity preserved; new image bound to separate subject','case-distinct grammar keys accepted and exact name retained; invalid name rejected','ELF arch/loader/library/RUNPATH mismatches rejected','owned structure/edit checker rejects wrong fields, unequal trees and missing negative error','frozen init exact identity; live/stopped/child/PID mismatch/forged command rejected','NUL/CRLF tar recovery preserves bytes; deleted/replaced bytes rejected','executable archive mode mismatch rejected','real Freeze/StopContainer receipt names stay unique with owned responses','actual supervisor recovers exact8MiB owned burst within2seconds and verifies cleanup','failed acquisition retains observed counter; unavailable/regressed counter remains NOT_VERIFIED');unix_mode_roundtrip=$(if($IsWindows){'NOT_APPLICABLE'}else{'PASS'});supervisor_burst=$supervisorBurst;fixture_processes_executed=5;native_fixture_processes_executed=5;upstream_native_invocations=0;run_sha256=(Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'run.ps1')).Hash.ToLowerInvariant();approval_sha256=(Get-FileHash -LiteralPath $approval).Hash.ToLowerInvariant()}|ConvertTo-Json -Depth 5|Set-Content -LiteralPath (Join-Path $root 'self-test.json') -Encoding utf8NoBOM
Write-Output 'P05 self-check PASS; owned Go supervisor fixture build1/execute5, upstream native0'
