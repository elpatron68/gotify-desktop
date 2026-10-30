//go:build !windows

package notify

import "github.com/gen2brain/beeep"

func showPlatformToast(title, body string) error {
	return beeep.Notify(title, body, "")
}
