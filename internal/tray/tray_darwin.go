//go:build darwin

package tray

// Controller is a no-op tray on macOS.
// fyne.io/systray conflicts with Wails (duplicate AppDelegate Objective-C symbols).
// Use the native application menu instead (see app menu setup).
type Controller struct {
	cb Callbacks
}

// New creates a macOS tray stub.
func New(icon []byte, cb Callbacks, opts Options) *Controller {
	_ = icon
	_ = opts
	return &Controller{cb: cb}
}

// Start is a no-op on macOS.
func (c *Controller) Start() {}

// Stop is a no-op on macOS.
func (c *Controller) Stop() {}

// SetStatus is a no-op on macOS.
func (c *Controller) SetStatus(text string) { _ = text }

// SetAutostartChecked is a no-op on macOS (app menu owns the checkbox).
func (c *Controller) SetAutostartChecked(enabled bool) { _ = enabled }
