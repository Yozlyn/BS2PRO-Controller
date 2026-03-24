Unicode true

!define UNINST_KEY_NAME "BS2PRO-Controller"

!include "wails_tools.nsh"
!include "MUI.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

VIProductVersion "2.7.0.1"
VIFileVersion "2.7.0.1"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "启动BS2PRO控制台"
!define MUI_FINISHPAGE_RUN_FUNCTION "LaunchAsNormalUser"
!define MUI_ABORTWARNING
!define MUI_UNCONFIRMPAGE_TEXT_TOP "您即将卸载 BS2PRO-Controller"
!define MUI_UNCONFIRMPAGE_TEXT_LOCATION "从以下位置卸载："

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" 
InstallDir "$PROGRAMFILES64\${INFO_PRODUCTNAME}" 
ShowInstDetails show 
AutoCloseWindow false

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
    end_detection:
    Pop $R2
    Pop $R1
    Pop $R0
FunctionEnd

Function StopRunningInstances
    DetailPrint "正在检查并停止运行中的进程..."
    
    # 检查并停止控制台程序
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
    
    DetailPrint "控制台进程停止完成"

    DetailPrint "检查 BS2PRO-Monitor.exe 进程..."
    nsExec::ExecToStack '"$SYSDIR\tasklist.exe" /FI "IMAGENAME eq BS2PRO-Monitor.exe"'
    Pop $0
    Pop $1
    ${If} $0 == 0
        DetailPrint "正在停止 BS2PRO-Monitor.exe..."
        nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "BS2PRO-Monitor.exe" /T'
        Pop $0
        Pop $1
        ${If} $0 == 0
            Sleep 300
        ${EndIf}
    ${Else}
        DetailPrint "BS2PRO-Monitor.exe 进程不存在，跳过终止"
    ${EndIf}
FunctionEnd

Function WaitForFileRelease
    Exch $0
    Push $1
    Push $2
    StrCpy $1 0
wait_loop:
    ${IfNot} ${FileExists} $0
        Goto wait_done
    ${EndIf}
    Sleep 300
    IntOp $1 $1 + 1
    ${If} $1 < 20
        Goto wait_loop
    ${EndIf}
wait_done:
    Pop $2
    Pop $1
    Exch $0
FunctionEnd

Function un.WaitForFileRelease
    Exch $0
    Push $1
    Push $2
    StrCpy $1 0
un_wait_loop:
    ${IfNot} ${FileExists} $0
        Goto un_wait_done
    ${EndIf}
    Sleep 300
    IntOp $1 $1 + 1
    ${If} $1 < 20
        Goto un_wait_loop
    ${EndIf}
un_wait_done:
    Pop $2
    Pop $1
    Exch $0
FunctionEnd

Function un.StopRunningInstances
    DetailPrint "正在检查并停止运行中的进程..."
    
    # 检查并停止控制台程序
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
    
    DetailPrint "控制台进程停止完成"

    DetailPrint "检查 BS2PRO-Monitor.exe 进程..."
    nsExec::ExecToStack '"$SYSDIR\tasklist.exe" /FI "IMAGENAME eq BS2PRO-Monitor.exe"'
    Pop $0
    Pop $1
    ${If} $0 == 0
        DetailPrint "正在停止 BS2PRO-Monitor.exe..."
        nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "BS2PRO-Monitor.exe" /T'
        Pop $0
        Pop $1
        ${If} $0 == 0
            Sleep 300
        ${EndIf}
    ${Else}
        DetailPrint "BS2PRO-Monitor.exe 进程不存在，跳过终止"
    ${EndIf}
FunctionEnd

Function LaunchAsNormalUser
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

Function LaunchMonitorAsNormalUser
    ${If} ${FileExists} "$INSTDIR\BS2PRO-Monitor.exe"
        Exec '"$WINDIR\explorer.exe" "$INSTDIR\BS2PRO-Monitor.exe"'
    ${Else}
        DetailPrint "未找到 BS2PRO-Monitor.exe，跳过拉起"
    ${EndIf}
FunctionEnd

Function SyncMonitorAfterInstall
    Push $R0
    Push $R1
    Push $R2
    Push $R3

    ReadEnvStr $R0 "PROGRAMDATA"
    ${If} $R0 == ""
        StrCpy $R0 "C:\ProgramData"
    ${EndIf}
    StrCpy $R0 "$R0\BS2PRO-Controller\config.json"
    StrCpy $R1 0

wait_config:
    ${If} ${FileExists} "$R0"
        Goto check_config
    ${EndIf}
    Sleep 300
    IntOp $R1 $R1 + 1
    ${If} $R1 < 10
        Goto wait_config
    ${EndIf}

    DetailPrint "未找到配置文件，按默认配置启用 Monitor"
    Goto enable_monitor

check_config:
    DetailPrint "正在检查 Monitor 相关配置..."
    nsExec::ExecToStack '"$SYSDIR\findstr.exe" /C:"\"monitorAutoStart\": true" "$R0"'
    Pop $R1
    Pop $R2
    ${If} $R1 == 0
        Goto enable_monitor
    ${EndIf}

    nsExec::ExecToStack '"$SYSDIR\findstr.exe" /C:"\"notificationsEnabled\": true" "$R0"'
    Pop $R1
    Pop $R2
    ${If} $R1 == 0
        Goto enable_monitor
    ${EndIf}

    nsExec::ExecToStack '"$SYSDIR\findstr.exe" /C:"\"processSwitchEnabled\": true" "$R0"'
    Pop $R1
    Pop $R2
    ${If} $R1 == 0
        Goto enable_monitor
    ${EndIf}

    nsExec::ExecToStack '"$SYSDIR\findstr.exe" /C:"    \"enabled\": true," "$R0"'
    Pop $R1
    Pop $R2
    ${If} $R1 == 0
        Goto enable_monitor
    ${EndIf}

    DetailPrint "配置显示无需启动 Monitor，跳过处理"
    Goto sync_done

enable_monitor:
    DetailPrint "检测到需要 Monitor，立即拉起 Monitor"
    Call LaunchMonitorAsNormalUser

sync_done:
    Pop $R3
    Pop $R2
    Pop $R1
    Pop $R0
FunctionEnd

Section "主程序" SEC_MAIN
    SectionIn RO
    !insertmacro wails.setShellContext

    ${If} ${FileExists} "$INSTDIR\BS2PRO-CoreService.exe"
        DetailPrint "正在停止核心服务..."
        nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" stop'
        Pop $0
        Pop $1
        Sleep 1200
        
        DetailPrint "正在卸载核心服务..."
        nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" uninstall'
        Pop $0
        Pop $1
        nsExec::ExecToStack '"$SYSDIR\sc.exe" delete "BS2PRO_CoreService"'
        Pop $0
        Pop $1
        Sleep 1000
        
        nsExec::ExecToStack '"$SYSDIR\taskkill.exe" /F /IM "BS2PRO-CoreService.exe" /T'
        Sleep 500
    ${EndIf}

    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        Call StopRunningInstances
        Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
        Delete "$INSTDIR\BS2PRO-CoreService.exe"
        Delete "$INSTDIR\BS2PRO-Monitor.exe"
        Push "$INSTDIR\${PRODUCT_EXECUTABLE}"
        Call WaitForFileRelease
        Push "$INSTDIR\BS2PRO-CoreService.exe"
        Call WaitForFileRelease
        Push "$INSTDIR\BS2PRO-Monitor.exe"
        Call WaitForFileRelease
    ${EndIf}
    
    !insertmacro wails.webview2runtime
    SetOutPath $INSTDIR
    !insertmacro wails.files
    
    File "..\..\bin\BS2PRO-CoreService.exe"
    File "..\..\bin\BS2PRO-Monitor.exe"
    
    DetailPrint "正在注册核心服务..."
    nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" install'
    Pop $0
    
    DetailPrint "正在启动核心服务..."
    nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" start'
    Pop $0
    ${If} $0 == 0
        DetailPrint "核心服务启动成功"
    ${Else}
        DetailPrint "核心服务启动失败，错误代码: $0"
    ${EndIf}
    Call SyncMonitorAfterInstall

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    ClearErrors
    !insertmacro wails.writeUninstaller
    ${If} ${Errors}
        DetailPrint "卸载器写入失败，等待后重试..."
        Sleep 500
        ClearErrors
        !insertmacro wails.writeUninstaller
    ${EndIf}
SectionEnd

Section /o "开始菜单快捷方式" SEC_STARTMENU
    DetailPrint "正在创建开始菜单快捷方式..."
    SetShellVarContext current
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\BS2PRO-Monitor.lnk"
    RMDir /r "$SMPROGRAMS\BS2PRO-Controller"
    SetShellVarContext all
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\BS2PRO-Monitor.lnk"
    RMDir /r "$SMPROGRAMS\BS2PRO-Controller"
    SetShellVarContext current
    CreateDirectory "$SMPROGRAMS\BS2PRO-Controller"
    CreateShortcut "$SMPROGRAMS\BS2PRO-Controller\BS2PRO Controller.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}" "" "$INSTDIR\${PRODUCT_EXECUTABLE}" 0
    CreateShortcut "$SMPROGRAMS\BS2PRO-Controller\BS2PRO Monitor.lnk" "$INSTDIR\BS2PRO-Monitor.exe" "" "$INSTDIR\BS2PRO-Monitor.exe" 0
SectionEnd

Section /o "桌面快捷方式" SEC_DESKTOP
    DetailPrint "正在创建桌面快捷方式..."
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
SectionEnd

Section /o "Monitor 自启动" SEC_AUTOSTART
    DetailPrint "正在配置 Monitor 开机自启..."
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Monitor" "powershell -WindowStyle Hidden -NoProfile -NonInteractive -Command `$\"Start-Process -WindowStyle Hidden -FilePath '$INSTDIR\BS2PRO-Monitor.exe'`$\""
SectionEnd

Function .onInit
   !insertmacro wails.checkArchitecture
    Call DetectExistingInstallation

    # 设置快捷方式组件默认选中
   SectionSetFlags ${SEC_STARTMENU} 1
   SectionSetFlags ${SEC_DESKTOP} 1
   SectionSetFlags ${SEC_AUTOSTART} 1
FunctionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
   !insertmacro MUI_DESCRIPTION_TEXT ${SEC_MAIN} "BS2PRO 控制器主程序和后台核心守护服务。"
   !insertmacro MUI_DESCRIPTION_TEXT ${SEC_STARTMENU} "（可选）在开始菜单创建快捷方式。"
   !insertmacro MUI_DESCRIPTION_TEXT ${SEC_DESKTOP} "（可选）在桌面创建快捷方式。"
   !insertmacro MUI_DESCRIPTION_TEXT ${SEC_AUTOSTART} "（可选）登录桌面时静默启动 Monitor，作为托盘、通知与快捷键常驻的服务。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "uninstall"
    !insertmacro wails.setShellContext

    # 只在程序或服务可能存在时才尝试停止
    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        Call un.StopRunningInstances
    ${EndIf}
    
    DetailPrint "正在停止并移除核心服务..."
    
    # 首先尝试使用服务停止命令
    ${If} ${FileExists} "$INSTDIR\BS2PRO-CoreService.exe"
        DetailPrint "停止核心服务..."
        nsExec::ExecToStack '"$INSTDIR\BS2PRO-CoreService.exe" stop'
        Pop $0
        Pop $1
        ${If} $0 == 0
            DetailPrint "服务停止成功，等待1秒..."
            Sleep 1000
        ${Else}
            DetailPrint "服务停止失败，回退到sc.exe停止..."
        ${EndIf}
    ${EndIf}
    
    # 检查服务是否存在
    nsExec::ExecToStack '"$SYSDIR\sc.exe" query "BS2PRO_CoreService"'
    Pop $0
    Pop $1
    ${If} $0 == 0
        # 服务存在，尝试停止
        DetailPrint "使用sc.exe停止服务..."
        nsExec::ExecToStack '"$SYSDIR\sc.exe" stop "BS2PRO_CoreService"'
        Pop $0
        Pop $1
        ${If} $0 == 0
            DetailPrint "服务停止成功，等待500ms..."
            Sleep 500
        ${EndIf}
        
        DetailPrint "删除服务..."
        nsExec::ExecToStack '"$SYSDIR\sc.exe" delete "BS2PRO_CoreService"'
        Pop $0
        Pop $1
        ${If} $0 == 0
            DetailPrint "服务删除成功"
        ${Else}
            DetailPrint "服务删除失败，错误代码: $0"
        ${EndIf}
    ${Else}
        DetailPrint "BS2PRO_CoreService 服务不存在，跳过停止和删除"
    ${EndIf}
    
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Controller"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "BS2PRO-Monitor"
    Delete "$INSTDIR\BS2PRO-Monitor.exe"
    Push "$INSTDIR\BS2PRO-Monitor.exe"
    Call un.WaitForFileRelease

    DetailPrint "正在移除控制台应用缓存数据..."
    SetShellVarContext current
    RMDir /r /REBOOTOK "$APPDATA\BS2PRO-Controller"
    SetShellVarContext all

    DetailPrint "正在删除安装目录..."
    RMDir /r /REBOOTOK $INSTDIR

    SetShellVarContext current
    Delete "$SMPROGRAMS\BS2PRO-Controller\BS2PRO Controller.lnk"
    Delete "$SMPROGRAMS\BS2PRO-Controller\BS2PRO Monitor.lnk"
    RMDir "$SMPROGRAMS\BS2PRO-Controller"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\BS2PRO-Monitor.lnk"
    SetShellVarContext all
    Delete "$SMPROGRAMS\BS2PRO-Controller\BS2PRO Controller.lnk"
    Delete "$SMPROGRAMS\BS2PRO-Controller\BS2PRO Monitor.lnk"
    RMDir "$SMPROGRAMS\BS2PRO-Controller"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\BS2PRO-Monitor.lnk"
    SetShellVarContext current
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols
    !insertmacro wails.deleteUninstaller
    
    MessageBox MB_YESNO|MB_ICONQUESTION "是否删除所有配置文件？$\n$\n如果您计划重新安装并希望保留设置，请选择“否”。" IDNO skip_uninst_config
    RMDir /r /REBOOTOK "$APPDATA\BS2PRO-Controller"
    Goto uninstall_done
    
    skip_uninst_config:
    RMDir /r /REBOOTOK "$APPDATA\BS2PRO-Controller\logs"
    
    uninstall_done:
    DetailPrint "卸载完成"
SectionEnd
