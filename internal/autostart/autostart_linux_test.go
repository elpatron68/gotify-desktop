//go:build linux || freebsd || openbsd || netbsd

package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxDesktopEnableDisable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	exe := filepath.Join(tmp, "gotify-desktop")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if isEnabled(exe) {
		t.Fatal("expected disabled initially")
	}
	if err := enable(exe); err != nil {
		t.Fatal(err)
	}
	if !isEnabled(exe) {
		t.Fatal("expected enabled after enable")
	}
	path, err := desktopPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Exec="+exe) {
		t.Fatalf("desktop file missing Exec: %s", data)
	}
	if err := disable(); err != nil {
		t.Fatal(err)
	}
	if isEnabled(exe) {
		t.Fatal("expected disabled after disable")
	}
}

func TestEscapeDesktopExec(t *testing.T) {
	if got := escapeDesktopExec(`/usr/bin/app`); got != `/usr/bin/app` {
		t.Fatalf("got %q", got)
	}
	got := escapeDesktopExec(`/path/with space/app`)
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Fatalf("expected quotes: %q", got)
	}
}
