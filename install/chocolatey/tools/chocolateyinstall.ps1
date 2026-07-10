$ErrorActionPreference = 'Stop'

$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$repo    = 'SomeoneUnlicensed/kurto'
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$version = $release.tag_name
$zipUrl  = "https://github.com/$repo/releases/download/$version/kurto-windows-amd64.zip"

$packageArgs = @{
  PackageName   = 'kurto'
  UnzipLocation = $toolsDir
  Url64bit      = $zipUrl
  Checksum64    = ''
  ChecksumType64= 'sha256'
}

Install-ChocolateyZipPackage @packageArgs
