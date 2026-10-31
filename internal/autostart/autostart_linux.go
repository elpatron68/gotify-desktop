//go:build linux || freebsd || openbsd || netbsd

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func desktopPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", appID+".desktop"), nil
}

func isEnabled(exe string) bool {
	path, err := desktopPath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Treat as enabled when the file exists and points at our binary (or any Exec).
	s := string(data)
	return strings.Contains(s, "Exec=") && !strings.Contains(s, "Hidden=true")
}

func enable(exe string) error {
	path, err := desktopPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Version=1.0
Name=%s
Comment=Gotify desktop notifications
Exec=%s
Terminal=false
Categories=Network;
X-GNOME-Autostart-enabled=true
`, appName, escapeDesktopExec(exe))
	return os.WriteFile(path, []byte(content), 0o600)
}

func disable() error {
	path, err := desktopPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// escapeDesktopExec quotes paths with spaces per the Desktop Entry Spec.
func escapeDesktopExec(path string) string {
	if strings.ContainsAny(path, " \t\"'\\$`") {
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")
		return `"` + r.Replace(path) + `"`
	}
	return path
}
