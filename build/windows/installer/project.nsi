Unicode true

!define UNINST_KEY_NAME "BS2PRO-Controller"

!include "wails_tools.nsh"
!include "MUI.nsh"
!include "FileFunc.nsh"
!include "DotNetChecker.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"
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
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "安装完成后立即启动 BS2PRO 控制器"
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
    nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "BS2PRO-CoreService.exe" /T'
    nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "${PRODUCT_EXECUTABLE}" /T'
    nsExec::ExecToStack '"$SYSDIR\schtasks.exe" /delete /tn "BS2PRO-Controller" /f'
    nsExec::ExecToStack '"$SYSDIR\schtasks.exe" /delete /tn "BS2PRO-Core" /f'

    Sleep 1000
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
    
    ExecWait '"$INSTDIR\BS2PRO-CoreService.exe" stop'
    ExecWait '"$INSTDIR\BS2PRO-CoreService.exe" uninstall'
    ExecWait '"$INSTDIR\BS2PRO-CoreService.exe" install'
    ExecWait '"$INSTDIR\BS2PRO-CoreService.exe" start'
    
    SetOutPath $INSTDIR
    Call RestoreUserData

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    !insertmacro wails.writeUninstaller
    
    ${If} ${FileExists} "$TEMP\bs2pro_config_backup.json"
        MessageBox MB_OK|MB_ICONINFORMATION "BS2PRO 控制器升级成功！$\n$\n您的设置已保留。"
        Delete "$TEMP\bs2pro_config_backup.json" 
    ${Else}
        MessageBox MB_OK|MB_ICONINFORMATION "BS2PRO 控制器安装成功！"
    ${EndIf}
SectionEnd

Section /o "GUI开机自启动 (建议关闭)" SEC_AUTOSTART
    DetailPrint "正在配置GUI开机自启..."
    nsExec::ExecToStack '"$SYSDIR\schtasks.exe" /delete /tn "BS2PRO-Controller" /f'
    nsExec::ExecToStack '"$SYSDIR\schtasks.exe" /delete /tn "BS2PRO-Core" /f'
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Controller" '"$INSTDIR\${PRODUCT_EXECUTABLE}" --autostart'
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SEC_MAIN} "BS2PRO 控制器主程序和后台核心守护服务。"
    !insertmacro MUI_DESCRIPTION_TEXT ${SEC_AUTOSTART} "（默认关闭）登录桌面时是否自动运行前端UI并隐藏至托盘。由于核心服务已随系统自启，完全可以在需要调节灯光时再手动打开GUI。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "uninstall"
    !insertmacro wails.setShellContext

    Call StopRunningInstances
    
    DetailPrint "正在停止并移除核心服务..."
    nsExec::ExecToStack '"$SYSDIR\sc.exe" stop "BS2PRO-CoreService"'
    Sleep 1000
    nsExec::ExecToStack '"$SYSDIR\sc.exe" delete "BS2PRO-CoreService"'
    
    ${If} ${FileExists} "$INSTDIR\BS2PRO-CoreService.exe"
        ExecWait '"$INSTDIR\BS2PRO-CoreService.exe" stop'
        ExecWait '"$INSTDIR\BS2PRO-CoreService.exe" uninstall'
    ${EndIf}
    
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Controller"
    DeleteRegValue HKLM "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Controller"

    DetailPrint "正在移除GUI应用缓存数据..."
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
    
    MessageBox MB_YESNO|MB_ICONQUESTION "是否删除您保留的风扇温度等配置文件？" IDNO skip_uninst_config
    SetShellVarContext current
    RMDir /r /REBOOTOK "$PROFILE\.bs2pro-controller"
    skip_uninst_config:
    DetailPrint "卸载完成"
SectionEnd