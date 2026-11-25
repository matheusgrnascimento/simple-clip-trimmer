<#
.SYNOPSIS
    Builds Simple Clip Trimmer into build\.

.DESCRIPTION
    1. Builds build\Simple Clip Trimmer.exe with cgo (needs GCC in PATH and
       the libmpv files in third_party\libmpv, see setup.ps1).
    2. Writes the Windows resources with tools\winres: app.manifest in place
       of the MinGW default manifest (Common Controls v6 and DPI awareness,
       which the walk UI library requires), the app icon, assets\icon.ico, and
       the version information (the version is in the VERSION file; Task
       Manager lists the process by its file description).
    3. Copies libmpv-2.dll next to the executable.

.PARAMETER Console
    Keeps the console window, to see logs and panics.
#>
[CmdletBinding()]
param([switch]$Console)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$env:CGO_ENABLED = "1"

$mpvDll = "third_party\libmpv\libmpv-2.dll"
if (-not (Test-Path $mpvDll)) {
    throw "$mpvDll not found. Run setup.ps1 first."
}

$out = "build\Simple Clip Trimmer.exe"
$ldflags = "-s -w"
if (-not $Console) { $ldflags += " -H windowsgui" }

go build -trimpath -ldflags $ldflags -o $out .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$version = (Get-Content VERSION -Raw).Trim()
go run ./tools/winres -manifest app.manifest -icon assets\icon.ico `
    -version $version -description "Simple Clip Trimmer" -product "Simple Clip Trimmer" `
    -company "Matheus Nascimento" `
    -copyright "Copyright (c) 2026 Matheus Nascimento. MIT License." $out
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Copy-Item $mpvDll build\ -Force
Write-Host "Built $(Join-Path $PSScriptRoot $out)"
