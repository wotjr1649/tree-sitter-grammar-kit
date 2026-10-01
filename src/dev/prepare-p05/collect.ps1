param([Parameter(Mandatory)][string]$Root)
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion.Major -ne 7){throw 'PowerShell 7 required'}
$task=[IO.Path]::GetFullPath($Root)
$prefix=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('/')+'/'
if(-not $task.StartsWith($prefix,[StringComparison]::Ordinal) -or $task.Substring($prefix.Length) -notmatch '^tsgk-p05-(?:remedy-)?[0-9]+-[0-9]+$'){throw 'Unexpected evidence root'}
if(-not (Test-Path -LiteralPath $task)){throw 'Task evidence missing'}
$exactLimits=$null;$summary=$null
$summaryPath=Join-Path $task 'records/summary.json'
if(Test-Path -LiteralPath $summaryPath){
    $summary=Get-Content -LiteralPath $summaryPath -Raw|ConvertFrom-Json -AsHashtable
    . (Join-Path $PSScriptRoot 'remedy.ps1')
    $exactLimits=ExactRemedyLimits $summary.remedy_stage
}
function GeneratedArtifactProvenance([string]$task,$outcome,[long]$bytes,[string]$sha256){
    $label=$outcome.label
    if($label -cnotmatch '^generate-[a-z0-9-]+$' -or $outcome.result_directory -cne $label){throw 'Generated artifact label mismatch'}
    $refs=@{}
    foreach($relative in @(('records/command-'+$label+'.json'),('records/'+$label+'-inputs.json'),('records/command-'+$label+'-copy.json'),('raw/'+$label+'.stdout'),('raw/'+$label+'.stderr'),('raw/'+$label+'-copy.stdout'),('raw/'+$label+'-copy.stderr'))){
        $file=Get-Item -LiteralPath (Join-Path $task $relative) -Force
        if($file.PSIsContainer -or $file.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Generated provenance is not a regular file'}
        $refs[$relative]=@{path=$relative;bytes=$file.Length;sha256=(Get-FileHash -LiteralPath $file.FullName).Hash.ToLowerInvariant()}
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
        $refs[$buildRelative]=@{path=$buildRelative;bytes=$file.Length;sha256=(Get-FileHash -LiteralPath $file.FullName).Hash.ToLowerInvariant()}
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
    $files+=Get-Item -LiteralPath (Join-Path $PSScriptRoot 'remedy-patches.json'),(Join-Path $PSScriptRoot 'remedy-cases.json'),(Join-Path $PSScriptRoot 'remedy-fact-oracles.json'),(Join-Path $PSScriptRoot 'remedy-sources.json'),(Join-Path $PSScriptRoot 'remedy-r2.json'),(Join-Path $PSScriptRoot 'remedy-exact-r1.json')
    foreach($directory in @('candidates','candidate-evaluation','materialized-lfs')){
        $path=Join-Path $task $directory
        if(Test-Path -LiteralPath $path){$files+=Get-ChildItem -LiteralPath $path -File -Recurse}
    }
}
if($exactLimits){
    # Preserve all original capture bytes and export a separately recoverable C object.
    # Gzip is lossless storage; the uncompressed identity remains the producer identity.
    $exports=@()
    foreach($outcome in $summary.outcomes|Where-Object {$_.kind -ceq 'generation' -and $_.scope -ceq 'REGISTERED_P05' -and $_.termination -ceq 'EXITED' -and $_.exit_code -eq 0}){
        $source=Join-Path $task ('results/'+$outcome.result_directory+'/generated/parser.c')
        $file=Get-Item -LiteralPath $source -Force
        if($file.Attributes -band [IO.FileAttributes]::ReparsePoint -or $file.Length -gt 536870912){throw 'Generated C export rejected'}
        $digest=(Get-FileHash -LiteralPath $source).Hash.ToLowerInvariant()
        $provenance=GeneratedArtifactProvenance $task $outcome $file.Length $digest
        $target=Join-Path $task ('generated-artifacts/'+$outcome.label+'/parser.c.gz')
        [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
        $sink=[IO.File]::Open($target,[IO.FileMode]::CreateNew);$gzip=[IO.Compression.GZipStream]::new($sink,[IO.Compression.CompressionLevel]::Optimal);$input=[IO.File]::OpenRead($source)
        try{
            $buffer=[byte[]]::new(65536);$count=0L
            while(($n=$input.Read($buffer,0,$buffer.Length)) -gt 0){$count+=$n;if($count -gt 536870912 -or $sink.Position+$n+65536 -gt $exactLimits.local_packed_max_bytes){throw 'Generated C export limit'};$gzip.Write($buffer,0,$n)}
        }finally{$input.Dispose();$gzip.Dispose();$sink.Dispose()}
        if($count -ne $file.Length -or (Get-FileHash -LiteralPath $source).Hash.ToLowerInvariant() -cne $digest){throw 'Generated C changed during export'}
        $exports+=@{producer=$outcome.label;source_path=[IO.Path]::GetRelativePath($task,$source);original_bytes=$file.Length;original_sha256=$digest;lossless_gzip_path=[IO.Path]::GetRelativePath($task,$target);provenance=$provenance;capture_original_retained=$true;identity_is_uncompressed_bytes=$true}
        $files+=Get-Item -LiteralPath $target
    }
    $exportRecord=Join-Path $task 'records/generated-artifacts.json'
    $stream=[IO.File]::Open($exportRecord,[IO.FileMode]::CreateNew)
    try{$bytes=[Text.Encoding]::UTF8.GetBytes(($exports|ConvertTo-Json -Depth 10 -AsArray));$stream.Write($bytes)}finally{$stream.Dispose()}
    $files+=Get-Item -LiteralPath $exportRecord
}
if($files.Count -gt 10000){throw 'Evidence file count limit'}
$manifest=@(foreach($file in $files){
    if($file.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Evidence link rejected'}
    $relative=if($file.FullName.StartsWith($task+'/')){[IO.Path]::GetRelativePath($task,$file.FullName)}else{'harness/'+$file.Name}
    if($relative -notmatch '^[a-zA-Z0-9_.\-/]+$' -or $relative.Length -gt 200){throw 'Evidence path rejected'}
    @{path=$relative;bytes=$file.Length;sha256=(Get-FileHash -LiteralPath $file.FullName).Hash.ToLowerInvariant();source=$file.FullName}
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
        $entry=$zip.CreateEntry($item.path,[IO.Compression.CompressionLevel]::Optimal);$sink=$entry.Open();$source=[IO.File]::OpenRead($item.source)
        try {
            $buffer=[byte[]]::new(65536)
            while(($n=$source.Read($buffer,0,$buffer.Length)) -gt 0){
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
$digest=(Get-FileHash -LiteralPath $partial).Hash.ToLowerInvariant()
[IO.File]::Move($partial,$complete)
$package=Get-Item -LiteralPath $complete
@{bytes=$package.Length;sha256=$digest;files=$manifest.Count;retention_days=7}|ConvertTo-Json -Compress
