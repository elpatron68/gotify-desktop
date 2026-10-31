package main

import (
	"embed"
	"log"
	"os"

	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/i18n"
	"github.com/elpatron68/gotify-desktop/internal/update"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
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
	desktop := NewApp(icon)
	notifier := notifications.New()
	desktop.setNotificationService(notifier)

	app := application.New(application.Options{
		Name:        i18n.T("app.name"),
		Description: "Cross-platform Gotify desktop notification client",
		Icon:        appIcon,
		Services: []application.Service{
			// Notifier starts first so App.ServiceStartup can call notify.Init.
			application.NewService(notifier),
			application.NewService(desktop),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: desktop.proxyMiddleware(),
		},
	})
	desktop.bindApp(app)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            i18n.T("app.name"),
		Width:            config.DefaultWindowWidth,
		Height:           config.DefaultWindowHeight,
		MinWidth:         config.MinWindowWidth,
		MinHeight:        config.MinWindowHeight,
		Hidden:           true,
		BackgroundColour: application.NewRGB(18, 24, 32),
		UseApplicationMenu: true,
	})
	desktop.bindWindow(window)

	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		desktop.onWindowClosing(e)
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
		desktop.onRuntimeReady()
	})

	if exe, err := os.Executable(); err == nil {
		update.CleanupOldBackup(exe)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
