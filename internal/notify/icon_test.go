package notify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestToastIconPathWritesPNG(t *testing.T) {
	dir := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
	}

	n := &Notifier{}
	n.SetIconPNG([]byte{0x89, 'P', 'N', 'G'})
	got := n.toastIconPath()
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cfg, "gotify-desktop", "toast-icon.png")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "\x89PNG" {
		t.Fatalf("wrote %q", data)
	}

	// Second call reuses the file.
	if again := n.toastIconPath(); again != want {
		t.Fatalf("second path = %q", again)
	}
}

func TestToastIconPathEmptyWithoutPNG(t *testing.T) {
	if got := (&Notifier{}).toastIconPath(); got != "" {
		t.Fatalf("path = %q, want empty", got)
	}
}

func TestWindowsFileURI(t *testing.T) {
	got := windowsFileURI(`C:\Users\me\toast icon.png`)
	want := `file:///C:/Users/me/toast%20icon.png`
	if got != want {
		t.Fatalf("uri = %q, want %q", got, want)
	}
	if windowsFileURI("") != "" {
		t.Fatal("empty path should stay empty")
	}
}
