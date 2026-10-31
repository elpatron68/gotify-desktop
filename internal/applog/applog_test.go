package applog

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resetForTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	if closer != nil {
		_ = closer.Close()
		closer = nil
	}
	logger = nil
	mu.Unlock()
}

func TestInitReopensAfterClose(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // Linux/WSL UserConfigDir
	// Also cover platforms that use HOME for config.
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("AppData", filepath.Join(tmp, "AppData", "Roaming"))

	resetForTest(t)
	t.Cleanup(func() { resetForTest(t) })

	Init()
	mu.Lock()
	first := closer
	mu.Unlock()
	if first == nil {
		// UserConfigDir may not honor our env on this platform; still verify Close/Init contract.
		t.Skip("could not open desktop.log under test config dir")
	}

	Close()
	mu.Lock()
	if closer != nil {
		mu.Unlock()
		t.Fatal("closer still set after Close")
	}
	if logger == nil {
		mu.Unlock()
		t.Fatal("logger should fall back to stderr after Close")
	}
	mu.Unlock()

	Init()
	mu.Lock()
	defer mu.Unlock()
	if closer == nil {
		t.Fatal("Init after Close should reopen the log file")
	}
}

func TestPrintfConcurrentWithInit(t *testing.T) {
	resetForTest(t)
	t.Cleanup(func() { resetForTest(t) })

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				Init()
			}
			Printf("concurrent %d", n)
		}(i)
	}
	wg.Wait()
}

func TestTokenFingerprint(t *testing.T) {
	if TokenFingerprint("") != "(empty)" {
		t.Fatal(TokenFingerprint(""))
	}
	got := TokenFingerprint("gtfyctokenvalueHERE123456789")
	if got == "" || got == "(empty)" {
		t.Fatal(got)
	}
	_ = os.DevNull
}
