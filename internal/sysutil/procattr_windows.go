//go:build windows

package sysutil

import (
	"os/exec"
	"syscall"
)

// HideCommandWindow prevents exec.Command from spawning a visible console
// window on Windows.
func HideCommandWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
