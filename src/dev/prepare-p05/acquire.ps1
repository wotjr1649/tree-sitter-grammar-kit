param([Parameter(Mandatory)][string]$Destination, [switch]$SelfTest, [switch]$PinnedTsql, [string]$AcquisitionApprovalSubject, [string]$ToolchainProfile='bookworm-r1', [string]$ToolchainApprovalSubject, [string]$RemedyStage, [string]$RemedyApprovalSubject)
$ErrorActionPreference = 'Stop'
$toolchain=& (Join-Path $PSScriptRoot 'approval.ps1') -Profile $(if($PinnedTsql){'pinned-tsql-r1'}else{'archive-r1'}) -Subject $AcquisitionApprovalSubject -ImageProfile $ToolchainProfile -ImageSubject $ToolchainApprovalSubject -RemedyStage $RemedyStage -RemedySubject $RemedyApprovalSubject
if(-not $toolchain){throw 'Explicit toolchain profile required'}
$script:acquisitionLimit=[long]$toolchain.acquisition_limit_bytes
if ($PSVersionTable.PSVersion.Major -ne 7) { throw 'PowerShell 7 required' }
Add-Type -AssemblyName System.Formats.Tar
$script:received = 0L
$script:expanded = 0L
$script:requests = 0
$script:filesWritten = 0
$script:imageReserve = 0L
$script:httpLimit=if($PinnedTsql){52}else{32}
$script:clock = [Diagnostics.Stopwatch]::StartNew()
$script:receipts = [Collections.Generic.List[object]]::new()
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$localPrefix=[IO.Path]::GetFullPath((Join-Path $repoRoot '.work/campaign-01-prepare-03')).TrimEnd('/','\')+[IO.Path]::DirectorySeparatorChar
$resolvedDestination=[IO.Path]::GetFullPath($Destination)
$localAllowed=$resolvedDestination.StartsWith($localPrefix,[StringComparison]::OrdinalIgnoreCase)
$hostedAllowed=$false
if($IsLinux -and $env:RUNNER_TEMP){
    $runnerPrefix=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('/')+'/'
    if($resolvedDestination.StartsWith($runnerPrefix,[StringComparison]::Ordinal)){
        $relative=$resolvedDestination.Substring($runnerPrefix.Length)
        $pattern=if($RemedyStage){'^tsgk-p05-remedy-[0-9]+-[0-9]+/acquisition$'}else{'^tsgk-p05-[0-9]+-[0-9]+/acquisition$'}
        $hostedAllowed=$relative -match $pattern
    }
}
if(-not ($localAllowed -or $hostedAllowed)){throw 'Destination outside approved task roots'}
if($RemedyStage){
    if(-not $hostedAllowed -and -not $SelfTest){throw 'Remedy acquisition requires fresh hosted task root'}
    . (Join-Path $PSScriptRoot 'remedy.ps1')
    $remedy=ReadRemedySubjects
}
function Portable([string]$name) {
    if (-not $name -or $name.Length -gt 1024 -or $name -match '[\\:\x00-\x1f\x7f]' -or $name -ne $name.Normalize([Text.NormalizationForm]::FormC)) { throw 'Unsafe member name' }
    foreach ($part in $name.Split('/')) {
        if ($part -in @('', '.', '..') -or $part.TrimEnd(' ', '.') -cne $part -or $part -match '^(?i:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)') { throw 'Unsafe member segment' }
    }
}
function FreshRoot([string]$path) {
    $full = [IO.Path]::GetFullPath($path)
    $parent = Get-Item -LiteralPath ([IO.Path]::GetDirectoryName($full)) -Force
    $cursor = $parent
    while ($null -ne $cursor) {
        if ($cursor.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Reparse ancestor' }
        $cursor = $cursor.Parent
    }
    if (Test-Path -LiteralPath $full) { throw 'Destination already exists' }
    [void][IO.Directory]::CreateDirectory($full)
    return $full
}
function SaveReceipt([string]$path, $value) {
    $bytes = [Text.Encoding]::UTF8.GetBytes(($value | ConvertTo-Json -Depth 24))
    $file = [IO.File]::Open($path, [IO.FileMode]::CreateNew)
    try { $file.Write($bytes) } finally { $file.Dispose() }
}
function Fetch([string]$url, [string]$target, [long]$limit) {
    $allowed = @('codeload.github.com','github.com','release-assets.githubusercontent.com','objects.githubusercontent.com','registry.npmjs.org')
    if($PinnedTsql){$allowed+='raw.githubusercontent.com'}
    if($RemedyStage -cin @('sql-pg-r1','sql-pg-r2') -and $url -ceq $remedy.sources.postgresql.provider){$allowed+='media.githubusercontent.com'}
    $handler = [Net.Http.HttpClientHandler]::new(); $handler.AllowAutoRedirect = $false
    $handler.UseProxy=$false;$handler.UseDefaultCredentials=$false;$handler.UseCookies=$false
    $client = [Net.Http.HttpClient]::new($handler)
    $client.Timeout = [TimeSpan]::FromSeconds(120)
    $cancel = [Threading.CancellationTokenSource]::new(120000)
    $redirects = [Collections.Generic.List[string]]::new()
    $response = $null; $stream = $null; $file = $null
    $count = 0L; $completed = $false
    try {
        $uri = [Uri]$url
        while ($true) {
            if ($uri.Scheme -cne 'https' -or $uri.DnsSafeHost -notin $allowed -or $uri.Port -ne 443 -or $uri.UserInfo) { throw 'Unapproved download destination' }
            if (++$script:requests -gt $script:httpLimit -or $script:clock.Elapsed.TotalSeconds -gt 600) { throw 'Acquisition count/time limit' }
            $response = $client.GetAsync($uri, [Net.Http.HttpCompletionOption]::ResponseHeadersRead, $cancel.Token).GetAwaiter().GetResult()
            if ([int]$response.StatusCode -in @(301,302,303,307,308)) {
                if ($redirects.Count -ge 5 -or -not $response.Headers.Location) { throw 'Redirect limit' }
                if($url -cne 'https://github.com/tree-sitter/tree-sitter/releases/download/v0.27.0/tree-sitter-linux-x64.gz'){throw 'Unexpected artifact redirect'}
                $uri = [Uri]::new($uri, $response.Headers.Location)
                if($uri.DnsSafeHost -notin @('release-assets.githubusercontent.com','objects.githubusercontent.com')){throw 'Unexpected release CDN'}
                $redirects.Add($uri.GetLeftPart([UriPartial]::Path))
                $response.Dispose(); $response = $null; continue
            }
            [void]$response.EnsureSuccessStatusCode(); break
        }
        if ($response.Content.Headers.ContentLength -gt $limit) { throw 'Download declared size limit' }
        $stream = $response.Content.ReadAsStreamAsync($cancel.Token).GetAwaiter().GetResult()
        $file = [IO.File]::Open($target, [IO.FileMode]::CreateNew)
        $buffer = [byte[]]::new(65536)
        while (($n = $stream.ReadAsync($buffer,0,$buffer.Length,$cancel.Token).GetAwaiter().GetResult()) -gt 0) {
            $count += $n; $script:received += $n
            # The bound stage selects the shared source/image envelope before effects.
            if ($count -gt $limit -or $script:received + $script:imageReserve -gt $script:acquisitionLimit -or $script:clock.Elapsed.TotalSeconds -gt 600) { throw 'Acquisition byte/time limit' }
            $file.Write($buffer,0,$n)
        }
        $completed = $true
    } catch {
        throw [InvalidOperationException]::new('Download failed: '+$_.Exception.GetType().FullName)
    } finally {
        if ($file) { $file.Dispose() }; if ($stream) { $stream.Dispose() }; if ($response) { $response.Dispose() }
        $client.Dispose(); $handler.Dispose(); $cancel.Dispose()
        $stored=if(Test-Path -LiteralPath $target){(Get-Item -LiteralPath $target).Length}else{0}
        $script:receipts.Add(@{url=$url;redirect_paths=$redirects;received_bytes=$count;stored_bytes=$stored;stored_sha256=$(if(Test-Path -LiteralPath $target){(Get-FileHash -LiteralPath $target).Hash.ToLowerInvariant()}else{$null});target=[IO.Path]::GetFileName($target);partial=(-not $completed);verified=$false})
    }
    return @{bytes=$count;sha256=(Get-FileHash -LiteralPath $target).Hash.ToLowerInvariant()}
}
function ExpandBoundedTar([string]$archive) {
    $path=$archive+'.tar'
    if(Test-Path -LiteralPath $path){throw 'Tar staging path exists'}
    $input=[IO.File]::OpenRead($archive);$gzip=[IO.Compression.GZipStream]::new($input,[IO.Compression.CompressionMode]::Decompress);$output=[IO.File]::Open($path,[IO.FileMode]::CreateNew)
    try {
        $buffer=[byte[]]::new(65536)
        while(($n=$gzip.Read($buffer,0,$buffer.Length)) -gt 0){$script:expanded+=$n;if($script:expanded -gt 536870912){throw 'Actual decompression limit'};$output.Write($buffer,0,$n)}
    } finally {$output.Dispose();$gzip.Dispose();$input.Dispose()}
    # Bound metadata allocation before passing bytes to the platform TAR reader.
    $stream=[IO.File]::OpenRead($path);$header=[byte[]]::new(512);$entries=0
    try {
        while($stream.Position -lt $stream.Length){
            if($stream.Read($header,0,512) -ne 512){throw 'Truncated tar block'}
            if($header[0] -eq 0){if(@($header|Where-Object {$_ -ne 0}).Count){throw 'Malformed tar terminator'};break}
            if(++$entries -gt 20000){throw 'Tar header count limit'}
            $sizeText=[Text.Encoding]::ASCII.GetString($header,124,12).Trim([char]0,' ')
            if($sizeText -notmatch '^[0-7]{1,11}$'){throw 'Unsupported tar size encoding'}
            $size=[Convert]::ToInt64($sizeText,8);$type=[char]$header[156]
            if($size -gt 67108864 -or ($type -in @('x','g','L','K') -and $size -gt 16384)){throw 'Tar member/metadata size limit'}
            $next=$stream.Position+[long]([Math]::Ceiling($size/512.0)*512)
            if($next -gt $stream.Length){throw 'Truncated tar data'}
            $stream.Position=$next
        }
    } finally {$stream.Dispose()}
    return $path
}
function InspectArchive([string]$archive, $expected, [string]$prefixMode) {
    $tarPath = ExpandBoundedTar $archive
    $input = [IO.File]::OpenRead($tarPath)
    $tar = [Formats.Tar.TarReader]::new($input)
    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    $types = @{}
    $matched = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $prefix = $null; $entries = 0
    try {
        while ($null -ne ($entry = $tar.GetNextEntry())) {
            if (++$entries -gt 20000) { throw 'Archive entry limit' }
            if ($entry.EntryType -eq [Formats.Tar.TarEntryType]::GlobalExtendedAttributes) { continue }
            $name = $entry.Name.TrimEnd('/')
            Portable $name
            if (-not $seen.Add($name)) { throw 'Archive collision' }
            if ($entry.EntryType -notin @([Formats.Tar.TarEntryType]::Directory,[Formats.Tar.TarEntryType]::RegularFile,[Formats.Tar.TarEntryType]::V7RegularFile)) { throw 'Archive link/special entry rejected' }
            $isDirectory=$entry.EntryType -eq [Formats.Tar.TarEntryType]::Directory
            $ancestor=$name
            while($ancestor.Contains('/')){$ancestor=$ancestor.Substring(0,$ancestor.LastIndexOf('/'));if($types.ContainsKey($ancestor) -and -not $types[$ancestor]){throw 'Archive file/directory overlap'}}
            if(-not $isDirectory -and @($types.Keys|Where-Object {$_.StartsWith($name+'/',[StringComparison]::OrdinalIgnoreCase)}).Count){throw 'Archive file/directory overlap'}
            $types[$name]=$isDirectory
            $parts = $name.Split('/',2)
            if ($null -eq $prefix) { $prefix=$parts[0] }
            if ($parts[0] -cne $prefix -or ($prefixMode -eq 'npm' -and $prefix -cne 'package')) { throw 'Archive prefix mismatch' }
            if ($entry.EntryType -eq [Formats.Tar.TarEntryType]::Directory) { continue }
            if ($parts.Count -ne 2 -or $entry.Length -gt 67108864 -or $entry.Length -lt 0) { throw 'Archive file size/path limit' }
            if ($expected.ContainsKey($parts[1])) {
                if ($null -ne $expected[$parts[1]].bytes -and $entry.Length -ne $expected[$parts[1]].bytes) { throw 'Pinned file length mismatch' }
                [void]$matched.Add($parts[1])
            }
        }
    } finally { $tar.Dispose(); $input.Dispose() }
    if ($matched.Count -ne $expected.Count) { throw 'Pinned archive input missing' }
    return $prefix
}
function Materialize([string]$archive, $expected, [string]$destination, [string]$prefixMode='github') {
    $prefix = InspectArchive $archive $expected $prefixMode
    $root = FreshRoot $destination
    $input=[IO.File]::OpenRead($archive+'.tar'); $tar=[Formats.Tar.TarReader]::new($input)
    $result=[Collections.Generic.List[object]]::new()
    try {
        while ($null -ne ($entry=$tar.GetNextEntry())) {
            if ($entry.EntryType -notin @([Formats.Tar.TarEntryType]::RegularFile,[Formats.Tar.TarEntryType]::V7RegularFile)) { continue }
            $path=$entry.Name.Substring($prefix.Length+1)
            if (-not $expected.ContainsKey($path)) { continue }
            Portable $path
            if (++$script:filesWritten -gt 5000) { throw 'Extracted file count limit' }
            $target=Join-Path $root $path
            [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
            $output=[IO.File]::Open($target,[IO.FileMode]::CreateNew)
            $sha1=[Security.Cryptography.IncrementalHash]::CreateHash([Security.Cryptography.HashAlgorithmName]::SHA1)
            $sha1.AppendData([Text.Encoding]::UTF8.GetBytes("blob $($entry.Length)"+[char]0))
            $count=0L; $buffer=[byte[]]::new(65536)
            try {
                while (($n=$entry.DataStream.Read($buffer,0,$buffer.Length)) -gt 0) {
                    $count+=$n; if($count -gt $entry.Length -or $count -gt 67108864){throw 'Actual extracted length limit'}
                    $sha1.AppendData($buffer,0,$n); $output.Write($buffer,0,$n)
                }
            } finally { $output.Dispose() }
            $blob=[Convert]::ToHexString($sha1.GetHashAndReset()).ToLowerInvariant(); $sha1.Dispose()
            $hash=(Get-FileHash -LiteralPath $target).Hash.ToLowerInvariant()
            $pin=$expected[$path]
            if ($count -ne $entry.Length -or ($pin.git_blob_sha1 -and $blob -cne $pin.git_blob_sha1) -or ($pin.sha256 -and $hash -cne $pin.sha256)) { throw 'Source byte identity mismatch' }
            $result.Add(@{path=$path;bytes=$count;git_blob_sha1=$blob;sha256=$hash})
        }
    } finally { $tar.Dispose(); $input.Dispose() }
    return ,$result.ToArray()
}
function MapFiles($files) { $map=[Collections.Generic.Dictionary[string,object]]::new([StringComparer]::Ordinal); foreach($f in $files){Portable $f.path; if($map.ContainsKey($f.path)){throw 'Duplicate pinned path'}; $map[$f.path]=$f}; return ,$map }
function VerifyRegularFile([string]$path,$pin){
    $item=Get-Item -LiteralPath $path -Force
    if($item.PSIsContainer -or $item.Attributes -band [IO.FileAttributes]::ReparsePoint -or $item.Length -ne $pin.bytes -or $item.Length -gt 67108864){throw 'Pinned regular file size/type mismatch'}
    $hash=[Security.Cryptography.IncrementalHash]::CreateHash([Security.Cryptography.HashAlgorithmName]::SHA1)
    $hash.AppendData([Text.Encoding]::UTF8.GetBytes("blob $($item.Length)"+[char]0))
    $stream=[IO.File]::OpenRead($path)
    try{$buffer=[byte[]]::new(65536);while(($n=$stream.Read($buffer,0,$buffer.Length)) -gt 0){$hash.AppendData($buffer,0,$n)};$blob=[Convert]::ToHexString($hash.GetHashAndReset()).ToLowerInvariant()}finally{$stream.Dispose();$hash.Dispose()}
    $digest=(Get-FileHash -LiteralPath $path).Hash.ToLowerInvariant()
    if($blob -cne $pin.git_blob_sha1 -or ($pin.sha256 -and $digest -cne $pin.sha256)){throw 'Pinned regular file identity mismatch'}
    return @{path=$pin.path;bytes=$item.Length;git_blob_sha1=$blob;sha256=$digest}
}

$root=FreshRoot $Destination
if ($SelfTest) {
    foreach($bad in @('../x','/x','a//b','a\b','C:x','a/CON.txt','a/x.','a/./x')) {
        $rejected=$false; try { Portable $bad } catch { $rejected=$true }; if(-not $rejected){throw 'Unsafe path accepted'}
    }
    Portable 'src/tree_sitter/parser.h'
    foreach($kind in @('safe','link','collision','escape','case-mismatch','zero-size')) {
        $archive=Join-Path $root ($kind+'.tgz'); $stream=[IO.File]::Create($archive); $gzip=[IO.Compression.GZipStream]::new($stream,[IO.Compression.CompressionLevel]::Optimal); $writer=[Formats.Tar.TarWriter]::new($gzip)
        try {
            $name=if($kind -eq 'escape'){'r/../x'}elseif($kind -eq 'case-mismatch'){'r/X'}else{'r/x'}
            $type=if($kind -eq 'link'){[Formats.Tar.TarEntryType]::SymbolicLink}else{[Formats.Tar.TarEntryType]::RegularFile}
            $entry=[Formats.Tar.PaxTarEntry]::new($type,$name)
            if($kind -eq 'link'){$entry.LinkName='/outside'}else{$entry.DataStream=[IO.MemoryStream]::new([byte[]]@(120))}
            $writer.WriteEntry($entry)
            if($kind -eq 'collision'){$duplicate=[Formats.Tar.PaxTarEntry]::new([Formats.Tar.TarEntryType]::RegularFile,'r/X');$duplicate.DataStream=[IO.MemoryStream]::new([byte[]]@(120));$writer.WriteEntry($duplicate)}
        } finally {$writer.Dispose();$gzip.Dispose();$stream.Dispose()}
        $expected=MapFiles @(@{path='x';bytes=$(if($kind -eq 'zero-size'){0}else{1})})
        $rejected=$false; try { [void](InspectArchive $archive $expected github) } catch {$rejected=$true}
        if($rejected -ne ($kind -ne 'safe')){throw "Archive self-check failed: $kind"}
    }
    $vector=Join-Path $root 'vector';[IO.File]::WriteAllBytes($vector,[byte[]]@(120))
    $blob=git hash-object -- $vector;if($LASTEXITCODE -ne 0 -or $blob -notmatch '^[0-9a-f]{40}$'){throw 'Git vector failed'}
    $copy=Join-Path $root 'materialize.tgz';[IO.File]::Copy((Join-Path $root 'safe.tgz'),$copy)
    $files=Materialize $copy (MapFiles @(@{path='x';bytes=1;git_blob_sha1=$blob;sha256=(Get-FileHash $vector).Hash.ToLowerInvariant()})) (Join-Path $root 'materialized')
    if($files.Count -ne 1 -or $files[0].git_blob_sha1 -cne $blob){throw 'Materialization vector failed'}
    [void](VerifyRegularFile $vector @{path='x';bytes=1;git_blob_sha1=$blob})
    $rejected=$false;try{[void](VerifyRegularFile $vector @{path='x';bytes=1;git_blob_sha1=('0'*40)})}catch{$rejected=$true};if(-not $rejected){throw 'Wrong raw file pin accepted'}
    SaveReceipt (Join-Path $root 'self-test.json') @{result='PASS';checks='portable paths, regular tar, link/traversal/case collision rejection';network_requests=0}
    Write-Output 'P05 acquisition self-check PASS'; return
}
$inputsPath=Join-Path $PSScriptRoot 'inputs.json'
if((Get-FileHash $inputsPath).Hash.ToLowerInvariant() -cne 'f998fb4e73b022cfc7b50196d471a72a0b7bbb4aabce2996be5f1bf596a5a1e4'){throw 'Approved input projection changed'}
$inputs=Get-Content -LiteralPath $inputsPath -Raw|ConvertFrom-Json
$script:imageReserve=[long]$toolchain.compressed_bytes
if($script:imageReserve -le 0 -or $script:imageReserve -ge $script:acquisitionLimit){throw 'Image reserve missing'}
foreach($name in @('archives','sources','tools','records')){[void][IO.Directory]::CreateDirectory((Join-Path $root $name))}
$results=[Collections.Generic.List[object]]::new(); $extraInputs=@(); $state='FAILED'; $failure=$null
try {
    foreach($repo in @($inputs.repositories)+@($inputs.runtime)) {
        $key=$repo.repository.Replace('/','--')+'--'+$repo.commit
        if($PinnedTsql -and $repo.repository -ceq 'Crary-Systems/tree-sitter-tsql'){
            if($repo.commit -cne '443d2bc774f1d779af7dcabcc99160fb24da96e6' -or $repo.files.Count -ne 21){throw 'Pinned TSQL input set changed'}
            $expected=MapFiles $repo.files
            $portableSet=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
            foreach($path in $expected.Keys){if(-not $portableSet.Add($path)){throw 'Pinned file case collision'}}
            $targetRoot=FreshRoot (Join-Path $root ('sources/'+$key));$files=@()
            foreach($pin in $repo.files){
                $url='https://raw.githubusercontent.com/Crary-Systems/tree-sitter-tsql/443d2bc774f1d779af7dcabcc99160fb24da96e6/'+(($pin.path.Split('/')|ForEach-Object {[Uri]::EscapeDataString($_)}) -join '/')
                $target=Join-Path $targetRoot $pin.path;[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
                if(++$script:filesWritten -gt 5000){throw 'Selected file count limit'}
                [void](Fetch $url $target ([long]$pin.bytes))
                $files+=VerifyRegularFile $target $pin
            }
            $results.Add(@{repository=$repo.repository;commit=$repo.commit;transport='PINNED_RAW_REGULAR_FILES';archive=$null;files=$files;rejected_archive_not_extracted=$true})
            continue
        }
        $archive=Join-Path $root ('archives/'+$key+'.tgz')
        $download=Fetch $repo.url $archive 67108864
        $files=Materialize $archive (MapFiles $repo.files) (Join-Path $root ('sources/'+$key))
        $results.Add(@{repository=$repo.repository;commit=$repo.commit;archive=$download;files=$files})
    }
    foreach($npm in $inputs.npm) {
        $archive=Join-Path $root ('archives/'+$npm.package+'-'+$npm.version+'.tgz')
        $download=Fetch $npm.url $archive 67108864
        $actual='sha512-'+[Convert]::ToBase64String([Convert]::FromHexString((Get-FileHash $archive -Algorithm SHA512).Hash))
        if($actual -cne $npm.integrity){throw 'Npm integrity mismatch'}
        $expected=MapFiles @(@{path='grammar.js'},@{path='package.json'},@{path='LICENSE'})
        $files=Materialize $archive $expected (Join-Path $root ('sources/'+$npm.package+'-'+$npm.version)) npm
        $results.Add(@{package=$npm.package;version=$npm.version;archive=$download;files=$files;install_scripts_executed=0})
    }
    $archive=Join-Path $root 'archives/tree-sitter-linux-x64.gz'
    $download=Fetch $inputs.cli.url $archive $inputs.cli.bytes
    if($download.bytes -ne $inputs.cli.bytes -or $download.sha256 -cne $inputs.cli.sha256){throw 'CLI artifact identity mismatch'}
    $input=[IO.File]::OpenRead($archive);$gzip=[IO.Compression.GZipStream]::new($input,[IO.Compression.CompressionMode]::Decompress);$output=[IO.File]::Open((Join-Path $root 'tools/tree-sitter'),[IO.FileMode]::CreateNew)
    try {$count=0L;$buffer=[byte[]]::new(65536);while(($n=$gzip.Read($buffer,0,$buffer.Length)) -gt 0){$count+=$n;if($count -gt 67108864){throw 'CLI expansion limit'};$output.Write($buffer,0,$n)}}finally{$output.Dispose();$gzip.Dispose();$input.Dispose()}
    if(-not $IsWindows){[IO.File]::SetUnixFileMode((Join-Path $root 'tools/tree-sitter'),[IO.UnixFileMode]493)}
    $results.Add(@{tool='tree-sitter';archive=$download;executable_sha256=(Get-FileHash (Join-Path $root 'tools/tree-sitter')).Hash.ToLowerInvariant()})
    if($RemedyStage -cin @('sql-pg-r1','sql-pg-r2')){
        $taskRoot=[IO.Path]::GetDirectoryName($root)
        $sql=$remedy.sources.sql
        $archive=Join-Path $root 'archives/derek-sql-97614d0.tgz'
        $download=Fetch $sql.provider $archive 8388608
        $files=Materialize $archive (MapFiles $sql.selected_regular_files) (Join-Path $taskRoot 'candidate-evaluation/derek-sql-97614d0')
        $extraInputs+=@{repository=$sql.repository;commit=$sql.revision;task_relative_source_root='candidate-evaluation/derek-sql-97614d0';files=$files;archive=$download;purpose='EVALUATION_ONLY_NO_ADOPTION'}
        $pg=$remedy.sources.postgresql
        $lfsRoot=FreshRoot (Join-Path $taskRoot 'materialized-lfs/postgres')
        [void][IO.Directory]::CreateDirectory((Join-Path $lfsRoot 'src'))
        $objectPath=Join-Path $lfsRoot 'src/parser.c'
        $object=Fetch $pg.provider $objectPath 104857600
        AssertRemedyObject $object.bytes $object.sha256 $pg
        if(++$script:filesWritten -gt 5000){throw 'Selected file count limit'}
        $extraInputs+=@{repository=$pg.repository;commit=$pg.revision;task_relative_source_root='materialized-lfs/postgres';files=@(@{path='src/parser.c';bytes=$object.bytes;sha256=$object.sha256});purpose='EXACT_LFS_OBJECT_ORIGINAL_POINTER_PRESERVED'}
    }
    $state='COMPLETED'
} catch { $failure=$_.Exception.GetType().FullName; throw }
finally {
    $retained=@(Get-ChildItem -LiteralPath $root -File -Recurse|ForEach-Object {@{path=[IO.Path]::GetRelativePath($root,$_.FullName).Replace('\','/');bytes=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName).Hash.ToLowerInvariant()}})
    SaveReceipt (Join-Path $root 'records/acquisition.json') @{state=$state;failure_type=$failure;http_requests=$script:requests;http_limit=$script:httpLimit;pinned_tsql_profile=$PinnedTsql.IsPresent;download_bytes=$script:received;expanded_archive_bytes=$script:expanded;selected_files=$script:filesWritten;wall_seconds=$script:clock.Elapsed.TotalSeconds;downloads=$script:receipts;identities=$results;remedy_stage=$RemedyStage;remedy_approval_subject=$RemedyApprovalSubject;remedy_identities=$extraInputs;retained_files=$retained;partial_materialization_verified=($state -eq 'COMPLETED');source_pin='Git blob SHA-1 + exact size; observed SHA-256';runtime_pin='prior SHA-256 + size';image_compressed_reserve=$script:imageReserve;acquisition_limit_bytes=$script:acquisitionLimit;native_invocations=0;install_scripts=0}
}
