//go:build windows

package autostart

import (
	"log"
	"os"

	"golang.org/x/sys/windows/registry"
)

const registryKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const valueName = "Multisnek"

func Set(enabled bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return k.SetStringValue(valueName, `"`+exe+`"`)
	}

	err = k.DeleteValue(valueName)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

func Get() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	_, _, err = k.GetStringValue(valueName)
	return err == nil
}

func Sync(enabled bool) {
	if err := Set(enabled); err != nil {
		log.Printf("autostart: %v", err)
	}
}
