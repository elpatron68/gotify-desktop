package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectTargetLinuxBinary(t *testing.T) {
	t.Setenv("APPIMAGE", "")
	tgt, err := DetectTarget("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Kind != InstallBinary {
		t.Fatalf("kind=%v", tgt.Kind)
	}
	if tgt.AssetName != "gotify-desktop-linux-amd64" {
		t.Fatalf("asset=%q", tgt.AssetName)
	}
}

func TestDetectTargetLinuxAppImage(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "Gotify.AppImage")
	if err := os.WriteFile(img, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPIMAGE", img)
	tgt, err := DetectTarget("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Kind != InstallAppImage {
		t.Fatalf("kind=%v", tgt.Kind)
	}
	if tgt.AssetName != "gotify-desktop-linux-amd64.AppImage" {
		t.Fatalf("asset=%q", tgt.AssetName)
	}
	if tgt.ReplacePath != img {
		t.Fatalf("replace=%q", tgt.ReplacePath)
	}
}

func TestDetectTargetWindows(t *testing.T) {
	tgt, err := DetectTarget("windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Kind != InstallWindowsEXE {
		t.Fatalf("kind=%v", tgt.Kind)
	}
	if tgt.AssetName != "gotify-desktop-windows-amd64.exe" {
		t.Fatalf("asset=%q", tgt.AssetName)
	}
}

func TestDetectTargetUnsupportedArch(t *testing.T) {
	_, err := DetectTarget("linux", "386")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMacAppBundle(t *testing.T) {
	exe := "/Applications/Gotify Desktop.app/Contents/MacOS/gotify-desktop"
	got := macAppBundle(exe)
	want := "/Applications/Gotify Desktop.app"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if macAppBundle("/usr/local/bin/gotify-desktop") != "" {
		t.Fatal("expected empty for non-bundle")
	}
}

func TestReleaseFindAsset(t *testing.T) {
	rel := Release{
		TagName: "v0.2.0",
		Assets: []Asset{
			{Name: "SHA256SUMS", BrowserDownloadURL: "https://example.com/SHA256SUMS"},
			{Name: "gotify-desktop-linux-amd64.AppImage", BrowserDownloadURL: "https://example.com/a"},
			{Name: "gotify-desktop-windows-amd64.exe", BrowserDownloadURL: "https://example.com/w"},
		},
	}
	a, err := rel.FindAsset("gotify-desktop-windows-amd64.exe")
	if err != nil || a.Name != "gotify-desktop-windows-amd64.exe" {
		t.Fatalf("find windows: %v %#v", err, a)
	}
	if _, err := rel.FindAsset("gotify-desktop-darwin-arm64.zip"); err == nil {
		t.Fatal("expected missing asset error")
	}
}
