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
	sendItem         *systray.MenuItem
	autostartItem    *systray.MenuItem
	languageItem     *systray.MenuItem
	aboutItem        *systray.MenuItem
	aboutAuthorItem       *systray.MenuItem
	aboutLicenseItem      *systray.MenuItem
	aboutVersionItem      *systray.MenuItem
	aboutCheckUpdatesItem *systray.MenuItem
	aboutRepoItem         *systray.MenuItem
	quitItem              *systray.MenuItem
	langItems             map[string]*systray.MenuItem
	started               bool
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
	send, autostart, language := c.sendItem, c.autostartItem, c.languageItem
	about, aboutAuthor, aboutLicense := c.aboutItem, c.aboutAuthorItem, c.aboutLicenseItem
	aboutVersion, aboutCheckUpdates := c.aboutVersionItem, c.aboutCheckUpdatesItem
	aboutRepo, quit := c.aboutRepoItem, c.quitItem
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
	if send != nil && l.Send != "" {
		send.SetTitle(l.Send)
	}
	if autostart != nil && l.Autostart != "" {
		autostart.SetTitle(l.Autostart)
	}
	if language != nil && l.Language != "" {
		language.SetTitle(l.Language)
	}
	if about != nil && l.About != "" {
		about.SetTitle(l.About)
	}
	if aboutAuthor != nil && l.AboutAuthor != "" {
		aboutAuthor.SetTitle(l.AboutAuthor)
	}
	if aboutLicense != nil && l.AboutLicense != "" {
		aboutLicense.SetTitle(l.AboutLicense)
	}
	if aboutVersion != nil && l.AboutVersion != "" {
		aboutVersion.SetTitle(l.AboutVersion)
	}
	if aboutCheckUpdates != nil && l.AboutCheckUpdates != "" {
		aboutCheckUpdates.SetTitle(l.AboutCheckUpdates)
	}
	if aboutRepo != nil && l.AboutRepo != "" {
		aboutRepo.SetTitle(l.AboutRepo)
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
	send := systray.AddMenuItem(or(l.Send, "Send message"), or(l.SendTooltip, "Send a message to an application"))
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

	about := systray.AddMenuItem(or(l.About, "About"), or(l.AboutTooltip, "About Gotify Desktop"))
	aboutAuthor := about.AddSubMenuItem(or(l.AboutAuthor, "Author"), "")
	aboutAuthor.Disable()
	aboutLicense := about.AddSubMenuItem(or(l.AboutLicense, "License"), or(l.AboutLicenseTooltip, "Open the license file"))
	aboutVersion := about.AddSubMenuItem(or(l.AboutVersion, "Version"), or(l.AboutVersionTooltip, "Open the latest release"))
	aboutCheckUpdates := about.AddSubMenuItem(or(l.AboutCheckUpdates, "Check for updates"), or(l.AboutCheckUpdatesTooltip, "Check for a newer version"))
	aboutRepo := about.AddSubMenuItem(or(l.AboutRepo, "Open repository"), or(l.AboutRepoTooltip, "Open the project page"))

	systray.AddSeparator()
	quit := systray.AddMenuItem(or(l.Quit, "Quit"), or(l.QuitTooltip, "Exit Gotify Desktop"))

	c.mu.Lock()
	c.statusItem = status
	c.openItem = open
	c.settingsItem = settings
	c.historyItem = history
	c.sendItem = send
	c.autostartItem = autostart
	c.languageItem = language
	c.aboutItem = about
	c.aboutAuthorItem = aboutAuthor
	c.aboutLicenseItem = aboutLicense
	c.aboutVersionItem = aboutVersion
	c.aboutCheckUpdatesItem = aboutCheckUpdates
	c.aboutRepoItem = aboutRepo
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
			case <-send.ClickedCh:
				if c.cb.OnSend != nil {
					c.cb.OnSend()
				}
			case <-autostart.ClickedCh:
				if c.cb.OnAutostart != nil {
					c.cb.OnAutostart()
				}
			case <-aboutLicense.ClickedCh:
				if c.cb.OnAboutLicense != nil {
					c.cb.OnAboutLicense()
				}
			case <-aboutVersion.ClickedCh:
				if c.cb.OnAboutVersion != nil {
					c.cb.OnAboutVersion()
				}
			case <-aboutCheckUpdates.ClickedCh:
				if c.cb.OnAboutCheckUpdates != nil {
					c.cb.OnAboutCheckUpdates()
				}
			case <-aboutRepo.ClickedCh:
				if c.cb.OnAboutRepo != nil {
					c.cb.OnAboutRepo()
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
