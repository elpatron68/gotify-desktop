//go:build darwin

package main

import _ "embed"

// Tray is stubbed on macOS (systray conflicts with Wails), but keep an icon
// available for future use / dock branding consistency.
//
//go:embed build/trayicon.png
var trayIcon []byte
