param([Parameter(Mandatory)][string]$Destination,[ValidateSet('archive-r1','pinned-tsql-r1')][string]$AcquisitionProfile='archive-r1',[string]$AcquisitionApprovalSubject,[string]$ExecutionApprovalSubject,[string]$ToolchainProfile='bookworm-r1',[string]$ToolchainApprovalSubject)
$ErrorActionPreference='Stop'
$toolchain=& (Join-Path $PSScriptRoot 'approval.ps1') -Profile $AcquisitionProfile -Subject $AcquisitionApprovalSubject -Execution -ExecutionSubject $ExecutionApprovalSubject -ImageProfile $ToolchainProfile -ImageSubject $ToolchainApprovalSubject
if(-not $toolchain){throw 'Explicit toolchain profile required'}
if(-not $IsLinux -or $PSVersionTable.PSVersion.Major -ne 7){throw 'P05 requires hosted Linux and PowerShell 7'}
$root=[IO.Path]::GetFullPath($Destination)
$runnerRoot=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('/')+'/'
if(-not $root.StartsWith($runnerRoot,[StringComparison]::Ordinal) -or $root.Substring($runnerRoot.Length) -notmatch '^tsgk-p05-[0-9]+-[0-9]+$' -or (Test-Path -LiteralPath $root)){throw 'Fresh runner task root required'}
$ancestor=Get-Item -LiteralPath $runnerRoot -Force
while($ancestor){if($ancestor.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Reparse runner ancestor'};$ancestor=$ancestor.Parent}
[void][IO.Directory]::CreateDirectory($root)
foreach($d in @('raw','records','cases','results','home')){[void][IO.Directory]::CreateDirectory((Join-Path $root $d))}
$inputsPath=Join-Path $PSScriptRoot 'inputs.json'
if((Get-FileHash $inputsPath).Hash.ToLowerInvariant() -cne 'f998fb4e73b022cfc7b50196d471a72a0b7bbb4aabce2996be5f1bf596a5a1e4'){throw 'Approved input projection changed'}
$inputs=Get-Content -Raw $inputsPath|ConvertFrom-Json
$reviewPath=Join-Path $PSScriptRoot 'case-review.json'
if((Get-FileHash -LiteralPath $reviewPath).Hash.ToLowerInvariant() -cne 'f3ad556cb3a525f23a3bddc927902e88acad08ca6c6d1b84559b329baf751554'){throw 'Independent case review changed'}
$caseReview=Get-Content -Raw -LiteralPath $reviewPath|ConvertFrom-Json
if($caseReview.inputs_sha256 -cne (Get-FileHash $inputsPath).Hash.ToLowerInvariant() -or $caseReview.open_blocker_material -ne 0 -or $caseReview.case_count -ne 57 -or $inputs.cases.Count -ne 57 -or @($inputs.cases.id|Sort-Object -Unique).Count -ne 57 -or $caseReview.expected_facts_review -cne 'PRIMARY_SYNTAX_FEATURE_FACTS_AND_DAMAGE_EXPECTATIONS_REVIEWED'){throw 'Case/spec review gate mismatch'}
$routeGroups=@($inputs.cases|Group-Object route)
if($routeGroups.Count -ne 6 -or @($caseReview.route_counts.PSObject.Properties).Count -ne 6){throw 'Reviewed route count mismatch'}
foreach($group in $routeGroups){if($caseReview.route_counts.($group.Name) -ne $group.Count){throw 'Reviewed route case count mismatch'}}
$editIds=@($inputs.cases|Where-Object edit|ForEach-Object id)
if($editIds.Count -ne 6 -or (Compare-Object $editIds @($caseReview.edit_ids))){throw 'Reviewed edit set changed'}
foreach($case in $inputs.cases){if(-not $case.feature_ids.Count -or -not $case.expected.facts -or ($case.edit -and -not $case.edit.negative_expected)){throw 'Missing reviewed feature/fact/negative expectation'}}
function Application([string]$name){
    $command=Get-Command -Name $name -CommandType Application -ErrorAction Stop|Select-Object -First 1
    if(-not $command.Source -or -not (Test-Path -LiteralPath $command.Source -PathType Leaf)){throw 'Application path unavailable'}
    return [string]$command.Source
}
$docker=Application docker
$pwsh=Application pwsh
$script:commands=[Collections.Generic.List[object]]::new()
$script:outcomes=[Collections.Generic.List[object]]::new()
$script:containers=[Collections.Generic.List[string]]::new()
$script:counts=@{generation=0;build=0;execution=0;preflight=0;diagnostic=0}
$script:failed=$false
$script:wall=[Diagnostics.Stopwatch]::StartNew()
$script:imageSize=2147483648L
$script:acquiring=$true
$script:captureToolVerified=$false
function NetworkReceived { return [long]((Get-ChildItem -Path '/sys/class/net/*/statistics/rx_bytes' | ForEach-Object {[long][IO.File]::ReadAllText($_.FullName)})|Measure-Object -Sum).Sum }
$script:networkStart=NetworkReceived
function Record([string]$name,$value){$path=Join-Path $root ('records/'+$name+'.json');if(Test-Path $path){throw 'Record exists'};$value|ConvertTo-Json -Depth 40|Set-Content -LiteralPath $path -Encoding utf8NoBOM}
function Run([string]$label,[string[]]$argv,[int]$seconds,[long]$limit=8388608,[string]$executable=$docker,[switch]$Cleanup){
    $deadline=if($Cleanup){4560}else{4500}
    if($label -notmatch '^[a-z0-9-]+$' -or $script:wall.Elapsed.TotalSeconds+$seconds -gt $deadline){throw 'Operation label/job time limit'}
    $toolDigest=(Get-FileHash -LiteralPath $executable).Hash.ToLowerInvariant()
    $stored=(Get-ChildItem -LiteralPath $root -File -Recurse|Measure-Object Length -Sum).Sum
    if($stored+$script:imageSize+2*$limit+536870912 -gt 8589934592){throw 'Runner storage reserve exceeded'}
    $paths=@((Join-Path $root "raw/$label.stdout"),(Join-Path $root "raw/$label.stderr"))
    if(@($paths|Where-Object {Test-Path -LiteralPath $_}).Count){throw 'Operation already recorded'}
    $files=@([IO.File]::Open($paths[0],[IO.FileMode]::CreateNew),[IO.File]::Open($paths[1],[IO.FileMode]::CreateNew))
    $start=[Diagnostics.ProcessStartInfo]::new();$start.FileName=$executable;$start.WorkingDirectory=$root;$start.UseShellExecute=$false;$start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    foreach($arg in $argv){$start.ArgumentList.Add($arg)}
    $start.Environment.Clear();$start.Environment['PATH']='/usr/local/bin:/usr/bin:/bin';$start.Environment['HOME']=(Join-Path $root 'home');$start.Environment['TMPDIR']=$root;$start.Environment['RUNNER_TEMP']=$runnerRoot
    $process=[Diagnostics.Process]::new();$process.StartInfo=$start
    $timer=[Diagnostics.Stopwatch]::StartNew();$reason='EXITED';$total=0L;$exitCode=$null;$started=$false;$cleanupVerified=$true
    try {
        $started=$process.Start();if(-not $started){throw 'Process start failed'}
        $streams=@($process.StandardOutput.BaseStream,$process.StandardError.BaseStream);$buffers=@([byte[]]::new(65536),[byte[]]::new(65536))
        $pending=@($streams[0].ReadAsync($buffers[0],0,65536),$streams[1].ReadAsync($buffers[1],0,65536))
        while(-not $process.HasExited -or $pending[0] -or $pending[1]){
            if($timer.Elapsed.TotalSeconds -gt $seconds){$reason='TIMEOUT';break}
            if($script:acquiring -and (NetworkReceived)-$script:networkStart -gt 1073741824){$reason='DOWNLOAD_LIMIT';break}
            for($i=0;$i -lt 2;$i++){
                if($pending[$i] -and $pending[$i].IsCompleted){
                    $n=$pending[$i].GetAwaiter().GetResult();$total+=$n
                    if($total -gt $limit){$reason='OUTPUT_LIMIT';break}
                    if($n){$files[$i].Write($buffers[$i],0,$n);$pending[$i]=$streams[$i].ReadAsync($buffers[$i],0,65536)}else{$pending[$i]=$null}
                }
            }
            if($reason -ne 'EXITED'){break}
            if(-not $process.HasExited){[void]$process.WaitForExit(20)}else{[Threading.Thread]::Sleep(20)}
        }
        if(-not $process.HasExited){$process.Kill($true);if(-not $process.WaitForExit(2000)){throw 'Host command cleanup unverified'}}
        $exitCode=$process.ExitCode
    } catch {$reason='SUPERVISOR_ERROR';throw} finally {
        try {if($started -and -not $process.HasExited){$process.Kill($true);$cleanupVerified=$process.WaitForExit(2000)}}catch{$cleanupVerified=$false}
        if(-not $cleanupVerified){$reason='UNKNOWN_CLEANUP'}
        foreach($file in $files){$file.Dispose()};$timer.Stop();$process.Dispose()
        $storedOut=(Get-Item -LiteralPath $paths[0]).Length;$storedErr=(Get-Item -LiteralPath $paths[1]).Length
        $receipt=@{label=$label;tool=$executable;tool_sha256=$toolDigest;argv=$argv;seconds_limit=$seconds;wall_seconds=$timer.Elapsed.TotalSeconds;output_limit=$limit;observed_bytes=$total;stored_stdout_bytes=$storedOut;stored_stderr_bytes=$storedErr;unstored_observed_bytes=($total-$storedOut-$storedErr);partial_output=($reason -ne 'EXITED');host_cleanup_verified=$cleanupVerified;termination=$reason;exit_code=$exitCode;stdout_sha256=(Get-FileHash $paths[0]).Hash.ToLowerInvariant();stderr_sha256=(Get-FileHash $paths[1]).Hash.ToLowerInvariant()}
        $script:commands.Add($receipt);Record ('command-'+$label) $receipt
    }
    if(-not $cleanupVerified){throw 'Host command cleanup unverified; receipt retained'}
    return $receipt
}
function Require($result){if($result.exit_code -ne 0 -or $result.termination -ne 'EXITED'){throw ('Command failed: '+$result.label)}}
function TextOutput($result){return [IO.File]::ReadAllText((Join-Path $root ('raw/'+$result.label+'.stdout')))}
function FileIdentity([string]$path){
    $file=Get-Item -LiteralPath $path -Force
    if($file.PSIsContainer -or $file.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Input is not a regular file'}
    return @{path=[IO.Path]::GetRelativePath($root,$file.FullName);bytes=$file.Length;sha256=(Get-FileHash -LiteralPath $file.FullName).Hash.ToLowerInvariant()}
}
function CheckJsInputs([string]$entry,[string]$sourceRoot){
    $queue=[Collections.Generic.Queue[string]]::new();$queue.Enqueue($entry)
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $files=[Collections.Generic.List[object]]::new()
    while($queue.Count){
        $path=[IO.Path]::GetFullPath($queue.Dequeue());if(-not $seen.Add($path)){continue}
        if(-not $path.StartsWith($sourceRoot+'/',[StringComparison]::Ordinal) -and -not $path.StartsWith((Join-Path $root 'npm')+'/',[StringComparison]::Ordinal)){throw 'JS dependency outside pinned inputs'}
        $files.Add((FileIdentity $path));$source=[IO.File]::ReadAllText($path)
        if($source -match '\b(?:createRequire|eval|Function)\s*\(' -or $source -match '\b(?:const|let|var)\s+\w+\s*=\s*require\b(?!\s*\()'){throw 'Unreviewed JS loader/evaluation'}
        $imports=[regex]::Matches($source,'\brequire\s*\(\s*["'']([^"'']+)["'']\s*\)')
        if([regex]::Matches($source,'\brequire\s*\(').Count -ne $imports.Count -or $source -match '\bimport\s*(?:\(|["''])'){throw 'Unresolved dynamic/ES module dependency'}
        foreach($match in $imports){
            $name=$match.Groups[1].Value
            if($name.StartsWith('.')){$next=Join-Path ([IO.Path]::GetDirectoryName($path)) $name}
            elseif($name -ceq 'tree-sitter-javascript/grammar'){$next=Join-Path $root 'npm/tree-sitter-javascript/grammar.js'}
            else{throw 'Unregistered JS dependency'}
            if(-not [IO.Path]::GetExtension($next)){$next+='.js'}
            $queue.Enqueue($next)
        }
    }
    return ,$files.ToArray()
}
function StopContainer([string]$id,[string]$label){
    $inspect=Run ($label+'-cleanup-state') @('inspect','--format','{{.State.Running}} {{.State.Paused}}',$id) 2 -Cleanup
    Require $inspect
    $state=(TextOutput $inspect).Trim()
    if($state -eq 'true true'){Require (Run ($label+'-cleanup-unpause') @('unpause',$id) 2 -Cleanup)}
    if($state.StartsWith('true')){Require (Run ($label+'-cleanup-kill') @('kill','--signal','KILL',$id) 2 -Cleanup)}
    $stopped=Run ($label+'-cleanup-stopped') @('inspect','--format','{{.State.Running}} {{.State.Pid}}',$id) 2 -Cleanup;Require $stopped
    if((TextOutput $stopped).Trim() -cne 'false 0'){throw 'Container child cleanup unverified'}
    Require (Run ($label+'-cleanup-remove') @('rm',$id) 2 -Cleanup)
    [void]$script:containers.Remove($id)
    Record ($label+'-cleanup') @{container=$id;running=$false;host_pid=0;removed=$true}
}
function Container([string]$label){
    $args=@('create','--network','none','--ipc','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--user','65534:65534','--pids-limit','64','--memory','4g','--memory-swap','4g','--cpus','1','--tmpfs','/work:rw,size=2147483648,mode=1777','--env','HOME=/work','--env','TMPDIR=/work','--env','NODE_PATH=/inputs/npm')
    foreach($mount in @(@{host=(Join-Path $root 'acquisition');target='/inputs/acquisition'},@{host=(Join-Path $root 'cases');target='/inputs/cases'},@{host=(Join-Path $root 'probe.c');target='/inputs/probe.c'},@{host=(Join-Path $root 'npm');target='/inputs/npm'},@{host=(Join-Path $root 'results');target='/inputs/results'})){
        $args+=@('--mount',('type=bind,source='+$mount.host+',target='+$mount.target+',readonly'))
    }
    $args+=@('--entrypoint','/bin/sleep',$toolchain.image,'infinity')
    $created=Run ($label+'-create') $args 10;Require $created;$id=(TextOutput $created).Trim()
    if($id -notmatch '^[a-f0-9]{64}$'){throw 'Invalid Docker identity'}
    $script:containers.Add($id);Require (Run ($label+'-start') @('start',$id) 10)
    $config=Run ($label+'-inspect') @('inspect',$id) 10;Require $config
    $observed=(TextOutput $config|ConvertFrom-Json)[0]
    if($observed.HostConfig.NetworkMode -cne 'none' -or $observed.HostConfig.IpcMode -cne 'none' -or $observed.HostConfig.PidMode -eq 'host' -or $observed.HostConfig.Privileged -or -not $observed.HostConfig.ReadonlyRootfs -or $observed.HostConfig.Memory -ne 4294967296 -or $observed.HostConfig.MemorySwap -ne 4294967296 -or $observed.HostConfig.NanoCpus -ne 1000000000 -or $observed.HostConfig.PidsLimit -ne 64 -or 'ALL' -notin $observed.HostConfig.CapDrop -or 'no-new-privileges' -notin $observed.HostConfig.SecurityOpt -or $observed.Config.User -cne '65534:65534' -or @($observed.Mounts|Where-Object {$_.Type -eq 'bind' -and $_.RW}).Count){throw 'Container boundary mismatch'}
    return $id
}
function ConfirmFrozenState([string]$state,[string]$top){
    if($state.Trim() -cnotmatch '^true true ([1-9][0-9]*)$'){throw 'Container is not running and frozen'}
    $expectedProcessId=[long]$Matches[1]
    $lines=@($top -split "`n"|Where-Object {$_.Trim()})
    if($lines.Count -ne 2 -or $lines[0].Trim() -cnotmatch '^PID\s+COMMAND$' -or $lines[1] -cnotmatch '^\s*([1-9][0-9]*)\s+/bin/sleep infinity\s*$'){throw 'Unexpected descendant remains at freeze'}
    if([long]$Matches[1] -ne $expectedProcessId){throw 'Frozen init process identity mismatch'}
    return $expectedProcessId
}
function Freeze([string]$id,[string]$label){
    Require (Run ($label+'-pause') @('pause',$id) 2)
    $state=Run ($label+'-state') @('inspect','--format','{{.State.Running}} {{.State.Paused}} {{.State.Pid}}',$id) 2;Require $state
    $top=Run ($label+'-top') @('top',$id,'-eo','pid,args') 2;Require $top
    return (ConfirmFrozenState (TextOutput $state) (TextOutput $top))
}
function Snapshot([string]$id,[string]$label,[long]$limit){
    $firstProcessId=Freeze $id $label
    Require (Run ($label+'-unpause') @('unpause',$id) 2)
    if(-not $script:captureToolVerified){
        if(++$script:counts.diagnostic -gt 16){throw 'Capture diagnostic budget exceeded'}
        $tool=Run 'capture-tool-identity' @('exec',$id,'/bin/sh','-ec','/usr/bin/tar --version; /usr/bin/sha256sum -- /usr/bin/tar') 10 1048576;Require $tool
        $digestLines=@((TextOutput $tool) -split "`n"|Where-Object {$_ -match '^[0-9a-f]{64}  /usr/bin/tar$'})
        if($digestLines.Count -ne 1){throw 'Capture tool identity missing'}
        Record 'capture-tool' @{image=$toolchain.image;path='/usr/bin/tar';sha256=$digestLines[0].Substring(0,64);identity_command='command-capture-tool-identity.json';capture_method='quiescent-tar-r1';execution_approval_subject=$ExecutionApprovalSubject}
        $script:captureToolVerified=$true
    }
    $copy=Run ($label+'-copy') @('exec','--env','TAR_OPTIONS=',$id,'/usr/bin/tar','--format=ustar','--create','--file=-','--directory=/work','--one-file-system','.') 10 $limit;Require $copy
    if((Freeze $id ($label+'-after-copy')) -ne $firstProcessId){throw 'Init process changed during capture'}
    return (Join-Path $root ('raw/'+$copy.label+'.stdout'))
}
function UnpackResult([string]$archive,[string]$destination,[long]$limit,[switch]$Executable){
    [void][IO.Directory]::CreateDirectory($destination)
    $stream=[IO.File]::OpenRead($archive);$reader=[Formats.Tar.TarReader]::new($stream);$seen=@{};$total=0L;$count=0
    try {
        while($null -ne ($entry=$reader.GetNextEntry())){
            $name=$entry.Name.TrimEnd('/');if($name -eq '.' -and $entry.EntryType -eq [Formats.Tar.TarEntryType]::Directory){continue}
            if($name.StartsWith('./')){$name=$name.Substring(2)}
            if($name -notmatch '^[a-zA-Z0-9_.\-/]+$' -or @($name.Split('/')|Where-Object {$_ -in @('','..','.')} ).Count -or $name.Length -gt 256 -or $seen.ContainsKey($name)){throw 'Unsafe result member'}
            $seen[$name]=$true
            if($entry.EntryType -eq [Formats.Tar.TarEntryType]::Directory){continue}
            if($entry.EntryType -notin @([Formats.Tar.TarEntryType]::RegularFile,[Formats.Tar.TarEntryType]::V7RegularFile) -or ++$count -gt 100 -or $entry.Length -gt $limit){throw 'Unexpected result entry'}
            $isProbe=$Executable -and $name -ceq 'probe'
            if($isProbe -and [int]$entry.Mode -ne 493){throw 'Unexpected executable archive mode'}
            $total+=$entry.Length;if($total -gt $limit){throw 'Result size limit'}
            $path=Join-Path $destination $name;[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($path));$file=[IO.File]::Open($path,[IO.FileMode]::CreateNew)
            try {$entry.DataStream.CopyTo($file)}finally{$file.Dispose()}
            if($isProbe){[IO.File]::SetUnixFileMode($path,$entry.Mode)}
        }
    } finally {$reader.Dispose();$stream.Dispose()}
}
function CheckRecoveredProof([string]$path){
    if((Get-Item -LiteralPath $path).Length -ne 12 -or [Convert]::ToHexString([IO.File]::ReadAllBytes($path)) -cne '707265736572766564000D0A'){throw 'Recovered binary proof changed'}
}
function Native([string]$kind,[string]$label,[string[]]$argv,[int]$seconds,[long]$resultLimit){
    $caps=@{generation=6;build=11;execution=128;preflight=8;diagnostic=16}
    if(++$script:counts[$kind] -gt $caps[$kind]){throw 'Native operation budget exceeded'}
    $id=Container $label
    try {
        $commandOutputLimit=if($kind -in @('preflight','diagnostic')){1048576}else{8388608}
        $result=Run $label (@('exec',$id)+$argv) $seconds $commandOutputLimit
        if($result.termination -ne 'EXITED'){throw ('Native resource limit: '+$label)}
        $archive=Snapshot $id $label $resultLimit
        $directory=Join-Path $root ('results/'+$label)
        UnpackResult $archive $directory $resultLimit -Executable:($kind -eq 'build')
        $script:outcomes.Add(@{kind=$kind;label=$label;exit_code=$result.exit_code;result_directory=$label})
        if($result.exit_code -ne 0){$script:failed=$true}
        return $result
    } finally {StopContainer $id $label}
}
function CheckBuildHeaders([string]$label,[string]$parserRoot){
    $headers=@([IO.File]::ReadAllLines((Join-Path $root ('raw/'+$label+'.stderr')))|ForEach-Object {if($_ -match '^\.+ (/.+)$'){[IO.Path]::GetFullPath($Matches[1])}}|Sort-Object -Unique)
    if(-not $headers.Count -or $headers.Count -gt 512){throw 'Compiler header closure unavailable'}
    $project=[Collections.Generic.List[object]]::new();$system=[Collections.Generic.List[string]]::new()
    foreach($header in $headers){
        if($header -notmatch '^/[a-zA-Z0-9_./+\-]+$'){throw 'Unsafe compiler header path'}
        if($header.StartsWith('/inputs/')){
            $relative=$header.Substring(8);$local=Join-Path $root $relative
            $identity=FileIdentity $local
            if($header.StartsWith('/inputs/acquisition/sources/')){
                if(-not $script:verifiedSource.ContainsKey($relative) -or $script:verifiedSource[$relative] -cne $identity.sha256){throw 'Unpinned project header'}
            }elseif(-not $local.StartsWith($parserRoot+'/',[StringComparison]::Ordinal)){throw 'Header outside this generated parser'}
            $project.Add($identity)
        }elseif($header.StartsWith('/usr/include/') -or $header.StartsWith('/usr/local/include/') -or $header.StartsWith('/usr/lib/gcc/')){$system.Add($header)}
        else{throw 'Compiler header outside pinned source/image roots'}
    }
    $systemHashes=@()
    if($system.Count){
        $diagnostic=Native diagnostic ($label+'-headers') (@('/usr/bin/sha256sum','--','/usr/bin/sha256sum')+$system.ToArray()) 10 1048576;Require $diagnostic
        $systemHashes=@((TextOutput $diagnostic)-split "`n"|Where-Object {$_}|ForEach-Object {if($_ -notmatch '^([0-9a-f]{64})  (/.+)$'){throw 'Invalid system header digest'};@{path=$Matches[2];sha256=$Matches[1]}})
        if($systemHashes.Count -ne $system.Count+1){throw 'Missing system header identity'}
    }
    Record ($label+'-headers') @{stage='AFTER_BUILD_BEFORE_EXECUTION';compiler_includes_verified=$true;project_headers=$project;system_headers=$systemHashes;system_image=$toolchain.image;raw_compiler_output=('raw/'+$label+'.stderr')}
}

$verdict='FAILED';$failure=$null
try {
    Add-Type -AssemblyName System.Formats.Tar
    Record 'case-review-gate' @{result='PASS';review_sha256=(Get-FileHash $reviewPath).Hash.ToLowerInvariant();review=$caseReview;native_support_result='NOT_RUN'}
    Record 'toolchain-profile' @{selected=$toolchain;original_inputs_sha256=(Get-FileHash $inputsPath).Hash.ToLowerInvariant();source_and_cases_changed=$false;actual_tool_identities='tool-identities command; NOT_VERIFIED until executed'}
    $acquireArgs=@('-NoProfile','-File',(Join-Path $PSScriptRoot 'acquire.ps1'),'-Destination',(Join-Path $root 'acquisition'),'-ToolchainProfile',$ToolchainProfile)
    if($ToolchainApprovalSubject){$acquireArgs+=@('-ToolchainApprovalSubject',$ToolchainApprovalSubject)}
    if($AcquisitionProfile -eq 'pinned-tsql-r1'){$acquireArgs+=@('-PinnedTsql','-AcquisitionApprovalSubject',$AcquisitionApprovalSubject)}
    $acquire=Run 'acquisition' $acquireArgs 600 8388608 $pwsh;Require $acquire
    [IO.File]::Copy((Join-Path $PSScriptRoot 'probe.c.in'),(Join-Path $root 'probe.c'))
    [void][IO.Directory]::CreateDirectory((Join-Path $root 'npm'))
    foreach($package in $inputs.npm){
        $target=Join-Path $root ('npm/'+$package.package);[void][IO.Directory]::CreateDirectory($target)
        foreach($name in @('grammar.js','package.json','LICENSE')){[IO.File]::Copy((Join-Path $root ('acquisition/sources/'+$package.package+'-'+$package.version+'/'+$name)),(Join-Path $target $name))}
    }
    $acquisition=Get-Content -Raw (Join-Path $root 'acquisition/records/acquisition.json')|ConvertFrom-Json
    if($acquisition.state -cne 'COMPLETED'){throw 'Acquisition incomplete'}
    $script:verifiedSource=@{}
    foreach($item in $acquisition.identities){
        if($item.repository){$directory=$item.repository.Replace('/','--')+'--'+$item.commit}
        elseif($item.package){$directory=$item.package+'-'+$item.version}else{continue}
        foreach($file in $item.files){$relative='acquisition/sources/'+$directory+'/'+$file.path;$actual=FileIdentity (Join-Path $root $relative);if($actual.sha256 -cne $file.sha256 -or $actual.bytes -ne $file.bytes){throw 'Source changed after acquisition'};$script:verifiedSource[$relative]=$actual.sha256}
    }
    Record 'verified-source-inputs' @{manifest='acquisition/records/acquisition.json';all_selected_bytes_rechecked=$true;stage='BEFORE_GENERATION_AND_BUILD';system_headers_and_tools=$toolchain.image;dependency_resolution='read-only pinned source roots and image; no install or fetch in native containers'}
    foreach($case in $inputs.cases){
        if($case.id -notmatch '^[A-Z0-9-]+$' -or $case.input_bytes -gt 65536){throw 'Case identity/size mismatch'}
        $path=Join-Path $root ('cases/'+$case.id);[IO.File]::WriteAllText($path,$case.input_utf8,[Text.UTF8Encoding]::new($false))
        if((Get-FileHash $path).Hash.ToLowerInvariant() -cne $case.input_sha256){throw 'Case bytes changed'}
    }
    Require (Run 'docker-version' @('version','--format','{{json .}}') 10 1048576)
    Require (Run 'image-pull' @('pull','--platform','linux/amd64',$toolchain.image) 120)
    if($script:wall.Elapsed.TotalSeconds -gt 600){throw 'Combined acquisition time limit'}
    $receivedUpperBound=(NetworkReceived)-$script:networkStart
    if($receivedUpperBound -gt 1073741824){throw 'Acquisition network budget exceeded'}
    $script:acquiring=$false
    Record 'download-budget' @{received_network_upper_bound_bytes=$receivedUpperBound;source_http_bytes=(Get-Content -Raw (Join-Path $root 'acquisition/records/acquisition.json')|ConvertFrom-Json).download_bytes;image_manifest_compressed_bytes=$toolchain.compressed_bytes;limit_bytes=1073741824;measurement='host network receive counters include protocol/runner traffic; sampled while acquisition is active';sample_ms=20}
    $image=Run 'image-identity' @('image','inspect',$toolchain.image) 10;Require $image
    $im=(TextOutput $image|ConvertFrom-Json)[0]
    if($im.Os -cne 'linux' -or $im.Architecture -cne 'amd64' -or $toolchain.image -notin $im.RepoDigests){throw 'Image digest/platform mismatch'}
    $script:imageSize=[long]$im.Size
    if($script:imageSize -gt 2147483648){throw 'Image storage reserve exceeded'}
    $security=Run 'docker-security' @('info','--format','{{json .SecurityOptions}}') 10 1048576;Require $security
    if((TextOutput $security) -notmatch 'seccomp.*profile=builtin'){throw 'Default seccomp is unavailable'}
    $sentinel=Join-Path $root 'host-only-sentinel'
    [IO.File]::WriteAllText($sentinel,'P05-owned-host-only-marker',[Text.UTF8Encoding]::new($false))
    Record 'host-only-sentinel' @{identity=FileIdentity $sentinel;secret=$false;mounted=$false}
    $checks=@(
        'test "$(id -u)" = 65534; grep -q "NoNewPrivs:[[:space:]]*1" /proc/self/status; grep -q "CapEff:[[:space:]]*0000000000000000" /proc/self/status',
        'test "$(cat /sys/fs/cgroup/memory.max)" = 4294967296; test "$(cat /sys/fs/cgroup/pids.max)" = 64',
        'if touch /inputs/cases/unapproved 2>/dev/null; then exit 1; fi; if touch /root/unapproved 2>/dev/null; then exit 1; fi',
        'test ! -S /var/run/docker.sock; test ! -e /home/runner/work; test ! -e /github/workspace; test ! -e "$1"; test "$(stat -f -c %S /work)" -gt 0; printf "preserved\000\r\n" > /work/proof'
    )
    for($i=0;$i -lt $checks.Count;$i++){Require (Native preflight ('preflight-'+$i) @('/bin/sh','-ec',$checks[$i],'preflight',$sentinel) 10 1048576)}
    $proof=Join-Path $root 'results/preflight-3/proof'
    CheckRecoveredProof $proof
    Record 'recovered-proof' @{result='PASS';file=FileIdentity $proof;expected_hex='707265736572766564000d0a';capture='quiescent-tar-r1';container_destroyed_after_capture=$true}
    Require (Native preflight 'preflight-tmpfs' @('/usr/local/bin/node','-e','let s=require("fs").statfsSync("/work");process.exit(s.bsize*s.blocks===2147483648?0:1)') 10 1048576)
    $network='const net=require("net");let s=net.connect({host:"1.1.1.1",port:443});s.on("connect",()=>process.exit(1));s.on("error",e=>process.exit(e.code==="ENETUNREACH"?0:2));setTimeout(()=>process.exit(3),2000);'
    Require (Native preflight 'preflight-network' @('/usr/local/bin/node','-e',$network) 10 1048576)
    $pidProbe='const{spawn}=require("child_process");let cs=[],errors=0,closed=0;for(let i=0;i<70;i++){let c=spawn("/bin/sleep",["5"]);cs.push(c);c.on("error",()=>errors++);c.on("close",()=>closed++);}setTimeout(()=>cs.forEach(c=>c.kill("SIGKILL")),1000);setTimeout(()=>process.exit(errors>0&&closed===70?0:1),2500);'
    Require (Native preflight 'preflight-pids' @('/usr/local/bin/node','-e',$pidProbe) 10 1048576)
    $timeoutId=Container 'preflight-timeout';$script:counts.preflight++
    try {$timeout=Run 'preflight-timeout' @('exec',$timeoutId,'/bin/sh','-c','sleep 30 & wait') 1 1048576;if($timeout.termination -ne 'TIMEOUT'){throw 'Timeout preflight failed'}}finally{StopContainer $timeoutId 'preflight-timeout'}
    Require (Native diagnostic 'tool-identities' @('/bin/sh','-ec','/inputs/acquisition/tools/tree-sitter --version; node --version; gcc --version; ld --version; getconf GNU_LIBC_VERSION; sha256sum /inputs/acquisition/tools/tree-sitter /usr/local/bin/node /usr/bin/gcc /usr/bin/ld /lib/x86_64-linux-gnu/libc.so.6 /bin/sh /usr/bin/stat /usr/bin/sha256sum') 10 1048576)
    Record 'preflight' @{result='PASS';counts=$script:counts;image=$toolchain.image;limitations='container setting and owned adverse checks; not S04 product supervisor qualification'}
    $runtime='/inputs/acquisition/sources/tree-sitter--tree-sitter--659cda7c7f86ebe31cc825dc5da59e9add172dc7'
    foreach($routeName in @('tsql','csharp','typescript','tsx','postgresql-sql','swift')){
        $route=@($inputs.selected_routes|Where-Object route -eq $routeName)[0]
        $key=$route.repository.Replace('/','--')+'--'+$route.commit
        $source='/inputs/acquisition/sources/'+$key
        if($route.subdirectory -ne '.'){$source+='/'+$route.subdirectory}
        $localSource=Join-Path $root ('acquisition/sources/'+$key+'/'+$route.subdirectory)
        $grammar=Get-Content -Raw (Join-Path $localSource 'src/grammar.json')|ConvertFrom-Json
        if($grammar.name -notmatch '^[A-Za-z_][A-Za-z0-9_]{0,63}$'){throw 'Grammar name rejected'}
        $generationInputs=if($route.generation -eq 'json'){@(FileIdentity (Join-Path $localSource 'src/grammar.json'))}else{CheckJsInputs (Join-Path $localSource 'grammar.js') (Join-Path $root ('acquisition/sources/'+$key))}
        Record ('generate-'+$route.route+'-inputs') @{files=$generationInputs;runtime='node in pinned image';literal_dependencies_verified=$true;arbitrary_javascript_dependency_proof=$false;exact_input_set='inputs.json repositories.files plus npm integrity; observed SHA256 in acquisition receipt';all_source_bytes='verified-source-inputs.json';dynamic_execution_boundary='preflight PASS; network none; read-only pinned sources and image; only task tmpfs writable'}
        $generation=Native generation ('generate-'+$route.route) @('/inputs/acquisition/tools/tree-sitter','generate','--abi','15','--js-runtime','node','--output','/work/generated',$(if($route.generation -eq 'json'){$source+'/src/grammar.json'}else{$source+'/grammar.js'})) 300 536870912
        foreach($variant in @('baseline','regenerated')){
            if($variant -eq 'baseline' -and $route.route -eq 'swift'){continue}
            if($variant -eq 'regenerated' -and $generation.exit_code -ne 0){continue}
            $parser=if($variant -eq 'baseline'){$source+'/src'}else{'/inputs/results/generate-'+$route.route+'/generated'}
            $label='build-'+$route.route+'-'+$variant
            $parserRoot=if($variant -eq 'baseline'){Join-Path $localSource 'src'}else{Join-Path $root ('results/generate-'+$route.route+'/generated')}
            $buildInputs=@((FileIdentity (Join-Path $parserRoot 'parser.c')),(FileIdentity (Join-Path $parserRoot 'tree_sitter/parser.h')),(FileIdentity (Join-Path $root 'probe.c')))
            if(Test-Path (Join-Path $localSource 'src/scanner.c')){$buildInputs+=FileIdentity (Join-Path $localSource 'src/scanner.c')}
            Record ($label+'-inputs') @{files=$buildInputs;runtime_pin=$inputs.runtime.commit;runtime_byte_manifest='acquisition/records/acquisition.json';compiler_image=$toolchain.image;query_execution=$false}
            $args=@('/usr/bin/gcc','-std=c11','-D_DEFAULT_SOURCE','-O0','-Wall','-Wextra','-H',('-DLANGUAGE=tree_sitter_'+$grammar.name),('-I'+$runtime+'/lib/include'),('-I'+$runtime+'/lib/src'),('-I'+$parser),('/inputs/probe.c'),($runtime+'/lib/src/lib.c'),($parser+'/parser.c'))
            if(Test-Path (Join-Path $localSource 'src/scanner.c')){$args+=($source+'/src/scanner.c')}
            $args+=@('-o','/work/probe')
            $build=Native build $label $args 120 134217728
            if($build.exit_code -ne 0){continue}
            CheckBuildHeaders $label $parserRoot
            $binary=Join-Path $root ('results/'+$label+'/probe');$binaryIdentity=FileIdentity $binary
            if([int][IO.File]::GetUnixFileMode($binary) -ne 493){throw 'Recovered executable mode mismatch'}
            Record ($label+'-executable') @{executable=$binaryIdentity;archive_and_recovered_unix_mode='0755';next_use='fresh read-only input mount; exact SHA/mode checked before each case';library_closure='pinned official image: GCC default C link, libc and loader identities in tool-identities';source_modified=$false}
            foreach($case in $inputs.cases|Where-Object route -eq $route.route){
                $caseIdentity=FileIdentity (Join-Path $root ('cases/'+$case.id))
                if($caseIdentity.sha256 -cne $case.input_sha256 -or $caseIdentity.bytes -ne $case.input_bytes){throw 'Case changed before execution'}
                $execCheck='expected=$1; shift; test "$(/usr/bin/stat -c %a -- "$1")" = 755 || exit 74; actual=$(/usr/bin/sha256sum -- "$1"); test "${actual%% *}" = "$expected" || exit 74; exec "$@"'
                $args=@('/bin/sh','-ec',$execCheck,'p05-exec',$binaryIdentity.sha256,('/inputs/results/'+$label+'/probe'),('/inputs/cases/'+$case.id))
                if($case.edit){$args+=@([string]$case.edit.start_byte,[string]$case.edit.old_end_byte)}
                [void](Native execution ('case-'+$variant+'-'+$case.id.ToLowerInvariant()) $args 10 8388608)
            }
        }
    }
    $verdict=if($script:failed){'REPRODUCED_FAILURES'}else{'BOUNDED_INPUTS_COMPLETED_REVIEW_REQUIRED'}
} catch {$failure=$_.Exception.GetType().FullName;throw}
finally {
    $cleanupErrors=@(foreach($id in @($script:containers)){try{StopContainer $id ('final-'+$id.Substring(0,12))}catch{@{container=$id;failure_type=$_.Exception.GetType().FullName}}})
    if($cleanupErrors.Count){$verdict='CLEANUP_NOT_VERIFIED';$script:failed=$true}
    Record 'summary' @{verdict=$verdict;failure_type=$failure;cleanup_errors=$cleanupErrors;counts=$script:counts;wall_seconds=$script:wall.Elapsed.TotalSeconds;outcomes=$script:outcomes;product_qualification=$false;whole_feature_support=$false;command_count=$script:commands.Count}
}
if($script:failed){throw 'Required P05 inputs failed; original evidence retained'}
