//go:build !windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// RunApplyHelper launches this executable with --apply-update flags and returns
// without waiting. The current process should exit immediately afterwards.
func RunApplyHelper(selfExe string, target Target, packagePath, version string) error {
	args := []string{
		"--apply-update",
		"--pid", fmt.Sprintf("%d", os.Getpid()),
		"--src", packagePath,
		"--dest", target.ReplacePath,
		"--kind", KindFlag(target.Kind),
	}
	if version != "" {
		args = append(args, "--version", version)
	}
	cmd := exec.Command(selfExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// ApplyUpdate performs the package swap after the parent process has exited.
func ApplyUpdate(args ApplyArgs) error {
	if err := WaitForPIDExit(args.PID, 60*time.Second); err != nil {
		return err
	}
	switch args.Kind {
	case InstallMacApp:
		return applyMacApp(args.Src, args.Dest)
	default:
		if err := ReplaceFile(args.Src, args.Dest); err != nil {
			return err
		}
		return startPath(args.Dest)
	}
}

func applyMacApp(zipPath, appPath string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("mac app update only supported on darwin")
	}
	tmp, err := os.MkdirTemp("", "gotify-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	cmd := exec.Command("ditto", "-x", "-k", zipPath, tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("unzip update: %w (%s)", err, string(out))
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	var srcApp string
	for _, e := range entries {
		if e.IsDir() && filepath.Ext(e.Name()) == ".app" {
			srcApp = filepath.Join(tmp, e.Name())
			break
		}
	}
	if srcApp == "" {
		return fmt.Errorf("zip contains no .app bundle")
	}
	// Replace existing bundle with ditto (preserves attributes).
	if err := os.RemoveAll(appPath); err != nil {
		return err
	}
	cmd = exec.Command("ditto", srcApp, appPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install .app: %w (%s)", err, string(out))
	}
	_ = os.Remove(zipPath)
	cmd = exec.Command("open", appPath)
	return cmd.Start()
}

func startPath(path string) error {
	cmd := exec.Command(path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
