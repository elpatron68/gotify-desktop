package tray

import (
	"sync"

	"fyne.io/systray"
)

// Callbacks wired by the application.
type Callbacks struct {
	OnOpen     func()
	OnSettings func()
	OnQuit     func()
}

// Controller manages the system tray icon and menu.
type Controller struct {
	icon []byte
	cb   Callbacks

	mu          sync.Mutex
	statusItem  *systray.MenuItem
	started     bool
}

// New creates a tray controller. icon should be PNG bytes (Linux/macOS) or ICO (Windows).
func New(icon []byte, cb Callbacks) *Controller {
	return &Controller{icon: icon, cb: cb}
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

func (c *Controller) onReady() {
	systray.SetTitle("Gotify")
	systray.SetTooltip("Gotify Desktop")
	if len(c.icon) > 0 {
		systray.SetIcon(c.icon)
	}

	status := systray.AddMenuItem("Status: starting…", "Connection status")
	status.Disable()
	c.mu.Lock()
	c.statusItem = status
	c.mu.Unlock()

	systray.AddSeparator()
	open := systray.AddMenuItem("Open window", "Show the Gotify window")
	settings := systray.AddMenuItem("Settings", "Open connection settings")
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
