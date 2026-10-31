//go:build windows

package autostart

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func isEnabled(exe string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	val, _, err := k.GetStringValue(windowsName)
	if err != nil {
		return false
	}
	return normalizeWinPath(val) != ""
}

func enable(exe string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(windowsName, quoteWinPath(exe))
}

func disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	err = k.DeleteValue(windowsName)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

func quoteWinPath(path string) string {
	if strings.ContainsAny(path, " \t\"") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}

func normalizeWinPath(val string) string {
	return strings.TrimSpace(strings.Trim(val, `"`))
}
