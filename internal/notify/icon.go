package notify

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// SetIconPNG stores the application icon used on desktop toasts.
// png is the raw PNG bytes (build/appicon.png). Safe to call before Init.
func (n *Notifier) SetIconPNG(png []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.iconPNG = append([]byte(nil), png...)
	n.iconPath = ""
}

// toastIconPath writes the embedded PNG once and returns its absolute path.
// An empty string means no icon is available; callers omit the icon.
func (n *Notifier) toastIconPath() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.iconPath != "" {
		if _, err := os.Stat(n.iconPath); err == nil {
			return n.iconPath
		}
		n.iconPath = ""
	}
	if len(n.iconPNG) == 0 {
		return ""
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, "gotify-desktop", "toast-icon.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return ""
	}
	if err := os.WriteFile(path, n.iconPNG, 0o644); err != nil {
		return ""
	}
	n.iconPath = path
	return path
}

// windowsFileURI turns an absolute path into a toast image src.
// Windows toast XML requires a file URI, not a raw path.
func windowsFileURI(filePath string) string {
	if filePath == "" {
		return ""
	}
	// path.Clean is slash-based, so a Windows path converts the same way on
	// every OS. filepath.ToSlash is a no-op on Linux and would leave backslashes.
	slash := path.Clean(strings.ReplaceAll(filePath, `\`, `/`))
	if slash == "." {
		return ""
	}
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String()
}
