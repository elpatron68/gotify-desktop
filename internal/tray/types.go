package tray

// Callbacks wired by the application.
type Callbacks struct {
	OnOpen      func()
	OnSettings  func()
	OnHistory   func()
	OnSend      func()
	OnAutostart func()
	OnLanguage  func(code string)
	OnQuit      func()
}

// Options configures initial tray menu state.
type Options struct {
	AutostartEnabled bool
	Locale           string
	Languages        []Language
	Labels           Labels
}

// Language is a tray language menu entry.
type Language struct {
	Code string
	Name string
}

// Labels are localized tray strings (resolved by the app via i18n).
type Labels struct {
	Tooltip          string
	StatusStarting   string
	StatusTooltip    string
	Open             string
	OpenTooltip      string
	Settings         string
	SettingsTooltip  string
	History          string
	HistoryTooltip   string
	Send             string
	SendTooltip      string
	Autostart        string
	AutostartTooltip string
	Language         string
	LanguageTooltip  string
	Quit             string
	QuitTooltip      string
}
