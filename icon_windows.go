//go:build windows

package main

import _ "embed"

// PNG works with the Wails v3 system tray on Windows; the previous .ico is kept
// for Windows branding assets under build/windows/.
//
//go:embed build/trayicon.png
var trayIcon []byte
