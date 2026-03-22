//go:build windows

package notification

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	shell32ProcSetCurrentProcessExplicitAppUserModelID = syscall.NewLazyDLL("shell32.dll").NewProc("SetCurrentProcessExplicitAppUserModelID")
)

func EnsureCurrentProcessAppID(appID string) error {
	if appID == "" {
		appID = defaultAppID
	}
	ptr, err := syscall.UTF16PtrFromString(appID)
	if err != nil {
		return err
	}
	hr, _, _ := shell32ProcSetCurrentProcessExplicitAppUserModelID.Call(uintptr(unsafe.Pointer(ptr)))
	if int32(hr) < 0 {
		return fmt.Errorf("设置进程AppUserModelID失败: 0x%X", hr)
	}
	return nil
}
