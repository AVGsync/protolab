//go:build !windows

package browser

import (
	"os/exec"
	"syscall"
)

// New session: the browser must survive Ctrl+C / terminal hang-up of the TUI.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
