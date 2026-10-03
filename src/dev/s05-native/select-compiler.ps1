#Requires -Version 7
# S05 development helper: selects the host's existing C compiler for the native driver
# build (no installation) and records its identity. Prints the identity line to the host
# and returns the absolute path. Windows prefers an existing MinGW gcc, then LLVM clang;
# Linux uses /usr/bin/gcc; macOS uses /usr/bin/clang.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$candidates = if ($IsWindows) {
  @('C:\mingw64\bin\gcc.exe', 'C:\msys64\ucrt64\bin\gcc.exe', 'C:\msys64\mingw64\bin\gcc.exe', 'C:\Program Files\LLVM\bin\clang.exe')
} elseif ($IsMacOS) { @('/usr/bin/clang') } else { @('/usr/bin/gcc') }
$cc = $candidates | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } | Select-Object -First 1
if (-not $cc) { throw "no existing C compiler among: $($candidates -join ', ')" }
$version = & $cc --version | Select-Object -First 1
if ($LASTEXITCODE -ne 0) { throw 'compiler --version failed' }
$item = Get-Item -LiteralPath $cc
if ($item.LinkTarget) { $cc = $item.ResolveLinkTarget($true).FullName } # report and return the file that runs
$sha = (Get-FileHash -LiteralPath $cc -Algorithm SHA256).Hash.ToLowerInvariant()
Write-Host ("compiler path={0} sha256={1} bytes={2} version={3}" -f $cc, $sha, (Get-Item -LiteralPath $cc).Length, $version)
return $cc
