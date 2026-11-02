package main

import (
	"fmt"
	"strings"

	"github.com/elpatron68/gotify-desktop/internal/applog"
	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/gotifyproxy"
	"github.com/elpatron68/gotify-desktop/internal/i18n"
	"github.com/elpatron68/gotify-desktop/internal/push"
	"github.com/elpatron68/gotify-desktop/internal/tray"
)

// ServerInfo is exposed to the frontend for the server list.
type ServerInfo struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	ServerURL string `json:"serverUrl"`
	Username  string `json:"username"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	Error     string `json:"error"`
	Active    bool   `json:"active"`
}

// ListServers returns all configured servers with runtime status.
func (a *App) ListServers() []ServerInfo {
	return a.buildServerInfos()
}

// AddServer authenticates, upserts by URL+username, starts the stream, and may set Active.
func (a *App) AddServer(serverURL, username, password, label string) (ServerInfo, error) {
	normalized, err := config.NormalizeServerURL(serverURL)
	if err != nil {
		return ServerInfo{}, err
	}
	token, err := a.auth.VerifyAndCreateClient(normalized, username, password)
	if err != nil {
		return ServerInfo{}, err
	}

	a.mu.Lock()
	wasConfigured := a.cfg.Configured()
	label = strings.TrimSpace(label)
	if label == "" {
		label = config.HostnameLabel(normalized)
	}

	var acc config.ServerAccount
	if existing := a.cfg.FindByURLUser(normalized, username); existing != nil {
		existing.ClientToken = token
		existing.Username = username
		existing.ServerURL = normalized
		existing.Enabled = true
		if label != "" {
			existing.Label = label
		} else if strings.TrimSpace(existing.Label) == "" {
			existing.Label = config.HostnameLabel(normalized)
		}
		acc = *existing
	} else {
		acc = config.ServerAccount{
			ID:          config.NewServerID(),
			Label:       label,
			ServerURL:   normalized,
			Username:    username,
			ClientToken: token,
			MinPriority: 0,
			Enabled:     true,
		}
		a.cfg.Servers = append(a.cfg.Servers, acc)
	}

	if a.cfg.ActiveServerID == "" || a.cfg.FindByID(a.cfg.ActiveServerID) == nil {
		a.cfg.ActiveServerID = acc.ID
	}
	// First server ever: always make it active.
	if !wasConfigured {
		a.cfg.ActiveServerID = acc.ID
	}
	a.cfg.Normalize()
	activeID := a.cfg.ActiveServerID
	if !wasConfigured {
		a.view = "gotify"
	}
	a.lastErr = ""
	cfg := a.cfg
	a.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		return ServerInfo{}, fmt.Errorf("save config: %w", err)
	}
	applog.Printf("add server id=%s server=%q user=%q token=%s",
		acc.ID, acc.ServerURL, acc.Username, applog.TokenFingerprint(acc.ClientToken))

	if a.sessions != nil {
		a.sessions.UpdateAccount(acc)
	}
	if activeID == acc.ID {
		a.applyActiveProxy(acc)
	}

	a.emitStatus()
	a.refreshTrayServers()
	a.updateTrayConnectionStatus()
	a.rebuildApplicationMenu()

	info := a.serverInfoFor(acc.ID)

	// Hide to tray only on first successful configure.
	if !wasConfigured {
		a.emit("show-gotify", a.gotifyPayload(acc))
		if a.window != nil {
			a.window.Hide()
		}
		applog.Printf("add server: first configure, window hidden to tray")
	} else {
		// Stay on setup for further adds; still refresh gotify if currently showing it.
		a.mu.Lock()
		view := a.view
		a.mu.Unlock()
		if view == "gotify" && activeID == acc.ID {
			a.emit("show-gotify", a.gotifyPayload(acc))
		}
	}
	return info, nil
}

// Connect is a compatibility alias for AddServer with an empty label.
func (a *App) Connect(serverURL, username, password string) error {
	_, err := a.AddServer(serverURL, username, password, "")
	return err
}

// RemoveServer stops the session and removes the account locally.
func (a *App) RemoveServer(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("server id required")
	}
	a.mu.Lock()
	if !a.cfg.RemoveServer(id) {
		a.mu.Unlock()
		return fmt.Errorf("server not found")
	}
	cfg := a.cfg
	active, hasActive := a.cfg.Active()
	if !a.cfg.Configured() {
		a.view = "setup"
	}
	view := a.view
	a.mu.Unlock()

	if a.sessions != nil {
		a.sessions.Stop(id)
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if hasActive {
		a.applyActiveProxy(active)
		if view == "gotify" {
			a.emit("show-gotify", a.gotifyPayload(active))
		}
	} else {
		_ = a.proxy.Update("", "")
		a.emit("show-setup", nil)
	}
	a.emitStatus()
	a.refreshTrayServers()
	a.updateTrayConnectionStatus()
	a.rebuildApplicationMenu()
	return nil
}

// SetActiveServer switches the embedded UI / default send target.
func (a *App) SetActiveServer(id string) error {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	acc := a.cfg.FindByID(id)
	if acc == nil {
		a.mu.Unlock()
		return fmt.Errorf("server not found")
	}
	a.cfg.ActiveServerID = id
	account := *acc
	view := a.view
	cfg := a.cfg
	a.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	a.applyActiveProxy(account)
	a.emitStatus()
	a.refreshTrayServers()
	a.rebuildApplicationMenu()
	if view == "gotify" {
		a.emit("show-gotify", a.gotifyPayload(account))
	}
	return nil
}

// RenameServer updates the display label.
func (a *App) RenameServer(id, label string) error {
	id = strings.TrimSpace(id)
	label = strings.TrimSpace(label)
	a.mu.Lock()
	acc := a.cfg.FindByID(id)
	if acc == nil {
		a.mu.Unlock()
		return fmt.Errorf("server not found")
	}
	if label == "" {
		label = config.HostnameLabel(acc.ServerURL)
	}
	acc.Label = label
	cfg := a.cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	a.emitStatus()
	a.refreshTrayServers()
	a.rebuildApplicationMenu()
	return nil
}

// SetServerEnabled enables or disables a server stream.
func (a *App) SetServerEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	acc := a.cfg.FindByID(id)
	if acc == nil {
		a.mu.Unlock()
		return fmt.Errorf("server not found")
	}
	acc.Enabled = enabled
	account := *acc
	cfg := a.cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if a.sessions != nil {
		a.sessions.UpdateAccount(account)
	}
	a.emitStatus()
	a.refreshTrayServers()
	a.updateTrayConnectionStatus()
	a.rebuildApplicationMenu()
	return nil
}

// ListApplicationsForServer lists apps on the given server.
func (a *App) ListApplicationsForServer(serverID string) ([]push.Application, error) {
	a.mu.Lock()
	acc := a.cfg.FindByID(serverID)
	if acc == nil || strings.TrimSpace(acc.ClientToken) == "" {
		a.mu.Unlock()
		return nil, fmt.Errorf("not configured")
	}
	url, token := acc.ServerURL, acc.ClientToken
	a.mu.Unlock()
	return a.push.ListApplications(url, token)
}

// SendApplicationMessageToServer sends a message via the given server.
func (a *App) SendApplicationMessageToServer(serverID string, appID uint, title, message string, priority int) error {
	a.mu.Lock()
	acc := a.cfg.FindByID(serverID)
	if acc == nil || strings.TrimSpace(acc.ClientToken) == "" {
		a.mu.Unlock()
		return fmt.Errorf("not configured")
	}
	url, token := acc.ServerURL, acc.ClientToken
	a.mu.Unlock()
	if appID == 0 {
		return fmt.Errorf("application is required")
	}
	if priority < 0 {
		priority = 0
	}
	if priority > 10 {
		priority = 10
	}
	applog.Printf("send message server=%s app=%d", serverID, appID)
	return a.push.Send(url, token, appID, title, message, priority)
}

func (a *App) applyActiveProxy(acc config.ServerAccount) {
	_ = a.proxy.Update(acc.ServerURL, acc.ClientToken)
	if _, err := a.proxy.StartWSProxy(); err != nil {
		applog.Printf("ws proxy start failed: %v", err)
	}
}

func (a *App) gotifyPayload(acc config.ServerAccount) map[string]string {
	return map[string]string{
		"serverUrl": acc.ServerURL,
		"token":     acc.ClientToken,
		"uiPath":    gotifyproxy.UIPrefix + "/",
		"serverId":  acc.ID,
		"label":     acc.Label,
	}
}

func (a *App) buildServerInfos() []ServerInfo {
	a.mu.Lock()
	servers := append([]config.ServerAccount(nil), a.cfg.Servers...)
	activeID := a.cfg.ActiveServerID
	a.mu.Unlock()

	var statuses []struct {
		id        string
		connected bool
		err       string
	}
	if a.sessions != nil {
		for _, st := range a.sessions.StatusFor(servers) {
			statuses = append(statuses, struct {
				id        string
				connected bool
				err       string
			}{st.ServerID, st.Connected, st.Error})
		}
	}
	statusByID := map[string]struct {
		connected bool
		err       string
	}{}
	for _, st := range statuses {
		statusByID[st.id] = struct {
			connected bool
			err       string
		}{st.connected, st.err}
	}

	out := make([]ServerInfo, 0, len(servers))
	for _, s := range servers {
		info := ServerInfo{
			ID:        s.ID,
			Label:     s.Label,
			ServerURL: s.ServerURL,
			Username:  s.Username,
			Enabled:   s.Enabled,
			Active:    s.ID == activeID,
		}
		if st, ok := statusByID[s.ID]; ok {
			info.Connected = st.connected
			info.Error = st.err
		}
		out = append(out, info)
	}
	return out
}

func (a *App) serverInfoFor(id string) ServerInfo {
	for _, s := range a.buildServerInfos() {
		if s.ID == id {
			return s
		}
	}
	return ServerInfo{ID: id}
}

func (a *App) refreshTrayServers() {
	if a.tray == nil {
		return
	}
	infos := a.buildServerInfos()
	entries := make([]tray.ServerEntry, 0, len(infos))
	for _, s := range infos {
		entries = append(entries, tray.ServerEntry{
			ID:     s.ID,
			Label:  s.Label,
			Active: s.Active,
		})
	}
	a.tray.SetServers(entries)
}

func (a *App) updateTrayConnectionStatus() {
	if a.tray == nil {
		return
	}
	a.mu.Lock()
	configured := a.cfg.Configured()
	a.mu.Unlock()
	if !configured {
		a.tray.SetStatus(i18n.T("tray.status_not_configured"))
		return
	}
	if a.sessions == nil {
		a.tray.SetStatus(i18n.T("tray.status_disconnected"))
		return
	}
	connected, total := a.sessions.Counts()
	switch {
	case total == 0:
		a.tray.SetStatus(i18n.T("tray.status_disconnected"))
	case connected == total:
		a.tray.SetStatus(i18n.T("tray.status_connected"))
	case connected > 0:
		a.tray.SetStatus(i18n.Tf("tray.status_partial", map[string]string{
			"Connected": fmt.Sprintf("%d", connected),
			"Total":     fmt.Sprintf("%d", total),
		}))
	case a.sessions.HasError():
		a.tray.SetStatus(i18n.T("tray.status_error"))
	default:
		a.tray.SetStatus(i18n.T("tray.status_disconnected"))
	}
}
