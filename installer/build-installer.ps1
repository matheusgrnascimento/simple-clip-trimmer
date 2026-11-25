<#
.SYNOPSIS
    Builds the Simple Clip Trimmer MSI installer.

.DESCRIPTION
    1. Builds the app with build.ps1.
    2. Stages the executable, libmpv and the license.
    3. Builds dist\SimpleClipTrimmer-<version>-x64.msi with the WiX Toolset.

    The version comes from the VERSION file. FFmpeg is not bundled; it must
    be in PATH on the machine that runs the app.

    Requires WiX Toolset 5 with the UI and Util extensions:
        dotnet tool install --global wix --version 5.0.2
        wix extension add -g WixToolset.UI.wixext/5.0.2
        wix extension add -g WixToolset.Util.wixext/5.0.2
#>
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Set-Location $root

if (-not (Get-Command wix -ErrorAction SilentlyContinue)) {
    throw "wix not found. Run: dotnet tool install --global wix --version 5.0.2"
}

$version = (Get-Content VERSION -Raw).Trim()
$stage = Join-Path $root "build\installer-stage"
$dist = Join-Path $root "dist"
$msi = Join-Path $dist "SimpleClipTrimmer-$version-x64.msi"

Write-Host "==> Building $version" -ForegroundColor Cyan
& (Join-Path $root "build.ps1")
if ($LASTEXITCODE -ne 0) { throw "build failed." }

Write-Host "==> Staging files" -ForegroundColor Cyan
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Path $stage -Force | Out-Null
Copy-Item "build\Simple Clip Trimmer.exe" $stage
Copy-Item "build\libmpv-2.dll" $stage
Copy-Item "assets\icon.ico" $stage
Copy-Item "LICENSE" "$stage\LICENSE.txt"

# License page of the installer (RTF).
$escape = { param($s) $s.Replace('\', '\\').Replace('{', '\{').Replace('}', '\}') }
$body = (Get-Content LICENSE | ForEach-Object { (& $escape $_) + "\par" }) -join "`n"
"{\rtf1\ansi\deff0{\fonttbl{\f0 Segoe UI;}}\fs18`n$body`n}" | Set-Content "$stage\license.rtf" -Encoding ascii

Write-Host "==> Building MSI" -ForegroundColor Cyan
New-Item -ItemType Directory -Path $dist -Force | Out-Null
wix build "installer\Package.wxs" -arch x64 -ext WixToolset.UI.wixext -ext WixToolset.Util.wixext `
    -d "Version=$version" -d "StageDir=$stage" -o $msi
if ($LASTEXITCODE -ne 0) { throw "wix build failed." }

Write-Host "==> $msi ($([math]::Round((Get-Item $msi).Length / 1MB, 1)) MB)" -ForegroundColor Green
