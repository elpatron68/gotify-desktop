package tray

import (
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Controller manages the system tray icon and menu via Wails v3.
type Controller struct {
	app  *application.App
	icon []byte
	cb   Callbacks
	opts Options

	mu            sync.Mutex
	tray          *application.SystemTray
	menu          *application.Menu
	statusItem    *application.MenuItem
	autostartItem *application.MenuItem
	langItems     map[string]*application.MenuItem
	started       bool
}

// New creates a tray controller. icon should be PNG bytes on all platforms.
func New(app *application.App, icon []byte, cb Callbacks, opts Options) *Controller {
	return &Controller{
		app:       app,
		icon:      icon,
		cb:        cb,
		opts:      opts,
		langItems: map[string]*application.MenuItem{},
	}
}

// Start creates the system tray and attaches the menu.
func (c *Controller) Start() {
	c.mu.Lock()
	if c.started || c.app == nil {
		c.mu.Unlock()
		return
	}
	c.started = true
	c.mu.Unlock()

	systray := c.app.SystemTray.New()
	if len(c.icon) > 0 {
		systray.SetIcon(c.icon)
		systray.SetTemplateIcon(c.icon)
	}
	applyTrayTitle(systray, c.opts.Labels.Tooltip)
	systray.OnClick(func() {
		if c.cb.OnOpen != nil {
			c.cb.OnOpen()
		}
	})

	menu := c.buildMenu()
	systray.SetMenu(menu)

	c.mu.Lock()
	c.tray = systray
	c.menu = menu
	c.mu.Unlock()
}

// Stop destroys the system tray.
func (c *Controller) Stop() {
	c.mu.Lock()
	t := c.tray
	c.tray = nil
	c.started = false
	c.mu.Unlock()
	if t != nil {
		t.Destroy()
	}
}

// SetStatus updates the status menu label.
func (c *Controller) SetStatus(text string) {
	c.mu.Lock()
	item := c.statusItem
	menu := c.menu
	c.mu.Unlock()
	if item == nil {
		return
	}
	item.SetLabel(text)
	if menu != nil {
		menu.Update()
	}
}

// SetAutostartChecked updates the Start at login checkbox.
func (c *Controller) SetAutostartChecked(enabled bool) {
	c.mu.Lock()
	item := c.autostartItem
	menu := c.menu
	c.opts.AutostartEnabled = enabled
	c.mu.Unlock()
	if item == nil {
		return
	}
	item.SetChecked(enabled)
	if menu != nil {
		menu.Update()
	}
}

// SetLabels refreshes tray menu titles after a locale change by rebuilding the menu.
func (c *Controller) SetLabels(l Labels) {
	c.mu.Lock()
	c.opts.Labels = l
	tray := c.tray
	c.mu.Unlock()
	if tray == nil {
		return
	}
	applyTrayTitle(tray, l.Tooltip)
	menu := c.buildMenu()
	tray.SetMenu(menu)
	c.mu.Lock()
	c.menu = menu
	c.mu.Unlock()
}

// SetLocaleChecked marks the active language in the submenu.
func (c *Controller) SetLocaleChecked(code string) {
	c.mu.Lock()
	items := c.langItems
	menu := c.menu
	c.opts.Locale = code
	c.mu.Unlock()
	for id, item := range items {
		if item == nil {
			continue
		}
		item.SetChecked(id == code)
	}
	if menu != nil {
		menu.Update()
	}
}

func (c *Controller) buildMenu() *application.Menu {
	l := c.opts.Labels
	menu := c.app.NewMenu()

	status := menu.Add(or(l.StatusStarting, "Status: starting…"))
	status.SetEnabled(false)

	menu.AddSeparator()
	menu.Add(or(l.Open, "Open window")).OnClick(func(_ *application.Context) {
		if c.cb.OnOpen != nil {
			c.cb.OnOpen()
		}
	})
	menu.Add(or(l.Settings, "Settings")).OnClick(func(_ *application.Context) {
		if c.cb.OnSettings != nil {
			c.cb.OnSettings()
		}
	})
	menu.Add(or(l.History, "Recent notifications")).OnClick(func(_ *application.Context) {
		if c.cb.OnHistory != nil {
			c.cb.OnHistory()
		}
	})
	menu.Add(or(l.Send, "Send message")).OnClick(func(_ *application.Context) {
		if c.cb.OnSend != nil {
			c.cb.OnSend()
		}
	})

	autostart := menu.AddCheckbox(or(l.Autostart, "Start at login"), c.opts.AutostartEnabled)
	autostart.OnClick(func(ctx *application.Context) {
		enabled := false
		if item := ctx.ClickedMenuItem(); item != nil {
			enabled = item.Checked()
		}
		if c.cb.OnAutostart != nil {
			c.cb.OnAutostart(enabled)
		}
	})

	langMenu := menu.AddSubmenu(or(l.Language, "Language"))
	langItems := map[string]*application.MenuItem{}
	for _, lang := range c.opts.Languages {
		code := lang.Code
		item := langMenu.AddRadio(lang.Name, code == c.opts.Locale)
		item.OnClick(func(_ *application.Context) {
			if c.cb.OnLanguage != nil {
				c.cb.OnLanguage(code)
			}
		})
		langItems[code] = item
	}

	about := menu.AddSubmenu(or(l.About, "About"))
	aboutAuthor := about.Add(or(l.AboutAuthor, "Author"))
	aboutAuthor.SetEnabled(false)
	about.Add(or(l.AboutLicense, "License")).OnClick(func(_ *application.Context) {
		if c.cb.OnAboutLicense != nil {
			c.cb.OnAboutLicense()
		}
	})
	about.Add(or(l.AboutVersion, "Version")).OnClick(func(_ *application.Context) {
		if c.cb.OnAboutVersion != nil {
			c.cb.OnAboutVersion()
		}
	})
	about.Add(or(l.AboutCheckUpdates, "Check for updates")).OnClick(func(_ *application.Context) {
		if c.cb.OnAboutCheckUpdates != nil {
			c.cb.OnAboutCheckUpdates()
		}
	})
	about.Add(or(l.AboutRepo, "Open repository")).OnClick(func(_ *application.Context) {
		if c.cb.OnAboutRepo != nil {
			c.cb.OnAboutRepo()
		}
	})

	menu.AddSeparator()
	menu.Add(or(l.Quit, "Quit")).OnClick(func(_ *application.Context) {
		if c.cb.OnQuit != nil {
			c.cb.OnQuit()
		}
	})

	c.mu.Lock()
	c.statusItem = status
	c.autostartItem = autostart
	c.langItems = langItems
	c.mu.Unlock()

	return menu
}

func or(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

// applyTrayTitle sets the hover/status title. On Linux, Wails SetTooltip is a
// no-op and StatusNotifierItem Title comes from SetLabel (empty → "Wails").
// macOS keeps an empty label so the menu bar stays icon-only.
func applyTrayTitle(systray *application.SystemTray, tooltip string) {
	if systray == nil {
		return
	}
	title := or(tooltip, "Gotify Desktop")
	systray.SetTooltip(title)
	if runtime.GOOS == "linux" {
		systray.SetLabel(title)
		return
	}
	systray.SetLabel("")
}
