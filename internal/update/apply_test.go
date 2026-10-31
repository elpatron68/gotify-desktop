package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseApplyArgs(t *testing.T) {
	args, ok, err := ParseApplyArgs([]string{"gotify-desktop"})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	args, ok, err = ParseApplyArgs([]string{
		"gotify-desktop", "--apply-update",
		"--pid", "1234",
		"--src", "/tmp/new",
		"--dest", "/tmp/old",
		"--kind", "appimage",
		"--version", "1.2.3",
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if args.PID != 1234 || args.Src != "/tmp/new" || args.Dest != "/tmp/old" {
		t.Fatalf("%+v", args)
	}
	if args.Kind != InstallAppImage || args.Version != "1.2.3" {
		t.Fatalf("%+v", args)
	}
}

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "gotify-desktop")
	src := filepath.Join(dir, "gotify-desktop.new")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new-binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceFile(src, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-binary" {
		t.Fatalf("content=%q", data)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("not executable: %v", info.Mode())
	}
	if _, err := os.Stat(dest + ".old"); !os.IsNotExist(err) {
		t.Fatalf("backup should be removed: %v", err)
	}
}

func TestCanWriteBeside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app")
	if !CanWriteBeside(path) {
		t.Fatal("expected writable temp dir")
	}
}
