@echo off
echo 正在构建温度桥接程序...

cd /d "%~dp0bridge\TempBridge"

echo 还原NuGet包...
dotnet restore TempBridge.csproj

echo 编译发布版本...
dotnet publish TempBridge.csproj -c Release -r win-x64 --self-contained false

cd ..\..

echo 复制编译产物所有文件到/build/bin/bridge

mkdir build\bin\bridge 2>nul

copy /y bridge\TempBridge\bin\Release\net4.7.2\win-x64\publish\* build\bin\bridge\

pause
