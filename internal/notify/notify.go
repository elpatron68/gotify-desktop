package notify

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Notifier wraps Wails native notifications with a platform fallback.
type Notifier struct {
	mu      sync.Mutex
	ctx     context.Context
	ready   bool
	initErr error
	lastErr error
	logger  *log.Logger
	logFile io.Closer
}

// Init initializes the platform notification system. Call from OnStartup.
func (n *Notifier) Init(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ctx = ctx
	n.ensureLogger()

	if err := wailsruntime.InitializeNotifications(ctx); err != nil {
		n.initErr = err
		n.ready = false
		n.logf("wails InitializeNotifications failed: %v (will use platform fallback)", err)
		return err
	}
	if !wailsruntime.IsNotificationAvailable(ctx) {
		n.initErr = fmt.Errorf("notifications not available on this platform")
		n.ready = false
		n.logf("%v (will use platform fallback)", n.initErr)
		return n.initErr
	}
	authorized, err := wailsruntime.CheckNotificationAuthorization(ctx)
	if err == nil && !authorized {
		authorized, err = wailsruntime.RequestNotificationAuthorization(ctx)
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
	ctx := n.ctx
	ready := n.ready
	n.ensureLogger()
	n.mu.Unlock()

	if title == "" {
		title = "Gotify"
	}

	if runtime.GOOS == "windows" {
		// Unpackaged Win32 apps need a registered AppUserModelID; Wails/WinRT
		// and bare PowerShell AUMIDs often report success with no visible toast.
		if err := showPlatformToast(title, body); err != nil {
			n.setLastErr(err)
			n.logf("windows toast failed id=%s: %v", id, err)
			return err
		}
		n.setLastErr(nil)
		n.logf("windows toast sent id=%s title=%q", id, title)
		return nil
	}

	var firstErr error
	if ready && ctx != nil {
		err := wailsruntime.SendNotification(ctx, wailsruntime.NotificationOptions{
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
