<#
.SYNOPSIS
    Sets up the Windows environment and builds Simple Clip Trimmer.

.DESCRIPTION
    Runs, end to end and from PowerShell:
      1. Installs Go, the MinGW-w64 GCC (WinLibs, for cgo) and FFmpeg with
         winget, skipping the ones already in PATH.
      2. Downloads the libmpv development files (DLL, import library and
         headers) into third_party\libmpv. The download is about 30 MB.
      3. Builds the app with build.ps1.

    The libmpv build is pinned so every machine builds against the same API.

    Run it from the project folder:
        powershell -ExecutionPolicy Bypass -File .\setup.ps1

.PARAMETER LibmpvUrl
    libmpv "mpv-dev-x86_64" 7z archive to download. Default: a pinned build
    from shinchiro/mpv-winbuild-cmake. If the link stops working, pick another
    "mpv-dev-x86_64-<date>-git-<hash>.7z" file at
    https://github.com/shinchiro/mpv-winbuild-cmake/releases and pass it here.

.PARAMETER SkipInstall
    Skips installing the tools with winget (use it if everything is already installed).

.PARAMETER SkipBuild
    Skips the build step.

.PARAMETER Force
    Downloads libmpv again even if third_party\libmpv already exists.
#>
[CmdletBinding()]
param(
    [string]$LibmpvUrl = "https://github.com/shinchiro/mpv-winbuild-cmake/releases/download/20261111/mpv-dev-x86_64-20261111-git-bcde6b93cc.7z",
    [switch]$SkipInstall,
    [switch]$SkipBuild,
    [switch]$Force
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

function Update-SessionPath {
    $env:Path = [Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
                [Environment]::GetEnvironmentVariable("Path", "User")
}

function Install-IfMissing([string]$Command, [string]$WingetId) {
    if (Get-Command $Command -ErrorAction SilentlyContinue) {
        Write-Host "$Command found: $((Get-Command $Command).Source)"
        return
    }
    Write-Host "Installing $WingetId..."
    winget install --id $WingetId --exact --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { throw "winget failed to install $WingetId" }
    Update-SessionPath
}

# 1. Tools
if (-not $SkipInstall) {
    Install-IfMissing "go" "GoLang.Go"
    Install-IfMissing "gcc" "BrechtSanders.WinLibs.POSIX.UCRT"
    Install-IfMissing "ffmpeg" "Gyan.FFmpeg"
}
Update-SessionPath

# 2. libmpv
$mpvDir = Join-Path $PSScriptRoot "third_party\libmpv"
if ($Force -or -not (Test-Path (Join-Path $mpvDir "libmpv-2.dll"))) {
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("libmpv-" + [Guid]::NewGuid())
    New-Item -ItemType Directory $tmp | Out-Null
    try {
        $archive = Join-Path $tmp "mpv-dev.7z"
        Write-Host "Downloading $LibmpvUrl..."
        Invoke-WebRequest -Uri $LibmpvUrl -OutFile $archive
        # The tar.exe that ships with Windows 11 reads 7z archives.
        tar -xf $archive -C $tmp
        if ($LASTEXITCODE -ne 0) { throw "could not extract $archive" }

        if (Test-Path $mpvDir) { Remove-Item $mpvDir -Recurse -Force }
        New-Item -ItemType Directory $mpvDir | Out-Null
        Copy-Item (Join-Path $tmp "libmpv-2.dll"), (Join-Path $tmp "libmpv.dll.a") $mpvDir
        Copy-Item (Join-Path $tmp "include") $mpvDir -Recurse
    }
    finally {
        Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
    Write-Host "libmpv ready in $mpvDir"
}
else {
    Write-Host "libmpv already in $mpvDir (use -Force to download it again)"
}

# 3. Build
if (-not $SkipBuild) {
    & (Join-Path $PSScriptRoot "build.ps1")
}
