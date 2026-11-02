package main

import (
	"context"
	"fmt"
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
	"github.com/elpatron68/gotify-desktop/internal/push"
	"github.com/elpatron68/gotify-desktop/internal/session"
	"github.com/elpatron68/gotify-desktop/internal/stream"
	"github.com/elpatron68/gotify-desktop/internal/tray"
	"github.com/elpatron68/gotify-desktop/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// AppStatus is exposed to the frontend.
type AppStatus struct {
	Configured     bool         `json:"configured"`
	Connected      bool         `json:"connected"` // any stream up
	ActiveServerID string       `json:"activeServerId"`
	ServerURL      string       `json:"serverUrl"` // active
	Username       string       `json:"username"`  // active
	Servers        []ServerInfo `json:"servers"`
	View           string       `json:"view"` // "setup" | "gotify" | "history" | "send"
	Error          string       `json:"error"`
	MinPriority    int          `json:"minPriority"` // active
	NotifyReady    bool         `json:"notifyReady"`
	NotifyError    string       `json:"notifyError"`
	LastMessageID  uint         `json:"lastMessageId"` // active
	ConnectedCount int          `json:"connectedCount"`
	EnabledCount   int          `json:"enabledCount"`
}

// App is the Wails-bound application core.
type App struct {
	wailsApp  *application.App
	window    *application.WebviewWindow
	notifySvc *notifications.NotificationService

	mu       sync.Mutex
	cfg      config.Config
	view     string
	lastErr  string
	quitting bool
	ready    bool

	auth      *auth.Client
	push      *push.Client
	sessions  *session.Manager
	notify    *notify.Notifier
	tray      *tray.Controller
	proxy     *gotifyproxy.Proxy
	history   *history.Store
	autostart *autostart.Manager
	trayIcon  []byte

	autostartMenuItem *application.MenuItem
	appMenu           *application.Menu
}

// NewApp creates the application.
func NewApp(trayIcon []byte) *App {
	i18n.Init("")
	as, err := autostart.New()
	if err != nil {
		as = nil
	}
	a := &App{
		auth:      auth.New(),
		push:      push.New(),
		sessions:  session.New(),
		notify:    &notify.Notifier{},
		proxy:     &gotifyproxy.Proxy{},
		autostart: as,
		view:      "setup",
		trayIcon:  trayIcon,
	}
	a.sessions.OnMessage = a.handleMessage
	a.sessions.OnStatus = a.onSessionStatus
	return a
}

func (a *App) bindApp(app *application.App) {
	a.wailsApp = app
}

func (a *App) bindWindow(window *application.WebviewWindow) {
	a.window = window
}

func (a *App) setNotificationService(svc *notifications.NotificationService) {
	a.notifySvc = svc
}

func (a *App) setAppIcon(png []byte) {
	if a.notify != nil {
		a.notify.SetIconPNG(png)
	}
}

// proxyMiddleware exposes the Gotify reverse-proxy for the Wails asset server.
//
//wails:ignore
func (a *App) proxyMiddleware() application.Middleware {
	return a.proxy.Middleware
}

// ServiceStartup implements application.ServiceStartup.
func (a *App) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	a.startup()
	return nil
}

// ServiceShutdown implements application.ServiceShutdown.
func (a *App) ServiceShutdown() error {
	a.shutdown()
	return nil
}

func (a *App) rebuildApplicationMenu() {
	if a.wailsApp == nil {
		return
	}
	menu := a.buildApplicationMenu()
	a.appMenu = menu
	a.wailsApp.Menu.SetApplicationMenu(menu)
	menu.Update()
}

func (a *App) buildApplicationMenu() *application.Menu {
	menu := a.wailsApp.NewMenu()
	root := menu.AddSubmenu(i18n.T("app.gotify"))

	root.Add(i18n.T("menu.show_window")).OnClick(func(_ *application.Context) { a.ShowWindow() })
	root.Add(i18n.T("menu.settings")).OnClick(func(_ *application.Context) { a.OpenSettings() })

	serversMenu := root.AddSubmenu(i18n.T("tray.servers"))
	infos := a.buildServerInfos()
	if len(infos) == 0 {
		empty := serversMenu.Add(i18n.T("tray.servers_empty"))
		empty.SetEnabled(false)
	} else {
		for _, srv := range infos {
			id := srv.ID
			label := srv.Label
			if label == "" {
				label = srv.ServerURL
			}
			item := serversMenu.AddRadio(label, srv.Active)
			item.OnClick(func(_ *application.Context) {
				_ = a.SetActiveServer(id)
			})
		}
	}

	root.Add(i18n.T("menu.recent_notifications")).OnClick(func(_ *application.Context) { a.OpenNotificationHistory() })
	root.Add(i18n.T("menu.send_message")).OnClick(func(_ *application.Context) { a.OpenSendMessage() })
	root.Add(i18n.T("menu.test_notification")).OnClick(func(_ *application.Context) { _ = a.TestNotification() })

	enabled := a.autostart != nil && a.autostart.IsEnabled()
	a.autostartMenuItem = root.AddCheckbox(i18n.T("menu.start_at_login"), enabled)
	a.autostartMenuItem.OnClick(func(ctx *application.Context) {
		checked := false
		if item := ctx.ClickedMenuItem(); item != nil {
			checked = item.Checked()
		}
		a.setAutostartEnabled(checked)
	})

	langMenu := root.AddSubmenu(i18n.T("menu.language"))
	current := i18n.Locale()
	for _, lang := range i18n.Languages() {
		code := lang.Code
		item := langMenu.AddRadio(lang.Name, code == current)
		item.OnClick(func(_ *application.Context) {
			a.SetLocale(code)
		})
	}

	aboutMenu := root.AddSubmenu(i18n.T("menu.about"))
	aboutAuthor := aboutMenu.Add(i18n.Tf("menu.about_author", map[string]string{"Author": version.Author}))
	aboutAuthor.SetEnabled(false)
	aboutMenu.Add(i18n.Tf("menu.about_license", map[string]string{"License": version.License})).OnClick(func(_ *application.Context) {
		a.OpenLicense()
	})
	aboutMenu.Add(i18n.Tf("menu.about_version", map[string]string{"Version": version.Version})).OnClick(func(_ *application.Context) {
		a.OpenLatestRelease()
	})
	aboutMenu.Add(i18n.T("menu.check_updates")).OnClick(func(_ *application.Context) {
		a.CheckForUpdates()
	})
	aboutMenu.AddSeparator()
	aboutMenu.Add(i18n.T("menu.about_repo")).OnClick(func(_ *application.Context) { a.OpenRepository() })

	root.AddSeparator()
	quit := root.Add(i18n.T("menu.quit"))
	quit.SetAccelerator("CmdOrCtrl+q")
	quit.OnClick(func(_ *application.Context) { a.Quit() })

	return menu
}

func trayLabels() tray.Labels {
	return tray.Labels{
		Tooltip:                  i18n.T("tray.tooltip"),
		StatusStarting:           i18n.T("tray.status_starting"),
		StatusTooltip:            i18n.T("tray.status_tooltip"),
		Open:                     i18n.T("tray.open"),
		OpenTooltip:              i18n.T("tray.open_tooltip"),
		Settings:                 i18n.T("tray.settings"),
		SettingsTooltip:          i18n.T("tray.settings_tooltip"),
		Servers:                  i18n.T("tray.servers"),
		ServersTooltip:           i18n.T("tray.servers_tooltip"),
		ServersEmpty:             i18n.T("tray.servers_empty"),
		History:                  i18n.T("tray.history"),
		HistoryTooltip:           i18n.T("tray.history_tooltip"),
		Send:                     i18n.T("tray.send"),
		SendTooltip:              i18n.T("tray.send_tooltip"),
		Autostart:                i18n.T("tray.autostart"),
		AutostartTooltip:         i18n.T("tray.autostart_tooltip"),
		Language:                 i18n.T("tray.language"),
		LanguageTooltip:          i18n.T("tray.language_tooltip"),
		About:                    i18n.T("tray.about"),
		AboutTooltip:             i18n.T("tray.about_tooltip"),
		AboutAuthor:              i18n.Tf("tray.about_author", map[string]string{"Author": version.Author}),
		AboutLicense:             i18n.Tf("tray.about_license", map[string]string{"License": version.License}),
		AboutLicenseTooltip:      i18n.T("tray.about_license_tooltip"),
		AboutVersion:             i18n.Tf("tray.about_version", map[string]string{"Version": version.Version}),
		AboutVersionTooltip:      i18n.T("tray.about_version_tooltip"),
		AboutCheckUpdates:        i18n.T("tray.about_check_updates"),
		AboutCheckUpdatesTooltip: i18n.T("tray.about_check_updates_tooltip"),
		AboutRepo:                i18n.T("tray.about_repo"),
		AboutRepoTooltip:         i18n.T("tray.about_repo_tooltip"),
		Quit:                     i18n.T("tray.quit"),
		QuitTooltip:              i18n.T("tray.quit_tooltip"),
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

func (a *App) startup() {
	applog.Init()
	if p, err := config.Path(); err == nil {
		applog.Printf("config path: %s", p)
	}
	_ = a.notify.Init(a.notifySvc)

	cfg, err := config.Load()
	if err != nil {
		applog.Printf("config load error: %v", err)
		a.setError(err.Error())
	} else {
		a.mu.Lock()
		a.cfg = cfg
		a.mu.Unlock()
		activeURL, activeUser, activeTok := "", "", ""
		if acc, ok := cfg.Active(); ok {
			activeURL, activeUser, activeTok = acc.ServerURL, acc.Username, acc.ClientToken
		}
		applog.Printf("config loaded configured=%v servers=%d active=%q user=%q token=%s",
			cfg.Configured(), len(cfg.Servers), activeURL, activeUser, applog.TokenFingerprint(activeTok))
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
	a.rebuildApplicationMenu()
	if acc, ok := a.cfg.Active(); ok {
		a.applyActiveProxy(acc)
	}

	autostartOn := a.syncAutostartOnStartup()
	a.tray = tray.New(a.wailsApp, a.trayIcon, tray.Callbacks{
		OnOpen:              a.ShowWindow,
		OnSettings:          a.OpenSettings,
		OnHistory:           a.OpenNotificationHistory,
		OnSend:              a.OpenSendMessage,
		OnSelectServer:      func(id string) { _ = a.SetActiveServer(id) },
		OnAutostart:         a.setAutostartEnabled,
		OnLanguage:          a.SetLocale,
		OnAboutLicense:      a.OpenLicense,
		OnAboutVersion:      a.OpenLatestRelease,
		OnAboutCheckUpdates: a.CheckForUpdates,
		OnAboutRepo:         a.OpenRepository,
		OnQuit:              a.Quit,
	}, tray.Options{
		AutostartEnabled: autostartOn,
		Locale:           i18n.Locale(),
		Languages:        trayLanguages(),
		Labels:           trayLabels(),
	})
	a.tray.Start()
	a.refreshTrayServers()

	if a.cfg.Configured() {
		a.mu.Lock()
		a.view = "gotify"
		servers := append([]config.ServerAccount(nil), a.cfg.Servers...)
		a.mu.Unlock()
		a.sessions.StartAll(servers)
		a.updateTrayConnectionStatus()
		applog.Printf("startup: opening gotify view uiPath=%s", gotifyproxy.UIPrefix+"/")
	} else {
		a.tray.SetStatus(i18n.T("tray.status_not_configured"))
		applog.Printf("startup: setup view (not configured)")
	}
	a.emitStatus()
	a.maybeAutoCheckUpdates()
}

// onRuntimeReady re-emits session state after the frontend has subscribed to events.
func (a *App) onRuntimeReady() {
	a.mu.Lock()
	a.ready = true
	a.mu.Unlock()
	a.restoreWindowState()
	a.emitStatus()
	a.mu.Lock()
	configured := a.cfg.Configured()
	view := a.view
	acc, hasActive := a.cfg.Active()
	a.mu.Unlock()
	if configured && view == "gotify" && hasActive {
		applog.Printf("runtimeReady: restoring gotify view (hiding to tray)")
		a.emit("show-gotify", a.gotifyPayload(acc))
		if a.window != nil {
			a.window.Hide()
		}
	} else if a.window != nil {
		a.window.Show()
	}
}

func (a *App) shutdown() {
	if a.sessions != nil {
		a.sessions.StopAll()
	}
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

func (a *App) onWindowClosing(e *application.WindowEvent) {
	a.SaveWindowState()
	a.mu.Lock()
	quitting := a.quitting
	a.mu.Unlock()
	if quitting {
		return
	}
	e.Cancel()
	if a.window != nil {
		a.window.Hide()
	}
}

// restoreWindowState applies the persisted window size, position and maximised
// state. It does not show the window.
func (a *App) restoreWindowState() {
	if a.window == nil {
		return
	}
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	w, h := cfg.ResolvedWindowSize()
	a.window.SetSize(w, h)
	if cfg.WindowPosSet && config.WindowPositionUsable(cfg.WindowX, cfg.WindowY, w, h) {
		a.window.SetPosition(cfg.WindowX, cfg.WindowY)
	}
	if cfg.WindowMaximised {
		a.window.Maximise()
	}
}

// SaveWindowState persists the current window geometry. While maximised, the
// last known normal bounds are kept and only the maximised flag is updated.
func (a *App) SaveWindowState() {
	if a.window == nil {
		return
	}
	if a.window.IsMinimised() {
		return
	}
	w, h := a.window.Size()
	x, y := a.window.Position()
	maxed := a.window.IsMaximised()

	a.mu.Lock()
	cfg := config.MergeWindowGeometry(a.cfg, w, h, x, y, maxed)
	unchanged := cfg.WindowWidth == a.cfg.WindowWidth &&
		cfg.WindowHeight == a.cfg.WindowHeight &&
		cfg.WindowX == a.cfg.WindowX &&
		cfg.WindowY == a.cfg.WindowY &&
		cfg.WindowPosSet == a.cfg.WindowPosSet &&
		cfg.WindowMaximised == a.cfg.WindowMaximised
	if unchanged {
		a.mu.Unlock()
		return
	}
	a.cfg = cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		applog.Printf("window geometry save: %v", err)
	}
}

// GetStatus returns the current app status for the UI.
func (a *App) GetStatus() AppStatus {
	servers := a.buildServerInfos()
	a.mu.Lock()
	defer a.mu.Unlock()
	st := AppStatus{
		Configured:     a.cfg.Configured(),
		ActiveServerID: a.cfg.ActiveServerID,
		Servers:        servers,
		View:           a.view,
		Error:          a.lastErr,
		NotifyReady:    a.notify.Ready(),
		NotifyError:    a.notify.LastError(),
	}
	if acc, ok := a.cfg.Active(); ok {
		st.ServerURL = acc.ServerURL
		st.Username = acc.Username
		st.MinPriority = acc.MinPriority
		st.LastMessageID = acc.LastMessageID
	}
	if a.sessions != nil {
		st.Connected = a.sessions.AnyConnected()
		st.ConnectedCount, st.EnabledCount = a.sessions.Counts()
		if st.Error == "" {
			st.Error = a.sessions.LastError()
		}
	}
	return st
}

// GetServerURL returns the active Gotify base URL.
func (a *App) GetServerURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if acc, ok := a.cfg.Active(); ok {
		return acc.ServerURL
	}
	return ""
}

// GetClientToken returns the active Gotify client token for stream + embedded UI auth.
func (a *App) GetClientToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if acc, ok := a.cfg.Active(); ok {
		return acc.ClientToken
	}
	return ""
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

// OpenSettings switches the UI back to the setup view.
func (a *App) OpenSettings() {
	a.mu.Lock()
	a.view = "setup"
	a.mu.Unlock()
	a.ShowWindow()
	a.emitStatus()
	a.emit("show-setup", nil)
}

// ShowWindow brings the main window to the front.
func (a *App) ShowWindow() {
	if a.window == nil {
		return
	}
	a.window.Show()
	a.window.UnMinimise()
}

// OpenRepository opens the project repository in the default browser.
func (a *App) OpenRepository() {
	a.openURL(version.RepositoryURL)
}

// OpenLicense opens the project LICENSE file in the default browser.
func (a *App) OpenLicense() {
	a.openURL(version.LicenseURL)
}

// OpenLatestRelease opens the latest GitHub release page in the default browser.
func (a *App) OpenLatestRelease() {
	a.openURL(version.ReleasesURL)
}

func (a *App) openURL(url string) {
	if a.wailsApp == nil || url == "" {
		return
	}
	_ = a.wailsApp.Browser.OpenURL(url)
}

// Quit exits the application for real.
func (a *App) Quit() {
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	if a.sessions != nil {
		a.sessions.StopAll()
	}
	if a.wailsApp != nil {
		a.wailsApp.Quit()
	}
}

func (a *App) onSessionStatus() {
	a.updateTrayConnectionStatus()
	a.emitStatus()
}

func (a *App) handleMessage(serverID string, _ config.ServerAccount, msg stream.Message) {
	a.mu.Lock()
	accPtr := a.cfg.FindByID(serverID)
	if accPtr == nil {
		a.mu.Unlock()
		return
	}
	acc := *accPtr
	multi := len(a.cfg.Servers) > 1
	mode := config.NormalizeHistoryMode(a.cfg.HistoryMode)
	a.mu.Unlock()

	if msg.ID != 0 && acc.LastMessageID != 0 && msg.ID <= acc.LastMessageID {
		return
	}

	eligible := config.ShouldNotify(acc.MinPriority, msg.Priority)

	id := history.EntryID(serverID, msg.ID, msg.AppID)
	entry := history.Entry{
		ID:          id,
		Title:       msg.Title,
		Body:        msg.Message,
		Priority:    msg.Priority,
		AppID:       msg.AppID,
		ServerID:    serverID,
		ServerLabel: acc.Label,
		ReceivedAt:  time.Now(),
		Notified:    eligible,
	}

	toastTitle := msg.Title
	if multi {
		label := acc.Label
		if label == "" {
			label = config.HostnameLabel(acc.ServerURL)
		}
		toastTitle = "[" + label + "] " + msg.Title
	}

	if !eligible {
		if mode == config.HistoryModeAll {
			a.recordHistory(entry)
		}
		if msg.ID > 0 {
			a.persistLastMessageID(serverID, msg.ID)
		}
		return
	}

	applog.Printf("message server=%s id=%s title=%q priority=%d → notify", serverID, id, msg.Title, msg.Priority)
	a.emit("gotify-message", map[string]string{
		"id":          id,
		"title":       msg.Title,
		"body":        msg.Message,
		"serverId":    serverID,
		"serverLabel": acc.Label,
	})
	if err := a.notify.Show(id, toastTitle, msg.Message); err != nil {
		a.setError("notification: " + err.Error())
		if a.tray != nil {
			a.tray.SetStatus(i18n.T("tray.status_notify_error"))
		}
		a.emitStatus()
	} else if a.tray != nil {
		a.tray.SetStatus(i18n.Tf("tray.msg_prefix", map[string]string{
			"Title": truncateRunes(toastTitle, 40),
		}))
	}
	a.recordHistory(entry)

	if msg.ID > 0 {
		a.persistLastMessageID(serverID, msg.ID)
	}
}

func (a *App) persistLastMessageID(serverID string, msgID uint) {
	a.mu.Lock()
	if !a.cfg.SetLastMessageID(serverID, msgID) {
		a.mu.Unlock()
		return
	}
	cfg := a.cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		applog.Printf("last message id save config: %v", err)
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
	a.emit("history-updated", nil)
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
	a.emit("history-updated", nil)
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
	a.emit("history-updated", nil)
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
	if a.window != nil {
		a.window.Show()
		a.window.UnMinimise()
	}
	a.emit("show-history", nil)
	a.emitStatus()
}

// LeaveNotificationHistory returns to the Gotify UI if configured, else setup.
func (a *App) LeaveNotificationHistory() {
	a.leaveOverlayView()
}

// ListApplications returns Gotify applications for the active account (no tokens).
func (a *App) ListApplications() ([]push.Application, error) {
	a.mu.Lock()
	acc, ok := a.cfg.Active()
	a.mu.Unlock()
	if !ok || strings.TrimSpace(acc.ClientToken) == "" {
		return nil, fmt.Errorf("not configured")
	}
	return a.push.ListApplications(acc.ServerURL, acc.ClientToken)
}

// SendApplicationMessage sends a push message via the active Gotify server.
func (a *App) SendApplicationMessage(appID uint, title, message string, priority int) error {
	a.mu.Lock()
	acc, ok := a.cfg.Active()
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("not configured")
	}
	return a.SendApplicationMessageToServer(acc.ID, appID, title, message, priority)
}

// OpenSendMessage shows the window with the send-message view.
func (a *App) OpenSendMessage() {
	a.mu.Lock()
	a.view = "send"
	a.mu.Unlock()
	if a.window != nil {
		a.window.Show()
		a.window.UnMinimise()
	}
	a.emit("show-send", nil)
	a.emitStatus()
}

// LeaveSendMessage returns to the Gotify UI if configured, else setup.
func (a *App) LeaveSendMessage() {
	a.leaveOverlayView()
}

func (a *App) leaveOverlayView() {
	a.mu.Lock()
	if a.cfg.Configured() {
		a.view = "gotify"
	} else {
		a.view = "setup"
	}
	view := a.view
	a.mu.Unlock()
	if view == "gotify" {
		a.mu.Lock()
		acc, ok := a.cfg.Active()
		a.mu.Unlock()
		if ok {
			a.emit("show-gotify", a.gotifyPayload(acc))
		} else {
			a.emit("show-setup", nil)
		}
	} else {
		a.emit("show-setup", nil)
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
	a.rebuildApplicationMenu()
	if a.window != nil {
		a.window.SetTitle(i18n.T("app.name"))
	}
	a.emit("locale-changed", map[string]string{
		"locale":  lang,
		"catalog": a.GetI18nCatalog(),
	})
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

func (a *App) setAutostartEnabled(enabled bool) {
	if a.autostart == nil {
		applog.Printf("autostart unavailable")
		return
	}
	if err := a.autostart.Set(enabled); err != nil {
		applog.Printf("autostart set failed: %v", err)
		a.setError("autostart: " + err.Error())
		a.applyAutostartUI(a.autostart.IsEnabled())
		a.emitStatus()
		return
	}
	a.persistAutostart(enabled)
	a.applyAutostartUI(enabled)
	applog.Printf("autostart enabled=%v", enabled)
}

func (a *App) syncAutostartOnStartup() bool {
	if a.autostart == nil {
		return false
	}
	enabled := a.autostart.IsEnabled()
	if a.cfg.Autostart && !enabled {
		if err := a.autostart.Enable(); err != nil {
			applog.Printf("autostart re-enable failed: %v", err)
		} else {
			enabled = true
		}
	}
	if enabled {
		if err := a.autostart.Enable(); err != nil {
			applog.Printf("autostart refresh failed: %v", err)
		}
	}
	if a.cfg.Autostart != enabled {
		a.persistAutostart(enabled)
	}
	if a.autostartMenuItem != nil {
		a.autostartMenuItem.SetChecked(enabled)
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
		a.autostartMenuItem.SetChecked(enabled)
		if a.appMenu != nil {
			a.appMenu.Update()
		}
	}
}

func (a *App) setError(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}

func (a *App) emit(name string, data any) {
	if a.wailsApp == nil {
		return
	}
	if data == nil {
		a.wailsApp.Event.Emit(name)
		return
	}
	a.wailsApp.Event.Emit(name, data)
}

func (a *App) emitStatus() {
	a.emit("status", a.GetStatus())
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
