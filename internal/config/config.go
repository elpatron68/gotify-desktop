package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const appDirName = "gotify-desktop"

const (
	HistoryModeToasts = "toasts"
	HistoryModeAll    = "all"
)

const (
	DefaultWindowWidth  = 1100
	DefaultWindowHeight = 800
	MinWindowWidth      = 640
	MinWindowHeight     = 480
)

// ServerAccount is one Gotify server credential set.
type ServerAccount struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ServerURL     string `json:"server_url"`
	Username      string `json:"username,omitempty"`
	ClientToken   string `json:"client_token"`
	MinPriority   int    `json:"min_priority"`
	LastMessageID uint   `json:"last_message_id,omitempty"`
	Enabled       bool   `json:"enabled"`
}

// UnmarshalJSON defaults Enabled to true when the field is omitted.
func (s *ServerAccount) UnmarshalJSON(data []byte) error {
	type wire struct {
		ID            string `json:"id"`
		Label         string `json:"label"`
		ServerURL     string `json:"server_url"`
		Username      string `json:"username,omitempty"`
		ClientToken   string `json:"client_token"`
		MinPriority   int    `json:"min_priority"`
		LastMessageID uint   `json:"last_message_id,omitempty"`
		Enabled       *bool  `json:"enabled"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	s.ID = w.ID
	s.Label = w.Label
	s.ServerURL = w.ServerURL
	s.Username = w.Username
	s.ClientToken = w.ClientToken
	s.MinPriority = w.MinPriority
	s.LastMessageID = w.LastMessageID
	if w.Enabled == nil {
		s.Enabled = true
	} else {
		s.Enabled = *w.Enabled
	}
	return nil
}

// Config holds persistent application settings.
// Passwords are never stored (same model as gotify/android Settings).
type Config struct {
	Servers        []ServerAccount `json:"servers"`
	ActiveServerID string          `json:"active_server_id,omitempty"`
	Autostart      bool            `json:"autostart,omitempty"`
	// Locale is a BCP-47 language tag (e.g. "de", "en"). Empty = follow the OS.
	Locale string `json:"locale,omitempty"`
	// HistoryMode is "toasts" (default) or "all". Unknown/empty values normalize to toasts.
	HistoryMode string `json:"history_mode,omitempty"`
	// Window geometry. Width/height <=0 means unset (use defaults). WindowPosSet
	// distinguishes a saved 0,0 position from "never placed".
	WindowWidth     int  `json:"window_width,omitempty"`
	WindowHeight    int  `json:"window_height,omitempty"`
	WindowX         int  `json:"window_x,omitempty"`
	WindowY         int  `json:"window_y,omitempty"`
	WindowPosSet    bool `json:"window_pos_set,omitempty"`
	WindowMaximised bool `json:"window_maximised,omitempty"`
	// UpdateCheckedUnix is the Unix timestamp of the last automatic update check.
	UpdateCheckedUnix int64 `json:"update_checked_unix,omitempty"`
}

// diskConfig is the on-disk shape including legacy singular fields for migration.
type diskConfig struct {
	Servers        []ServerAccount `json:"servers"`
	ActiveServerID string          `json:"active_server_id,omitempty"`
	// Legacy singular fields (pre multi-server).
	ServerURL     string `json:"server_url"`
	Username      string `json:"username,omitempty"`
	ClientToken   string `json:"client_token"`
	MinPriority   int    `json:"min_priority"`
	LastMessageID uint   `json:"last_message_id,omitempty"`

	Autostart         bool   `json:"autostart,omitempty"`
	Locale            string `json:"locale,omitempty"`
	HistoryMode       string `json:"history_mode,omitempty"`
	WindowWidth       int    `json:"window_width,omitempty"`
	WindowHeight      int    `json:"window_height,omitempty"`
	WindowX           int    `json:"window_x,omitempty"`
	WindowY           int    `json:"window_y,omitempty"`
	WindowPosSet      bool   `json:"window_pos_set,omitempty"`
	WindowMaximised   bool   `json:"window_maximised,omitempty"`
	UpdateCheckedUnix int64  `json:"update_checked_unix,omitempty"`
}

func Default() Config {
	return Config{HistoryMode: HistoryModeToasts}
}

func ClampWindowSize(w, h int) (int, int) {
	if w < MinWindowWidth {
		w = MinWindowWidth
	}
	if h < MinWindowHeight {
		h = MinWindowHeight
	}
	return w, h
}

func (c Config) ResolvedWindowSize() (int, int) {
	w, h := c.WindowWidth, c.WindowHeight
	if w <= 0 {
		w = DefaultWindowWidth
	}
	if h <= 0 {
		h = DefaultWindowHeight
	}
	return ClampWindowSize(w, h)
}

func WindowPositionUsable(x, y, w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if x+w < 50 || y+h < 50 {
		return false
	}
	if x > 10000 || y > 10000 {
		return false
	}
	return true
}

// Dir returns the platform config directory for this app.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDirName), nil
}

// Path returns the full path to config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads config from disk. Missing file returns Default() with nil error.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	return Parse(data)
}

// Parse unmarshals config JSON with legacy migration and normalization.
func Parse(data []byte) (Config, error) {
	var disk diskConfig
	if err := json.Unmarshal(data, &disk); err != nil {
		return Default(), fmt.Errorf("parse config: %w", err)
	}
	cfg := migrateDisk(disk)
	cfg.HistoryMode = NormalizeHistoryMode(cfg.HistoryMode)
	cfg.Normalize()
	return cfg, nil
}

func migrateDisk(disk diskConfig) Config {
	cfg := Config{
		Servers:           disk.Servers,
		ActiveServerID:    disk.ActiveServerID,
		Autostart:         disk.Autostart,
		Locale:            disk.Locale,
		HistoryMode:       disk.HistoryMode,
		WindowWidth:       disk.WindowWidth,
		WindowHeight:      disk.WindowHeight,
		WindowX:           disk.WindowX,
		WindowY:           disk.WindowY,
		WindowPosSet:      disk.WindowPosSet,
		WindowMaximised:   disk.WindowMaximised,
		UpdateCheckedUnix: disk.UpdateCheckedUnix,
	}
	if len(cfg.Servers) == 0 &&
		strings.TrimSpace(disk.ServerURL) != "" &&
		strings.TrimSpace(disk.ClientToken) != "" {
		label := HostnameLabel(disk.ServerURL)
		id := NewServerID()
		cfg.Servers = []ServerAccount{{
			ID:            id,
			Label:         label,
			ServerURL:     disk.ServerURL,
			Username:      disk.Username,
			ClientToken:   disk.ClientToken,
			MinPriority:   disk.MinPriority,
			LastMessageID: disk.LastMessageID,
			Enabled:       true,
		}}
		cfg.ActiveServerID = id
	}
	return cfg
}

// Normalize ensures IDs, labels, and ActiveServerID are valid.
func (c *Config) Normalize() {
	for i := range c.Servers {
		if strings.TrimSpace(c.Servers[i].ID) == "" {
			c.Servers[i].ID = NewServerID()
		}
		if strings.TrimSpace(c.Servers[i].Label) == "" {
			c.Servers[i].Label = HostnameLabel(c.Servers[i].ServerURL)
		}
	}
	c.normalizeActive()
}

func (c *Config) normalizeActive() {
	if len(c.Servers) == 0 {
		c.ActiveServerID = ""
		return
	}
	if c.FindByID(c.ActiveServerID) != nil {
		return
	}
	for _, s := range c.Servers {
		if s.Enabled && strings.TrimSpace(s.ClientToken) != "" {
			c.ActiveServerID = s.ID
			return
		}
	}
	c.ActiveServerID = c.Servers[0].ID
}

// saveMu serialises Save so concurrent writers cannot clobber config.json.tmp.
var saveMu sync.Mutex

// MergeWindowGeometry applies observed window geometry to cfg. While maximised,
// existing normal bounds are kept and the maximised flag is set. If size was
// never saved, defaults are used — never the maximised (often fullscreen) size.
// Otherwise bounds are overwritten and maximised cleared.
func MergeWindowGeometry(cfg Config, w, h, x, y int, maximised bool) Config {
	w, h = ClampWindowSize(w, h)
	if maximised {
		cfg.WindowMaximised = true
		if cfg.WindowWidth <= 0 || cfg.WindowHeight <= 0 {
			cfg.WindowWidth = DefaultWindowWidth
			cfg.WindowHeight = DefaultWindowHeight
		}
		if !cfg.WindowPosSet {
			cfg.WindowX, cfg.WindowY = x, y
			cfg.WindowPosSet = true
		}
		return cfg
	}
	cfg.WindowWidth, cfg.WindowHeight = w, h
	cfg.WindowX, cfg.WindowY = x, y
	cfg.WindowPosSet = true
	cfg.WindowMaximised = false
	return cfg
}

// Save writes config atomically. Safe for concurrent use.
// Only the multi-server target schema is written (no legacy singular fields).
func Save(cfg Config) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	cfg.HistoryMode = NormalizeHistoryMode(cfg.HistoryMode)
	cfg.Normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Configured reports whether at least one server has a client token.
func (c Config) Configured() bool {
	for _, s := range c.Servers {
		if strings.TrimSpace(s.ClientToken) != "" {
			return true
		}
	}
	return false
}

// Active returns the active server account, if any.
func (c Config) Active() (ServerAccount, bool) {
	if s := c.FindByID(c.ActiveServerID); s != nil {
		return *s, true
	}
	return ServerAccount{}, false
}

// FindByID returns a pointer into c.Servers, or nil.
func (c Config) FindByID(id string) *ServerAccount {
	if id == "" {
		return nil
	}
	for i := range c.Servers {
		if c.Servers[i].ID == id {
			return &c.Servers[i]
		}
	}
	return nil
}

// FindByURLUser returns the account matching normalized URL + username.
func (c Config) FindByURLUser(serverURL, username string) *ServerAccount {
	for i := range c.Servers {
		if c.Servers[i].ServerURL == serverURL && c.Servers[i].Username == username {
			return &c.Servers[i]
		}
	}
	return nil
}

// UpdateServer replaces the account with the same ID.
func (c *Config) UpdateServer(acc ServerAccount) bool {
	for i := range c.Servers {
		if c.Servers[i].ID == acc.ID {
			c.Servers[i] = acc
			return true
		}
	}
	return false
}

// RemoveServer deletes the account by ID. Returns whether it was found.
func (c *Config) RemoveServer(id string) bool {
	for i := range c.Servers {
		if c.Servers[i].ID == id {
			c.Servers = append(c.Servers[:i], c.Servers[i+1:]...)
			c.normalizeActive()
			return true
		}
	}
	return false
}

// SetLastMessageID updates LastMessageID for the given server.
func (c *Config) SetLastMessageID(serverID string, id uint) bool {
	s := c.FindByID(serverID)
	if s == nil {
		return false
	}
	s.LastMessageID = id
	return true
}

// NewServerID returns a random UUIDv4 string.
func NewServerID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// HostnameLabel derives a default label from a server URL.
func HostnameLabel(serverURL string) string {
	u, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || u.Host == "" {
		raw := strings.TrimSpace(serverURL)
		if raw == "" {
			return "server"
		}
		return raw
	}
	host := u.Hostname()
	if host == "" {
		return u.Host
	}
	return host
}

// NormalizeServerURL trims trailing slashes and validates http(s) scheme.
func NormalizeServerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("server URL is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme %q (use http or https)", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("server URL must include a host")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// StreamURL builds the Gotify websocket stream URL from a server base URL.
func StreamURL(serverURL, token string) (string, error) {
	base, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	switch base.Scheme {
	case "https":
		base.Scheme = "wss"
	case "http":
		base.Scheme = "ws"
	default:
		return "", fmt.Errorf("unsupported scheme %q", base.Scheme)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/stream"
	q := base.Query()
	q.Set("token", token)
	base.RawQuery = q.Encode()
	return base.String(), nil
}

// ShouldNotify returns true if message priority meets the configured minimum.
func ShouldNotify(minPriority, messagePriority int) bool {
	return messagePriority >= minPriority
}

// NormalizeHistoryMode maps unknown or empty values to toasts; "all" (any case) stays all.
func NormalizeHistoryMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case HistoryModeAll:
		return HistoryModeAll
	default:
		return HistoryModeToasts
	}
}
