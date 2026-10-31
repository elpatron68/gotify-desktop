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
	stderr = log.New(os.Stderr, "gotify-desktop: ", log.LstdFlags|log.Lmsgprefix)
)

// Init opens %ConfigDir%/gotify-desktop/desktop.log (append). Safe to call multiple times.
// After Close(), a subsequent Init() reopens the file.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	initLocked()
}

// initLocked opens the log file if it is not already open. Caller must hold mu.
func initLocked() {
	if closer != nil {
		return
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		if logger == nil {
			logger = stderr
		}
		return
	}
	logDir := filepath.Join(dir, "gotify-desktop")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		if logger == nil {
			logger = stderr
		}
		return
	}
	f, err := os.OpenFile(filepath.Join(logDir, "desktop.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		if logger == nil {
			logger = stderr
		}
		return
	}
	closer = f
	logger = log.New(f, "", log.LstdFlags)
}

// Close releases the log file. Later Printf calls go to stderr until Init() runs again.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if closer != nil {
		_ = closer.Close()
		closer = nil
	}
	logger = stderr
}

// Printf writes a log line. Lazily initializes on first use.
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if logger == nil {
		initLocked()
	}
	if logger != nil {
		logger.Printf(format, args...)
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
