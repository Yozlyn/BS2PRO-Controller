//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                    = syscall.NewLazyDLL("user32.dll")
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetForegroundWindow   = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcID = user32.NewProc("GetWindowThreadProcessId")
	procEnumWindows           = user32.NewProc("EnumWindows")
	procIsWindowVisible       = user32.NewProc("IsWindowVisible")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procGetWindowTextLengthW  = user32.NewProc("GetWindowTextLengthW")
	procGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	procCreateSnapshot        = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW       = kernel32.NewProc("Process32FirstW")
	procProcess32NextW        = kernel32.NewProc("Process32NextW")
	procCloseHandle           = kernel32.NewProc("CloseHandle")
)

const th32csSnapProcess = 0x00000002
const (
	smCxScreen = 0
	smCyScreen = 1
)

type processEntry32W struct {
	DwSize              uint32
	CntUsage            uint32
	Th32ProcessID       uint32
	Th32DefaultHeapID   uintptr
	Th32ModuleID        uint32
	CntThreads          uint32
	Th32ParentProcessID uint32
	PcPriClassBase      int32
	DwFlags             uint32
	SzExeFile           [260]uint16
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

func getForegroundProcessName() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		hwnd = findFullscreenWindow()
		if hwnd == 0 {
			return ""
		}
	}
	var pid uint32
	procGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		hwnd = findFullscreenWindow()
		if hwnd == 0 {
			return ""
		}
		procGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == 0 {
			return ""
		}
	}
	handle, _, _ := procCreateSnapshot.Call(th32csSnapProcess, 0)
	if handle == 0 || handle == ^uintptr(0) {
		return ""
	}
	defer procCloseHandle.Call(handle)
	var entry processEntry32W
	entry.DwSize = uint32(unsafe.Sizeof(entry))
	ret, _, _ := procProcess32FirstW.Call(handle, uintptr(unsafe.Pointer(&entry)))
	for ret != 0 {
		if entry.Th32ProcessID == pid {
			return strings.TrimSpace(syscall.UTF16ToString(entry.SzExeFile[:]))
		}
		ret, _, _ = procProcess32NextW.Call(handle, uintptr(unsafe.Pointer(&entry)))
	}
	return ""
}

func findFullscreenWindow() uintptr {
	screenW, _, _ := procGetSystemMetrics.Call(smCxScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCyScreen)
	minWidth := int32(float64(screenW) * 0.85)
	minHeight := int32(float64(screenH) * 0.85)
	var candidate uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		textLen, _, _ := procGetWindowTextLengthW.Call(hwnd)
		if textLen == 0 {
			return 1
		}
		var r rect
		ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		if ok == 0 {
			return 1
		}
		width := r.Right - r.Left
		height := r.Bottom - r.Top
		if width >= minWidth && height >= minHeight {
			candidate = hwnd
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return candidate
}
