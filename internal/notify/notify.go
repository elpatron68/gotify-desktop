package notify

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/gen2brain/beeep"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Notifier wraps Wails native notifications with a beeep/D-Bus fallback.
type Notifier struct {
	mu      sync.Mutex
	ctx     context.Context
	ready   bool
	initErr error
	lastErr error
	logger  *log.Logger
}

// Init initializes the platform notification system. Call from OnStartup.
func (n *Notifier) Init(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ctx = ctx
	n.ensureLogger()

	if err := runtime.InitializeNotifications(ctx); err != nil {
		n.initErr = err
		n.ready = false
		n.logf("wails InitializeNotifications failed: %v (will use beeep fallback)", err)
		return err
	}
	if !runtime.IsNotificationAvailable(ctx) {
		n.initErr = fmt.Errorf("notifications not available on this platform")
		n.ready = false
		n.logf("%v (will use beeep fallback)", n.initErr)
		return n.initErr
	}
	authorized, err := runtime.CheckNotificationAuthorization(ctx)
	if err == nil && !authorized {
		authorized, err = runtime.RequestNotificationAuthorization(ctx)
		if err != nil || !authorized {
			n.initErr = fmt.Errorf("notification permission denied")
			n.ready = false
			n.logf("%v (will use beeep fallback)", n.initErr)
			return n.initErr
		}
	}
	n.ready = true
	n.initErr = nil
	n.logf("wails notifications ready")
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

// Show displays a desktop notification (Wails first, then beeep fallback).
func (n *Notifier) Show(id, title, body string) error {
	n.mu.Lock()
	ctx := n.ctx
	ready := n.ready
	n.ensureLogger()
	n.mu.Unlock()

	if title == "" {
		title = "Gotify"
	}

	var firstErr error
	if ready && ctx != nil {
		err := runtime.SendNotification(ctx, runtime.NotificationOptions{
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
		n.logf("wails SendNotification failed: %v — trying beeep", err)
	}

	if err := beeep.Notify(title, body, ""); err != nil {
		if firstErr != nil {
			err = fmt.Errorf("wails: %v; beeep: %w", firstErr, err)
		}
		n.setLastErr(err)
		n.logf("notification failed: %v", err)
		return err
	}
	n.setLastErr(nil)
	n.logf("beeep notification sent title=%q", title)
	return nil
}

func (n *Notifier) setLastErr(err error) {
	n.mu.Lock()
	n.lastErr = err
	n.mu.Unlock()
}

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
	n.logger = log.New(f, "", log.LstdFlags)
}

func (n *Notifier) logf(format string, args ...any) {
	if n.logger != nil {
		n.logger.Printf(format, args...)
	}
}
