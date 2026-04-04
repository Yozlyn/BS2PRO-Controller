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
	procCreateSnapshot        = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW       = kernel32.NewProc("Process32FirstW")
	procProcess32NextW        = kernel32.NewProc("Process32NextW")
	procCloseHandle           = kernel32.NewProc("CloseHandle")
)

const th32csSnapProcess = 0x00000002

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

func getForegroundProcessName() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	_, _, _ = procGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	handle, _, _ := procCreateSnapshot.Call(th32csSnapProcess, 0)
	if handle == 0 || handle == ^uintptr(0) {
		return ""
	}
	defer func() { _, _, _ = procCloseHandle.Call(handle) }()
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
