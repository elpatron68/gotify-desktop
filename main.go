package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	// Prefer the dedicated tray icon; fall back to the app icon if missing.
	icon := trayIcon
	if len(icon) == 0 {
		icon = appIcon
	}
	app := NewApp(icon)

	err := wails.Run(&options.App{
		Title:             "Gotify Desktop",
		Width:             1100,
		Height:            800,
		MinWidth:          640,
		MinHeight:         480,
		HideWindowOnClose: true,
		Menu:              app.buildMenu(),
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: app.ProxyMiddleware(),
		},
		BackgroundColour: &options.RGBA{R: 18, G: 24, B: 32, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
