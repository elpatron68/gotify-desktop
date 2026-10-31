//go:build windows

package i18n

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultLocaleName = modKernel32.NewProc("GetUserDefaultLocaleName")
)

func detectOSLocalePlatform() string {
	const localeNameMax = 85
	buf := make([]uint16, localeNameMax)
	r, _, _ := procGetUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(localeNameMax),
	)
	if r == 0 {
		return "en"
	}
	return windows.UTF16ToString(buf)
}
