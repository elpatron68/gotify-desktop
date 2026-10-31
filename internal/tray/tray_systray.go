//go:build !darwin

package tray

import (
	"sync"

	"fyne.io/systray"
)

// Controller manages the system tray icon and menu.
type Controller struct {
	icon []byte
	cb   Callbacks
	opts Options

	mu            sync.Mutex
	statusItem    *systray.MenuItem
	autostartItem *systray.MenuItem
	started       bool
}

// New creates a tray controller. icon should be PNG bytes (Linux) or ICO (Windows).
func New(icon []byte, cb Callbacks, opts Options) *Controller {
	return &Controller{icon: icon, cb: cb, opts: opts}
}

// Start runs the systray event loop in a background goroutine.
func (c *Controller) Start() {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return
	}
	c.started = true
	c.mu.Unlock()
	go systray.Run(c.onReady, c.onExit)
}

// Stop quits the systray loop.
func (c *Controller) Stop() {
	systray.Quit()
}

// SetStatus updates the status menu label.
func (c *Controller) SetStatus(text string) {
	c.mu.Lock()
	item := c.statusItem
	c.mu.Unlock()
	if item != nil {
		item.SetTitle(text)
	}
}

// SetAutostartChecked updates the Start at login checkbox.
func (c *Controller) SetAutostartChecked(enabled bool) {
	c.mu.Lock()
	item := c.autostartItem
	c.mu.Unlock()
	if item == nil {
		return
	}
	if enabled {
		item.Check()
	} else {
		item.Uncheck()
	}
}

func (c *Controller) onReady() {
	systray.SetTooltip("Gotify Desktop")
	if len(c.icon) > 0 {
		systray.SetTemplateIcon(c.icon, c.icon)
		systray.SetIcon(c.icon)
	}
	// Title text next to the icon is mainly useful on macOS; keep empty on Linux/Windows.
	systray.SetTitle("")

	status := systray.AddMenuItem("Status: starting…", "Connection status")
	status.Disable()
	c.mu.Lock()
	c.statusItem = status
	c.mu.Unlock()

	systray.AddSeparator()
	open := systray.AddMenuItem("Open window", "Show the Gotify window")
	settings := systray.AddMenuItem("Settings", "Open connection settings")
	autostart := systray.AddMenuItemCheckbox("Start at login", "Launch Gotify Desktop when you log in", c.opts.AutostartEnabled)
	c.mu.Lock()
	c.autostartItem = autostart
	c.mu.Unlock()
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Exit Gotify Desktop")
	go func() {
		for {
			select {
			case <-open.ClickedCh:
				if c.cb.OnOpen != nil {
					c.cb.OnOpen()
				}
			case <-settings.ClickedCh:
				if c.cb.OnSettings != nil {
					c.cb.OnSettings()
				}
			case <-autostart.ClickedCh:
				if c.cb.OnAutostart != nil {
					c.cb.OnAutostart()
				}
			case <-quit.ClickedCh:
				if c.cb.OnQuit != nil {
					c.cb.OnQuit()
				}
				systray.Quit()
				return
			}
		}
	}()
}

func (c *Controller) onExit() {}
