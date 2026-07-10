#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Installs kurto on Windows.
.DESCRIPTION
    Downloads the latest kurto release and adds it to PATH.
.PARAMETER Version
    Version to install (default: latest).
.PARAMETER InstallDir
    Installation directory (default: $env:USERPROFILE\.kurto\bin).
.PARAMETER AddToPath
    Add kurto to PATH (default: true).
.EXAMPLE
    .\install.ps1
    .\install.ps1 -Version v0.1.0
    .\install.ps1 -InstallDir "C:\tools\kurto"
#>

param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:USERPROFILE\.kurto\bin",
    [switch]$AddToPath = $true
)

$Repo = "SomeoneUnlicensed/kurto"
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }

if ($Version -eq "latest") {
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $release.tag_name
}

$zipName = "kurto-windows-$Arch.zip"
$downloadUrl = "https://github.com/$Repo/releases/download/$Version/$zipName"

Write-Host "Downloading kurto $Version ($Arch)..." -ForegroundColor Cyan
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null

$zipPath = "$env:TEMP\kurto-$Version.zip"
Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath

Expand-Archive -Path $zipPath -DestinationPath $InstallDir -Force
Remove-Item $zipPath -Force

$exePath = Join-Path $InstallDir "kurto-windows-$Arch.exe"
$targetPath = Join-Path $InstallDir "kurto.exe"
if (Test-Path $exePath) {
    Move-Item -Path $exePath -Destination $targetPath -Force
}

if ($AddToPath) {
    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($currentPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$currentPath;$InstallDir", "User")
        $env:PATH = "$env:PATH;$InstallDir"
        Write-Host "Added $InstallDir to PATH" -ForegroundColor Green
    }
}

Write-Host "kurto $Version installed to $InstallDir" -ForegroundColor Green
Write-Host "Run 'kurto --help' to get started." -ForegroundColor Green
