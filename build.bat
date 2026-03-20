@echo off
setlocal enabledelayedexpansion

echo Synchronizing Versions across project...
powershell -NoProfile -ExecutionPolicy Bypass -File "scripts\sync_version.ps1"

echo Building BS2PRO-Controller...

REM 从 wails.json 提取版本号
for /f "tokens=2 delims=:, " %%a in ('findstr /C:"\"productVersion\"" wails.json') do (
    set DISPLAY_VERSION=%%a
    set DISPLAY_VERSION=!DISPLAY_VERSION:"=!
)

if "!DISPLAY_VERSION!"=="" (
    echo WARNING: Could not extract version from wails.json, using dev
    set DISPLAY_VERSION=dev
)


REM 分离前后端编译参数
set CORE_LDFLAGS=-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=!DISPLAY_VERSION! -s -w
set WAILS_LDFLAGS=-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=!DISPLAY_VERSION!

REM Build core service first
echo Building core service...
if exist "cmd\core\winres\winres.json" (
    go-winres make --in cmd/core/winres/winres.json --out cmd/core/rsrc
)
go build -trimpath -ldflags "!CORE_LDFLAGS!" -o build/bin/BS2PRO-CoreService.exe ./cmd/core/

REM Build monitor probe
echo Building monitor probe...
if exist "cmd\bs2pro-monitor\winres\winres.json" (
    go-winres make --in cmd/bs2pro-monitor/winres/winres.json --out cmd/bs2pro-monitor/rsrc
)
go build -trimpath -ldflags "!CORE_LDFLAGS!" -o build/bin/BS2PRO-Monitor.exe ./cmd/bs2pro-monitor/

REM Add NSIS to PATH for installer creation
set PATH=%PATH%;C:\Program Files (x86)\NSIS\Bin

REM Build main application with wails
echo Building main application...
wails build -nsis -trimpath -webview2 browser -ldflags "!WAILS_LDFLAGS!"

REM Ensure core service and monitor are in the bin directory for installer
if exist "build\bin\BS2PRO-CoreService.exe" (
    echo Core service built successfully
) else (
    echo ERROR: Core service build failed!
    exit /b 1
)

if exist "build\bin\BS2PRO-Monitor.exe" (
    echo Monitor probe built successfully
) else (
    echo ERROR: Monitor probe build failed!
    exit /b 1
)

echo Build completed successfully with display version !DISPLAY_VERSION!
endlocal
