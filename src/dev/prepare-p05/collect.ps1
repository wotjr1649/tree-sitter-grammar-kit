param([Parameter(Mandatory)][string]$Root,[string]$RemedyStage,[string]$RemedyApprovalSubject)
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion.Major -ne 7){throw 'PowerShell 7 required'}
$script:collectionClock=[Diagnostics.Stopwatch]::StartNew()
function CheckCollectionTime {if($script:collectionClock.Elapsed.TotalSeconds -gt 120){throw 'Total evidence collection time limit'}}
function CollectionHash([string]$path){
    CheckCollectionTime
    $input=[IO.File]::OpenRead($path);$hash=[Security.Cryptography.IncrementalHash]::CreateHash([Security.Cryptography.HashAlgorithmName]::SHA256)
    try{$buffer=[byte[]]::new(65536);while(($n=$input.Read($buffer,0,$buffer.Length)) -gt 0){CheckCollectionTime;$hash.AppendData($buffer,0,$n)};CheckCollectionTime;return [Convert]::ToHexString($hash.GetHashAndReset()).ToLowerInvariant()}finally{$hash.Dispose();$input.Dispose()}
}
function ReadCollectedRecord([string]$task,[string]$relative){
    CheckCollectionTime
    $file=Get-Item -LiteralPath (Join-Path $task $relative) -Force
    if($file.PSIsContainer -or $file.Attributes -band [IO.FileAttributes]::ReparsePoint -or $file.Length -gt 8388608){throw 'Evidence record is not a bounded regular file'}
    $value=Get-Content -LiteralPath $file.FullName -Raw|ConvertFrom-Json -AsHashtable
    CheckCollectionTime;return $value
}
function AssertFollowupEvidenceBinding([string]$task,[string]$stage,[string]$subject,$summary,[string]$inputProjection){
    if(-not (FollowupStageKey $stage) -or $subject -cne 'a68d0717a75b6b769f3ea44eef4591c49bfedfc14f578abfc65657c43bffd1f2' -or $summary.remedy_stage -cne $stage -or $summary.remedy_approval_subject -cne $subject){throw 'Evidence expected stage/subject mismatch'}
    $limits=ExactRemedyLimits $stage
    if($summary.acquisition_limit_bytes -ne $limits.source_image_counter_bytes){throw 'Evidence exact stage counter mismatch'}
    if((CollectionHash $inputProjection) -cne 'f998fb4e73b022cfc7b50196d471a72a0b7bbb4aabce2996be5f1bf596a5a1e4'){throw 'Evidence input projection changed'}
    $subjects=ReadRemedySubjects;$original=Get-Content -LiteralPath $inputProjection -Raw|ConvertFrom-Json -AsHashtable
    $expected=NewRemedyRows $stage $original.cases $subjects.cases.cases
    $ledger=ReadCollectedRecord $task 'records/case-ledger.json'
    if($ledger.remedy_stage -cne $stage -or $ledger.remedy_approval_subject -cne $subject -or $ledger.source_inputs_sha256 -cne 'f998fb4e73b022cfc7b50196d471a72a0b7bbb4aabce2996be5f1bf596a5a1e4' -or $ledger.planned_stage_rows -ne $limits.X -or $ledger.planned_producer_edits -ne $limits.producer_edit -or $ledger.rows.Count -ne $expected.Count -or @($ledger.rows.command_label|Sort-Object -Unique).Count -ne $expected.Count){throw 'Evidence planned rows/edit binding mismatch'}
    for($i=0;$i -lt $expected.Count;$i++){
        CheckCollectionTime
        foreach($key in @('id','route','producer','case','command_label')){if(-not (EqualRemedyData $ledger.rows[$i][$key] $expected[$i][$key])){throw 'Evidence frozen case/input/expectation changed'}}
    }
    $commands=@(foreach($file in Get-ChildItem -LiteralPath (Join-Path $task 'records') -Filter 'command-*.json' -File){ReadCollectedRecord $task ('records/'+$file.Name)})
    if($summary.command_count -ne $commands.Count -or @($commands.label|Sort-Object -Unique).Count -ne $commands.Count -or @($commands|Where-Object {-not $_.host_cleanup_verified}).Count -or $summary.cleanup_errors.Count){throw 'Evidence command/cleanup binding mismatch'}
    $attempts=@{generation=$limits.G;build=$limits.B;execution=$limits.X;preflight=8;diagnostic=16}
    foreach($kind in $attempts.Keys){$actual=$summary.counts[$kind];if($null -eq $actual -or $actual -lt 0 -or [Math]::Truncate($actual) -ne $actual -or $actual -gt $attempts[$kind]){throw 'Evidence native attempt cap mismatch'}}
    foreach($kind in @('generation','build','execution')){if($null -eq $summary.owned_counts[$kind] -or $summary.owned_counts[$kind] -notin @(0,1)){throw 'Evidence owned attempt cap mismatch'}}
    if($null -eq $summary.capture_count -or $summary.capture_count -lt 0 -or $summary.capture_count -gt 169 -or [Math]::Truncate($summary.capture_count) -ne $summary.capture_count){throw 'Evidence capture cap mismatch'}
    $acquisition=ReadCollectedRecord $task 'acquisition/records/acquisition.json'
    if($acquisition.remedy_stage -cne $stage -or $acquisition.remedy_approval_subject -cne $subject -or $acquisition.acquisition_limit_bytes -ne $limits.source_image_counter_bytes -or $null -eq $acquisition.download_bytes -or $acquisition.download_bytes -lt 0 -or [Math]::Truncate($acquisition.download_bytes) -ne $acquisition.download_bytes -or $null -eq $acquisition.http_requests -or $acquisition.http_requests -lt 0 -or $acquisition.http_requests -gt 52 -or [Math]::Truncate($acquisition.http_requests) -ne $acquisition.http_requests){throw 'Evidence acquisition stage/counter mismatch'}
    $completed=Test-Path -LiteralPath (Join-Path $task 'records/download-budget.json')
    $budget=ReadCollectedRecord $task $(if($completed){'records/download-budget.json'}else{'records/download-budget-incomplete.json'})
    $snapshot=if($completed){$budget.final_snapshot}else{$budget}
    if($budget.limit_bytes -ne $limits.source_image_counter_bytes -or $snapshot.limit_bytes -ne $limits.source_image_counter_bytes){throw 'Evidence download counter cap mismatch'}
    $networkState='NOT_VERIFIED'
    if($snapshot.state -ceq 'OBSERVED'){
        foreach($key in @('start_counter_bytes','end_counter_bytes','received_counter_bytes')){if($null -eq $snapshot[$key] -or $snapshot[$key] -lt 0 -or [Math]::Truncate($snapshot[$key]) -ne $snapshot[$key]){throw 'Evidence download counter unknown'}}
        if($snapshot.end_counter_bytes -lt $snapshot.start_counter_bytes -or $snapshot.received_counter_bytes -ne $snapshot.end_counter_bytes-$snapshot.start_counter_bytes){throw 'Evidence download counter regressed or changed'}
        $networkState=if($snapshot.received_counter_bytes -gt $limits.source_image_counter_bytes){'OVER_CAP_REPORTED'}else{'OBSERVED_WITHIN_CAP'}
    }
    if($completed){
        if($networkState -cne 'OBSERVED_WITHIN_CAP' -or $acquisition.state -cne 'COMPLETED' -or $budget.received_network_upper_bound_bytes -ne $snapshot.received_counter_bytes -or $budget.source_http_bytes -ne $acquisition.download_bytes){throw 'Evidence completed acquisition transition mismatch'}
    }elseif($summary.counts.generation+$summary.counts.build+$summary.counts.execution -ne 0){throw 'Evidence native work without completed acquisition'}
    $registered=@($summary.outcomes|Where-Object {$_.scope -cin @('REGISTERED_P05','REGISTERED_P05_REMEDY') -and $_.kind -cin @('generation','build','execution')})
    $observed=@{}
    foreach($kind in @('generation','build','execution')){$observed[$kind]=@($registered|Where-Object kind -CEQ $kind).Count;if($observed[$kind] -gt $summary.counts[$kind]){throw 'Evidence observed native count exceeds attempts'}}
    foreach($outcome in $registered){
        $command=@($commands|Where-Object label -CEQ $outcome.label)
        if($command.Count -ne 1 -or $command[0].termination -cne $outcome.termination -or $command[0].exit_code -ne $outcome.exit_code -or $outcome.memory_limit_bytes -ne (RemedyMemoryBytes $stage $outcome.kind $outcome.label $false)){throw 'Evidence native outcome/command/memory mismatch'}
        $seconds=switch($outcome.kind){'generation'{300};'build'{120};'execution'{10}}
        if($command[0].seconds_limit -ne $seconds -or $command[0].output_limit -ne 8388608 -or $command[0].stored_stdout_bytes+$command[0].stored_stderr_bytes -gt 8388608){throw 'Evidence native command resource envelope mismatch'}
        foreach($stream in @('stdout','stderr')){if((Get-Item -LiteralPath (Join-Path $task ('raw/'+$outcome.label+'.'+$stream))).Length -ne $command[0]['stored_'+$stream+'_bytes'] -or (CollectionHash (Join-Path $task ('raw/'+$outcome.label+'.'+$stream))) -cne $command[0][$stream+'_sha256']){throw 'Evidence native output changed'}}
        if($outcome.kind -ceq 'execution'){
            $row=@($ledger.rows|Where-Object command_label -CEQ $outcome.label)
            if($row.Count -ne 1 -or $row[0].state -cne $outcome.termination -or $row[0].exit_code -ne $outcome.exit_code -or $row[0].raw_stdout -cne ('raw/'+$outcome.label+'.stdout')){throw 'Evidence executed row binding mismatch'}
        }elseif($outcome.label -cne ($(if($outcome.kind -ceq 'generation'){'generate-'}else{'build-'})+$expected[0].producer)){throw 'Evidence producer label mismatch'}
        if($stage -ceq 'pg-legacy-g6-r1' -and $outcome.kind -ceq 'generation'){
            $events=ReadCollectedRecord $task ('records/'+$outcome.label+'-memory-events.json')
            $texts=@(foreach($when in @('before','after')){
                $relative='raw/'+$outcome.label+'-memory-'+$when+'.stdout';$pin=$events[$when+'_raw'];$receipt=@($commands|Where-Object label -CEQ ($outcome.label+'-memory-'+$when))
                if($receipt.Count -ne 1 -or $receipt[0].termination -cne 'EXITED' -or $receipt[0].exit_code -ne 0 -or -not (EqualRemedyData $receipt[0].argv @('exec',$events.container,'/bin/cat','/sys/fs/cgroup/memory.events')) -or $pin.path -cne $relative -or (Get-Item -LiteralPath (Join-Path $task $relative)).Length -ne $pin.bytes -or (CollectionHash (Join-Path $task $relative)) -cne $pin.sha256){throw 'Evidence memory event raw/container mismatch'}
                [IO.File]::ReadAllText((Join-Path $task $relative))
            })
            $delta=MemoryEventDelta $texts[0] $texts[1]
            if($events.native_command -cne $outcome.label -or $events.container -cne $command[0].argv[1] -or $events.memory_limit_bytes -ne 6442450944 -or $events.original_exit_code -ne $outcome.exit_code -or $events.original_termination -cne $outcome.termination -or -not (EqualRemedyData $delta $events.counters) -or $events.state -cne $delta.state -or $outcome.resource_state -cne $delta.state -or $outcome.memory_event_record -cne ('records/'+$outcome.label+'-memory-events.json')){throw 'Evidence memory event delta binding mismatch'}
            $capacity=ReadCollectedRecord $task 'records/pg-generation-capacity.json';AssertGenerationCapacity $capacity.MemAvailable_bytes $capacity.resolved_parents
            $limit=ReadCollectedRecord $task ('records/'+$outcome.label+'-memory-limit.json')
            $inspect=(Get-Content -LiteralPath (Join-Path $task ('raw/'+$outcome.label+'-inspect.stdout')) -Raw|ConvertFrom-Json)[0]
            foreach($suffix in @('inspect','memory-max')){
                $receipt=@($commands|Where-Object label -CEQ ($outcome.label+'-'+$suffix));$relative='raw/'+$outcome.label+'-'+$suffix+'.stdout'
                $argv=if($suffix -ceq 'inspect'){@('inspect',$events.container)}else{@('exec',$events.container,'/bin/cat','/sys/fs/cgroup/memory.max')}
                if($receipt.Count -ne 1 -or $receipt[0].termination -cne 'EXITED' -or $receipt[0].exit_code -ne 0 -or -not (EqualRemedyData $receipt[0].argv $argv) -or (Get-Item -LiteralPath (Join-Path $task $relative)).Length -ne $receipt[0].stored_stdout_bytes -or (CollectionHash (Join-Path $task $relative)) -cne $receipt[0].stdout_sha256){throw 'Evidence PG inspect/cgroup command mismatch'}
            }
            AssertMemoryEnvelope 6442450944 $inspect.HostConfig.Memory $inspect.HostConfig.MemorySwap ([IO.File]::ReadAllText((Join-Path $task ('raw/'+$outcome.label+'-memory-max.stdout'))))
            if($capacity.result -cne 'PASS' -or -not $capacity.before_G -or $capacity.generation_memory_bytes -ne 6442450944 -or $inspect.Id -cne $events.container -or $limit.container -cne $events.container -or $limit.bytes -ne 6442450944 -or $limit.inspect_and_cgroup -cne 'MATCH' -or ($delta.state -ceq 'OOM_OBSERVED' -and ($outcome.result_directory -or $summary.counts.build -gt 0 -or $summary.counts.execution -gt 0))){throw 'Evidence PG capacity/resource outcome mismatch'}
        }
    }
    if(@($ledger.rows|Where-Object {$_.state -cne 'NOT_RUN'}).Count -ne $observed.execution){throw 'Evidence executed/NOT_RUN rows mismatch'}
    if($summary.verdict -ceq 'BOUNDED_INPUTS_COMPLETED_REVIEW_REQUIRED'){
        foreach($kind in @('generation','build','execution')){if($summary.counts[$kind] -ne $attempts[$kind] -or $observed[$kind] -ne $attempts[$kind]){throw 'Evidence completion promoted partial attempts'}}
        if($summary.failure_type -or @($registered|Where-Object {$_.termination -cne 'EXITED' -or $_.exit_code -ne 0 -or $_.resource_state -ceq 'OOM_OBSERVED'}).Count){throw 'Evidence completion promoted native failure'}
    }
    return @{result='MATCH';stage=$stage;subject=$subject;planned_rows=$limits.X;planned_producer_edits=$limits.producer_edit;native_attempts=$summary.counts;observed_captured_native=$observed;network_state=$networkState;support_assessment='REVIEW_REQUIRED_OR_NOT_RUN';product_qualification=$false}
}
$task=[IO.Path]::GetFullPath($Root)
$prefix=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('/')+'/'
if(-not $task.StartsWith($prefix,[StringComparison]::Ordinal) -or $task.Substring($prefix.Length) -notmatch '^tsgk-p05-(?:remedy-)?[0-9]+-[0-9]+$'){throw 'Unexpected evidence root'}
if(-not (Test-Path -LiteralPath $task)){throw 'Task evidence missing'}
$exactLimits=$null;$summary=$null
. (Join-Path $PSScriptRoot 'remedy.ps1')
$summaryPath=Join-Path $task 'records/summary.json'
if($RemedyStage){$exactLimits=ExactRemedyLimits $RemedyStage}
if(Test-Path -LiteralPath $summaryPath){
    $summary=ReadCollectedRecord $task 'records/summary.json'
    if(-not $RemedyStage){$exactLimits=ExactRemedyLimits $summary.remedy_stage}
}
$bindingFailure=$false
if((FollowupStageKey $RemedyStage) -or ($summary -and (FollowupStageKey $summary.remedy_stage))){
    try{$binding=AssertFollowupEvidenceBinding $task $RemedyStage $RemedyApprovalSubject $summary (Join-Path $PSScriptRoot 'inputs.json')}catch{$bindingFailure=$true;$binding=@{result='NOT_VERIFIED';stage=$RemedyStage;subject=$RemedyApprovalSubject;reason=$_.Exception.Message;raw_failed_evidence_preserved=$true;support_assessment='NOT_VERIFIED'}}
    $binding|ConvertTo-Json -Depth 12|Set-Content -LiteralPath (Join-Path $task 'records/evidence-binding.json') -Encoding utf8NoBOM
}
function GeneratedArtifactProvenance([string]$task,$outcome,[long]$bytes,[string]$sha256){
    $label=$outcome.label
    if($label -cnotmatch '^generate-[a-z0-9-]+$' -or $outcome.result_directory -cne $label){throw 'Generated artifact label mismatch'}
    $refs=@{}
    foreach($relative in @(('records/command-'+$label+'.json'),('records/'+$label+'-inputs.json'),('records/command-'+$label+'-copy.json'),('raw/'+$label+'.stdout'),('raw/'+$label+'.stderr'),('raw/'+$label+'-copy.stdout'),('raw/'+$label+'-copy.stderr'))){
        $file=Get-Item -LiteralPath (Join-Path $task $relative) -Force
        if($file.PSIsContainer -or $file.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Generated provenance is not a regular file'}
        $refs[$relative]=@{path=$relative;bytes=$file.Length;sha256=(CollectionHash $file.FullName)}
    }
    foreach($suffix in @('','-copy')){
        $command=Get-Content -LiteralPath (Join-Path $task ('records/command-'+$label+$suffix+'.json')) -Raw|ConvertFrom-Json
        if($command.label -cne $label+$suffix -or $command.termination -cne 'EXITED' -or $command.exit_code -ne 0 -or -not $command.host_cleanup_verified){throw 'Generated provenance command did not complete'}
        foreach($stream in @('stdout','stderr')){
            $identity=$refs['raw/'+$label+$suffix+'.'+$stream]
            if($identity.bytes -ne $command.('stored_'+$stream+'_bytes') -or $identity.sha256 -cne $command.($stream+'_sha256')){throw 'Generated command/capture bytes changed'}
        }
    }
    $inputs=Get-Content -LiteralPath (Join-Path $task ('records/'+$label+'-inputs.json')) -Raw|ConvertFrom-Json
    if(-not $inputs.before_generation -or -not $inputs.files.Count){throw 'Generated input provenance missing'}
    $buildRelative='records/build-'+$label.Substring('generate-'.Length)+'-inputs.json'
    $buildState='NOT_RUN_INPUT_RECORD_ABSENT'
    if(Test-Path -LiteralPath (Join-Path $task $buildRelative)){
        $file=Get-Item -LiteralPath (Join-Path $task $buildRelative) -Force
        if($file.PSIsContainer -or $file.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Generated build provenance is not a regular file'}
        $build=Get-Content -LiteralPath $file.FullName -Raw|ConvertFrom-Json
        $parser=@($build.files|Where-Object path -CEQ ('results/'+$label+'/generated/parser.c'))
        if(-not $build.before_build -or $parser.Count -ne 1 -or $parser[0].bytes -ne $bytes -or $parser[0].sha256 -cne $sha256){throw 'Generated C differs from build input'}
        $refs[$buildRelative]=@{path=$buildRelative;bytes=$file.Length;sha256=(CollectionHash $file.FullName)}
        $buildState='OBSERVED_BEFORE_BUILD_NOT_BUILD_SUCCESS'
    }
    return @{references=@($refs.Values|Sort-Object path);build_inputs_state=$buildState;capture_original_retained=$true}
}
$files=@(foreach($directory in @('raw','records','cases','owned','acquisition/archives','acquisition/records','acquisition/sources/Crary-Systems--tree-sitter-tsql--443d2bc774f1d779af7dcabcc99160fb24da96e6')){
    $path=Join-Path $task $directory
    if(Test-Path -LiteralPath $path){Get-ChildItem -LiteralPath $path -File -Recurse|Where-Object {-not $_.Name.EndsWith('.tar')} }
})
$files+=Get-Item -LiteralPath (Join-Path $PSScriptRoot 'inputs.json'),(Join-Path $PSScriptRoot 'case-review.json'),(Join-Path $PSScriptRoot 'probe.c.in')
if($task.Substring($prefix.Length).StartsWith('tsgk-p05-remedy-')){
    $files+=Get-Item -LiteralPath (Join-Path $PSScriptRoot 'remedy-patches.json'),(Join-Path $PSScriptRoot 'remedy-cases.json'),(Join-Path $PSScriptRoot 'remedy-fact-oracles.json'),(Join-Path $PSScriptRoot 'remedy-sources.json'),(Join-Path $PSScriptRoot 'remedy-r2.json'),(Join-Path $PSScriptRoot 'remedy-exact-r1.json'),(Join-Path $PSScriptRoot 'remedy-followup-r2.json')
    foreach($directory in @('candidates','candidate-evaluation','materialized-lfs')){
        $path=Join-Path $task $directory
        if(Test-Path -LiteralPath $path){$files+=Get-ChildItem -LiteralPath $path -File -Recurse}
    }
}
if($exactLimits){
    # Preserve all original capture bytes and export a separately recoverable C object.
    # Gzip is lossless storage; the uncompressed identity remains the producer identity.
    $exports=@()
    foreach($outcome in $summary.outcomes|Where-Object {-not $bindingFailure -and $_.kind -ceq 'generation' -and $_.scope -ceq 'REGISTERED_P05' -and $_.termination -ceq 'EXITED' -and $_.exit_code -eq 0 -and $_.resource_state -cne 'OOM_OBSERVED'}){
        $source=Join-Path $task ('results/'+$outcome.result_directory+'/generated/parser.c')
        $file=Get-Item -LiteralPath $source -Force
        if($file.Attributes -band [IO.FileAttributes]::ReparsePoint -or $file.Length -gt 536870912){throw 'Generated C export rejected'}
        $digest=CollectionHash $source
        $provenance=GeneratedArtifactProvenance $task $outcome $file.Length $digest
        $target=Join-Path $task ('generated-artifacts/'+$outcome.label+'/parser.c.gz')
        [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
        $sink=[IO.File]::Open($target,[IO.FileMode]::CreateNew);$gzip=[IO.Compression.GZipStream]::new($sink,[IO.Compression.CompressionLevel]::Optimal);$input=[IO.File]::OpenRead($source)
        try{
            $buffer=[byte[]]::new(65536);$count=0L
            while(($n=$input.Read($buffer,0,$buffer.Length)) -gt 0){CheckCollectionTime;$count+=$n;if($count -gt 536870912 -or $sink.Position+$n+65536 -gt $exactLimits.local_packed_max_bytes){throw 'Generated C export limit'};$gzip.Write($buffer,0,$n)}
        }finally{$input.Dispose();$gzip.Dispose();$sink.Dispose()}
        if($count -ne $file.Length -or (CollectionHash $source) -cne $digest){throw 'Generated C changed during export'}
        $exports+=@{producer=$outcome.label;source_path=[IO.Path]::GetRelativePath($task,$source);original_bytes=$file.Length;original_sha256=$digest;lossless_gzip_path=[IO.Path]::GetRelativePath($task,$target);provenance=$provenance;capture_original_retained=$true;identity_is_uncompressed_bytes=$true}
        $files+=Get-Item -LiteralPath $target
    }
    $exportRecord=Join-Path $task 'records/generated-artifacts.json'
    $stream=[IO.File]::Open($exportRecord,[IO.FileMode]::CreateNew)
    try{$bytes=[Text.Encoding]::UTF8.GetBytes((ConvertTo-Json -InputObject $exports -Depth 10));$stream.Write($bytes)}finally{$stream.Dispose()}
    $files+=Get-Item -LiteralPath $exportRecord
}
if($files.Count -gt 10000){throw 'Evidence file count limit'}
$manifest=@(foreach($file in $files){
    CheckCollectionTime
    if($file.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Evidence link rejected'}
    $relative=if($file.FullName.StartsWith($task+'/')){[IO.Path]::GetRelativePath($task,$file.FullName)}else{'harness/'+$file.Name}
    if($relative -notmatch '^[a-zA-Z0-9_.\-/]+$' -or $relative.Length -gt 200){throw 'Evidence path rejected'}
    @{path=$relative;bytes=$file.Length;sha256=(CollectionHash $file.FullName);source=$file.FullName}
})
if($exactLimits -and ($manifest|Measure-Object bytes -Sum).Sum+8388608 -gt $exactLimits.local_expanded_max_bytes){throw 'Exact stage expanded evidence reserve exceeded; preserve source without truncation'}
$manifest|Select-Object path,bytes,sha256|ConvertTo-Json -Depth 4|Set-Content -LiteralPath (Join-Path $task 'evidence-manifest.json') -Encoding utf8NoBOM
$manifest+=@{path='evidence-manifest.json';source=(Join-Path $task 'evidence-manifest.json');bytes=(Get-Item (Join-Path $task 'evidence-manifest.json')).Length}
$partial=Join-Path $task 'evidence.partial.zip'
$complete=Join-Path $task 'evidence.zip'
if(Test-Path -LiteralPath $complete){throw 'Completed evidence already exists'}
$output=[IO.File]::Open($partial,[IO.FileMode]::CreateNew)
$zip=[IO.Compression.ZipArchive]::new($output,[IO.Compression.ZipArchiveMode]::Create,$true)
try {
    foreach($item in $manifest){
        CheckCollectionTime
        $entry=$zip.CreateEntry($item.path,[IO.Compression.CompressionLevel]::Optimal);$sink=$entry.Open();$source=[IO.File]::OpenRead($item.source)
        try {
            $buffer=[byte[]]::new(65536)
            while(($n=$source.Read($buffer,0,$buffer.Length)) -gt 0){
                CheckCollectionTime
                # Reserve 8 MiB for ZIP metadata and bounded compressor buffering.
                $packageLimit=if($exactLimits){$exactLimits.local_packed_max_bytes}else{268435456L}
                if($output.Position+$n+65536 -gt $packageLimit-8388608){throw 'Evidence package budget exceeded'}
                $sink.Write($buffer,0,$n)
            }
        } finally {$source.Dispose();$sink.Dispose()}
    }
} finally {$zip.Dispose();$output.Dispose()}
$package=Get-Item -LiteralPath $partial
if($package.Length -gt $(if($exactLimits){$exactLimits.local_packed_max_bytes}else{268435456L})){throw 'Evidence package size limit'}
$digest=CollectionHash $partial
[IO.File]::Move($partial,$complete)
$package=Get-Item -LiteralPath $complete
CheckCollectionTime
@{bytes=$package.Length;sha256=$digest;files=$manifest.Count;retention_days=7;total_collection_wall_seconds=$script:collectionClock.Elapsed.TotalSeconds;seconds_limit=120;evidence_binding=$(if($bindingFailure){'NOT_VERIFIED_FAILED_BYTES_RETAINED'}elseif($binding){$binding.result}else{'LEGACY_SCOPE'})}|ConvertTo-Json -Compress
if($bindingFailure){throw 'Evidence binding failed; bounded original artifact retained'}
