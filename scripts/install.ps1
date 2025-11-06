param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\pim"
)

$ErrorActionPreference = "Stop"
$repo = "jeircul/pim"
$binary = "pim"
$os = "windows"

$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
switch ($architecture) {
    "X64" { $arch = "amd64" }
    "Arm64" { $arch = "arm64" }
    default {
        Write-Error "Unsupported architecture: $architecture"
        exit 1
    }
}

$asset = "{0}_{1}_{2}.zip" -f $binary, $os, $arch
if ($Version -eq "latest") {
    $downloadUrl = "https://github.com/$repo/releases/latest/download/$asset"
} else {
    if ($Version -notmatch '^v') {
        $Version = "v$Version"
    }
    $downloadUrl = "https://github.com/$repo/releases/download/$Version/$asset"
}

$temp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $temp -Force | Out-Null
$zipPath = Join-Path $temp $asset

try {
    Write-Host "Downloading $downloadUrl"
    Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath -UseBasicParsing

    Expand-Archive -Path $zipPath -DestinationPath $temp -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $temp ("{0}.exe" -f $binary)) -Destination (Join-Path $InstallDir ("{0}.exe" -f $binary)) -Force

    Write-Host "Installed $binary to $InstallDir"
    Write-Host "Add $InstallDir to your PATH if it is not already."
    Write-Host "Sign in with 'az login' or 'Connect-AzAccount' before using pim. Set PIM_ALLOW_DEVICE_LOGIN=true to allow device-code fallback."
}
finally {
    Remove-Item -LiteralPath $temp -Recurse -Force
}
