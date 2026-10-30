package notify

import (
	"context"
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Notifier wraps Wails native notifications.
type Notifier struct {
	mu      sync.Mutex
	ctx     context.Context
	ready   bool
	initErr error
}

// Init initializes the platform notification system. Call from OnStartup.
func (n *Notifier) Init(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ctx = ctx
	if err := runtime.InitializeNotifications(ctx); err != nil {
		n.initErr = err
		n.ready = false
		return err
	}
	if !runtime.IsNotificationAvailable(ctx) {
		n.initErr = fmt.Errorf("notifications not available on this platform")
		n.ready = false
		return n.initErr
	}
	authorized, err := runtime.CheckNotificationAuthorization(ctx)
	if err == nil && !authorized {
		authorized, err = runtime.RequestNotificationAuthorization(ctx)
		if err != nil || !authorized {
			n.initErr = fmt.Errorf("notification permission denied")
			n.ready = false
			return n.initErr
		}
	}
	n.ready = true
	n.initErr = nil
	return nil
}

// Ready reports whether notifications can be sent.
func (n *Notifier) Ready() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ready
}

// Show displays a desktop notification.
func (n *Notifier) Show(id, title, body string) error {
	n.mu.Lock()
	ctx := n.ctx
	ready := n.ready
	n.mu.Unlock()
	if !ready || ctx == nil {
		return fmt.Errorf("notifications not initialized")
	}
	if title == "" {
		title = "Gotify"
	}
	return runtime.SendNotification(ctx, runtime.NotificationOptions{
		ID:    id,
		Title: title,
		Body:  body,
	})
}
