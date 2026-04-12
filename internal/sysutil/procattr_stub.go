//go:build !windows

package sysutil

import "os/exec"

func HideCommandWindow(cmd *exec.Cmd) {}
