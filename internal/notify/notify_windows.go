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

const (
	windowsAppID    = "Gotify.Desktop"
	powershellAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`
)

var (
	windowsToastOnce sync.Once
	windowsRegErr    error
)

func showPlatformToast(title, body string) error {
	windowsToastOnce.Do(func() {
		windowsRegErr = ensureWindowsToastApp()
	})

	// Prefer explicit PowerShell Show(). go-toast COM often returns nil while the
	// toast stays invisible for unpackaged apps (missing Start Menu AUMID binding).
	var errs []string
	if windowsRegErr != nil {
		errs = append(errs, "register: "+windowsRegErr.Error())
	}

	if err := powershellToast(windowsAppID, title, body); err != nil {
		errs = append(errs, "aumid: "+err.Error())
	} else {
		return nil
	}

	if err := powershellToast(powershellAppID, title, body); err != nil {
		errs = append(errs, "powershell-host: "+err.Error())
		return fmt.Errorf("windows toast failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func ensureWindowsToastApp() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	if err := toast.SetAppData(toast.AppData{
		AppID:         windowsAppID,
		ActivationExe: exe,
	}); err != nil {
		return err
	}
	if err := setCurrentProcessAUMID(windowsAppID); err != nil {
		return err
	}
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

func powershellToast(appID, title, body string) error {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$xmlText = @'
<toast><visual><binding template="ToastGeneric"><text>%s</text><text>%s</text></binding></visual></toast>
'@
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($xmlText)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier(%s).Show($toast)
`, xmlEscape(title), xmlEscape(body), psQuote(appID))

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
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

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&apos;",
	)
	return r.Replace(s)
}
