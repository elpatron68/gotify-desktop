//go:build windows

package notify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
	"golang.org/x/sys/windows"
)

const windowsAppID = "Gotify.Desktop"

var (
	windowsToastOnce sync.Once
	windowsToastErr  error
)

// showPlatformToast registers an AppUserModelID (required for unpackaged Win32
// toasts) and pushes a notification via go-toast / WinRT.
//
// Using only the PowerShell host AUMID often returns success with no visible
// toast when PowerShell notifications are disabled.
func showPlatformToast(title, body string) error {
	windowsToastOnce.Do(func() {
		windowsToastErr = ensureWindowsToastApp()
	})
	if windowsToastErr != nil {
		return windowsToastErr
	}

	n := toast.Notification{
		AppID: windowsAppID,
		Title: title,
		Body:  body,
		Audio: toast.Default,
	}
	if err := n.Push(); err != nil {
		return fmt.Errorf("windows toast: %w", err)
	}
	return nil
}

func ensureWindowsToastApp() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("absolute executable path: %w", err)
	}

	if err := toast.SetAppData(toast.AppData{
		AppID:         windowsAppID,
		ActivationExe: exe,
	}); err != nil {
		return fmt.Errorf("register AppUserModelID: %w", err)
	}
	if err := setCurrentProcessAUMID(windowsAppID); err != nil {
		return err
	}
	// Best-effort Start Menu shortcut so Action Center can attribute toasts.
	_ = ensureStartMenuShortcut(exe)
	return nil
}

func setCurrentProcessAUMID(appID string) error {
	ptr, err := windows.UTF16PtrFromString(appID)
	if err != nil {
		return err
	}
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	proc := shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	hr, _, _ := proc.Call(uintptr(unsafe.Pointer(ptr)))
	if hr != 0 {
		return fmt.Errorf("SetCurrentProcessExplicitAppUserModelID: HRESULT 0x%X", hr)
	}
	return nil
}

func ensureStartMenuShortcut(exe string) error {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return fmt.Errorf("APPDATA not set")
	}
	dir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs")
	lnk := filepath.Join(dir, "Gotify Desktop.lnk")
	if st, err := os.Stat(lnk); err == nil && !st.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$WshShell = New-Object -ComObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut(%s)
$Shortcut.TargetPath = %s
$Shortcut.WorkingDirectory = %s
$Shortcut.Description = 'Gotify Desktop'
$Shortcut.Save()
`, psQuote(lnk), psQuote(exe), psQuote(filepath.Dir(exe)))

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
