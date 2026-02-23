@echo off
setlocal enabledelayedexpansion

REM 版本后缀(例如 -r1, -beta)，不需要，就留空
set VERSION_SUFFIX=-r1

echo Building BS2PRO-Controller...

REM 从 wails.json 提取版本号
for /f "tokens=2 delims=:, " %%a in ('findstr /C:"\"productVersion\"" wails.json') do (
    set BASE_VERSION=%%a
    set BASE_VERSION=!BASE_VERSION:"=!
)

if "!BASE_VERSION!"=="" (
    echo WARNING: Could not extract version from wails.json, using dev
    set BASE_VERSION=dev
)

REM 基础版本号 + 后缀 = 2.6.0-r1
set DISPLAY_VERSION=!BASE_VERSION!!VERSION_SUFFIX!
echo Building physical version: !BASE_VERSION!
echo Building display version : !DISPLAY_VERSION!

REM 分离前后端编译参数
set CORE_LDFLAGS=-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=!DISPLAY_VERSION! -s -w
set WAILS_LDFLAGS=-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=!DISPLAY_VERSION!

REM Build core service first
echo Building core service...
if exist "cmd\core\winres\winres.json" (
    go-winres make --in cmd/core/winres/winres.json --out cmd/core/rsrc
)
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

echo Build completed successfully with display version !DISPLAY_VERSION!
echo endlocal