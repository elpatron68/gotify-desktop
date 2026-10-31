//go:build darwin

package main

import _ "embed"

//go:embed build/trayicon.png
var trayIcon []byte
