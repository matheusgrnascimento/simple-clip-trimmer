<#
.SYNOPSIS
    Builds the Windows installer (build\simple-clip-trimmer-<version>.msi).
.DESCRIPTION
    Runs build.ps1, then packages build\simple-clip-trimmer.exe and
    libmpv-2.dll with the WiX Toolset v5 (the wix dotnet tool, plus the
    WixToolset.UI.wixext extension). FFmpeg is not bundled; it must be in PATH.
.PARAMETER Version
    Installer version, as major.minor.build.
#>
[CmdletBinding()]
param([string]$Version = "1.0.0")
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
if (-not (Get-Command wix -ErrorAction SilentlyContinue)) {
    throw "wix not found. Run: dotnet tool install --global wix --version 5.0.2"
}
.\build.ps1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# WixUI needs an RTF license; build one from LICENSE.
$text = (Get-Content LICENSE -Raw) -replace '\\', '\\' -replace '[{}]', '' -replace "`r?`n", '\par '
Set-Content installer\license.rtf "{\rtf1\ansi\deff0 $text}" -Encoding ascii

$msi = "build\simple-clip-trimmer-$Version.msi"
wix build installer\Product.wxs -ext WixToolset.UI.wixext -d Version=$Version -arch x64 -o $msi
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Built $(Join-Path $PSScriptRoot $msi)"
