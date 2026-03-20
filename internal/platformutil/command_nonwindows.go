//go:build !windows

package platformutil

import "os/exec"

func HideCommandWindow(cmd *exec.Cmd) {
}
