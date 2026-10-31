package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ApplyArgs are CLI flags for the --apply-update helper process.
type ApplyArgs struct {
	PID     int
	Src     string
	Dest    string
	Kind    InstallKind
	Version string
}

// ParseApplyArgs extracts --apply-update flags from argv.
// Returns ok=false when the helper mode was not requested.
func ParseApplyArgs(argv []string) (args ApplyArgs, ok bool, err error) {
	args.Kind = InstallBinary
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--apply-update":
			ok = true
		case "--pid":
			if i+1 >= len(argv) {
				return args, true, fmt.Errorf("--pid requires a value")
			}
			i++
			pid, e := strconv.Atoi(argv[i])
			if e != nil || pid <= 0 {
				return args, true, fmt.Errorf("invalid --pid")
			}
			args.PID = pid
		case "--src":
			if i+1 >= len(argv) {
				return args, true, fmt.Errorf("--src requires a value")
			}
			i++
			args.Src = argv[i]
		case "--dest":
			if i+1 >= len(argv) {
				return args, true, fmt.Errorf("--dest requires a value")
			}
			i++
			args.Dest = argv[i]
		case "--kind":
			if i+1 >= len(argv) {
				return args, true, fmt.Errorf("--kind requires a value")
			}
			i++
			k, e := parseKind(argv[i])
			if e != nil {
				return args, true, e
			}
			args.Kind = k
		case "--version":
			if i+1 >= len(argv) {
				return args, true, fmt.Errorf("--version requires a value")
			}
			i++
			args.Version = argv[i]
		}
	}
	if !ok {
		return ApplyArgs{}, false, nil
	}
	if args.PID <= 0 || args.Src == "" || args.Dest == "" {
		return args, true, fmt.Errorf("--apply-update requires --pid, --src and --dest")
	}
	return args, true, nil
}

func parseKind(s string) (InstallKind, error) {
	switch strings.ToLower(s) {
	case "binary", "":
		return InstallBinary, nil
	case "appimage":
		return InstallAppImage, nil
	case "windows", "exe":
		return InstallWindowsEXE, nil
	case "mac", "darwin", "app":
		return InstallMacApp, nil
	default:
		return 0, fmt.Errorf("unknown --kind %q", s)
	}
}

// KindFlag returns the CLI --kind value for a target.
func KindFlag(k InstallKind) string {
	switch k {
	case InstallAppImage:
		return "appimage"
	case InstallWindowsEXE:
		return "windows"
	case InstallMacApp:
		return "mac"
	default:
		return "binary"
	}
}

// WaitForPIDExit polls until pid is gone or timeout elapses.
func WaitForPIDExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if !processAlive(pid) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for pid %d to exit", pid)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ReplaceFile atomically replaces dest with src (rename when possible).
// On failure the original dest is preserved when a backup was created.
func ReplaceFile(src, dest string) error {
	if src == "" || dest == "" {
		return fmt.Errorf("src and dest are required")
	}
	if _, err := os.Stat(src); err != nil {
		return err
	}
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	backup := dest + ".old"
	_ = os.Remove(backup)

	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, backup); err != nil {
			// Fallback: try direct overwrite (e.g. some Unix cases).
			if err2 := copyOverwrite(src, dest); err2 != nil {
				return fmt.Errorf("replace %s: rename failed (%v) and copy failed (%w)", dest, err, err2)
			}
			_ = os.Chmod(dest, 0o755)
			_ = os.Remove(src)
			return nil
		}
	}

	if err := os.Rename(src, dest); err != nil {
		// Cross-device: copy then remove.
		if err2 := copyOverwrite(src, dest); err2 != nil {
			// Restore backup if we moved it aside.
			if _, e := os.Stat(backup); e == nil {
				_ = os.Rename(backup, dest)
			}
			return fmt.Errorf("replace %s: %w", dest, err2)
		}
		_ = os.Remove(src)
	}
	_ = os.Chmod(dest, 0o755)
	_ = os.Remove(backup)
	return nil
}

func copyOverwrite(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dest + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// CanWriteBeside reports whether a new file can be created next to path
// (directory is writable). Used to decide if elevation is needed on Windows.
func CanWriteBeside(path string) bool {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".gotify-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// CleanupOldBackup removes path+".old" if present (best effort).
func CleanupOldBackup(exePath string) {
	if exePath == "" {
		return
	}
	_ = os.Remove(exePath + ".old")
}
