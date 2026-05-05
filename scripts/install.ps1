param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\pim",
    [switch]$Dev
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
if ($Dev) {
    $releases = Invoke-WebRequest -Uri "https://api.github.com/repos/$repo/releases?per_page=100" -UseBasicParsing | ConvertFrom-Json
    $pre = $releases |
        Where-Object { $_.prerelease -eq $true } |
        Sort-Object {
            $t = $_.tag_name
            $base = [System.Version]($t -replace '^v' -replace '-.*')
            $n = if ($t -match '-dev\.(\d+)-') { [int]$Matches[1] } else { 0 }
            [tuple]::Create($base, $n)
        } -Descending |
        Select-Object -First 1
    if (-not $pre) {
        Write-Error "No pre-release found for $repo"
        exit 1
    }
    $Version = $pre.tag_name
    $downloadUrl = "https://github.com/$repo/releases/download/$Version/$asset"
} elseif ($Version -eq "latest") {
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
