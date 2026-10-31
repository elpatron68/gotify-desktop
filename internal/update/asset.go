package update

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallKind describes how the running app was packaged.
type InstallKind int

const (
	InstallBinary InstallKind = iota
	InstallAppImage
	InstallWindowsEXE
	InstallMacApp
)

// Target describes the update package to download and where to install it.
type Target struct {
	Kind        InstallKind
	AssetName   string
	ReplacePath string // path that --apply-update will replace
}

// DetectTarget inspects the running process and chooses the matching release asset.
func DetectTarget(goos, goarch string) (Target, error) {
	arch := releaseArch(goarch)
	if arch == "" {
		return Target{}, fmt.Errorf("unsupported architecture %q", goarch)
	}

	switch goos {
	case "linux":
		if appImage := strings.TrimSpace(os.Getenv("APPIMAGE")); appImage != "" {
			return Target{
				Kind:        InstallAppImage,
				AssetName:   fmt.Sprintf("gotify-desktop-linux-%s.AppImage", arch),
				ReplacePath: appImage,
			}, nil
		}
		exe, err := os.Executable()
		if err != nil {
			return Target{}, err
		}
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return Target{}, err
		}
		return Target{
			Kind:        InstallBinary,
			AssetName:   fmt.Sprintf("gotify-desktop-linux-%s", arch),
			ReplacePath: exe,
		}, nil

	case "windows":
		exe, err := os.Executable()
		if err != nil {
			return Target{}, err
		}
		return Target{
			Kind:        InstallWindowsEXE,
			AssetName:   fmt.Sprintf("gotify-desktop-windows-%s.exe", arch),
			ReplacePath: exe,
		}, nil

	case "darwin":
		exe, err := os.Executable()
		if err != nil {
			return Target{}, err
		}
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return Target{}, err
		}
		app := macAppBundle(exe)
		if app == "" {
			return Target{}, fmt.Errorf("running binary is not inside a .app bundle")
		}
		return Target{
			Kind:        InstallMacApp,
			AssetName:   fmt.Sprintf("gotify-desktop-darwin-%s.zip", arch),
			ReplacePath: app,
		}, nil

	default:
		return Target{}, fmt.Errorf("unsupported OS %q", goos)
	}
}

// CurrentTarget detects the update target for the running binary.
func CurrentTarget() (Target, error) {
	return DetectTarget(runtime.GOOS, runtime.GOARCH)
}

func releaseArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return ""
	}
}

// macAppBundle walks up from an executable path to the enclosing .app directory.
// Typical layout: Foo.app/Contents/MacOS/gotify-desktop
func macAppBundle(exe string) string {
	dir := filepath.Dir(exe)          // .../Contents/MacOS
	contents := filepath.Dir(dir)     // .../Contents
	app := filepath.Dir(contents)     // .../Foo.app
	if strings.HasSuffix(strings.ToLower(app), ".app") {
		return app
	}
	return ""
}
