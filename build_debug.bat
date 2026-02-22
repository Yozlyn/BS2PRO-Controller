@echo off
setlocal enabledelayedexpansion
echo Building BS2PRO-Controller (DEBUG VERSION)...

REM 提取版本号
for /f "tokens=2 delims=:, " %%a in ('findstr /C:"\"productVersion\"" wails.json') do (
    set VERSION=%%a
    set VERSION=!VERSION:"=!
)

if "!VERSION!"=="" (
    set VERSION=dev
)

set CORE_LDFLAGS=-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=!VERSION!-debug
set WAILS_LDFLAGS=-X github.com/TIANLI0/BS2PRO-Controller/internal/version.BuildVersion=!VERSION!-debug

echo [1/2] Building core service (DEBUG)...
if exist "cmd\core\winres\winres.json" (
    go-winres make --in cmd/core/winres/winres.json --out cmd/core/rsrc
)
go build -ldflags "!CORE_LDFLAGS!" -o build/bin/BS2PRO-CoreService.exe ./cmd/core/

echo [2/2] Building main application (DEBUG)...
REM Wails 会生成带F12开发者工具的程序
wails build -debug -ldflags "!WAILS_LDFLAGS!"

echo =======================================================
echo Debug Build completed successfully!
echo You can find the debug executables in the build\bin folder.
echo =======================================================
pause
endlocal