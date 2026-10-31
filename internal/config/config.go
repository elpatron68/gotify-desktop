package config

import (
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

// Config holds persistent application settings.
// Passwords are never stored (same model as gotify/android Settings).
type Config struct {
	ServerURL     string `json:"server_url"`
	Username      string `json:"username,omitempty"`
	ClientToken   string `json:"client_token"`
	MinPriority   int    `json:"min_priority"`
	LastMessageID uint   `json:"last_message_id,omitempty"`
	Autostart     bool   `json:"autostart,omitempty"`
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

func Default() Config {
	return Config{MinPriority: 0, HistoryMode: HistoryModeToasts}
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
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parse config: %w", err)
	}
	cfg.HistoryMode = NormalizeHistoryMode(cfg.HistoryMode)
	return cfg, nil
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

// Configured reports whether server URL and client token are present.
func (c Config) Configured() bool {
	return strings.TrimSpace(c.ServerURL) != "" && strings.TrimSpace(c.ClientToken) != ""
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
