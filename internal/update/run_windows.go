//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// RunApplyHelper launches this executable with --apply-update flags.
// If the install directory is not writable, the helper is started elevated (UAC).
func RunApplyHelper(selfExe string, target Target, packagePath, version string) error {
	args := []string{
		"--apply-update",
		"--pid", fmt.Sprintf("%d", os.Getpid()),
		"--src", packagePath,
		"--dest", target.ReplacePath,
		"--kind", KindFlag(target.Kind),
	}
	if version != "" {
		args = append(args, "--version", version)
	}
	if CanWriteBeside(target.ReplacePath) {
		cmd := exec.Command(selfExe, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Start()
	}
	return startElevated(selfExe, args)
}

func startElevated(exe string, args []string) error {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	// ShellExecuteW parameters: join args with spaces; quote paths with spaces.
	paramStr := quoteArgs(args)
	params, err := windows.UTF16PtrFromString(paramStr)
	if err != nil {
		return err
	}
	var show int32 = 1 // SW_SHOWNORMAL
	ret, _, _ := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW").Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		uintptr(show),
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW elevation failed: code %d", ret)
	}
	return nil
}

func quoteArgs(args []string) string {
	out := make([]byte, 0, 128)
	for i, a := range args {
		if i > 0 {
			out = append(out, ' ')
		}
		if needsQuotes(a) {
			out = append(out, '"')
			out = append(out, a...)
			out = append(out, '"')
		} else {
			out = append(out, a...)
		}
	}
	return string(out)
}

func needsQuotes(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\t' {
			return true
		}
	}
	return false
}

// ApplyUpdate performs the package swap after the parent process has exited.
func ApplyUpdate(args ApplyArgs) error {
	if err := WaitForPIDExit(args.PID, 60*time.Second); err != nil {
		return err
	}
	if err := ReplaceFile(args.Src, args.Dest); err != nil {
		return err
	}
	_ = updateUninstallDisplayVersion(args.Dest, args.Version)
	return startPath(args.Dest)
}

func startPath(path string) error {
	cmd := exec.Command(path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\elpatron68Gotify Desktop`

// updateUninstallDisplayVersion updates DisplayVersion when DisplayIcon matches our exe.
func updateUninstallDisplayVersion(exePath, version string) error {
	if version == "" {
		return nil
	}
	abs, err := filepath.Abs(exePath)
	if err != nil {
		abs = exePath
	}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		if tryUpdateUninstallKey(root, abs, version) {
			return nil
		}
	}
	return nil
}

func tryUpdateUninstallKey(root registry.Key, absExe, version string) bool {
	key, err := registry.OpenKey(root, uninstallKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	icon, _, err := key.GetStringValue("DisplayIcon")
	if err != nil {
		return false
	}
	iconPath := icon
	if i := strings.IndexByte(icon, ','); i >= 0 {
		iconPath = icon[:i]
	}
	iconAbs, _ := filepath.Abs(iconPath)
	if !equalPath(iconAbs, absExe) && !equalPath(iconPath, absExe) {
		return false
	}
	return key.SetStringValue("DisplayVersion", version) == nil
}

func equalPath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
