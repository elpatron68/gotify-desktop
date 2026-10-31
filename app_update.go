package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/elpatron68/gotify-desktop/internal/applog"
	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/i18n"
	"github.com/elpatron68/gotify-desktop/internal/update"
	"github.com/elpatron68/gotify-desktop/internal/version"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const updateCheckInterval = 24 * time.Hour

// CheckForUpdates runs a manual update check (menu / tray).
func (a *App) CheckForUpdates() {
	go a.runUpdateCheck(true)
}

func (a *App) maybeAutoCheckUpdates() {
	go func() {
		// Let DomReady and the window settle before showing a dialog.
		time.Sleep(3 * time.Second)
		a.runUpdateCheck(false)
	}()
}

func (a *App) runUpdateCheck(manual bool) {
	if version.Version == "0.0.0" {
		if manual {
			a.showUpdateInfo(i18n.T("update.dev_build"))
		}
		return
	}

	if !manual {
		a.mu.Lock()
		last := a.cfg.UpdateCheckedUnix
		a.mu.Unlock()
		if last > 0 && time.Since(time.Unix(last, 0)) < updateCheckInterval {
			return
		}
	}

	client := &update.Client{UserAgent: "gotify-desktop/" + version.Version}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rel, newer, err := client.CheckAvailable(ctx, version.Version)
	a.markUpdateChecked()
	if err != nil {
		applog.Printf("update check: %v", err)
		if manual {
			a.showUpdateError(fmt.Sprintf("%s\n%s", i18n.T("update.check_failed"), err.Error()))
		}
		return
	}
	if !newer {
		if manual {
			a.showUpdateInfo(i18n.Tf("update.up_to_date", map[string]string{"Version": version.Version}))
		}
		return
	}

	remote := rel.TagName
	if len(remote) > 0 && (remote[0] == 'v' || remote[0] == 'V') {
		remote = remote[1:]
	}
	msg := i18n.Tf("update.available", map[string]string{
		"Current": version.Version,
		"Latest":  remote,
	})
	choice, err := a.askUpdateQuestion(msg)
	if err != nil {
		applog.Printf("update dialog: %v", err)
		return
	}
	// Wails QuestionDialog returns the pressed button label.
	if !isAffirmativeUpdateChoice(choice) {
		applog.Printf("update deferred by user (choice=%q)", choice)
		return
	}

	target, err := update.CurrentTarget()
	if err != nil {
		applog.Printf("update target: %v", err)
		a.showUpdateError(fmt.Sprintf("%s\n%s\n%s", i18n.T("update.no_asset"), err.Error(), version.ReleasesURL))
		return
	}
	if _, err := rel.FindAsset(target.AssetName); err != nil {
		applog.Printf("update asset: %v", err)
		a.showUpdateError(fmt.Sprintf("%s\n%s", i18n.T("update.no_asset"), version.ReleasesURL))
		return
	}

	if a.notify != nil {
		_ = a.notify.Show("update-download", i18n.T("app.name"), i18n.T("update.downloading"))
	}

	dir, err := os.MkdirTemp("", "gotify-desktop-update-*")
	if err != nil {
		a.showUpdateError(err.Error())
		return
	}

	dlCtx, dlCancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer dlCancel()
	res, err := client.DownloadAndVerify(dlCtx, rel, target, dir)
	if err != nil {
		applog.Printf("update download: %v", err)
		_ = os.RemoveAll(dir)
		a.showUpdateError(fmt.Sprintf("%s\n%s", i18n.T("update.download_failed"), err.Error()))
		return
	}

	if target.Kind == update.InstallBinary || target.Kind == update.InstallAppImage || target.Kind == update.InstallWindowsEXE {
		if !update.CanWriteBeside(target.ReplacePath) && target.Kind != update.InstallWindowsEXE {
			_ = os.RemoveAll(dir)
			a.showUpdateError(i18n.T("update.not_writable"))
			return
		}
	}

	self, err := os.Executable()
	if err != nil {
		_ = os.Remove(res.PackagePath)
		a.showUpdateError(err.Error())
		return
	}
	if err := update.RunApplyHelper(self, target, res.PackagePath, res.Version); err != nil {
		applog.Printf("update apply helper: %v", err)
		_ = os.Remove(res.PackagePath)
		a.showUpdateError(fmt.Sprintf("%s\n%s", i18n.T("update.apply_failed"), err.Error()))
		return
	}
	applog.Printf("update helper started; quitting for version %s", res.Version)
	a.Quit()
}

func isAffirmativeUpdateChoice(choice string) bool {
	switch choice {
	case "Yes", "Ok", "OK":
		return true
	}
	install := i18n.T("update.install")
	return choice == install
}

func (a *App) markUpdateChecked() {
	now := time.Now().Unix()
	a.mu.Lock()
	a.cfg.UpdateCheckedUnix = now
	cfg := a.cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		applog.Printf("save update_checked_unix: %v", err)
	}
}

func (a *App) askUpdateQuestion(message string) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no runtime context")
	}
	return runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         i18n.T("update.title"),
		Message:       message,
		Buttons:       []string{i18n.T("update.install"), i18n.T("update.later")},
		DefaultButton: i18n.T("update.install"),
		CancelButton:  i18n.T("update.later"),
	})
}

func (a *App) showUpdateInfo(message string) {
	if a.ctx == nil {
		return
	}
	_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:    runtime.InfoDialog,
		Title:   i18n.T("update.title"),
		Message: message,
	})
}

func (a *App) showUpdateError(message string) {
	if a.ctx == nil {
		return
	}
	_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:    runtime.ErrorDialog,
		Title:   i18n.T("update.title"),
		Message: message,
	})
}
