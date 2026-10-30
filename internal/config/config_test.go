package config

import (
	"os"
	"path/filepath"
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
	// UserConfigDir on Linux uses XDG_CONFIG_HOME; on other platforms may differ.
	// Force path by overriding via writing through Dir after setting home-like env.
	if dir, err := Dir(); err == nil {
		_ = os.MkdirAll(dir, 0o700)
	}

	cfg := Config{
		ServerURL:   "https://gotify.example.com",
		Username:    "admin",
		ClientToken: "secret",
		MinPriority: 2,
	}
	// Save via direct path if UserConfigDir doesn't respect XDG in this env
	path := filepath.Join(tmp, appDirName, "config.json")
	if d, err := Dir(); err == nil {
		path = filepath.Join(d, "config.json")
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)

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
	if loaded.ServerURL != cfg.ServerURL || loaded.Username != cfg.Username || loaded.ClientToken != cfg.ClientToken || loaded.MinPriority != cfg.MinPriority {
		t.Fatalf("loaded %+v want %+v", loaded, cfg)
	}
}
