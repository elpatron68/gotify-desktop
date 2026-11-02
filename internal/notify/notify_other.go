//go:build !windows && !linux

package notify

import "github.com/gen2brain/beeep"

func showPlatformToast(title, body, icon string) error {
	beeep.AppName = appDisplayName
	if icon == "" {
		return beeep.Notify(title, body, "")
	}
	return beeep.Notify(title, body, icon)
}
