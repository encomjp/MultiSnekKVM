//go:build !windows

package autostart

func Set(enabled bool) error { return nil }
func Get() bool              { return false }
func Sync(enabled bool)      {}
