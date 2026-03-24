$ErrorActionPreference = 'Stop'
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir

Set-Location $repoRoot

Write-Host 'Synchronizing Versions across project...'
& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $scriptDir 'sync_version.ps1')

Write-Host 'Building BS2PRO-Controller...'

$displayVersion = ''
$wailsJson = Get-Content -LiteralPath 'wails.json' -Raw
if ($wailsJson -match '"productVersion"\s*:\s*"([^"]+)"') {
    $displayVersion = $Matches[1]
}

if ([string]::IsNullOrWhiteSpace($displayVersion)) {
    Write-Warning 'Could not extract version from wails.json, using dev'
    $displayVersion = 'dev'
}

$coreLdflags = "-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=$displayVersion -s -w"
$monitorLdflags = "$coreLdflags -H windowsgui"
$wailsLdflags = "-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=$displayVersion"

Write-Host 'Building core service...'
if (Test-Path 'cmd/core/winres/winres.json') {
    & go-winres make --in cmd/core/winres/winres.json --out cmd/core/rsrc
}
& go build -trimpath -ldflags $coreLdflags -o 'build/bin/BS2PRO-CoreService.exe' './cmd/core/'

Write-Host 'Building monitor probe...'
if (Test-Path 'cmd/bs2pro-monitor/winres/winres.json') {
    & go-winres make --in cmd/bs2pro-monitor/winres/winres.json --out cmd/bs2pro-monitor/rsrc
}
& go build -trimpath -ldflags $monitorLdflags -o 'build/bin/BS2PRO-Monitor.exe' './cmd/bs2pro-monitor/'

$env:PATH = "$env:PATH;C:\Program Files (x86)\NSIS\Bin"

Write-Host 'Building main application...'
& wails build -nsis -trimpath -webview2 browser -ldflags $wailsLdflags

if (-not (Test-Path 'build/bin/BS2PRO-CoreService.exe')) {
    throw 'Core service build failed!'
}

if (-not (Test-Path 'build/bin/BS2PRO-Monitor.exe')) {
    throw 'Monitor probe build failed!'
}

Write-Host "Build completed successfully with display version $displayVersion"
