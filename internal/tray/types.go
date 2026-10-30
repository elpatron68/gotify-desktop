package tray

// Callbacks wired by the application.
type Callbacks struct {
	OnOpen     func()
	OnSettings func()
	OnQuit     func()
}
