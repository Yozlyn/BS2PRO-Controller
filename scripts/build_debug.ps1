$ErrorActionPreference = 'Stop'
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir

Set-Location $repoRoot

Write-Host 'Synchronizing Versions across project...'
& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $scriptDir 'sync_version.ps1')

Write-Host 'Building BS2PRO-Controller (DEBUG VERSION)...'

$displayVersion = ''
$wailsJson = Get-Content -LiteralPath 'wails.json' -Raw
if ($wailsJson -match '"productVersion"\s*:\s*"([^"]+)"') {
    $displayVersion = $Matches[1]
}

if ([string]::IsNullOrWhiteSpace($displayVersion)) {
    $displayVersion = 'dev'
}

$coreLdflags = "-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=$displayVersion-debug"
$monitorLdflags = "$coreLdflags -H windowsgui"
$wailsLdflags = $coreLdflags

Write-Host '[1/3] Building core service (DEBUG)...'
if (Test-Path 'cmd/core/winres/winres.json') {
    & go-winres make --in cmd/core/winres/winres.json --out cmd/core/rsrc
}
& go build -ldflags $coreLdflags -o 'build/bin/BS2PRO-CoreService.exe' './cmd/core/'

Write-Host '[2/3] Building monitor probe (DEBUG)...'
if (Test-Path 'cmd/bs2pro-monitor/winres/winres.json') {
    & go-winres make --in cmd/bs2pro-monitor/winres/winres.json --out cmd/bs2pro-monitor/rsrc
}
& go build -trimpath -ldflags $monitorLdflags -o 'build/bin/BS2PRO-Monitor.exe' './cmd/bs2pro-monitor/'

Write-Host '[3/3] Building main application (DEBUG)...'
& wails build -debug -ldflags $wailsLdflags

Write-Host '======================================================='
Write-Host 'Debug Build completed successfully!'
Write-Host 'You can find the debug executables in the build/bin folder.'
Write-Host '======================================================='
