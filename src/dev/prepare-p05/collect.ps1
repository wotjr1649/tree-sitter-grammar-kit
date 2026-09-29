param([Parameter(Mandatory)][string]$Root)
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion.Major -ne 7){throw 'PowerShell 7 required'}
$task=[IO.Path]::GetFullPath($Root)
$prefix=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('/')+'/'
if(-not $task.StartsWith($prefix,[StringComparison]::Ordinal) -or $task.Substring($prefix.Length) -notmatch '^tsgk-p05-[0-9]+-[0-9]+$'){throw 'Unexpected evidence root'}
if(-not (Test-Path -LiteralPath $task)){throw 'Task evidence missing'}
$files=@(foreach($directory in @('raw','records','cases','owned','acquisition/archives','acquisition/records','acquisition/sources/Crary-Systems--tree-sitter-tsql--443d2bc774f1d779af7dcabcc99160fb24da96e6')){
    $path=Join-Path $task $directory
    if(Test-Path -LiteralPath $path){Get-ChildItem -LiteralPath $path -File -Recurse|Where-Object {-not $_.Name.EndsWith('.tar')} }
})
$files+=Get-Item -LiteralPath (Join-Path $PSScriptRoot 'inputs.json'),(Join-Path $PSScriptRoot 'case-review.json'),(Join-Path $PSScriptRoot 'probe.c.in')
if($files.Count -gt 10000){throw 'Evidence file count limit'}
$manifest=@(foreach($file in $files){
    if($file.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Evidence link rejected'}
    $relative=if($file.FullName.StartsWith($task+'/')){[IO.Path]::GetRelativePath($task,$file.FullName)}else{'harness/'+$file.Name}
    if($relative -notmatch '^[a-zA-Z0-9_.\-/]+$' -or $relative.Length -gt 200){throw 'Evidence path rejected'}
    @{path=$relative;bytes=$file.Length;sha256=(Get-FileHash -LiteralPath $file.FullName).Hash.ToLowerInvariant();source=$file.FullName}
})
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
                if($output.Position+$n+65536 -gt 260046848){throw 'Evidence package budget exceeded'}
                $sink.Write($buffer,0,$n)
            }
        } finally {$source.Dispose();$sink.Dispose()}
    }
} finally {$zip.Dispose();$output.Dispose()}
$package=Get-Item -LiteralPath $partial
if($package.Length -gt 268435456){throw 'Evidence package size limit'}
$digest=(Get-FileHash -LiteralPath $partial).Hash.ToLowerInvariant()
[IO.File]::Move($partial,$complete)
$package=Get-Item -LiteralPath $complete
@{bytes=$package.Length;sha256=$digest;files=$manifest.Count;retention_days=7}|ConvertTo-Json -Compress
