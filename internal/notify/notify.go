package notify

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/elpatron68/gotify-desktop/internal/applog"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Notifier wraps Wails native notifications with a platform fallback.
type Notifier struct {
	mu      sync.Mutex
	svc     *notifications.NotificationService
	ready   bool
	initErr error
	lastErr error
	logger  *log.Logger
	logFile io.Closer
}

// Init initializes the platform notification system.
// On Windows the Wails notifier is skipped in favour of native AUMID toasts.
func (n *Notifier) Init(svc *notifications.NotificationService) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.svc = svc
	n.ensureLogger()

	// Windows: never use Wails/WebView2 notifications — they appear as
	// "wails.localhost". Native AUMID toasts show "Gotify Desktop" instead.
	if runtime.GOOS == "windows" {
		n.ready = false
		n.initErr = nil
		n.logf("using native Windows toasts (skipping Wails notifier)")
		applog.Printf("notify init ready GOOS=windows backend=native-aumid")
		return nil
	}

	if svc == nil {
		n.initErr = fmt.Errorf("notifications service unavailable")
		n.ready = false
		n.logf("%v (will use platform fallback)", n.initErr)
		return n.initErr
	}

	authorized, err := svc.CheckNotificationAuthorization()
	if err == nil && !authorized {
		authorized, err = svc.RequestNotificationAuthorization()
		if err != nil || !authorized {
			n.initErr = fmt.Errorf("notification permission denied")
			n.ready = false
			n.logf("%v (will use platform fallback)", n.initErr)
			return n.initErr
		}
	}
	n.ready = true
	n.initErr = nil
	n.logf("wails notifications ready (GOOS=%s)", runtime.GOOS)
	applog.Printf("notify init ready GOOS=%s", runtime.GOOS)
	return nil
}

// Ready reports whether the Wails notification backend initialized.
func (n *Notifier) Ready() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ready
}

// LastError returns the last notification send/init error, if any.
func (n *Notifier) LastError() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.lastErr != nil {
		return n.lastErr.Error()
	}
	if n.initErr != nil && !n.ready {
		return n.initErr.Error()
	}
	return ""
}

// Show displays a desktop notification.
//
// On Windows, Wails/WinRT often reports success for unpackaged apps without a
// visible toast (missing Start Menu AUMID shortcut). We therefore always use the
// PowerShell AUMID fallback on Windows. On other OSes, try Wails first, then beeep.
func (n *Notifier) Show(id, title, body string) error {
	n.mu.Lock()
	svc := n.svc
	ready := n.ready
	n.ensureLogger()
	n.mu.Unlock()

	if title == "" {
		title = "Gotify" // brand fallback; UI should pass a localized title when empty matters
	}

	if runtime.GOOS == "windows" {
		if err := showPlatformToast(title, body); err != nil {
			n.setLastErr(err)
			n.logf("windows toast failed id=%s: %v", id, err)
			return err
		}
		n.setLastErr(nil)
		n.logf("windows toast sent id=%s title=%q (also see desktop.log for method/Setting)", id, title)
		applog.Printf("notify Show ok id=%s title=%q", id, title)
		return nil
	}

	var firstErr error
	if ready && svc != nil {
		err := svc.SendNotification(notifications.NotificationOptions{
			ID:    id,
			Title: title,
			Body:  body,
		})
		if err == nil {
			n.setLastErr(nil)
			n.logf("wails notification sent id=%s title=%q", id, title)
			return nil
		}
		firstErr = err
		n.logf("wails SendNotification failed: %v — trying fallback", err)
	}

	if err := showPlatformToast(title, body); err != nil {
		if firstErr != nil {
			err = fmt.Errorf("wails: %v; fallback: %w", firstErr, err)
		}
		n.setLastErr(err)
		n.logf("notification failed: %v", err)
		return err
	}
	n.setLastErr(nil)
	n.logf("fallback notification sent title=%q", title)
	return nil
}

func (n *Notifier) setLastErr(err error) {
	n.mu.Lock()
	n.lastErr = err
	n.mu.Unlock()
}

// Close releases the notify.log file handle. Safe to call multiple times.
func (n *Notifier) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.logFile != nil {
		_ = n.logFile.Close()
		n.logFile = nil
	}
	// Keep a stderr logger so late logf calls after Close stay safe.
	n.logger = log.New(os.Stderr, "gotify-desktop/notify: ", log.LstdFlags|log.Lmsgprefix)
}

// ensureLogger initializes the file logger. Caller must hold n.mu.
func (n *Notifier) ensureLogger() {
	if n.logger != nil {
		return
	}
	n.logger = log.New(os.Stderr, "gotify-desktop/notify: ", log.LstdFlags|log.Lmsgprefix)
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	logDir := filepath.Join(dir, "gotify-desktop")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(logDir, "notify.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	n.logFile = f
	n.logger = log.New(f, "", log.LstdFlags)
}

func (n *Notifier) logf(format string, args ...any) {
	// logger is set under mu and only swapped in Close; reading the pointer
	// without the lock is safe. Avoid locking here: Init calls logf while
	// already holding n.mu.
	if n.logger != nil {
		n.logger.Printf(format, args...)
	}
}
