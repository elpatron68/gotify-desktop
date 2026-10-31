package main

import (
	"embed"
	"log"
	"os"

	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/i18n"
	"github.com/elpatron68/gotify-desktop/internal/update"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	if args, ok, err := update.ParseApplyArgs(os.Args); ok {
		if err != nil {
			log.Fatal(err)
		}
		if err := update.ApplyUpdate(args); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Allow the embedded UI (wails.localhost) to open ws://127.0.0.1 for the
	// local Gotify stream bridge. Without this, WebView2 Local Network Access
	// blocks the socket and the iframe never receives live messages.
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS",
		"--disable-features=BlockInsecurePrivateNetworkRequests,LocalNetworkAccessChecks")

	// Prefer the dedicated tray icon; fall back to the app icon if missing.
	icon := trayIcon
	if len(icon) == 0 {
		icon = appIcon
	}
	app := NewApp(icon)

	if exe, err := os.Executable(); err == nil {
		update.CleanupOldBackup(exe)
	}

	err := wails.Run(&options.App{
		Title:             i18n.T("app.name"),
		Width:             config.DefaultWindowWidth,
		Height:            config.DefaultWindowHeight,
		MinWidth:          config.MinWindowWidth,
		MinHeight:         config.MinWindowHeight,
		HideWindowOnClose: true,
		// Start hidden; DomReady shows the window only when setup is required.
		StartHidden: true,
		Menu:        app.buildMenu(),
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: app.ProxyMiddleware(),
		},
		BackgroundColour: &options.RGBA{R: 18, G: 24, B: 32, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
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
