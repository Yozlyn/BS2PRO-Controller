//go:build windows

package procswitch

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                    = syscall.NewLazyDLL("user32.dll")
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetForegroundWindow   = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcID = user32.NewProc("GetWindowThreadProcessId")
	procOpenProcess           = kernel32.NewProc("OpenProcess")
	procCloseHandle           = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImage = kernel32.NewProc("QueryFullProcessImageNameW")
)

const processQueryLimitedInformation = 0x1000

func getForegroundProcessName() (string, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", nil
	}

	var pid uint32
	procGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return "", nil
	}

	handle, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return "", fmt.Errorf("打开前台进程失败")
	}
	defer procCloseHandle.Call(handle)

	buf := make([]uint16, syscall.MAX_PATH)
	size := uint32(len(buf))
	r1, _, _ := procQueryFullProcessImage.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r1 == 0 || size == 0 {
		return "", fmt.Errorf("读取前台进程路径失败")
	}

	fullPath := syscall.UTF16ToString(buf[:size])
	if fullPath == "" {
		return "", nil
	}
	parts := strings.Split(fullPath, "\\")
	return strings.TrimSpace(parts[len(parts)-1]), nil
}
