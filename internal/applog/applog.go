package applog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu     sync.Mutex
	logger *log.Logger
	closer *os.File
)

// Init opens %ConfigDir%/gotify-desktop/desktop.log (append). Safe to call multiple times.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		return
	}
	logger = log.New(os.Stderr, "gotify-desktop: ", log.LstdFlags|log.Lmsgprefix)
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	logDir := filepath.Join(dir, "gotify-desktop")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(logDir, "desktop.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	closer = f
	logger = log.New(f, "", log.LstdFlags)
}

// Close releases the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if closer != nil {
		_ = closer.Close()
		closer = nil
	}
	logger = log.New(os.Stderr, "gotify-desktop: ", log.LstdFlags|log.Lmsgprefix)
}

// Printf writes a log line.
func Printf(format string, args ...any) {
	mu.Lock()
	l := logger
	mu.Unlock()
	if l == nil {
		Init()
		mu.Lock()
		l = logger
		mu.Unlock()
	}
	if l != nil {
		l.Printf(format, args...)
	}
}

// TokenFingerprint returns a short non-secret identifier for logs.
func TokenFingerprint(token string) string {
	if token == "" {
		return "(empty)"
	}
	if len(token) <= 12 {
		return fmt.Sprintf("%s…(len=%d)", token[:min(4, len(token))], len(token))
	}
	return fmt.Sprintf("%s…%s(len=%d)", token[:6], token[len(token)-4:], len(token))
}
