package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/elpatron68/gotify-desktop/internal/auth"
	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/gotifyproxy"
	"github.com/elpatron68/gotify-desktop/internal/notify"
	"github.com/elpatron68/gotify-desktop/internal/stream"
	"github.com/elpatron68/gotify-desktop/internal/tray"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// AppStatus is exposed to the frontend.
type AppStatus struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	ServerURL  string `json:"serverUrl"`
	View       string `json:"view"` // "setup" | "gotify"
	Error      string `json:"error"`
	MinPriority int   `json:"minPriority"`
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

	auth    *auth.Client
	stream  *stream.Client
	notify  *notify.Notifier
	tray    *tray.Controller
	proxy   *gotifyproxy.Proxy
	trayIcon []byte
}

// NewApp creates the application.
func NewApp(trayIcon []byte) *App {
	return &App{
		auth:     auth.New(),
		notify:   &notify.Notifier{},
		proxy:    &gotifyproxy.Proxy{},
		view:     "setup",
		trayIcon: trayIcon,
	}
}

// ProxyMiddleware exposes the Gotify reverse-proxy for the Wails asset server.
func (a *App) ProxyMiddleware() func(http.Handler) http.Handler {
	return a.proxy.Middleware
}

// buildMenu creates the native application menu (needed on macOS where systray is disabled).
func (a *App) buildMenu() *menu.Menu {
	return menu.NewMenuFromItems(
		menu.SubMenu("Gotify", menu.NewMenuFromItems(
			menu.Text("Show Window", nil, func(_ *menu.CallbackData) { a.ShowWindow() }),
			menu.Text("Settings", nil, func(_ *menu.CallbackData) { a.OpenSettings() }),
			menu.Separator(),
			menu.Text("Quit", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) { a.Quit() }),
		)),
	)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_ = a.notify.Init(ctx)

	cfg, err := config.Load()
	if err != nil {
		a.setError(err.Error())
	} else {
		a.mu.Lock()
		a.cfg = cfg
		a.mu.Unlock()
	}
	_ = a.proxy.Update(a.cfg.ServerURL, a.cfg.ClientToken)

	a.tray = tray.New(a.trayIcon, tray.Callbacks{
		OnOpen:     a.ShowWindow,
		OnSettings: a.OpenSettings,
		OnQuit:     a.Quit,
	})
	a.tray.Start()

	if a.cfg.Configured() {
		a.mu.Lock()
		a.view = "gotify"
		a.mu.Unlock()
		a.startStream()
	} else {
		a.tray.SetStatus("Status: not configured")
	}
	a.emitStatus()
}

func (a *App) shutdown(ctx context.Context) {
	a.stopStream()
	if a.tray != nil {
		a.tray.Stop()
	}
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
		Configured:  a.cfg.Configured(),
		Connected:   a.connected,
		ServerURL:   a.cfg.ServerURL,
		View:        a.view,
		Error:       a.lastErr,
		MinPriority: a.cfg.MinPriority,
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

// GetUIPath returns the local reverse-proxy path for the embedded Gotify UI.
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
	a.cfg.ClientToken = token
	a.view = "gotify"
	a.lastErr = ""
	cfg := a.cfg
	a.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	_ = a.proxy.Update(cfg.ServerURL, cfg.ClientToken)

	a.stopStream()
	a.startStream()
	a.emitStatus()
	runtime.EventsEmit(a.ctx, "show-gotify", map[string]string{
		"serverUrl": cfg.ServerURL,
		"token":     cfg.ClientToken,
		"uiPath":    gotifyproxy.UIPrefix + "/",
	})
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
					a.tray.SetStatus("Status: connected")
				} else if errMsg != "" {
					a.tray.SetStatus("Status: error")
				} else {
					a.tray.SetStatus("Status: disconnected")
				}
			}
			a.emitStatus()
			if errMsg != "" && containsUnauthorized(errMsg) {
				a.OpenSettings()
			}
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
	a.mu.Unlock()

	if !config.ShouldNotify(minP, msg.Priority) {
		return
	}
	if msg.ID != 0 && lastID != 0 && msg.ID <= lastID {
		return
	}

	id := strconv.FormatUint(uint64(msg.ID), 10)
	if id == "0" {
		id = fmt.Sprintf("msg-%d", msg.AppID)
	}
	_ = a.notify.Show(id, msg.Title, msg.Message)

	if msg.ID > 0 {
		a.mu.Lock()
		a.cfg.LastMessageID = msg.ID
		cfg := a.cfg
		a.mu.Unlock()
		_ = config.Save(cfg)
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

func containsUnauthorized(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "unauthorized") || strings.Contains(lower, "invalid client token")
}
