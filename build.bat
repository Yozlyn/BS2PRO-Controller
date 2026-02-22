@echo off
setlocal enabledelayedexpansion
echo Building BS2PRO-Controller...

REM Extract version from wails.json
for /f "tokens=2 delims=:, " %%a in ('findstr /C:"\"productVersion\"" wails.json') do (
    set VERSION=%%a
    set VERSION=!VERSION:"=!
)

if "!VERSION!"=="" (
    echo WARNING: Could not extract version from wails.json, using dev
    set VERSION=dev
) else (
    echo Building version: !VERSION!
)

REM 分离前后端编译参数
set CORE_LDFLAGS=-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=!VERSION! -s -w
REM Wails默认会处理GUI隐藏
set WAILS_LDFLAGS=-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=!VERSION!

REM Build core service first
echo Building core service...
if exist "cmd\core\winres\winres.json" (
    go-winres make --in cmd/core/winres/winres.json --out cmd/core/rsrc
)
REM 规范命名
go build -ldflags "!CORE_LDFLAGS!" -o build/bin/BS2PRO-CoreService.exe ./cmd/core/

REM Add NSIS to PATH for installer creation
set PATH=%PATH%;C:\Program Files (x86)\NSIS\Bin

REM Build main application with wails
echo Building main application...
wails build -nsis -ldflags "!WAILS_LDFLAGS!"

REM Ensure core service is in the bin directory for installer
if exist "build\bin\BS2PRO-CoreService.exe" (
    echo Core service built successfully
) else (
    echo ERROR: Core service build failed!
    exit /b 1
)

echo Build completed successfully with version !VERSION!
echo endlocal