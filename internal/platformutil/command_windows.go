//go:build windows

package platformutil

import (
	"os/exec"
	"syscall"
)

func HideCommandWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
