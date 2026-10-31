package tray

// Callbacks wired by the application.
type Callbacks struct {
	OnOpen      func()
	OnSettings  func()
	OnAutostart func()
	OnQuit      func()
}

// Options configures initial tray menu state.
type Options struct {
	AutostartEnabled bool
}
