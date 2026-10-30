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

	"github.com/elpatron68/gotify-desktop/internal/applog"
	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
	"golang.org/x/sys/windows"
)

const (
	windowsAppID = "Gotify.Desktop"
	// Explorer's toast host is almost always allowed to show banners.
	explorerAppID = `Microsoft.Windows.Explorer`
	powershellAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`
)

var (
	windowsToastOnce sync.Once
	windowsRegErr    error
)

func showPlatformToast(title, body string) error {
	windowsToastOnce.Do(func() {
		windowsRegErr = ensureWindowsToastApp()
		if windowsRegErr != nil {
			applog.Printf("toast register: %v", windowsRegErr)
		} else {
			applog.Printf("toast register: AUMID=%s ok", windowsAppID)
		}
	})

	// Prefer hosts Windows already trusts for banners. Custom AUMIDs often report
	// Setting=Enabled while producing no visible toast on unpackaged Win32 apps.
	attempts := []struct {
		name  string
		appID string
	}{
		{"explorer-host", explorerAppID},
		{"powershell-host", powershellAppID},
		{"gotify-aumid", windowsAppID},
	}

	var errs []string
	var anyOK bool
	for _, a := range attempts {
		applog.Printf("toast try method=%s appID=%s title=%q", a.name, a.appID, title)
		if err := powershellToast(a.appID, title, body); err != nil {
			applog.Printf("toast fail method=%s: %v", a.name, err)
			errs = append(errs, a.name+": "+err.Error())
			continue
		}
		applog.Printf("toast Show() returned ok method=%s", a.name)
		anyOK = true
		// Explorer/PowerShell hosts are the ones users actually see — stop there.
		if a.name != "gotify-aumid" {
			return nil
		}
	}

	// Native go-toast as an extra channel (uses the registered AUMID + shortcut).
	if err := goToastPush(title, body); err != nil {
		applog.Printf("toast go-toast Push failed: %v", err)
		errs = append(errs, "go-toast: "+err.Error())
	} else {
		applog.Printf("toast go-toast Push ok")
		anyOK = true
	}

	if anyOK {
		return nil
	}
	return fmt.Errorf("windows toast failed: %s", strings.Join(errs, "; "))
}

func goToastPush(title, body string) error {
	n := toast.Notification{
		AppID: windowsAppID,
		Title: title,
		Body:  body,
	}
	return n.Push()
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
	applog.Printf("toast exe=%s", exe)
	if err := toast.SetAppData(toast.AppData{
		AppID:         windowsAppID,
		ActivationExe: exe,
	}); err != nil {
		return err
	}
	if err := setCurrentProcessAUMID(windowsAppID); err != nil {
		return err
	}
	if err := ensureStartMenuShortcut(exe); err != nil {
		applog.Printf("toast shortcut: %v", err)
	} else {
		applog.Printf("toast shortcut ensured under Start Menu\\Programs\\Gotify Desktop.lnk")
	}
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
<toast><visual><binding template="ToastGeneric"><text>%s</text><text>%s</text></binding></visual><audio src="ms-winsoundevent:Notification.Default"/></toast>
'@
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($xmlText)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
$n = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier(%s)
$n.Show($toast)
Write-Output ('notifier_ok setting=' + $n.Setting)
`, xmlEscape(title), xmlEscape(body), psQuote(appID))

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "" {
		applog.Printf("toast powershell output: %s", trimmed)
	}
	if err != nil {
		return fmt.Errorf("%w (%s)", err, trimmed)
	}
	// Setting enum: 0=Enabled, 1=DisabledForApplication, 2=DisabledForUser, 3=DisabledByGroupPolicy, 4=DisabledByManifest
	if strings.Contains(trimmed, "setting=1") || strings.Contains(trimmed, "setting=2") {
		return fmt.Errorf("notifications disabled for this app (%s)", trimmed)
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
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Create the .lnk and stamp System.AppUserModel.ID so Windows associates
	// toasts with this unpackaged app.
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$WshShell = New-Object -ComObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut(%s)
$Shortcut.TargetPath = %s
$Shortcut.WorkingDirectory = %s
$Shortcut.Description = 'Gotify Desktop'
$Shortcut.Save()

$sig = @'
using System;
using System.Runtime.InteropServices;
using System.Runtime.InteropServices.ComTypes;
public static class LnkAumid {
  [ComImport, Guid("886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
  interface IPropertyStore {
    void GetCount(out uint cProps);
    void GetAt(uint iProp, out PROPERTYKEY pkey);
    void GetValue(ref PROPERTYKEY key, out PROPVARIANT pv);
    void SetValue(ref PROPERTYKEY key, ref PROPVARIANT pv);
    void Commit();
  }
  [StructLayout(LayoutKind.Sequential, Pack=4)]
  struct PROPERTYKEY { public Guid fmtid; public uint pid; }
  [StructLayout(LayoutKind.Sequential)]
  struct PROPVARIANT {
    public ushort vt; public ushort w1,w2,w3; public IntPtr p; public int p2;
  }
  [DllImport("shell32.dll", CharSet=CharSet.Unicode, PreserveSig=false)]
  static extern void SHGetPropertyStoreFromParsingName(string pszPath, IntPtr pbc, uint flags, [MarshalAs(UnmanagedType.LPStruct)] Guid riid, out IPropertyStore ppv);
  public static void Set(string lnk, string aumid) {
    IPropertyStore store;
    SHGetPropertyStoreFromParsingName(lnk, IntPtr.Zero, 2 /*GPS_READWRITE*/, typeof(IPropertyStore).GUID, out store);
    var key = new PROPERTYKEY{ fmtid=new Guid("9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3"), pid=5 };
    var pv = new PROPVARIANT();
    pv.vt = 31; // VT_LPWSTR
    pv.p = Marshal.StringToCoTaskMemUni(aumid);
    store.SetValue(ref key, ref pv);
    store.Commit();
    Marshal.FreeCoTaskMem(pv.p);
  }
}
'@
Add-Type -TypeDefinition $sig -ErrorAction SilentlyContinue
[LnkAumid]::Set(%s, %s)
`, psQuote(lnk), psQuote(exe), psQuote(filepath.Dir(exe)), psQuote(lnk), psQuote(windowsAppID))
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
