//go:build windows

package main

import _ "embed"

//go:embed build/windows/trayicon.ico
var trayIcon []byte
