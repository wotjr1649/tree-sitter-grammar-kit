param([Parameter(Mandatory)][string]$Destination)
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion.Major -ne 7){throw 'PowerShell 7 required'}
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
$function=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Application'},$true))
if($function.Count -ne 1){throw 'Application helper identity mismatch'}
. ([scriptblock]::Create($function[0].Extent.Text))
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
@{result='PASS';checks=@('duplicate PATH applications choose first exact path','missing application rejected');fixture_processes_executed=0;run_sha256=(Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'run.ps1')).Hash.ToLowerInvariant()}|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $root 'self-test.json') -Encoding utf8NoBOM
Write-Output 'P05 application discovery self-check PASS; fixture execution 0'
