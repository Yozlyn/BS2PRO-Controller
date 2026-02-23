Unicode true

!define UNINST_KEY_NAME "BS2PRO-Controller"

!include "wails_tools.nsh"
!include "MUI.nsh"
!include "FileFunc.nsh"
!include "DotNetChecker.nsh"

VIProductVersion "1.0.0.0"
VIFileVersion    "1.0.0.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE 
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "启动BS2PRO控制台"
!define MUI_FINISHPAGE_RUN_FUNCTION "LaunchAsNormalUser"
!define MUI_ABORTWARNING 

!insertmacro MUI_PAGE_WELCOME 
!insertmacro MUI_PAGE_DIRECTORY 
!insertmacro MUI_PAGE_COMPONENTS 
!insertmacro MUI_PAGE_INSTFILES 
!insertmacro MUI_PAGE_FINISH 

!insertmacro MUI_UNPAGE_INSTFILES 
!insertmacro MUI_LANGUAGE "SimpChinese" 

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" 
InstallDir "$PROGRAMFILES64\${INFO_PRODUCTNAME}" 
ShowInstDetails show 

Function .onInit
   !insertmacro wails.checkArchitecture
   !insertmacro CheckNetFramework 472
   Pop $0
   ${If} $0 == "false"
       MessageBox MB_OK|MB_ICONSTOP "需要 .NET Framework 4.7.2 或更高版本。$\n$\n请先安装 .NET Framework 4.7.2。"
       Abort
   ${EndIf}
   Call DetectExistingInstallation
FunctionEnd

Function CleanLegacyRegistryKeys
    SetRegView 64
    Push $R0
    Push $R1
    ReadRegStr $R0 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\BS2PRO-controllerBS2PRO-controller" "UninstallString"
    ${If} $R0 != ""
        DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\BS2PRO-controllerBS2PRO-controller"
    ${EndIf}
    ReadRegStr $R0 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TIANLI0BS2PRO-Controller" "UninstallString"
    ${If} $R0 != ""
        DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TIANLI0BS2PRO-Controller"
    ${EndIf}
    ReadRegStr $R0 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TIANLI0BS2PRO" "UninstallString"
    ${If} $R0 != ""
        DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TIANLI0BS2PRO"
    ${EndIf}
    Pop $R1
    Pop $R0
FunctionEnd

Function DetectExistingInstallation
    SetRegView 64
    Push $R0
    Push $R1
    Push $R2
    
    ReadRegStr $R0 HKLM "${UNINST_KEY}" "InstallLocation"
    ${If} $R0 != ""
        ${If} ${FileExists} "$R0\${PRODUCT_EXECUTABLE}"
            StrCpy $INSTDIR $R0
            Goto found_installation
        ${EndIf}
    ${EndIf}

    ${If} ${FileExists} "$PROGRAMFILES64\${INFO_PRODUCTNAME}\${PRODUCT_EXECUTABLE}"
        StrCpy $INSTDIR "$PROGRAMFILES64\${INFO_PRODUCTNAME}"
        Goto found_installation
    ${EndIf}
    
    StrCpy $INSTDIR "$PROGRAMFILES64\BS2PRO-Controller"
    Goto end_detection
    
    found_installation:
    Call CleanLegacyRegistryKeys
    
    end_detection:
    Pop $R2
    Pop $R1
    Pop $R0
FunctionEnd

Function TrimQuotes
    Exch $R0
    Push $R1
    Push $R2
    StrCpy $R1 $R0 1
    StrCmp $R1 '"' 0 +2
    StrCpy $R0 $R0 "" 1
    StrLen $R2 $R0
    IntOp $R2 $R2 - 1
    StrCpy $R1 $R0 1 $R2
    StrCmp $R1 '"' 0 +2
    StrCpy $R0 $R0 $R2
    Pop $R2
    Pop $R1
    Exch $R0
FunctionEnd

Function StopRunningInstances
    DetailPrint "正在检查并停止运行中的进程..."
    
    # 检查并停止 GUI 程序
    DetailPrint "检查 ${PRODUCT_EXECUTABLE} 进程..."
    nsExec::ExecToStack '"$SYSDIR\tasklist.exe" /FI "IMAGENAME eq ${PRODUCT_EXECUTABLE}"'
    Pop $0
    Pop $1
    ${If} $0 == 0
        # 进程存在，尝试终止
        DetailPrint "正在停止 ${PRODUCT_EXECUTABLE}..."
        nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "${PRODUCT_EXECUTABLE}" /T'
        Pop $0
        Pop $1
        ${If} $0 == 0
            Sleep 300
        ${EndIf}
    ${Else}
        DetailPrint "${PRODUCT_EXECUTABLE} 进程不存在，跳过终止"
    ${EndIf}
    
    # 检查并停止核心服务
    DetailPrint "检查 BS2PRO-CoreService.exe 进程..."
    nsExec::ExecToStack '"$SYSDIR\tasklist.exe" /FI "IMAGENAME eq BS2PRO-CoreService.exe"'
    Pop $0
    Pop $1
    ${If} $0 == 0
        # 进程存在，尝试终止
        DetailPrint "正在停止 BS2PRO-CoreService.exe..."
        nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "BS2PRO-CoreService.exe" /T'
        Pop $0
        Pop $1
        ${If} $0 == 0
            Sleep 300
        ${EndIf}
    ${Else}
        DetailPrint "BS2PRO-CoreService.exe 进程不存在，跳过终止"
    ${EndIf}
    
    # 清理计划任务（如果存在）
    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        DetailPrint "清理计划任务..."
        nsExec::ExecToStack '"$SYSDIR\schtasks.exe" /delete /tn "BS2PRO-Controller" /f'
        Pop $0
        Pop $1
        nsExec::ExecToStack '"$SYSDIR\schtasks.exe" /delete /tn "BS2PRO-Core" /f'
        Pop $0
        Pop $1
    ${EndIf}
    
    DetailPrint "进程停止完成"
FunctionEnd

Function un.StopRunningInstances
    DetailPrint "正在检查并停止运行中的进程..."
    
    # 检查并停止 GUI 程序
    DetailPrint "检查 ${PRODUCT_EXECUTABLE} 进程..."
    nsExec::ExecToStack '"$SYSDIR\tasklist.exe" /FI "IMAGENAME eq ${PRODUCT_EXECUTABLE}"'
    Pop $0
    Pop $1
    ${If} $0 == 0
        # 进程存在，尝试终止
        DetailPrint "正在停止 ${PRODUCT_EXECUTABLE}..."
        nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "${PRODUCT_EXECUTABLE}" /T'
        Pop $0
        Pop $1
        ${If} $0 == 0
            Sleep 300
        ${EndIf}
    ${Else}
        DetailPrint "${PRODUCT_EXECUTABLE} 进程不存在，跳过终止"
    ${EndIf}
    
    # 检查并停止核心服务
    DetailPrint "检查 BS2PRO-CoreService.exe 进程..."
    nsExec::ExecToStack '"$SYSDIR\tasklist.exe" /FI "IMAGENAME eq BS2PRO-CoreService.exe"'
    Pop $0
    Pop $1
    ${If} $0 == 0
        # 进程存在，尝试终止
        DetailPrint "正在停止 BS2PRO-CoreService.exe..."
        nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "BS2PRO-CoreService.exe" /T'
        Pop $0
        Pop $1
        ${If} $0 == 0
            Sleep 300
        ${EndIf}
    ${Else}
        DetailPrint "BS2PRO-CoreService.exe 进程不存在，跳过终止"
    ${EndIf}
    
    DetailPrint "进程停止完成"
FunctionEnd

Function BackupUserData
    ${If} ${FileExists} "$INSTDIR\config.json"
        CopyFiles "$INSTDIR\config.json" "$TEMP\bs2pro_config_backup.json"
    ${EndIf}
FunctionEnd

Function RestoreUserData
    ${If} ${FileExists} "$TEMP\bs2pro_config_backup.json"
        CopyFiles "$TEMP\bs2pro_config_backup.json" "$INSTDIR\config.json"
    ${EndIf}
FunctionEnd

Function LaunchAsNormalUser
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

Section "主程序 (必需)" SEC_MAIN
    SectionIn RO 
    !insertmacro wails.setShellContext

    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        Call BackupUserData
        Call StopRunningInstances
        Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
        Delete "$INSTDIR\BS2PRO-CoreService.exe"
        Delete "$INSTDIR\logs\*.log" 
    ${Else}
        Call StopRunningInstances
        Delete "$INSTDIR\logs\*.*"
    ${EndIf}
    
    !insertmacro wails.webview2runtime
    SetOutPath $INSTDIR
    !insertmacro wails.files
    
    DetailPrint "正在安装核心服务..."
    File "..\..\bin\BS2PRO-CoreService.exe"
    
    nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" stop'
    Pop $0
    Pop $1
    nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" uninstall'
    Pop $0
    Pop $1
    nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" install'
    Pop $0
    Pop $1
    nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" start'
    Pop $0
    Pop $1
    
    SetOutPath $INSTDIR
    Call RestoreUserData

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    !insertmacro wails.writeUninstaller
    
    ${If} ${FileExists} "$TEMP\bs2pro_config_backup.json"
        Delete "$TEMP\bs2pro_config_backup.json"
    ${EndIf}
SectionEnd

Section /o "控制台自启动(建议关闭)" SEC_AUTOSTART
    DetailPrint "正在配置GUI开机自启..."
    nsExec::ExecToStack '"$SYSDIR\schtasks.exe" /delete /tn "BS2PRO-Controller" /f'
    Pop $0
    Pop $1
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Controller" '"$INSTDIR\${PRODUCT_EXECUTABLE}" --autostart'
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SEC_MAIN} "BS2PRO 控制器主程序和后台核心守护服务。"
    !insertmacro MUI_DESCRIPTION_TEXT ${SEC_AUTOSTART} "（默认关闭）登录桌面时静默启动控制台。核心服务已随系统自启，控制台无需自启。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "uninstall"
    !insertmacro wails.setShellContext

    Call un.StopRunningInstances
    
    DetailPrint "正在停止并移除核心服务..."
    nsExec::ExecToStack '"$SYSDIR\sc.exe" stop "BS2PRO_CoreService"'
    Pop $0
    Pop $1
    ${If} $0 == 0
        Sleep 500
    ${EndIf}
    nsExec::ExecToStack '"$SYSDIR\sc.exe" delete "BS2PRO_CoreService"'
    Pop $0
    Pop $1
    
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Controller"
    DeleteRegValue HKLM "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Controller"

    DetailPrint "正在移除控制台应用缓存数据..."
    SetShellVarContext current
    RMDir /r /REBOOTOK "$APPDATA\BS2PRO-Controller"
    RMDir /r /REBOOTOK "$LOCALAPPDATA\BS2PRO-Controller"
    SetShellVarContext all
    RMDir /r /REBOOTOK "$APPDATA\BS2PRO-Controller"

    DetailPrint "正在删除安装目录..."
    RMDir /r "$INSTDIR\logs"
    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols
    !insertmacro wails.deleteUninstaller
    
    MessageBox MB_YESNO|MB_ICONQUESTION "是否删除所有配置文件？" IDNO skip_uninst_config
    SetShellVarContext current
    RMDir /r /REBOOTOK "$PROFILE\.bs2pro-controller"
    skip_uninst_config:
    DetailPrint "卸载完成"
SectionEnd