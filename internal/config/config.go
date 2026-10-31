package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const appDirName = "gotify-desktop"

// Config holds persistent application settings.
// Passwords are never stored (same model as gotify/android Settings).
type Config struct {
	ServerURL     string `json:"server_url"`
	Username      string `json:"username,omitempty"`
	ClientToken   string `json:"client_token"`
	MinPriority   int    `json:"min_priority"`
	LastMessageID uint   `json:"last_message_id,omitempty"`
	Autostart     bool   `json:"autostart,omitempty"`
}

func Default() Config {
	return Config{MinPriority: 0}
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
	return cfg, nil
}

// Save writes config atomically.
func Save(cfg Config) error {
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
