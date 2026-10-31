package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elpatron68/gotify-desktop/internal/applog"
	"github.com/elpatron68/gotify-desktop/internal/auth"
	"github.com/elpatron68/gotify-desktop/internal/autostart"
	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/gotifyproxy"
	"github.com/elpatron68/gotify-desktop/internal/history"
	"github.com/elpatron68/gotify-desktop/internal/i18n"
	"github.com/elpatron68/gotify-desktop/internal/notify"
	"github.com/elpatron68/gotify-desktop/internal/stream"
	"github.com/elpatron68/gotify-desktop/internal/tray"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// AppStatus is exposed to the frontend.
type AppStatus struct {
	Configured    bool   `json:"configured"`
	Connected     bool   `json:"connected"`
	ServerURL     string `json:"serverUrl"`
	Username      string `json:"username"`
	View          string `json:"view"` // "setup" | "gotify" | "history"
	Error         string `json:"error"`
	MinPriority   int    `json:"minPriority"`
	NotifyReady   bool   `json:"notifyReady"`
	NotifyError   string `json:"notifyError"`
	LastMessageID uint   `json:"lastMessageId"`
}

// App is the Wails-bound application core.
type App struct {
	ctx context.Context

	mu        sync.Mutex
	cfg       config.Config
	view      string
	connected bool
	lastErr   string
	quitting  bool

	auth      *auth.Client
	stream    *stream.Client
	notify    *notify.Notifier
	tray      *tray.Controller
	proxy     *gotifyproxy.Proxy
	history   *history.Store
	autostart *autostart.Manager
	trayIcon  []byte

	autostartMenuItem *menu.MenuItem
}

// NewApp creates the application.
func NewApp(trayIcon []byte) *App {
	// Locale before menus/tray: buildMenu runs before OnStartup.
	i18n.Init("")
	as, err := autostart.New()
	if err != nil {
		as = nil
	}
	return &App{
		auth:      auth.New(),
		notify:    &notify.Notifier{},
		proxy:     &gotifyproxy.Proxy{},
		autostart: as,
		view:      "setup",
		trayIcon:  trayIcon,
	}
}

// ProxyMiddleware exposes the Gotify reverse-proxy for the Wails asset server.
func (a *App) ProxyMiddleware() func(http.Handler) http.Handler {
	return a.proxy.Middleware
}

// buildMenu creates the native application menu (needed on macOS where systray is disabled).
func (a *App) buildMenu() *menu.Menu {
	enabled := a.autostart != nil && a.autostart.IsEnabled()
	a.autostartMenuItem = menu.Checkbox(i18n.T("menu.start_at_login"), enabled, nil, func(_ *menu.CallbackData) {
		a.ToggleAutostart()
	})
	current := i18n.Locale()
	langMenu := menu.NewMenu()
	for _, lang := range i18n.Languages() {
		code := lang.Code
		langMenu.Append(menu.Checkbox(lang.Name, code == current, nil, func(_ *menu.CallbackData) {
			a.SetLocale(code)
		}))
	}
	return menu.NewMenuFromItems(
		menu.SubMenu(i18n.T("app.gotify"), menu.NewMenuFromItems(
			menu.Text(i18n.T("menu.show_window"), nil, func(_ *menu.CallbackData) { a.ShowWindow() }),
			menu.Text(i18n.T("menu.settings"), nil, func(_ *menu.CallbackData) { a.OpenSettings() }),
			menu.Text(i18n.T("menu.test_notification"), nil, func(_ *menu.CallbackData) { _ = a.TestNotification() }),
			a.autostartMenuItem,
			menu.SubMenu(i18n.T("menu.language"), langMenu),
			menu.Separator(),
			menu.Text(i18n.T("menu.quit"), keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) { a.Quit() }),
		)),
	)
}

func trayLabels() tray.Labels {
	return tray.Labels{
		Tooltip:          i18n.T("tray.tooltip"),
		StatusStarting:   i18n.T("tray.status_starting"),
		StatusTooltip:    i18n.T("tray.status_tooltip"),
		Open:             i18n.T("tray.open"),
		OpenTooltip:      i18n.T("tray.open_tooltip"),
		Settings:         i18n.T("tray.settings"),
		SettingsTooltip:  i18n.T("tray.settings_tooltip"),
		Autostart:        i18n.T("tray.autostart"),
		AutostartTooltip: i18n.T("tray.autostart_tooltip"),
		Language:         i18n.T("tray.language"),
		LanguageTooltip:  i18n.T("tray.language_tooltip"),
		Quit:             i18n.T("tray.quit"),
		QuitTooltip:      i18n.T("tray.quit_tooltip"),
	}
}

func trayLanguages() []tray.Language {
	langs := i18n.Languages()
	out := make([]tray.Language, 0, len(langs))
	for _, l := range langs {
		out = append(out, tray.Language{Code: l.Code, Name: l.Name})
	}
	return out
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	applog.Init()
	if p, err := config.Path(); err == nil {
		applog.Printf("config path: %s", p)
	}
	_ = a.notify.Init(ctx)

	cfg, err := config.Load()
	if err != nil {
		applog.Printf("config load error: %v", err)
		a.setError(err.Error())
	} else {
		a.mu.Lock()
		a.cfg = cfg
		a.mu.Unlock()
		applog.Printf("config loaded configured=%v server=%q user=%q token=%s",
			cfg.Configured(), cfg.ServerURL, cfg.Username, applog.TokenFingerprint(cfg.ClientToken))
	}
	a.mu.Lock()
	a.cfg.HistoryMode = config.NormalizeHistoryMode(a.cfg.HistoryMode)
	a.mu.Unlock()
	if path, err := history.DefaultPath(); err == nil {
		a.history = history.New(path)
		if err := a.history.Load(); err != nil {
			applog.Printf("history load: %v", err)
		}
	} else {
		applog.Printf("history path: %v", err)
	}
	lang := i18n.Init(a.cfg.Locale)
	applog.Printf("locale=%s (config=%q)", lang, a.cfg.Locale)
	// Rebuild native menu after config locale (buildMenu ran earlier with OS default).
	if a.ctx != nil {
		runtime.MenuSetApplicationMenu(a.ctx, a.buildMenu())
		runtime.MenuUpdateApplicationMenu(a.ctx)
	}
	_ = a.proxy.Update(a.cfg.ServerURL, a.cfg.ClientToken)
	if _, err := a.proxy.StartWSProxy(); err != nil {
		applog.Printf("ws proxy start failed: %v", err)
	}

	autostartOn := a.syncAutostartOnStartup()
	a.tray = tray.New(a.trayIcon, tray.Callbacks{
		OnOpen:      a.ShowWindow,
		OnSettings:  a.OpenSettings,
		OnAutostart: a.ToggleAutostart,
		OnLanguage:  a.SetLocale,
		OnQuit:      a.Quit,
	}, tray.Options{
		AutostartEnabled: autostartOn,
		Locale:           i18n.Locale(),
		Languages:        trayLanguages(),
		Labels:           trayLabels(),
	})
	a.tray.Start()

	if a.cfg.Configured() {
		a.mu.Lock()
		a.view = "gotify"
		a.mu.Unlock()
		a.startStream()
		applog.Printf("startup: opening gotify view uiPath=%s", gotifyproxy.UIPrefix+"/")
	} else {
		a.tray.SetStatus(i18n.T("tray.status_not_configured"))
		applog.Printf("startup: setup view (not configured)")
	}
	a.emitStatus()
}

// domReady re-emits session state after the frontend has subscribed to events.
// OnStartup's emitStatus often races ahead of EventsOn and left users on the
// Connect form despite a saved client token.
func (a *App) domReady(ctx context.Context) {
	a.ctx = ctx
	a.emitStatus()
	a.mu.Lock()
	configured := a.cfg.Configured()
	view := a.view
	token := a.cfg.ClientToken
	a.mu.Unlock()
	if configured && view == "gotify" {
		applog.Printf("domReady: restoring gotify view (hiding to tray)")
		runtime.EventsEmit(a.ctx, "show-gotify", map[string]string{
			"token":  token,
			"uiPath": gotifyproxy.UIPrefix + "/",
		})
		// Already connected from a saved session — run in tray, don't leave the window open.
		runtime.WindowHide(a.ctx)
	} else {
		runtime.WindowShow(a.ctx)
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.stopStream()
	if a.tray != nil {
		a.tray.Stop()
	}
	if a.notify != nil {
		a.notify.Close()
	}
	if a.proxy != nil {
		a.proxy.CloseWS()
	}
	applog.Close()
}

func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	a.mu.Lock()
	quitting := a.quitting
	a.mu.Unlock()
	if quitting {
		return false
	}
	runtime.WindowHide(ctx)
	return true
}

// GetStatus returns the current app status for the UI.
func (a *App) GetStatus() AppStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AppStatus{
		Configured:    a.cfg.Configured(),
		Connected:     a.connected,
		ServerURL:     a.cfg.ServerURL,
		Username:      a.cfg.Username,
		View:          a.view,
		Error:         a.lastErr,
		MinPriority:   a.cfg.MinPriority,
		NotifyReady:   a.notify.Ready(),
		NotifyError:   a.notify.LastError(),
		LastMessageID: a.cfg.LastMessageID,
	}
}

// GetServerURL returns the configured Gotify base URL.
func (a *App) GetServerURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.ServerURL
}

// GetClientToken returns the Gotify client token used for stream + embedded UI auth.
func (a *App) GetClientToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.ClientToken
}

// GetUIPath returns the same-origin reverse-proxy path for the embedded Gotify UI.
func (a *App) GetUIPath() string {
	return gotifyproxy.UIPrefix + "/"
}

// IsConfigured reports whether a client token exists.
func (a *App) IsConfigured() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Configured()
}

// Connect authenticates with username/password, creates a client token, and starts the stream.
func (a *App) Connect(serverURL, username, password string) error {
	normalized, err := config.NormalizeServerURL(serverURL)
	if err != nil {
		return err
	}
	token, err := a.auth.VerifyAndCreateClient(normalized, username, password)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.cfg.ServerURL = normalized
	a.cfg.Username = username
	a.cfg.ClientToken = token
	a.view = "gotify"
	a.lastErr = ""
	cfg := a.cfg
	a.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	applog.Printf("connect saved server=%q user=%q token=%s", cfg.ServerURL, cfg.Username, applog.TokenFingerprint(cfg.ClientToken))
	_ = a.proxy.Update(cfg.ServerURL, cfg.ClientToken)
	if _, err := a.proxy.StartWSProxy(); err != nil {
		applog.Printf("ws proxy start failed: %v", err)
	}

	a.stopStream()
	a.startStream()
	a.emitStatus()
	runtime.EventsEmit(a.ctx, "show-gotify", map[string]string{
		"serverUrl": cfg.ServerURL,
		"token":     cfg.ClientToken,
		"uiPath":    gotifyproxy.UIPrefix + "/",
	})
	// After a successful connect, return to the tray like a notification client.
	runtime.WindowHide(a.ctx)
	applog.Printf("connect: window hidden to tray")
	return nil
}

// OpenSettings switches the UI back to the setup view.
func (a *App) OpenSettings() {
	a.mu.Lock()
	a.view = "setup"
	a.mu.Unlock()
	a.ShowWindow()
	a.emitStatus()
	runtime.EventsEmit(a.ctx, "show-setup")
}

// ShowWindow brings the main window to the front.
func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// Quit exits the application for real.
func (a *App) Quit() {
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	a.stopStream()
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

func (a *App) startStream() {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	if !cfg.Configured() {
		return
	}
	wsURL, err := config.StreamURL(cfg.ServerURL, cfg.ClientToken)
	if err != nil {
		a.setError(err.Error())
		a.emitStatus()
		return
	}

	client := &stream.Client{
		URL: wsURL,
		OnMessage: func(msg stream.Message) {
			a.handleMessage(msg)
		},
		OnStatus: func(connected bool, errMsg string) {
			a.mu.Lock()
			a.connected = connected
			if errMsg != "" {
				a.lastErr = errMsg
			} else if connected {
				a.lastErr = ""
			}
			a.mu.Unlock()
			if a.tray != nil {
				if connected {
					a.tray.SetStatus(i18n.T("tray.status_connected"))
				} else if errMsg != "" {
					a.tray.SetStatus(i18n.T("tray.status_error"))
				} else {
					a.tray.SetStatus(i18n.T("tray.status_disconnected"))
				}
			}
			a.emitStatus()
			// Do not force the setup form on transient unauthorized blips;
			// the user can open Settings manually. Auto-open made every stream
			// hiccup look like "login required again".
		},
	}
	a.mu.Lock()
	a.stream = client
	a.mu.Unlock()
	client.Start()
}

func (a *App) stopStream() {
	a.mu.Lock()
	s := a.stream
	a.stream = nil
	a.connected = false
	a.mu.Unlock()
	if s != nil {
		s.Stop()
	}
}

func (a *App) handleMessage(msg stream.Message) {
	a.mu.Lock()
	minP := a.cfg.MinPriority
	lastID := a.cfg.LastMessageID
	mode := config.NormalizeHistoryMode(a.cfg.HistoryMode)
	a.mu.Unlock()

	// 1. Dedup via last_message_id.
	if msg.ID != 0 && lastID != 0 && msg.ID <= lastID {
		return
	}

	// 2. Toast eligibility via min_priority.
	eligible := config.ShouldNotify(minP, msg.Priority)

	id := strconv.FormatUint(uint64(msg.ID), 10)
	if id == "0" {
		id = fmt.Sprintf("msg-%d", msg.AppID)
	}
	entry := history.Entry{
		ID:         id,
		Title:      msg.Title,
		Body:       msg.Message,
		Priority:   msg.Priority,
		AppID:      msg.AppID,
		ReceivedAt: time.Now(),
		Notified:   eligible,
	}

	if !eligible {
		// 4. Below min priority: history only in "all" mode (notified=false).
		// LastMessageID is intentionally not advanced (unchanged behavior).
		if mode == config.HistoryModeAll {
			a.recordHistory(entry)
		}
		return
	}

	// 3. Eligible: toast as before, record history, advance LastMessageID.
	applog.Printf("message id=%s title=%q priority=%d → notify", id, msg.Title, msg.Priority)
	// Refresh the iframe message list if it is open; OS toast is only via notify.Show.
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "gotify-message", map[string]string{
			"id":    id,
			"title": msg.Title,
			"body":  msg.Message,
		})
	}
	if err := a.notify.Show(id, msg.Title, msg.Message); err != nil {
		a.setError("notification: " + err.Error())
		if a.tray != nil {
			a.tray.SetStatus(i18n.T("tray.status_notify_error"))
		}
		a.emitStatus()
	} else if a.tray != nil {
		a.tray.SetStatus(i18n.Tf("tray.msg_prefix", map[string]string{
			"Title": truncateRunes(msg.Title, 40),
		}))
	}
	a.recordHistory(entry)

	if msg.ID > 0 {
		a.mu.Lock()
		a.cfg.LastMessageID = msg.ID
		cfg := a.cfg
		a.mu.Unlock()
		if err := config.Save(cfg); err != nil {
			applog.Printf("last message id save config: %v", err)
		}
	}
}

func (a *App) recordHistory(e history.Entry) {
	if a.history == nil {
		return
	}
	if err := a.history.Append(e); err != nil {
		applog.Printf("history append: %v", err)
		return
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "history-updated", nil)
	}
}

// TestNotification sends a local test notification (helps diagnose desktop notify setup).
func (a *App) TestNotification() error {
	title := i18n.T("notify.test_title")
	body := i18n.T("notify.test_body")
	err := a.notify.Show("test", title, body)
	now := time.Now()
	a.recordHistory(history.Entry{
		ID:         fmt.Sprintf("test-%d", now.Unix()),
		Title:      title,
		Body:       body,
		ReceivedAt: now,
		Notified:   true,
	})
	return err
}

// GetHistoryMode returns the normalized history capture mode ("toasts" | "all").
func (a *App) GetHistoryMode() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return config.NormalizeHistoryMode(a.cfg.HistoryMode)
}

// SetHistoryMode persists the history capture mode.
func (a *App) SetHistoryMode(mode string) {
	mode = config.NormalizeHistoryMode(mode)
	a.mu.Lock()
	a.cfg.HistoryMode = mode
	cfg := a.cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		applog.Printf("history mode save config: %v", err)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "history-updated", nil)
	}
}

// GetNotificationHistory returns stored entries filtered by the current mode.
func (a *App) GetNotificationHistory() []history.Entry {
	if a.history == nil {
		return nil
	}
	toastsOnly := a.GetHistoryMode() == config.HistoryModeToasts
	return a.history.List(toastsOnly)
}

// ClearNotificationHistory removes all stored history entries.
func (a *App) ClearNotificationHistory() {
	if a.history == nil {
		return
	}
	if err := a.history.Clear(); err != nil {
		applog.Printf("history clear: %v", err)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "history-updated", nil)
	}
}

// FormatHistoryEntryMarkdown returns the Markdown for the entry with the given id
// (looked up in the unfiltered store), or "" if not found.
func (a *App) FormatHistoryEntryMarkdown(id string) string {
	if a.history == nil {
		return ""
	}
	for _, e := range a.history.List(false) {
		if e.ID == id {
			return history.FormatMarkdown(e, time.Local)
		}
	}
	return ""
}

// OpenNotificationHistory shows the window with the history view.
func (a *App) OpenNotificationHistory() {
	a.mu.Lock()
	a.view = "history"
	a.mu.Unlock()
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		runtime.WindowUnminimise(a.ctx)
		runtime.EventsEmit(a.ctx, "show-history", nil)
	}
	a.emitStatus()
}

// LeaveNotificationHistory returns to the Gotify UI if configured, else setup.
func (a *App) LeaveNotificationHistory() {
	a.mu.Lock()
	if a.cfg.Configured() {
		a.view = "gotify"
	} else {
		a.view = "setup"
	}
	view := a.view
	a.mu.Unlock()
	if a.ctx != nil {
		if view == "gotify" {
			runtime.EventsEmit(a.ctx, "show-gotify", map[string]string{
				"token":  a.GetClientToken(),
				"uiPath": a.GetUIPath(),
			})
		} else {
			runtime.EventsEmit(a.ctx, "show-setup", nil)
		}
	}
	a.emitStatus()
}

// GetLocale returns the active UI language code (e.g. "en", "de").
func (a *App) GetLocale() string {
	return i18n.Locale()
}

// GetI18nCatalog returns the active translation catalog as JSON for the web UI.
func (a *App) GetI18nCatalog() string {
	raw, err := i18n.CatalogJSON()
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// SetLocale switches UI language, persists it, and refreshes tray / menu / web UI.
func (a *App) SetLocale(code string) {
	code = strings.TrimSpace(strings.ToLower(code))
	if code == "" {
		return
	}
	lang := i18n.Init(code)
	a.mu.Lock()
	a.cfg.Locale = lang
	cfg := a.cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		applog.Printf("locale save config: %v", err)
	}
	applog.Printf("locale set to %s", lang)

	if a.tray != nil {
		a.tray.SetLabels(trayLabels())
		a.tray.SetLocaleChecked(lang)
	}
	if a.ctx != nil {
		runtime.MenuSetApplicationMenu(a.ctx, a.buildMenu())
		runtime.MenuUpdateApplicationMenu(a.ctx)
		runtime.WindowSetTitle(a.ctx, i18n.T("app.name"))
		runtime.EventsEmit(a.ctx, "locale-changed", map[string]string{
			"locale":  lang,
			"catalog": a.GetI18nCatalog(),
		})
	}
	a.emitStatus()
}

// ToggleAutostart flips login autostart and updates tray / app menu checkmarks.
func (a *App) ToggleAutostart() {
	if a.autostart == nil {
		applog.Printf("autostart unavailable")
		return
	}
	enabled, err := a.autostart.Toggle()
	if err != nil {
		applog.Printf("autostart toggle failed: %v", err)
		a.setError("autostart: " + err.Error())
		a.applyAutostartUI(a.autostart.IsEnabled())
		a.emitStatus()
		return
	}
	a.persistAutostart(enabled)
	a.applyAutostartUI(enabled)
	applog.Printf("autostart enabled=%v", enabled)
}

// syncAutostartOnStartup refreshes OS registration from config / current path.
func (a *App) syncAutostartOnStartup() bool {
	if a.autostart == nil {
		return false
	}
	enabled := a.autostart.IsEnabled()
	// Config preference wins when set but OS registration was lost (moved binary).
	if a.cfg.Autostart && !enabled {
		if err := a.autostart.Enable(); err != nil {
			applog.Printf("autostart re-enable failed: %v", err)
		} else {
			enabled = true
		}
	}
	// Keep registration path current when already enabled.
	if enabled {
		if err := a.autostart.Enable(); err != nil {
			applog.Printf("autostart refresh failed: %v", err)
		}
	}
	if a.cfg.Autostart != enabled {
		a.persistAutostart(enabled)
	}
	if a.autostartMenuItem != nil {
		a.autostartMenuItem.Checked = enabled
	}
	return enabled
}

func (a *App) persistAutostart(enabled bool) {
	a.mu.Lock()
	a.cfg.Autostart = enabled
	cfg := a.cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		applog.Printf("autostart save config: %v", err)
	}
}

func (a *App) applyAutostartUI(enabled bool) {
	if a.tray != nil {
		a.tray.SetAutostartChecked(enabled)
	}
	if a.autostartMenuItem != nil {
		a.autostartMenuItem.Checked = enabled
		if a.ctx != nil {
			runtime.MenuUpdateApplicationMenu(a.ctx)
		}
	}
}

func (a *App) setError(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}

func (a *App) emitStatus() {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "status", a.GetStatus())
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
