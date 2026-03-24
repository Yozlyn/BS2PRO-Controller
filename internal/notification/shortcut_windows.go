//go:build windows

package notification

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/TIANLI0/BS2PRO-Controller/internal/platformutil"
)

const (
	startMenuDirName      = "BS2PRO-Controller"
	monitorShortcutName   = "BS2PRO Monitor.lnk"
	monitorShortcutTarget = "BS2PRO-Monitor.exe"
)

func EnsureMonitorStartMenuShortcut() error {
	installDir := config.GetInstallDir()
	monitorPath := filepath.Join(installDir, monitorShortcutTarget)
	iconPath := filepath.Join(installDir, "BS2PRO-Monitor.exe")
	shortcutDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", startMenuDirName)
	shortcutPath := filepath.Join(shortcutDir, monitorShortcutName)
	if _, err := os.Stat(monitorPath); err != nil {
		return fmt.Errorf("monitor程序不存在: %w", err)
	}
	if err := os.MkdirAll(shortcutDir, 0755); err != nil {
		return fmt.Errorf("创建开始菜单目录失败: %w", err)
	}

	safeShortcutPath := psQuote(shortcutPath)
	safeMonitorPath := psQuote(monitorPath)
	safeInstallDir := psQuote(installDir)
	safeIconPath := psQuote(iconPath)
	safeAppID := psQuote(defaultAppID)

	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$shortcutPath = '%s'
$targetPath = '%s'
$workingDir = '%s'
$iconPath = '%s'
$appId = '%s'

$code = @"
using System;
using System.Runtime.InteropServices;
using System.Text;

[ComImport, Guid("00021401-0000-0000-C000-000000000046")]
class ShellLink {}

[ComImport, InterfaceType(ComInterfaceType.InterfaceIsIUnknown), Guid("000214F9-0000-0000-C000-000000000046")]
interface IShellLinkW {
    void GetPath([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszFile, int cchMaxPath, IntPtr pfd, int fFlags);
    void GetIDList(out IntPtr ppidl);
    void SetIDList(IntPtr pidl);
    void GetDescription([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszName, int cchMaxName);
    void SetDescription([MarshalAs(UnmanagedType.LPWStr)] string pszName);
    void GetWorkingDirectory([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszDir, int cchMaxPath);
    void SetWorkingDirectory([MarshalAs(UnmanagedType.LPWStr)] string pszDir);
    void GetArguments([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszArgs, int cchMaxPath);
    void SetArguments([MarshalAs(UnmanagedType.LPWStr)] string pszArgs);
    void GetHotkey(out short pwHotkey);
    void SetHotkey(short wHotkey);
    void GetShowCmd(out int piShowCmd);
    void SetShowCmd(int iShowCmd);
    void GetIconLocation([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder pszIconPath, int cchIconPath, out int piIcon);
    void SetIconLocation([MarshalAs(UnmanagedType.LPWStr)] string pszIconPath, int iIcon);
    void SetRelativePath([MarshalAs(UnmanagedType.LPWStr)] string pszPathRel, int dwReserved);
    void Resolve(IntPtr hwnd, int fFlags);
    void SetPath([MarshalAs(UnmanagedType.LPWStr)] string pszFile);
}

[ComImport, InterfaceType(ComInterfaceType.InterfaceIsIUnknown), Guid("0000010b-0000-0000-C000-000000000046")]
interface IPersistFile {
    void GetClassID(out Guid pClassID);
    void IsDirty();
    void Load([MarshalAs(UnmanagedType.LPWStr)] string pszFileName, int dwMode);
    void Save([MarshalAs(UnmanagedType.LPWStr)] string pszFileName, bool fRemember);
    void SaveCompleted([MarshalAs(UnmanagedType.LPWStr)] string pszFileName);
    void GetCurFile([MarshalAs(UnmanagedType.LPWStr)] out string ppszFileName);
}

[ComImport, InterfaceType(ComInterfaceType.InterfaceIsIUnknown), Guid("886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99")]
interface IPropertyStore {
    uint GetCount(out uint cProps);
    uint GetAt(uint iProp, out PROPERTYKEY pkey);
    uint GetValue(ref PROPERTYKEY key, out PROPVARIANT pv);
    uint SetValue(ref PROPERTYKEY key, ref PROPVARIANT propvar);
    uint Commit();
}

[StructLayout(LayoutKind.Sequential, Pack = 4)]
struct PROPERTYKEY {
    public Guid fmtid;
    public uint pid;
}

[StructLayout(LayoutKind.Sequential)]
struct PROPVARIANT {
    public ushort vt;
    public ushort wReserved1;
    public ushort wReserved2;
    public ushort wReserved3;
    public IntPtr p;
    public int p2;
}

static class NativeMethods {
    [DllImport("ole32.dll")]
    public static extern int PropVariantClear(ref PROPVARIANT pvar);
}

public static class ShortcutInstaller {
    static PROPERTYKEY AppUserModelID = new PROPERTYKEY {
        fmtid = new Guid("9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3"),
        pid = 5
    };

    public static void Install(string shortcutPath, string targetPath, string workingDir, string iconPath, string appId) {
        var link = (IShellLinkW)new ShellLink();
        link.SetPath(targetPath);
        link.SetWorkingDirectory(workingDir);
        link.SetDescription("BS2PRO Controller Monitor");
        link.SetIconLocation(iconPath, 0);

        var store = (IPropertyStore)link;
        var pv = new PROPVARIANT();
        pv.vt = 31;
        pv.p = Marshal.StringToCoTaskMemUni(appId);
        try {
            store.SetValue(ref AppUserModelID, ref pv);
            store.Commit();
        } finally {
            NativeMethods.PropVariantClear(ref pv);
        }

        var file = (IPersistFile)link;
        file.Save(shortcutPath, true);
    }
}
"@

Add-Type -TypeDefinition $code -Language CSharp
[ShortcutInstaller]::Install($shortcutPath, $targetPath, $workingDir, $iconPath, $appId)
`, safeShortcutPath, safeMonitorPath, safeInstallDir, safeIconPath, safeAppID)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	platformutil.HideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("创建monitor开始菜单快捷方式失败: %v, output=%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func psQuote(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}
