package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	appName     = "Gotify Desktop"
	appID       = "gotify-desktop"
	macosLabel  = "me.elpatron.gotify-desktop"
	windowsName = "GotifyDesktop"
)

// Manager registers the current executable to start at user login.
type Manager struct {
	exe string
}

// New resolves the running executable path for autostart registration.
func New() (*Manager, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("executable path: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return &Manager{exe: exe}, nil
}

// Executable returns the absolute path that would be registered.
func (m *Manager) Executable() string {
	if m == nil {
		return ""
	}
	return m.exe
}

// IsEnabled reports whether login autostart is currently registered.
func (m *Manager) IsEnabled() bool {
	if m == nil {
		return false
	}
	return isEnabled(m.exe)
}

// Enable registers the app to start at login.
func (m *Manager) Enable() error {
	if m == nil {
		return fmt.Errorf("autostart: nil manager")
	}
	return enable(m.exe)
}

// Disable removes the login autostart registration.
func (m *Manager) Disable() error {
	if m == nil {
		return fmt.Errorf("autostart: nil manager")
	}
	return disable()
}

// Set enables or disables autostart.
func (m *Manager) Set(enabled bool) error {
	if enabled {
		return m.Enable()
	}
	return m.Disable()
}

// Toggle flips the current state and returns the new enabled flag.
func (m *Manager) Toggle() (bool, error) {
	if m.IsEnabled() {
		if err := m.Disable(); err != nil {
			return true, err
		}
		return false, nil
	}
	if err := m.Enable(); err != nil {
		return false, err
	}
	return true, nil
}
