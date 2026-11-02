//go:build linux

package notify

import (
	"fmt"

	"github.com/gen2brain/beeep"
	"github.com/godbus/dbus/v5"
)

func showPlatformToast(title, body, icon string) error {
	if err := dbusToast(title, body, icon); err != nil {
		beeep.AppName = appDisplayName
		fallback := beeep.Notify(title, body, icon)
		if fallback != nil {
			return fmt.Errorf("dbus: %w; fallback: %v", err, fallback)
		}
	}
	return nil
}

func dbusToast(title, body, icon string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := obj.Call(
		"org.freedesktop.Notifications.Notify",
		0,
		appDisplayName,
		uint32(0),
		icon,
		title,
		body,
		[]string{},
		map[string]dbus.Variant{},
		int32(-1),
	)
	if call.Err != nil {
		return call.Err
	}
	var id uint32
	return call.Store(&id)
}
