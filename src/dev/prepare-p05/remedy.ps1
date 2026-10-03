# PREPARE-only exact remedy work; shared run.ps1 owns containment and supervision.
function ReadRemedySubjects {
    $pins=@{
        'remedy-patches.json'='988deb9d119c753ab53f5e5b60b6571acc8442e2fc3a283beb7be2cb387546c9'
        'remedy-cases.json'='d29fb48c55cbadc3d584e02246af06937744c16524379cdd48d595f459cd4903'
        'remedy-fact-oracles.json'='af130694ac859f9d32a867af45cccfbcfdea3655d716ba4c2e8285744a3285c4'
        'remedy-sources.json'='f2c26f754bd80ee58719d5b2d12271d338938ebaed6f952a3499faf774e76bc6'
        'remedy-r2.json'='389803c2d8f9da35a5ff913b2748c59e7a0504322bd09c1ee2f004aa283ae6cc'
        'remedy-exact-r1.json'='6e195dffed56ae6385c0eb2bf1da6d2498cec11916a1f6d7a36fd8f2ac7c4b42'
        'remedy-followup-r2.json'='6372d231f57cc463c1a3089a2de9716a604b8759a98885c191326c827423ecaf'
        'remedy-csharp-r5.json'='6b4b43351a247e2bd4b7e366e38967eeaa74e487e70811cdd17ea3e66c4157b8'
    }
    $result=@{}
    foreach($name in $pins.Keys){
        $path=Join-Path $PSScriptRoot $name
        if((Get-FileHash -LiteralPath $path).Hash.ToLowerInvariant() -cne $pins[$name]){throw 'Exact remedy subject changed'}
        $result[$name.Replace('remedy-','').Replace('.json','')]=Get-Content -LiteralPath $path -Raw|ConvertFrom-Json -AsHashtable
    }
    if($result.cases.cases.Count -ne 34 -or @($result.cases.cases|Where-Object edit).Count -ne 6 -or $result.patches.patches.Count -ne 3 -or $result.sources.sql.selected_regular_files.Count -ne 30 -or $result.sources.sql.adoption_authorized -or $result.sources.sql.grammar_patch_authorized){throw 'Remedy scope changed'}
    if($result.r2.schema -cne 'tsgk.p05.exact-remedy-proposal.r2' -or $result.r2.cases.Count -ne 39 -or $result.r2.patches.Count -ne 2 -or $result.r2.candidate_adoption){throw 'Exact r2 scope changed'}
    # Repository LF projection reconstructs the immutable approved CRLF JSON exactly.
    $projection=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'remedy-r2.json'))
    $original=[Text.Encoding]::UTF8.GetBytes($projection.Substring(0,$projection.Length-1).Replace("`n","`r`n")+"`n")
    AssertRemedyObject $original.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($original)).ToLowerInvariant()) @{bytes=31295;sha256='a216d31a0242ac161291e3f00cacf721d602dcdf7f76d8e86ef5a8bf161f5ba2'}
    $projection=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'remedy-exact-r1.json'))
    $original=[Text.Encoding]::UTF8.GetBytes($projection.Substring(0,$projection.Length-1).Replace("`n","`r`n")+"`n")
    AssertRemedyObject $original.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($original)).ToLowerInvariant()) @{bytes=204857;sha256='80a69edf69a32263fc92efe3b5363583a6ee4b28e281752e33453333af617824'}
    $exact=$result['exact-r1']
    if($exact.schema -cne 'tsgk.prepare06.exact-remedy-effects-proposal/r1' -or $exact.cases.Count -ne 103 -or $exact.patches.Count -ne 2 -or $exact.new_pg_cases.Count -ne 4 -or $exact.final_provider_adoption){throw 'Exact three-effect scope changed'}
    $projection=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'remedy-followup-r2.json'))
    $original=[Text.Encoding]::UTF8.GetBytes($projection.Substring(0,$projection.Length-1).Replace("`n","`r`n")+"`n")
    AssertRemedyObject $original.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($original)).ToLowerInvariant()) @{bytes=235604;sha256='d6781d9736d1871488563ac458d22f97e78a0311194d417370e24a0fa01763d4'}
    $followup=$result['followup-r2']
    if($followup.schema -cne 'tsgk.prepare06.followup-exact-effects/r2' -or $followup.cases.Count -ne 80 -or $followup.new_cases.Count -ne 12 -or $followup.patches.Count -ne 2 -or $followup.final_provider_adoption){throw 'Exact followup scope changed'}
    $projection=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'remedy-csharp-r5.json'))
    $original=[Text.Encoding]::UTF8.GetBytes($projection.Replace("`n","`r`n"))
    AssertRemedyObject $original.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($original)).ToLowerInvariant()) @{bytes=50057;sha256='5d66340b2e0559bedc827e9fc3fc48f1d4af549e37b1cd2d6af5c9a089b488a7'}
    $r5=$result['csharp-r5']
    if($r5.schema -cne 'tsgk.prepare06.csharp-r5-local-storage-proposal/r2' -or $r5.stage -cne 'csharp-r5' -or $r5.rows.Count -ne 27 -or $r5.patch.replacements.Count -ne 2 -or $r5.final_provider_adoption -or -not (EqualRemedyData $r5.unchanged_tool_runtime_image.tools $followup.tools)){throw 'Exact C# r5 scope/tool tuple changed'}
    return $result
}
function FollowupStageKey([string]$stage){
    switch -CaseSensitive ($stage){'csharp-r4'{'csharp_r4'};'csharp-r5'{'csharp_r5'};'pg-legacy-g6-r1'{'pg_legacy_g6'};'mssql-patch-r1'{'mssql_r1'};default{return $null}}
}
function AssertFollowupCase($case){
    $bytes=[Text.Encoding]::UTF8.GetBytes($case.input_utf8)
    if($case.id -cnotmatch '^[A-Z0-9-]+(?:-r[0-9]+)?$' -or $bytes.Length -le 0 -or $bytes.Length -gt 65536){throw 'Empty or invalid followup input'}
    AssertRemedyObject $bytes.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()) @{bytes=$case.input_bytes;sha256=$case.input_sha256}
    if($case.expected.error_window){$window=$case.expected.error_window;if($null -eq $window.start -or $null -eq $window.end -or $window.start -lt 0 -or $window.start -ge $window.end -or $window.end -gt $bytes.Length){throw 'Followup error window outside source'}}
}
function ExactRemedyLimits([string]$stage){
    if($stage -ceq 'csharp-r5'){
        $subject=(ReadRemedySubjects)['csharp-r5'];$limits=$subject.operations.Clone()
        $limits.source_image_counter_bytes=$subject.limits.source_image_receive_bytes
        $limits.local_packed_max_bytes=$subject.limits.inner_bytes
        $limits.local_expanded_max_bytes=$subject.limits.expanded_CSharp_bytes
        return $limits
    }
    $followupKey=FollowupStageKey $stage
    if($followupKey){
        $subject=(ReadRemedySubjects)['followup-r2'];$limits=$subject.stage_operations[$followupKey].Clone()
        $limits.source_image_counter_bytes=$subject.stage_transfer_limits[$followupKey]
        $limits.local_packed_max_bytes=$subject.storage_amendment.per_artifact_inner_bytes
        $limits.local_expanded_max_bytes=if($stage -ceq 'pg-legacy-g6-r1'){$subject.storage_amendment.expanded_PG}else{$subject.storage_amendment.expanded_CSharp_MSSQL}
        return $limits
    }
    $key=switch -CaseSensitive ($stage){'csharp-r3'{'csharp_r3'};'pg-legacy-r1'{'pg_legacy_r1'};'mssql-evaluate-r1'{'mssql_evaluate_r1'};default{return $null}}
    return (ReadRemedySubjects)['exact-r1'].stage_operations[$key]
}
function AssertExactAcquisitionLimit([string]$stage,[long]$limit){
    $approved=ExactRemedyLimits $stage
    if(-not $approved -or $limit -ne $approved.source_image_counter_bytes){throw 'Exact stage counter differs from approved proposal'}
    return @{stage=$stage;actual_limit_bytes=$limit;proposal_limit_bytes=$approved.source_image_counter_bytes;match=$true}
}
function RemedyMemoryBytes([string]$stage,[string]$kind,[string]$label,[bool]$owned){
    if(-not $owned -and $stage -ceq 'pg-legacy-g6-r1' -and $kind -ceq 'generation' -and $label -ceq 'generate-postgresql-legacy-candidate-r1-g6'){return 6442450944L}
    return 4294967296L
}
function AssertGenerationCapacity([long]$available,$parents){
    if($available -lt 8589934592 -or -not $parents.Count){throw 'PG generation host capacity unavailable'}
    foreach($parent in $parents){
        if($parent.max -ceq 'NOT_APPLICABLE_HIERARCHY_ROOT'){
            if($parent.path -isnot [string] -or $parent.path -cne '/sys/fs/cgroup' -or $parent.mount_root -isnot [string] -or $parent.mount_root -cne '/' -or $parent.memory_controller_available -isnot [bool] -or $parent.memory_controller_available -ne $true -or $parent.max_file_present -isnot [bool] -or $parent.max_file_present -ne $false -or $parent.current_file_present -isnot [bool] -or $parent.current_file_present -ne $false -or $null -ne $parent.current -or $parent.primary_source -isnot [string] -or $parent.primary_source -cne 'https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#memory-interface-files'){throw 'PG generation hierarchy root not verified'}
            continue
        }
        if($null -eq $parent.current -or $parent.current -lt 0 -or [Math]::Truncate($parent.current) -ne $parent.current -or ($parent.max -cne 'max' -and ($parent.max -cnotmatch '^[0-9]+$' -or [long]$parent.max-$parent.current -lt 8589934592))){throw 'PG generation parent capacity unavailable'}
    }
}
function AssertMemoryEnvelope([long]$expected,[long]$observed,[long]$swap,[string]$cgroup){
    if($expected -notin @(4294967296L,6442450944L) -or $observed -ne $expected -or $swap -ne $expected -or $cgroup.Trim() -cne [string]$expected){throw 'Container cgroup memory mismatch'}
}
function MemoryEventDelta([string]$before,[string]$after){
    $snapshots=@(foreach($text in @($before,$after)){
        $values=@{}
        foreach($line in $text -split "`n"|Where-Object {$_}){
            if($line.Trim() -cnotmatch '^([a-z_]+) ([0-9]+)$' -or $values.ContainsKey($Matches[1])){throw 'Memory event counter unknown'}
            $values[$Matches[1]]=[long]$Matches[2]
        }
        foreach($key in @('max','oom','oom_kill','oom_group_kill')){if(-not $values.ContainsKey($key)){throw 'Memory event counter missing'}}
        $values
    })
    if($snapshots.Count -ne 2 -or $snapshots[0].Count -ne $snapshots[1].Count){throw 'Memory event counter set changed'}
    $delta=@{}
    foreach($key in $snapshots[0].Keys){
        if(-not $snapshots[1].ContainsKey($key) -or $snapshots[1][$key] -lt $snapshots[0][$key]){throw 'Memory event counter regressed'}
        $delta[$key]=$snapshots[1][$key]-$snapshots[0][$key]
    }
    return @{before=$snapshots[0];after=$snapshots[1];delta=$delta;state=$(if($delta.oom -gt 0 -or $delta.oom_kill -gt 0 -or $delta.oom_group_kill -gt 0){'OOM_OBSERVED'}else{'NO_OOM_OBSERVED'})}
}
function ConfirmGenerationCapacity {
    $mem=[IO.File]::ReadAllText('/proc/meminfo')
    if($mem -cnotmatch '(?m)^MemAvailable:\s+([0-9]+) kB$'){throw 'PG generation host MemAvailable unknown'};$available=[long]$Matches[1]*1024
    $membership=[IO.File]::ReadAllText('/proc/self/cgroup').Trim()
    if($membership -cnotmatch '^0::(/[A-Za-z0-9_./-]*)$'){throw 'PG generation cgroup membership unknown'};$member=$Matches[1]
    $mounts=@([IO.File]::ReadAllLines('/proc/self/mountinfo')|Where-Object {$_ -cmatch ' - cgroup2 '})
    if($mounts.Count -ne 1){throw 'PG generation cgroup mount unknown'};$fields=$mounts[0].Split(' ');$mountRoot=$fields[3];$mountPoint=$fields[4]
    if($mountPoint -cne '/sys/fs/cgroup' -or $mountRoot -cnotmatch '^/[A-Za-z0-9_./-]*$' -or @($member.Split('/')|Where-Object {$_ -in @('.','..')}).Count -or @($mountRoot.Split('/')|Where-Object {$_ -in @('.','..')}).Count){throw 'PG generation cgroup path rejected'}
    $prefix=$mountRoot.TrimEnd('/')+'/'
    if($member -ceq $mountRoot){$relative=''}elseif($member.StartsWith($prefix,[StringComparison]::Ordinal)){$relative=$member.Substring($prefix.Length)}else{throw 'PG generation cgroup root mismatch'}
    $cursor=[IO.Path]::GetFullPath((Join-Path $mountPoint $relative));$parents=@()
    while($true){
        if($parents.Count -ge 64 -or ($cursor -cne $mountPoint -and -not $cursor.StartsWith($mountPoint+'/',[StringComparison]::Ordinal)) -or (Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'PG generation parent path rejected'}
        $maxFile=Join-Path $cursor 'memory.max';$currentFile=Join-Path $cursor 'memory.current'
        if($cursor -ceq $mountPoint -and $mountRoot -ceq '/' -and -not (Test-Path -LiteralPath $maxFile) -and -not (Test-Path -LiteralPath $currentFile) -and 'memory' -cin ([IO.File]::ReadAllText((Join-Path $cursor 'cgroup.controllers')).Trim() -split '\s+')){
            # The actual cgroup2 hierarchy root has no memory.current/memory.max interfaces.
            $parents+=@{path=$cursor;mount_root=$mountRoot;max='NOT_APPLICABLE_HIERARCHY_ROOT';current=$null;memory_controller_available=$true;max_file_present=$false;current_file_present=$false;primary_source='https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#memory-interface-files'}
            break
        }
        $max=[IO.File]::ReadAllText($maxFile).Trim();$current=[IO.File]::ReadAllText($currentFile).Trim()
        if($current -cnotmatch '^[0-9]+$'){throw 'PG generation parent usage unknown'};$parents+=@{path=$cursor;max=$max;current=[long]$current}
        if($cursor -ceq $mountPoint){break};$cursor=[IO.Path]::GetDirectoryName($cursor)
    }
    AssertGenerationCapacity $available $parents
    Record 'pg-generation-capacity' @{result='PASS';before_G=$true;MemAvailable_bytes=$available;minimum_available_bytes=8589934592L;resolved_parents=$parents;generation_memory_bytes=6442450944L;fallback=$false}
}
function EqualRemedyData($a,$b){
    if($a -is [Collections.IDictionary]){
        if($b -isnot [Collections.IDictionary] -or $a.Count -ne $b.Count){return $false}
        foreach($key in $a.Keys){if(-not $b.Contains($key) -or -not (EqualRemedyData $a[$key] $b[$key])){return $false}};return $true
    }
    if($a -is [array]){
        if($b -isnot [array] -or $a.Count -ne $b.Count){return $false}
        for($i=0;$i -lt $a.Count;$i++){if(-not (EqualRemedyData $a[$i] $b[$i])){return $false}};return $true
    }
    return $a -ceq $b
}
function AssertRemedyObject([long]$bytes,[string]$sha256,$pin){
    if($bytes -ne $pin.bytes -or $sha256 -cne $pin.sha256){throw 'Exact remedy object identity mismatch'}
}
function ApplyLiteralPatch([string]$source,$pin){
    $bytes=[Text.Encoding]::UTF8.GetBytes($source)
    AssertRemedyObject $bytes.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()) @{bytes=$pin.original_bytes;sha256=$pin.original_sha256}
    foreach($operation in $pin.operations){
        if([regex]::Matches($source,[regex]::Escape($operation.before)).Count -ne $operation.occurrences){throw 'Literal patch occurrence mismatch'}
        $source=$source.Replace($operation.before,$operation.after)
    }
    $bytes=[Text.Encoding]::UTF8.GetBytes($source)
    AssertRemedyObject $bytes.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()) @{bytes=$pin.proposed_result_bytes;sha256=$pin.proposed_result_sha256}
    return $source
}
function NewRemedyRows([string]$stage,$original,$additional){
    $rows=[Collections.Generic.List[object]]::new()
    if($stage -ceq 'patch-r1'){
        foreach($route in @('csharp','typescript','tsx','swift')){
            foreach($producer in @(($route+'-candidate'),($route+'-original'))){
                $cases=@($additional|Where-Object route -CEQ $route)
                if($producer.EndsWith('-candidate')){$cases=@($original|Where-Object route -CEQ $route)+$cases}
                foreach($case in $cases){$rows.Add(@{id=$case.id;route=$route;producer=$producer;case=$case;state='NOT_RUN';dependency='GENERATION_BUILD_AND_SAFETY_PREREQUISITES';command_label=('case-'+$producer+'-'+$case.id.ToLowerInvariant());raw_stdout=$null;exit_code=$null})}
            }
        }
        $expected=85
    }elseif($stage -ceq 'patch-r2'){
        $r2=(ReadRemedySubjects).r2
        foreach($registered in $r2.cases){
            $matches=@(@($original)+@($additional)|Where-Object id -CEQ $registered.id)
            if($matches.Count -ne 1 -or $registered.route -cnotin @('csharp','typescript','tsx') -or $registered.producer -cne ($registered.route+'-candidate-r2')){throw 'Exact r2 case mapping mismatch'}
            $case=$matches[0]|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable
            foreach($key in @('id','route','input_bytes','input_sha256','feature_ids','expected','edit')){if(-not (EqualRemedyData $case.$key $registered.$key)){throw 'Exact r2 input/expectation/edit changed'}}
            $rows.Add(@{id=$case.id;route=$case.route;producer=$registered.producer;case=$case;state='NOT_RUN';dependency='GENERATION_BUILD_AND_SAFETY_PREREQUISITES';command_label=('case-'+$registered.producer+'-'+$case.id.ToLowerInvariant());raw_stdout=$null;exit_code=$null})
        }
        if(@($rows|Where-Object {$_.case.edit}).Count -ne 7){throw 'Exact r2 edit count changed'}
        $expected=39
    }elseif(FollowupStageKey $stage){
        $subjects=ReadRemedySubjects;$followup=$subjects['followup-r2']
        $producer=switch -CaseSensitive ($stage){'csharp-r4'{'csharp-candidate-r4'};'csharp-r5'{'csharp-candidate-r5'};'pg-legacy-g6-r1'{'postgresql-legacy-candidate-r1-g6'};'mssql-patch-r1'{'mssql-candidate-r1'}}
        $registeredRows=if($stage -ceq 'csharp-r5'){$subjects['csharp-r5'].rows}else{@($followup.cases|Where-Object producer -CEQ $producer)}
        foreach($registered in $registeredRows){
            $matches=@(@($original)+@($additional)+@($subjects['exact-r1'].new_pg_cases)+@($followup.new_cases)|Where-Object id -CEQ $registered.id)
            if($matches.Count -ne 1){throw 'Followup case mapping mismatch'}
            $case=$matches[0]|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable
            $keys=@('id','route','input_bytes','input_sha256','feature_ids','expected','edit');if($stage -cne 'csharp-r5'){$keys+='input_utf8'}
            foreach($key in $keys){if(-not (EqualRemedyData $case.$key $registered.$key)){throw 'Followup input/expectation/edit changed'}}
            AssertFollowupCase $case
            $rows.Add(@{id=$case.id;route=$case.route;producer=$producer;case=$case;state='NOT_RUN';dependency='GENERATION_BUILD_AND_SAFETY_PREREQUISITES';command_label=('case-'+$producer+'-'+$case.id.ToLowerInvariant());raw_stdout=$null;exit_code=$null})
        }
        $limits=ExactRemedyLimits $stage;$expected=$limits.X
        if(@($rows|Where-Object {$_.case.edit}).Count -ne $limits.producer_edit){throw 'Followup edit budget changed'}
    }elseif($stage -cin @('csharp-r3','pg-legacy-r1','mssql-evaluate-r1')){
        $exact=(ReadRemedySubjects)['exact-r1']
        $producers=switch -CaseSensitive ($stage){'csharp-r3'{@('csharp-candidate-r3')};'pg-legacy-r1'{@('postgresql-legacy-candidate-r1','postgresql-lfs-baseline-new-regression')};'mssql-evaluate-r1'{@('mssql-baseline-8620fbc','mssql-regenerated-8620fbc')}}
        foreach($registered in $exact.cases|Where-Object {$_.producer -cin $producers}){
            $matches=@(@($original)+@($additional)+@($exact.new_pg_cases)|Where-Object id -CEQ $registered.id)
            if($matches.Count -ne 1){throw 'Exact three-effect case mapping mismatch'}
            $case=$matches[0]|ConvertTo-Json -Depth 40|ConvertFrom-Json -AsHashtable
            foreach($key in @('id','route','input_utf8','input_bytes','input_sha256','feature_ids','expected','edit')){if(-not (EqualRemedyData $case.$key $registered.$key)){throw 'Exact three-effect input/expectation/edit changed'}}
            $rows.Add(@{id=$case.id;route=$case.route;producer=$registered.producer;case=$case;state='NOT_RUN';dependency='GENERATION_BUILD_AND_SAFETY_PREREQUISITES';command_label=('case-'+$registered.producer+'-'+$case.id.ToLowerInvariant());raw_stdout=$null;exit_code=$null})
        }
        $limits=ExactRemedyLimits $stage
        if(@($rows|Where-Object {$_.case.edit}).Count -ne $limits.producer_edit){throw 'Exact three-effect edit count changed'}
        $expected=$limits.X
    }elseif($stage -cin @('sql-pg-r1','sql-pg-r2','sql-only-r2')){
        $producers=if($stage -ceq 'sql-only-r2'){@('derek-sql-candidate')}else{@('derek-sql-candidate','postgresql-lfs-baseline','postgresql-noopt-regenerated')}
        foreach($producer in $producers){
            $route=if($producer -ceq 'derek-sql-candidate'){'tsql'}else{'postgresql-sql'}
            $cases=@($original|Where-Object route -CEQ $route)
            if($route -ceq 'tsql'){$cases+=@($additional|Where-Object route -CEQ $route)}
            foreach($case in $cases){$rows.Add(@{id=$case.id;route=$route;producer=$producer;case=$case;state='NOT_RUN';dependency='GENERATION_BUILD_AND_SAFETY_PREREQUISITES';command_label=('case-'+$producer+'-'+$case.id.ToLowerInvariant());raw_stdout=$null;exit_code=$null})}
        }
        $expected=if($stage -ceq 'sql-only-r2'){30}else{46}
    }else{throw 'Unknown exact remedy stage'}
    if($rows.Count -ne $expected -or @($rows.command_label|Sort-Object -Unique).Count -ne $expected){throw 'Remedy producer/case plan mismatch'}
    return ,$rows.ToArray()
}
function TryRemedyPrecheck([string]$producer,[string]$stage,[scriptblock]$check){
    try{return @{passed=$true;value=(& $check)}}catch{
        # Only diagnosed producer input/closure failures are recoverable here.
        # Identity, authority, native limits, isolation and cleanup failures propagate.
        $known=@('Unreviewed JS loader/evaluation','Unresolved dynamic/ES module dependency','Unregistered JS dependency','Unreviewed SQL loader/evaluation','Unresolved SQL ESM import','Unregistered SQL dependency','Quoted include input missing before build','Quoted include count limit','Unpinned quoted include input','Parser ABI unavailable before build','Parser ABI outside fixed runtime range','Grammar name rejected','SQL candidate license mismatch','Compiler header closure unavailable','Unpinned project header','Executable ELF/library closure mismatch','Recovered executable mode mismatch')
        if($_.Exception.Message -notin $known -and $_.Exception.Message -cnotmatch '^Required (JS dependency|SQL dependency|generated parser|recovered executable|ELF report) unavailable: [A-Za-z0-9_./-]+$'){throw}
        $failure=@{producer=$producer;stage=$stage;failure_type=$_.Exception.GetType().FullName;reason=$_.Exception.Message;operation='BOUNDED_PRODUCER_PRECHECK';state='NOT_RUN';native_rerun=$false}
        Record ('blocked-'+$producer+'-'+$stage) $failure
        foreach($row in $script:remedyRows|Where-Object producer -CEQ $producer){$row.state='NOT_RUN';$row.dependency='PRECHECK_FAILED_'+$stage.ToUpperInvariant();$row['precheck_failure']=$failure}
        $script:failed=$true
        return @{passed=$false;value=$null}
    }
}
function PrepareRemedyInputs($subjects){
    Record 'remedy-authority-binding' @{stage=$RemedyStage;execution_subject=$RemedyApprovalSubject;actual_user_authority='caller acceptance record separately required; subject alone is not permission';prior_r2_subject='a72b87c3dfe6561855749b64cce03bdaa5d7c231948f42dfa6ef41ce84a7747e';prior_r2_projection=@{path='harness/remedy-r2.json';bytes=(Get-Item -LiteralPath (Join-Path $PSScriptRoot 'remedy-r2.json')).Length;sha256=(Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'remedy-r2.json')).Hash.ToLowerInvariant();approved_raw_bytes=31295;approved_raw_sha256='a216d31a0242ac161291e3f00cacf721d602dcdf7f76d8e86ef5a8bf161f5ba2';serialization='LF projection; internal CRLF and terminal LF reconstructed and checked by ReadRemedySubjects'};original_inputs_sha256=(Get-FileHash $inputsPath).Hash.ToLowerInvariant();prior_aggregate_sha256='68053aac3fc0dab788b48d9c78cbd577a3f182dfb7f2e092f4d0f944f64de274';prior_execution_is_not_current=$true;conditional_registration='NOT_ADOPTED';source_application_execution=$false}
    foreach($identity in @($acquisition.remedy_identities)){
        foreach($file in $identity.files){
            $relative=$identity.task_relative_source_root+'/'+$file.path
            $actual=FileIdentity (Join-Path $root $relative)
            AssertRemedyObject $actual.bytes $actual.sha256 $file
            $script:verifiedSource[$relative]=$actual.sha256
        }
    }
    if($RemedyStage -cin @('csharp-r3','pg-legacy-r1','mssql-evaluate-r1')){
        Record 'exact-remedy-authority-binding' @{human_subject=$RemedyApprovalSubject;approved_machine_sha256='80a69edf69a32263fc92efe3b5363583a6ee4b28e281752e33453333af617824';projection=(FileIdentity (Join-Path $PSScriptRoot 'remedy-exact-r1.json'));stage=$RemedyStage;limits=(ExactRemedyLimits $RemedyStage);candidate_adopted=$false;old_failed_evidence_preserved=$true}
    }
    if(FollowupStageKey $RemedyStage){
        $machine=if($RemedyStage -ceq 'csharp-r5'){'5d66340b2e0559bedc827e9fc3fc48f1d4af549e37b1cd2d6af5c9a089b488a7'}else{'d6781d9736d1871488563ac458d22f97e78a0311194d417370e24a0fa01763d4'}
        $projection=if($RemedyStage -ceq 'csharp-r5'){'remedy-csharp-r5.json'}else{'remedy-followup-r2.json'}
        Record 'followup-remedy-authority-binding' @{human_subject=$RemedyApprovalSubject;approved_machine_sha256=$machine;projection=(FileIdentity (Join-Path $PSScriptRoot $projection));stage=$RemedyStage;limits=(ExactRemedyLimits $RemedyStage);candidate_adopted=$false;original_expectations_and_failures_preserved=$true}
    }
    if($RemedyStage -ceq 'mssql-patch-r1'){
        $patch=@($subjects['followup-r2'].patches|Where-Object id -CEQ 'P05-MSSQL-REMEDY-r1')[0]
        CopyExactRemedyCandidate (Join-Path $root 'candidate-evaluation/mssql-8620fbc') $patch 'd6781d9736d1871488563ac458d22f97e78a0311194d417370e24a0fa01763d4'
        return
    }
    if($RemedyStage -cin @('pg-legacy-r1','pg-legacy-g6-r1')){
        $patch=@($subjects['exact-r1'].patches|Where-Object id -CEQ 'P05-PG-LEGACY-REMEDY-r1')[0]
        $sourceRoot=Join-Path $root ('acquisition/sources/'+$patch.repository.Replace('/','--')+'--'+$patch.revision)
        CopyExactRemedyCandidate $sourceRoot $patch
        return
    }
    if($RemedyStage -cnotin @('patch-r1','patch-r2','csharp-r3','csharp-r4','csharp-r5')){return}
    foreach($patch in $subjects.patches.patches){
        if($RemedyStage -cin @('csharp-r3','csharp-r4','csharp-r5') -and $patch.repository -cne 'tree-sitter/tree-sitter-c-sharp'){continue}
        if($RemedyStage -ceq 'patch-r2' -and $patch.repository -ceq 'alex-pinkus/tree-sitter-swift'){continue}
        $key=$patch.repository.Replace('/','--')+'--'+$patch.base_commit
        $sourceRoot=Join-Path $root ('acquisition/sources/'+$key)
        $candidateRoot=Join-Path $root ('candidates/'+$patch.id)
        if(Test-Path -LiteralPath $candidateRoot){throw 'Candidate already exists'}
        [void][IO.Directory]::CreateDirectory($candidateRoot)
        $records=@()
        foreach($file in Get-ChildItem -LiteralPath $sourceRoot -File -Recurse){
            $identity=FileIdentity $file.FullName
            if(-not $script:verifiedSource.ContainsKey($identity.path) -or $script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'Candidate copy contains unverified source'}
            $relative=[IO.Path]::GetRelativePath($sourceRoot,$file.FullName).Replace('\','/')
            $target=Join-Path $candidateRoot $relative
            [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
            $filePatch=@($patch.files|Where-Object target -CEQ $relative)
            if($filePatch.Count -gt 1){throw 'Duplicate file patch'}
            if($filePatch.Count){[IO.File]::WriteAllText($target,(ApplyLiteralPatch ([IO.File]::ReadAllText($file.FullName)) $filePatch[0]),[Text.UTF8Encoding]::new($false))}
            else{[IO.File]::Copy($file.FullName,$target)}
            $candidateIdentity=FileIdentity $target;$script:verifiedSource[$candidateIdentity.path]=$candidateIdentity.sha256
            $records+=@{original=$identity;candidate=$candidateIdentity;patched=[bool]$filePatch.Count}
        }
        if(@($records|Where-Object patched).Count -ne $patch.files.Count){throw 'Missing literal patch target'}
        Record ('candidate-'+$patch.id.ToLowerInvariant()) @{proposal_id=$patch.id;original_repository=$patch.repository;original_commit=$patch.base_commit;files=$records;source_repository_mutated=$false;adopted=$false;native_support='NOT_RUN'}
        if($RemedyStage -cin @('patch-r2','csharp-r3','csharp-r4','csharp-r5')){
            $next=@($subjects.r2.patches|Where-Object repository -CEQ $patch.repository)
            if($next.Count -ne 1 -or $next[0].base_commit -cne $patch.base_commit){throw 'Exact r2 predecessor mismatch'}
            $next=$next[0];$nextRoot=Join-Path $root ('candidates/'+$next.id)
            if(Test-Path -LiteralPath $nextRoot){throw 'r2 candidate already exists'}
            $pin=$next.file.Clone();$pin.original_bytes=$pin.base_result_bytes;$pin.original_sha256=$pin.base_result_sha256
            $nextRecords=@()
            foreach($file in Get-ChildItem -LiteralPath $candidateRoot -File -Recurse){
                $identity=FileIdentity $file.FullName
                if($script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'r1 candidate changed before r2'}
                $relative=[IO.Path]::GetRelativePath($candidateRoot,$file.FullName).Replace('\','/')
                $target=Join-Path $nextRoot $relative;[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
                $patched=$relative -ceq $pin.target
                if($patched){[IO.File]::WriteAllText($target,(ApplyLiteralPatch ([IO.File]::ReadAllText($file.FullName)) $pin),[Text.UTF8Encoding]::new($false))}else{[IO.File]::Copy($file.FullName,$target)}
                $candidate=FileIdentity $target;$script:verifiedSource[$candidate.path]=$candidate.sha256
                $nextRecords+=@{predecessor=$identity;candidate=$candidate;patched=$patched}
            }
            if(@($nextRecords|Where-Object patched).Count -ne 1){throw 'Exact r2 patch target missing'}
            Record ('candidate-'+$next.id.ToLowerInvariant()) @{proposal_id=$next.id;predecessor=$patch.id;machine_subject='a216d31a0242ac161291e3f00cacf721d602dcdf7f76d8e86ef5a8bf161f5ba2';files=$nextRecords;source_repository_mutated=$false;adopted=$false;native_support='NOT_RUN'}
            if($RemedyStage -cin @('csharp-r3','csharp-r4','csharp-r5')){
                $last=@($subjects['exact-r1'].patches|Where-Object id -CEQ 'P05-CSHARP-REMEDY-r3')[0]
                CopyExactRemedyCandidate $nextRoot $last
                $scanner=FileIdentity (Join-Path $root ('candidates/'+$last.id+'/'+$last.scanner.target))
                AssertRemedyObject $scanner.bytes $scanner.sha256 $last.scanner
                if($RemedyStage -cin @('csharp-r4','csharp-r5')){
                    $followup=@($subjects['followup-r2'].patches|Where-Object id -CEQ 'P05-CSHARP-REMEDY-r4')[0]
                    CopyExactRemedyCandidate (Join-Path $root ('candidates/'+$last.id)) $followup 'd6781d9736d1871488563ac458d22f97e78a0311194d417370e24a0fa01763d4'
                    if($RemedyStage -ceq 'csharp-r5'){CopyExactRemedyCandidate (Join-Path $root ('candidates/'+$followup.id)) (CsharpR5Patch $subjects['csharp-r5']) '5d66340b2e0559bedc827e9fc3fc48f1d4af549e37b1cd2d6af5c9a089b488a7'}
                }
            }
        }
    }
}
function CsharpR5Patch($subject){
    $patch=$subject.patch;$scanner=$patch.scanner.Clone();$scanner.target='src/scanner.c'
    return @{id=$patch.id;scanner=$scanner;files=@(@{target=$patch.file;base_file=$patch.base_file;proposed_result_bytes=$patch.proposed_result.bytes;proposed_result_sha256=$patch.proposed_result.sha256;operations=@($patch.replacements|ForEach-Object {@{before=$_.old;after=$_.new;occurrences=1}})})}
}
function CopyExactRemedyCandidate([string]$sourceRoot,$patch,[ValidateSet('80a69edf69a32263fc92efe3b5363583a6ee4b28e281752e33453333af617824','d6781d9736d1871488563ac458d22f97e78a0311194d417370e24a0fa01763d4','5d66340b2e0559bedc827e9fc3fc48f1d4af549e37b1cd2d6af5c9a089b488a7')][string]$machineSubject='80a69edf69a32263fc92efe3b5363583a6ee4b28e281752e33453333af617824'){
    $candidateRoot=Join-Path $root ('candidates/'+$patch.id)
    if(Test-Path -LiteralPath $candidateRoot){throw 'Exact candidate already exists'}
    $pins=@(if($patch.files){$patch.files}else{$pin=$patch.change.Clone();$pin.target=$patch.file;$pin})
    $records=@()
    foreach($file in Get-ChildItem -LiteralPath $sourceRoot -File -Recurse){
        $identity=FileIdentity $file.FullName
        if($script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'Exact candidate contains changed/unverified source'}
        $relative=[IO.Path]::GetRelativePath($sourceRoot,$file.FullName).Replace('\','/')
        $target=Join-Path $candidateRoot $relative;[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
        $matches=@($pins|Where-Object target -CEQ $relative);if($matches.Count -gt 1){throw 'Duplicate exact patch target'};$patched=$matches.Count -eq 1
        if($patched){$pin=$matches[0].Clone();$pin.original_bytes=$pin.base_file.bytes;$pin.original_sha256=$pin.base_file.sha256;[IO.File]::WriteAllText($target,(ApplyLiteralPatch ([IO.File]::ReadAllText($file.FullName)) $pin),[Text.UTF8Encoding]::new($false))}else{[IO.File]::Copy($file.FullName,$target)}
        $candidate=FileIdentity $target;$script:verifiedSource[$candidate.path]=$candidate.sha256
        $records+=@{predecessor=$identity;candidate=$candidate;patched=$patched}
    }
    if(@($records|Where-Object patched).Count -ne $pins.Count){throw 'Exact patch target missing'}
    if($patch.scanner){$scannerPath=if($patch.scanner.target){$patch.scanner.target}else{$patch.scanner.path};if($scannerPath -cne 'src/scanner.c'){throw 'Exact scanner path mismatch'};$scanner=FileIdentity (Join-Path $candidateRoot $scannerPath);AssertRemedyObject $scanner.bytes $scanner.sha256 $patch.scanner}
    Record ('candidate-'+$patch.id.ToLowerInvariant()) @{proposal_id=$patch.id;machine_subject=$machineSubject;files=$records;source_repository_mutated=$false;adopted=$false;native_support='NOT_RUN'}
}
function CheckRemedyJson([string]$path){
    $identity=FileIdentity $path
    if($script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'Unverified JSON generation input'}
    $grammar=Get-Content -LiteralPath $path -Raw|ConvertFrom-Json -AsHashtable
    if(-not $grammar.rules.Count -or $grammar.name -cnotmatch '^[A-Za-z_][A-Za-z0-9_]{0,63}$'){throw 'Invalid JSON generation input'}
    return ,@($identity)
}
function RemedyProbePin($tools,[string]$stage){
    # 2026-10-02 user-approved MSSQL rerun: probe case-name check aligned with ^[A-Z0-9-]+(?:-r[0-9]+)?$; approved subjects keep the prior probe pin.
    if($stage -ceq 'mssql-patch-r1'){return @{path='src/dev/prepare-p05/probe.c.in';bytes=5910;sha256='5171da776dd6dbcdf379b106522e7d716ddcf45174bfd870aee40174cbe91a70';amends=$tools.probe}}
    return $tools.probe
}
function CheckExactRemedyTools($tools,[string]$output,[string]$stage=''){
    foreach($pin in @($tools.node,$tools.gcc,$tools.ld,$tools.loader,@{path='/lib/x86_64-linux-gnu/libc.so.6';sha256=$tools.libc.sha256},@{path='/inputs/acquisition/tools/tree-sitter';sha256='5a228811cdb3a01b7e4dd493c5fc5e05b0040a49ffede94e866c4c58ff2605db'})){
        $lines=@($output -split "`n"|Where-Object {$_ -cmatch ('^[0-9a-f]{64}  '+[regex]::Escape($pin.path)+'$')})
        if($lines.Count -ne 1 -or $lines[0].Substring(0,64) -cne $pin.sha256){throw 'Exact remedy tool bytes mismatch'}
    }
    $probe=FileIdentity (Join-Path $root 'probe.c');AssertRemedyObject $probe.bytes $probe.sha256 (RemedyProbePin $tools $stage)
    Record 'exact-remedy-tool-gate' @{result='PASS';source='raw/tool-environment.stdout';image=$tools.image;before_upstream_generation_build_execution=$true;whole_support=$false}
}
function CheckSqlJsInputs([string]$entry,[string]$sourceRoot){
    $queue=[Collections.Generic.Queue[string]]::new();$queue.Enqueue($entry)
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal);$identities=@()
    $prefix=[IO.Path]::GetFullPath($sourceRoot).TrimEnd('/','\')+[IO.Path]::DirectorySeparatorChar
    while($queue.Count){
        $path=[IO.Path]::GetFullPath($queue.Dequeue());if(-not $seen.Add($path)){continue}
        if(-not $path.StartsWith($prefix,[StringComparison]::Ordinal)){throw 'SQL import outside fixed root'}
        if(-not (Test-Path -LiteralPath $path -PathType Leaf)){throw ('Required SQL dependency unavailable: '+[IO.Path]::GetRelativePath($root,$path).Replace('\','/'))}
        $identity=FileIdentity $path
        if(-not $script:verifiedSource.ContainsKey($identity.path)){throw 'Unregistered SQL dependency'}
        if($script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'SQL dependency bytes changed'}
        $source=[IO.File]::ReadAllText($path)
        # JavaScript identifiers are case-sensitive: function is not the Function constructor.
        $loaderWords=@([regex]::Matches($source,'\b(?:require|createRequire|eval|Function)\b'))
        $relative=[IO.Path]::GetRelativePath($sourceRoot,$path).Replace('\','/')
        # One reviewed prose occurrence; no comment stripping or JS execution permission.
        $inertComment=$relative -ceq 'grammar/statements/create-function.js' -and $identity.bytes -eq 2539 -and $identity.sha256 -ceq 'f395d3e20195f86c3e3902f0ca05f32e0b6df242ecc744e54732ca71d8161b4a' -and $loaderWords.Count -eq 1 -and $loaderWords[0].Index -eq 1289 -and $loaderWords[0].Value -ceq 'require'
        if($loaderWords.Count -and -not $inertComment){throw 'Unreviewed SQL loader/evaluation'}
        $imports=[regex]::Matches($source,'(?m)^\s*import\s+(?:[A-Za-z_$][\w$]*|\{[\w\s,$]*\})\s+from\s+["''](\.{1,2}/[A-Za-z0-9_./-]+\.js)["''];?\s*$')
        if([regex]::Matches($source,'\bimport\b').Count -ne $imports.Count){throw 'Unresolved SQL ESM import'}
        foreach($import in $imports){$queue.Enqueue((Join-Path ([IO.Path]::GetDirectoryName($path)) $import.Groups[1].Value))}
        $identities+=$identity
    }
    return ,$identities
}
function CheckRemedyQuotedIncludes([string[]]$entries,[string]$parserRoot){
    $queue=[Collections.Generic.Queue[string]]::new();foreach($entry in $entries){$queue.Enqueue($entry)}
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal);$identities=@()
    $runtimeRoot=Join-Path $root 'acquisition/sources/tree-sitter--tree-sitter--659cda7c7f86ebe31cc825dc5da59e9add172dc7'
    $parserPrefix=[IO.Path]::GetFullPath($parserRoot).TrimEnd('/','\')+[IO.Path]::DirectorySeparatorChar
    while($queue.Count){
        $path=[IO.Path]::GetFullPath($queue.Dequeue());if(-not $seen.Add($path)){continue}
        if($seen.Count -gt 512){throw 'Quoted include count limit'}
        $taskPrefix=[IO.Path]::GetFullPath($root).TrimEnd('/','\')+[IO.Path]::DirectorySeparatorChar
        if(-not $path.StartsWith($taskPrefix,[StringComparison]::Ordinal)){throw 'Quoted include outside task root'}
        $identity=FileIdentity $path
        if($path -cne (Join-Path $root 'probe.c') -and -not $script:verifiedSource.ContainsKey($identity.path) -and -not $path.StartsWith($parserPrefix,[StringComparison]::Ordinal)){throw 'Unpinned quoted include input'}
        if($script:verifiedSource.ContainsKey($identity.path) -and $script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'Quoted include bytes changed'}
        $source=[IO.File]::ReadAllText($path)
        foreach($match in [regex]::Matches($source,'(?m)^\s*#\s*include\s*"([^"\r\n]+)"')){
            $name=$match.Groups[1].Value
            if($name -notmatch '^[A-Za-z0-9_./-]+$'){throw 'Unsafe quoted include'}
            $candidates=@((Join-Path ([IO.Path]::GetDirectoryName($path)) $name),(Join-Path $parserRoot $name),(Join-Path $runtimeRoot ('lib/src/'+$name)),(Join-Path $runtimeRoot ('lib/include/'+$name)))
            $next=@($candidates|Where-Object {Test-Path -LiteralPath $_ -PathType Leaf})
            if(-not $next.Count){throw 'Quoted include input missing before build'}
            $queue.Enqueue($next[0])
        }
        $identities+=$identity
    }
    return ,$identities
}
function InvokeRemedyBuildAndCases([string]$producer,[string]$grammarName,[string]$parser,[string]$parserRoot,[string]$scanner,[string]$scannerRoot){
    if($grammarName -notmatch '^[A-Za-z_][A-Za-z0-9_]{0,63}$'){throw 'Invalid remedy entry point'}
    $label='build-'+$producer
    $entries=@((Join-Path $parserRoot 'parser.c'),(Join-Path $root 'probe.c'))
    if($scanner){$entries+=Join-Path $root $scannerRoot}
    $precheck=TryRemedyPrecheck $producer 'build' {
    if(-not (Test-Path -LiteralPath (Join-Path $parserRoot 'parser.c') -PathType Leaf)){throw ('Required generated parser unavailable: '+[IO.Path]::GetRelativePath($root,(Join-Path $parserRoot 'parser.c')).Replace('\','/'))}
    $closure=CheckRemedyQuotedIncludes $entries $parserRoot
    $parserText=[IO.File]::ReadAllText((Join-Path $parserRoot 'parser.c'))
    if($parserText -cnotmatch '(?m)^#define LANGUAGE_VERSION ([0-9]+)\s*$'){throw 'Parser ABI unavailable before build'}
    $languageAbi=[int]$Matches[1]
    $runtimeApi=[IO.File]::ReadAllText((Join-Path $root 'acquisition/sources/tree-sitter--tree-sitter--659cda7c7f86ebe31cc825dc5da59e9add172dc7/lib/include/tree_sitter/api.h'))
    if($runtimeApi -cnotmatch '(?m)^#define TREE_SITTER_LANGUAGE_VERSION ([0-9]+)\s*$'){throw 'Runtime maximum ABI unavailable'};$maxAbi=[int]$Matches[1]
    if($runtimeApi -cnotmatch '(?m)^#define TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION ([0-9]+)\s*$'){throw 'Runtime minimum ABI unavailable'};$minAbi=[int]$Matches[1]
    if($languageAbi -lt $minAbi -or $languageAbi -gt $maxAbi){throw 'Parser ABI outside fixed runtime range'}
    Record ($label+'-inputs') @{before_build=$true;files=$closure;parser_abi=$languageAbi;runtime_abi_min=$minAbi;runtime_abi_max=$maxAbi;runtime_commit=$inputs.runtime.commit;compiler_image=$toolchain.image;scanner=$scanner;source_or_candidate_identity_preserved=$true}
    }
    if(-not $precheck.passed){return}
    $arguments=@('-std=c11','-D_DEFAULT_SOURCE','-O0','-Wall','-Wextra','-H',('-DLANGUAGE=tree_sitter_'+$grammarName),('-I'+$runtime+'/lib/include'),('-I'+$runtime+'/lib/src'),('-I'+$parser),'/inputs/probe.c',($runtime+'/lib/src/lib.c'),($parser+'/parser.c'))
    if($scanner){$arguments+=$scanner}
    $arguments+=@('-o','/work/probe')
    $build=Native build $label (@('/bin/sh','-ec',$compileAndInspect,'p05-remedy-gcc')+$arguments) 120 134217728
    if($build.termination -cne 'EXITED' -or $build.exit_code -ne 0){foreach($row in $script:remedyRows|Where-Object producer -CEQ $producer){$row.dependency='BUILD_FAILED';$row['blocked_by']=@{command=$label;termination=$build.termination;exit_code=$build.exit_code}};return}
    $precheck=TryRemedyPrecheck $producer 'executable' {
        foreach($file in @(@{name='probe';kind='recovered executable'},@{name='probe.elf';kind='ELF report'})){
            $relative='results/'+$label+'/'+$file.name
            if(-not (Test-Path -LiteralPath (Join-Path $root $relative) -PathType Leaf)){throw ('Required '+$file.kind+' unavailable: '+$relative)}
        }
        CheckBuildHeaders $label $parserRoot;RecoveredExecutable $label
    }
    if(-not $precheck.passed){return};$binary=$precheck.value
    foreach($row in $script:remedyRows|Where-Object producer -CEQ $producer){
        $case=$row.case;$identity=FileIdentity (Join-Path $root ('cases/'+$case.id))
        AssertRemedyObject $identity.bytes $identity.sha256 @{bytes=$case.input_bytes;sha256=$case.input_sha256}
        $arguments=@('/bin/sh','-ec',$execCheck,'p05-remedy-exec',$binary.sha256,('/inputs/results/'+$label+'/probe'),('/inputs/cases/'+$case.id))
        if($case.edit){$arguments+=@([string]$case.edit.start_byte,[string]$case.edit.old_end_byte)}
        $command=Native execution $row.command_label $arguments 10 8388608
        $row.state=$command.termination;$row.dependency='BUILT_EXE_MODE_HASH_ELF_RECHECKED';$row.exit_code=$command.exit_code;$row.raw_stdout='raw/'+$row.command_label+'.stdout'
        $row['binary']=$binary;$row['structure_assessment']='REVIEW_REQUIRED_ORIGINAL_EXPECTATIONS_UNCHANGED'
    }
}
function InvokeRemedyGeneration([string]$producer,[string]$grammarPath,[string]$sourceRoot,[switch]$Sql,[switch]$Postgres,[switch]$Json,[switch]$MemoryEvents){
    $label='generate-'+$producer
    $localGrammar=Join-Path $root $grammarPath
    $precheck=TryRemedyPrecheck $producer 'generation' {if($Json){CheckRemedyJson $localGrammar}elseif($Postgres){@(FileIdentity $localGrammar)}elseif($Sql){CheckSqlJsInputs $localGrammar $sourceRoot}else{CheckJsInputs $localGrammar $sourceRoot}}
    if(-not $precheck.passed){return $null};$files=$precheck.value
    if($RemedyStage -ceq 'mssql-patch-r1'){
        $patch=@((ReadRemedySubjects)['followup-r2'].patches|Where-Object id -CEQ 'P05-MSSQL-REMEDY-r1')[0]
        if($files.Count -ne 24 -or $patch.module_closure.Count -ne 24){throw 'MSSQL exact ESM module count mismatch'}
        $imports=0
        foreach($module in $patch.module_closure){
            $path=Join-Path $sourceRoot $module.path;$identity=FileIdentity $path;$changed=@($patch.files|Where-Object target -CEQ $module.path)
            $pin=if($changed.Count){@{bytes=$changed[0].proposed_result_bytes;sha256=$changed[0].proposed_result_sha256}}else{$module}
            AssertRemedyObject $identity.bytes $identity.sha256 $pin
            if(@($files|Where-Object path -CEQ $identity.path).Count -ne 1){throw 'MSSQL ESM closure mismatch'}
            $imports+=[regex]::Matches([IO.File]::ReadAllText($path),'\bimport\b').Count
        }
        $package=FileIdentity (Join-Path $sourceRoot 'package.json')
        if($script:verifiedSource[$package.path] -cne $package.sha256 -or (Get-Content -LiteralPath (Join-Path $sourceRoot 'package.json') -Raw|ConvertFrom-Json).type -cne 'module' -or $imports -ne 42){throw 'MSSQL exact ESM metadata mismatch'}
        Record 'mssql-esm-closure' @{result='PASS';modules=$files;literal_imports=$imports;package_metadata=$package;package_execution=$false;before_G=$true}
    }
    Record ($label+'-inputs') @{files=$files;before_generation=$true;literal_dependencies_verified=$true;arbitrary_javascript_dependency_proof=$false;options=$(if($Postgres){'--disable-optimizations'}elseif($Json){'JSON_ONLY_NO_JAVASCRIPT_RUNTIME'}else{'--js-runtime node'});toolchain_image=$toolchain.image}
    $arguments=@('/inputs/acquisition/tools/tree-sitter','generate','--abi','15','--output','/work/generated')
    if($Postgres){$arguments+='--disable-optimizations'}elseif(-not $Json){$arguments+=@('--js-runtime','node')}
    $arguments+=('/inputs/'+$grammarPath)
    $result=Native generation $label $arguments 300 536870912 -MemoryEvents:($Postgres -or $MemoryEvents)
    if($result.termination -cne 'EXITED' -or $result.exit_code -ne 0 -or $result.resource_healthy -eq $false){foreach($row in $script:remedyRows|Where-Object producer -CEQ $producer){$row.dependency='GENERATION_FAILED';$row['blocked_by']=@{command=$label;termination=$result.termination;exit_code=$result.exit_code;memory_events=$result.memory_event_record}};return $null}
    return 'results/'+$label+'/generated'
}
function InvokeRemedyProducers($subjects){
    if($RemedyStage -cin @('csharp-r3','pg-legacy-r1','mssql-evaluate-r1') -or (FollowupStageKey $RemedyStage)){InvokeExactRemedyProducers $subjects;return}
    if($RemedyStage -cin @('patch-r1','patch-r2')){
        foreach($routeName in @('csharp','typescript','tsx','swift')){
            if($RemedyStage -ceq 'patch-r2' -and $routeName -ceq 'swift'){continue}
            $route=@($inputs.selected_routes|Where-Object route -CEQ $routeName)[0]
            $key=$route.repository.Replace('/','--')+'--'+$route.commit
            $originalRoot='acquisition/sources/'+$key
            $patches=if($RemedyStage -ceq 'patch-r2'){$subjects.r2.patches}else{$subjects.patches.patches}
            $patch=@($patches|Where-Object repository -CEQ $route.repository)[0]
            $candidateRoot='candidates/'+$patch.id
            $selector=if($route.subdirectory -ceq '.') {''}else{'/'+$route.subdirectory}
            $variants=if($RemedyStage -ceq 'patch-r2'){@('candidate-r2')}else{@('original','candidate')}
            foreach($variant in $variants){
                $producer=$routeName+'-'+$variant
                $sourceRoot=if($variant.StartsWith('candidate')){$candidateRoot}else{$originalRoot}
                $name=TryRemedyPrecheck $producer 'name' {ReadGrammarName (Join-Path $root ($sourceRoot+$selector+'/src/grammar.json'))}
                if(-not $name.passed){continue}
                $parser=if($variant.StartsWith('candidate') -or $routeName -ceq 'swift'){InvokeRemedyGeneration $producer ($sourceRoot+$selector+'/grammar.js') (Join-Path $root $sourceRoot)}else{$sourceRoot+$selector+'/src'}
                if($parser){
                    $scanner=if(Test-Path (Join-Path $root ($sourceRoot+$selector+'/src/scanner.c'))){$sourceRoot+$selector+'/src/scanner.c'}else{''}
                    InvokeRemedyBuildAndCases $producer $name.value ('/inputs/'+$parser) (Join-Path $root $parser) $(if($scanner){'/inputs/'+$scanner}else{''}) $scanner
                }
            }
        }
    }else{
        $sqlRoot='candidate-evaluation/derek-sql-97614d0'
        $licenseCheck=TryRemedyPrecheck 'derek-sql-candidate' 'license' {$license=[IO.File]::ReadAllText((Join-Path $root ($sqlRoot+'/LICENSE')));if($license -notmatch 'MIT License' -or $license -notmatch 'Permission is hereby granted'){throw 'SQL candidate license mismatch'}}
        if($licenseCheck.passed){
        Record 'sql-license' @{identity=(FileIdentity (Join-Path $root ($sqlRoot+'/LICENSE')));license='MIT';adoption_authorized=$false}
        $parser=InvokeRemedyGeneration 'derek-sql-candidate' ($sqlRoot+'/grammar.js') (Join-Path $root $sqlRoot) -Sql
        if($parser){$name=TryRemedyPrecheck 'derek-sql-candidate' 'name' {ReadGrammarName (Join-Path $root ($parser+'/grammar.json'))};if($name.passed){InvokeRemedyBuildAndCases 'derek-sql-candidate' $name.value ('/inputs/'+$parser) (Join-Path $root $parser) ('/inputs/'+$sqlRoot+'/src/scanner.c') ($sqlRoot+'/src/scanner.c')}}
        }
        if($RemedyStage -ceq 'sql-only-r2'){return}
        $pgRoot='acquisition/sources/gmr--tree-sitter-postgres--59d0d8cd7506d68de1229fb4bbce838c83b60c8a/postgres'
        $pgParser='materialized-lfs/postgres/src'
        foreach($name in @('parser.h','alloc.h','array.h')){
            $original=Join-Path $root ($pgRoot+'/src/tree_sitter/'+$name)
            if(-not (Test-Path -LiteralPath $original)){continue}
            $target=Join-Path $root ($pgParser+'/tree_sitter/'+$name);[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target));[IO.File]::Copy($original,$target)
            $originalId=FileIdentity $original;$identity=FileIdentity $target
            if($originalId.sha256 -cne $identity.sha256 -or $script:verifiedSource[$originalId.path] -cne $identity.sha256){throw 'LFS companion header changed'};$script:verifiedSource[$identity.path]=$identity.sha256
        }
        $scanner=$pgRoot+'/src/scanner.c';if(-not (Test-Path (Join-Path $root $scanner))){$scanner=''}
        foreach($producer in @('postgresql-lfs-baseline','postgresql-noopt-regenerated')){
            $name=TryRemedyPrecheck $producer 'name' {ReadGrammarName (Join-Path $root ($pgRoot+'/src/grammar.json'))}
            if(-not $name.passed){continue}
            $parser=if($producer -ceq 'postgresql-lfs-baseline'){$pgParser}else{InvokeRemedyGeneration $producer ($pgRoot+'/src/grammar.json') (Join-Path $root $pgRoot) -Postgres}
            if($parser){InvokeRemedyBuildAndCases $producer $name.value ('/inputs/'+$parser) (Join-Path $root $parser) $(if($scanner){'/inputs/'+$scanner}else{''}) $scanner}
        }
    }
}
function InvokeExactRemedyProducers($subjects){
    $pgOriginal='acquisition/sources/gmr--tree-sitter-postgres--59d0d8cd7506d68de1229fb4bbce838c83b60c8a/postgres'
    foreach($producer in @($script:remedyRows.producer|Select-Object -Unique)){
        $json=$false;$memory=$false;$sql=$false
        if($producer -ceq 'csharp-candidate-r3'){$sourceRoot='candidates/P05-CSHARP-REMEDY-r3';$entry=$sourceRoot+'/grammar.js';$parser=$null}
        elseif($producer -ceq 'csharp-candidate-r4'){$sourceRoot='candidates/P05-CSHARP-REMEDY-r4';$entry=$sourceRoot+'/grammar.js';$parser=$null}
        elseif($producer -ceq 'csharp-candidate-r5'){$sourceRoot='candidates/P05-CSHARP-REMEDY-r5';$entry=$sourceRoot+'/grammar.js';$parser=$null}
        elseif($producer -ceq 'postgresql-legacy-candidate-r1'){$sourceRoot='candidates/P05-PG-LEGACY-REMEDY-r1/postgres';$entry=$sourceRoot+'/grammar.js';$parser=$null;$memory=$true}
        elseif($producer -ceq 'postgresql-legacy-candidate-r1-g6'){$sourceRoot='candidates/P05-PG-LEGACY-REMEDY-r1/postgres';$entry=$sourceRoot+'/grammar.js';$parser=$null;$memory=$true}
        elseif($producer -ceq 'mssql-candidate-r1'){$sourceRoot='candidates/P05-MSSQL-REMEDY-r1';$entry=$sourceRoot+'/grammar.js';$parser=$null;$sql=$true}
        elseif($producer -ceq 'postgresql-lfs-baseline-new-regression'){
            $sourceRoot=$pgOriginal;$parser='materialized-lfs/postgres/src'
            foreach($header in @('parser.h','alloc.h','array.h')){
                $original=Join-Path $root ($pgOriginal+'/src/tree_sitter/'+$header)
                if(-not (Test-Path -LiteralPath $original)){continue}
                $target=Join-Path $root ($parser+'/tree_sitter/'+$header);[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target));[IO.File]::Copy($original,$target)
                $id=FileIdentity $original;$copy=FileIdentity $target
                if($script:verifiedSource[$id.path] -cne $copy.sha256){throw 'LFS companion header changed'};$script:verifiedSource[$copy.path]=$copy.sha256
            }
        }else{
            $sourceRoot='candidate-evaluation/mssql-8620fbc';$entry=$sourceRoot+'/src/grammar.json';$json=$true
            $parser=if($producer -ceq 'mssql-baseline-8620fbc'){$sourceRoot+'/src'}else{$null}
            $license=FileIdentity (Join-Path $root ($sourceRoot+'/LICENSE'))
            if($script:verifiedSource[$license.path] -cne $license.sha256 -or [IO.File]::ReadAllText((Join-Path $root $license.path)) -cnotmatch 'MIT License'){throw 'MSSQL license mismatch'}
        }
        $name=TryRemedyPrecheck $producer 'name' {ReadGrammarName (Join-Path $root ($sourceRoot+'/src/grammar.json'))}
        if(-not $name.passed){continue}
        if(-not $parser){$parser=InvokeRemedyGeneration $producer $entry (Join-Path $root $sourceRoot) -Json:$json -Sql:$sql -MemoryEvents:$memory}
        if($parser){
            $scanner=$sourceRoot+'/src/scanner.c';if(-not (Test-Path -LiteralPath (Join-Path $root $scanner))){$scanner=''}
            InvokeRemedyBuildAndCases $producer $name.value ('/inputs/'+$parser) (Join-Path $root $parser) $(if($scanner){'/inputs/'+$scanner}else{''}) $scanner
        }
    }
}
