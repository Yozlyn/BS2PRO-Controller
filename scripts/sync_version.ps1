$ErrorActionPreference = 'Stop'
$Utf8NoBom = New-Object System.Text.UTF8Encoding $false
$Utf8Bom = New-Object System.Text.UTF8Encoding $true

# 读取 wails.json 获取基准版本号
$wailsJson = Get-Content -Raw -Encoding UTF8 'wails.json'
if ($wailsJson -match '"productVersion"\s*:\s*"(.*?)"') {
    $displayVer = $matches[1]
} else {
    $displayVer = 'dev'
    Write-Host "Warning: Could not find productVersion in wails.json"
}

# 将字符串版本转换为Windows强制的4位纯数字版本
$numericVer = '1.0.0.0'
if ($displayVer -match '^(\d+\.\d+\.\d+)(?:-r(\d+))?') {
    $base = $matches[1]
    $rev = if ($matches[2]) { $matches[2] } else { '0' }
    $numericVer = "$base.$rev"
} elseif ($displayVer -match '^(\d+\.\d+\.\d+\.\d+)') {
    $numericVer = $matches[1]
}

Write-Host "String Version  : $displayVer"
Write-Host "Numeric Version : $numericVer"

# 更新前端主程序配置
$infoPath = 'build/windows/info.json'
if (Test-Path $infoPath) {
    $info = Get-Content -Raw -Encoding UTF8 $infoPath
    $info = $info -replace '"fileVersion"\s*:\s*".*?"', "`"fileVersion`": `"$numericVer`""
    $info = $info -replace '"productVersion"\s*:\s*".*?"', "`"productVersion`": `"$displayVer`""
    [IO.File]::WriteAllText((Get-Item $infoPath).FullName, $info, $Utf8NoBom)
    Write-Host "Updated $infoPath"
}

# 更新核心守护服务配置
$winresPath = 'cmd/core/winres/winres.json'
if (Test-Path $winresPath) {
    $winres = Get-Content -Raw -Encoding UTF8 $winresPath
    $winres = $winres -replace '"file_version"\s*:\s*".*?"', "`"file_version`": `"$numericVer`""
    $winres = $winres -replace '"product_version"\s*:\s*".*?"', "`"product_version`": `"$numericVer`""
    $winres = $winres -replace '"FileVersion"\s*:\s*".*?"', "`"FileVersion`": `"$displayVer`""
    $winres = $winres -replace '"ProductVersion"\s*:\s*".*?"', "`"ProductVersion`": `"$displayVer`""
    [IO.File]::WriteAllText((Get-Item $winresPath).FullName, $winres, $Utf8NoBom)
    Write-Host "Updated $winresPath"
}

# 更新安装包配置文件
$nsiPath = 'build/windows/installer/project.nsi'
if (Test-Path $nsiPath) {
    $nsi = Get-Content -Raw -Encoding UTF8 $nsiPath
    $nsi = $nsi -replace '(?m)^VIProductVersion\s+".*?"', "VIProductVersion `"$numericVer`""
    $nsi = $nsi -replace '(?m)^VIFileVersion\s+".*?"', "VIFileVersion `"$numericVer`""
    [IO.File]::WriteAllText((Get-Item $nsiPath).FullName, $nsi, $Utf8Bom)
    Write-Host "Updated $nsiPath"
}