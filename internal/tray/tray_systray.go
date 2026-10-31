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
	openItem      *systray.MenuItem
	settingsItem  *systray.MenuItem
	historyItem   *systray.MenuItem
	autostartItem *systray.MenuItem
	languageItem  *systray.MenuItem
	quitItem      *systray.MenuItem
	langItems     map[string]*systray.MenuItem
	started       bool
}

// New creates a tray controller. icon should be PNG bytes (Linux) or ICO (Windows).
func New(icon []byte, cb Callbacks, opts Options) *Controller {
	return &Controller{icon: icon, cb: cb, opts: opts, langItems: map[string]*systray.MenuItem{}}
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

// SetLabels refreshes tray menu titles after a locale change.
func (c *Controller) SetLabels(l Labels) {
	c.mu.Lock()
	c.opts.Labels = l
	status, open, settings, history := c.statusItem, c.openItem, c.settingsItem, c.historyItem
	autostart, language, quit := c.autostartItem, c.languageItem, c.quitItem
	c.mu.Unlock()

	if l.Tooltip != "" {
		systray.SetTooltip(l.Tooltip)
	}
	_ = status
	if open != nil && l.Open != "" {
		open.SetTitle(l.Open)
	}
	if settings != nil && l.Settings != "" {
		settings.SetTitle(l.Settings)
	}
	if history != nil && l.History != "" {
		history.SetTitle(l.History)
	}
	if autostart != nil && l.Autostart != "" {
		autostart.SetTitle(l.Autostart)
	}
	if language != nil && l.Language != "" {
		language.SetTitle(l.Language)
	}
	if quit != nil && l.Quit != "" {
		quit.SetTitle(l.Quit)
	}
}

// SetLocaleChecked marks the active language in the submenu.
func (c *Controller) SetLocaleChecked(code string) {
	c.mu.Lock()
	items := c.langItems
	c.opts.Locale = code
	c.mu.Unlock()
	for id, item := range items {
		if item == nil {
			continue
		}
		if id == code {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

func (c *Controller) onReady() {
	l := c.opts.Labels
	tooltip := l.Tooltip
	if tooltip == "" {
		tooltip = "Gotify Desktop"
	}
	systray.SetTooltip(tooltip)
	if len(c.icon) > 0 {
		systray.SetTemplateIcon(c.icon, c.icon)
		systray.SetIcon(c.icon)
	}
	systray.SetTitle("")

	status := systray.AddMenuItem(or(l.StatusStarting, "Status: starting…"), or(l.StatusTooltip, "Connection status"))
	status.Disable()

	systray.AddSeparator()
	open := systray.AddMenuItem(or(l.Open, "Open window"), or(l.OpenTooltip, "Show the Gotify window"))
	settings := systray.AddMenuItem(or(l.Settings, "Settings"), or(l.SettingsTooltip, "Open connection settings"))
	history := systray.AddMenuItem(or(l.History, "Recent notifications"), or(l.HistoryTooltip, "Show recent notifications"))
	autostart := systray.AddMenuItemCheckbox(
		or(l.Autostart, "Start at login"),
		or(l.AutostartTooltip, "Launch Gotify Desktop when you log in"),
		c.opts.AutostartEnabled,
	)
	language := systray.AddMenuItem(or(l.Language, "Language"), or(l.LanguageTooltip, "Choose the interface language"))

	langItems := map[string]*systray.MenuItem{}
	for _, lang := range c.opts.Languages {
		item := language.AddSubMenuItemCheckbox(lang.Name, lang.Name, lang.Code == c.opts.Locale)
		langItems[lang.Code] = item
	}

	systray.AddSeparator()
	quit := systray.AddMenuItem(or(l.Quit, "Quit"), or(l.QuitTooltip, "Exit Gotify Desktop"))

	c.mu.Lock()
	c.statusItem = status
	c.openItem = open
	c.settingsItem = settings
	c.historyItem = history
	c.autostartItem = autostart
	c.languageItem = language
	c.quitItem = quit
	c.langItems = langItems
	c.mu.Unlock()

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
			case <-history.ClickedCh:
				if c.cb.OnHistory != nil {
					c.cb.OnHistory()
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

	for code, item := range langItems {
		code, item := code, item
		go func() {
			for range item.ClickedCh {
				if c.cb.OnLanguage != nil {
					c.cb.OnLanguage(code)
				}
			}
		}()
	}
}

func (c *Controller) onExit() {}

func or(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
