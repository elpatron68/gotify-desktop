package config

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNormalizeServerURL(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://gotify.example.com", "https://gotify.example.com", false},
		{"https://gotify.example.com/", "https://gotify.example.com", false},
		{"http://localhost:8080/", "http://localhost:8080", false},
		{"gotify.example.com", "https://gotify.example.com", false},
		{"", "", true},
		{"ftp://x", "", true},
	}
	for _, tc := range tests {
		got, err := NormalizeServerURL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("NormalizeServerURL(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeServerURL(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeServerURL(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestStreamURL(t *testing.T) {
	got, err := StreamURL("https://gotify.example.com", "tok")
	if err != nil {
		t.Fatal(err)
	}
	want := "wss://gotify.example.com/stream?token=tok"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got, err = StreamURL("http://localhost:80", "abc")
	if err != nil {
		t.Fatal(err)
	}
	want = "ws://localhost:80/stream?token=abc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeHistoryMode(t *testing.T) {
	if NormalizeHistoryMode("") != HistoryModeToasts {
		t.Fatal()
	}
	if NormalizeHistoryMode("ALL") != HistoryModeAll {
		t.Fatal()
	}
	if NormalizeHistoryMode("nope") != HistoryModeToasts {
		t.Fatal()
	}
}

func TestDefaultHistoryModeToasts(t *testing.T) {
	if Default().HistoryMode != HistoryModeToasts {
		t.Fatalf("Default().HistoryMode=%q want %q", Default().HistoryMode, HistoryModeToasts)
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HistoryMode != HistoryModeToasts {
		t.Fatalf("Load missing file HistoryMode=%q want %q", loaded.HistoryMode, HistoryModeToasts)
	}
}

func TestShouldNotify(t *testing.T) {
	if !ShouldNotify(0, 0) {
		t.Fatal("priority 0 should notify when min is 0")
	}
	if ShouldNotify(5, 4) {
		t.Fatal("priority 4 should not notify when min is 5")
	}
	if !ShouldNotify(5, 5) {
		t.Fatal("priority 5 should notify when min is 5")
	}
}

func TestSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	if dir, err := Dir(); err == nil {
		_ = os.MkdirAll(dir, 0o700)
	}

	id := NewServerID()
	cfg := Config{
		Servers: []ServerAccount{{
			ID:          id,
			Label:       "gotify.example.com",
			ServerURL:   "https://gotify.example.com",
			Username:    "admin",
			ClientToken: "secret",
			MinPriority: 2,
			Enabled:     true,
		}},
		ActiveServerID: id,
	}
	if d, err := Dir(); err == nil {
		_ = os.MkdirAll(d, 0o700)
	}

	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Configured() {
		t.Fatal("expected configured")
	}
	if len(loaded.Servers) != 1 {
		t.Fatalf("servers=%d", len(loaded.Servers))
	}
	s := loaded.Servers[0]
	if s.ServerURL != "https://gotify.example.com" || s.Username != "admin" || s.ClientToken != "secret" || s.MinPriority != 2 {
		t.Fatalf("loaded %+v", s)
	}
	if loaded.ActiveServerID != id {
		t.Fatalf("active=%q want %q", loaded.ActiveServerID, id)
	}

	// Save must not write legacy singular fields.
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["client_token"]; ok {
		t.Fatal("legacy top-level client_token should not be saved")
	}
	if _, ok := top["server_url"]; ok {
		t.Fatal("legacy top-level server_url should not be saved")
	}
}

func TestParseLegacyMigration(t *testing.T) {
	raw := []byte(`{
  "server_url": "https://gotify.example.com",
  "username": "admin",
  "client_token": "secret",
  "min_priority": 3,
  "last_message_id": 9
}`)
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("servers=%d", len(cfg.Servers))
	}
	s := cfg.Servers[0]
	if s.ServerURL != "https://gotify.example.com" || s.Username != "admin" || s.ClientToken != "secret" {
		t.Fatalf("%+v", s)
	}
	if s.MinPriority != 3 || s.LastMessageID != 9 || !s.Enabled {
		t.Fatalf("%+v", s)
	}
	if s.Label != "gotify.example.com" {
		t.Fatalf("label=%q", s.Label)
	}
	if cfg.ActiveServerID != s.ID || s.ID == "" {
		t.Fatalf("active=%q id=%q", cfg.ActiveServerID, s.ID)
	}
}

func TestParseAlreadyMigratedUntouched(t *testing.T) {
	raw := []byte(`{
  "servers": [{
    "id": "abc-1",
    "label": "home",
    "server_url": "https://a.example",
    "username": "u",
    "client_token": "tok",
    "enabled": true
  }],
  "active_server_id": "abc-1"
}`)
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers[0].ID != "abc-1" || cfg.ActiveServerID != "abc-1" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Servers[0].Label != "home" {
		t.Fatalf("label=%q", cfg.Servers[0].Label)
	}
}

func TestParseCorruptActiveRepaired(t *testing.T) {
	raw := []byte(`{
  "servers": [{
    "id": "abc-1",
    "label": "home",
    "server_url": "https://a.example",
    "client_token": "tok",
    "enabled": true
  }],
  "active_server_id": "missing"
}`)
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveServerID != "abc-1" {
		t.Fatalf("active=%q", cfg.ActiveServerID)
	}
}

func TestParseEnabledDefaultsTrue(t *testing.T) {
	raw := []byte(`{
  "servers": [{
    "id": "abc-1",
    "label": "home",
    "server_url": "https://a.example",
    "client_token": "tok"
  }]
}`)
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Servers[0].Enabled {
		t.Fatal("enabled should default true")
	}
}

func TestHostnameLabel(t *testing.T) {
	if HostnameLabel("https://gotify.example.com/path") != "gotify.example.com" {
		t.Fatal(HostnameLabel("https://gotify.example.com/path"))
	}
}

func TestFindByURLUser(t *testing.T) {
	cfg := Config{Servers: []ServerAccount{{
		ID: "1", ServerURL: "https://a.example", Username: "u", ClientToken: "t", Enabled: true,
	}}}
	if cfg.FindByURLUser("https://a.example", "u") == nil {
		t.Fatal("expected find")
	}
	if cfg.FindByURLUser("https://a.example", "other") != nil {
		t.Fatal("expected miss")
	}
}


func TestResolvedWindowSizeDefaults(t *testing.T) {
	c := Default()
	w, h := c.ResolvedWindowSize()
	if w != DefaultWindowWidth || h != DefaultWindowHeight {
		t.Fatalf("%d×%d", w, h)
	}
}

func TestResolvedWindowSizeClamps(t *testing.T) {
	c := Config{WindowWidth: 100, WindowHeight: 50}
	w, h := c.ResolvedWindowSize()
	if w != MinWindowWidth || h != MinWindowHeight {
		t.Fatalf("%d×%d", w, h)
	}
}

func TestWindowPositionUsable(t *testing.T) {
	if !WindowPositionUsable(100, 100, 800, 600) {
		t.Fatal("expected usable")
	}
	if WindowPositionUsable(-9000, -9000, 800, 600) {
		t.Fatal("expected unusable")
	}
}

func TestMergeWindowGeometryMaximisedPreservesBounds(t *testing.T) {
	in := Config{WindowWidth: 900, WindowHeight: 700, WindowX: 50, WindowY: 60, WindowPosSet: true}
	got := MergeWindowGeometry(in, 1920, 1080, 0, 0, true)
	if !got.WindowMaximised {
		t.Fatal("expected maximised")
	}
	if got.WindowWidth != 900 || got.WindowHeight != 700 || got.WindowX != 50 || got.WindowY != 60 || !got.WindowPosSet {
		t.Fatalf("bounds changed: %+v", got)
	}
}

func TestMergeWindowGeometryNormalUpdates(t *testing.T) {
	in := Config{WindowWidth: 900, WindowHeight: 700, WindowX: 50, WindowY: 60, WindowPosSet: true, WindowMaximised: true}
	got := MergeWindowGeometry(in, 1000, 800, 120, 130, false)
	if got.WindowMaximised {
		t.Fatal("expected not maximised")
	}
	if got.WindowWidth != 1000 || got.WindowHeight != 800 || got.WindowX != 120 || got.WindowY != 130 || !got.WindowPosSet {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestMergeWindowGeometryIncompleteThenFill(t *testing.T) {
	// Only size known, position unset: maximised fills missing position, keeps size.
	in := Config{WindowWidth: 900, WindowHeight: 700}
	got := MergeWindowGeometry(in, 1920, 1080, 10, 20, true)
	if got.WindowWidth != 900 || got.WindowHeight != 700 {
		t.Fatalf("size overwritten: %+v", got)
	}
	if !got.WindowPosSet || got.WindowX != 10 || got.WindowY != 20 || !got.WindowMaximised {
		t.Fatalf("position not filled: %+v", got)
	}
}

func TestMergeWindowGeometryMaximisedWithoutPriorBoundsUsesDefaults(t *testing.T) {
	// First maximise with no saved geometry must not persist fullscreen size as
	// the "normal" restore bounds (e.g. 1920×1080).
	got := MergeWindowGeometry(Config{}, 1920, 1080, 0, 0, true)
	if !got.WindowMaximised {
		t.Fatal("expected maximised")
	}
	if got.WindowWidth != DefaultWindowWidth || got.WindowHeight != DefaultWindowHeight {
		t.Fatalf("want defaults %d×%d, got %d×%d", DefaultWindowWidth, DefaultWindowHeight, got.WindowWidth, got.WindowHeight)
	}
	if !got.WindowPosSet || got.WindowX != 0 || got.WindowY != 0 {
		t.Fatalf("position not filled: %+v", got)
	}
}
