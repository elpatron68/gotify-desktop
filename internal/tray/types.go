package tray

// Callbacks wired by the application.
type Callbacks struct {
	OnOpen      func()
	OnSettings  func()
	OnHistory   func()
	OnSend      func()
	OnAutostart func()
	OnLanguage  func(code string)
	OnAboutLicense func()
	OnAboutVersion func()
	OnAboutRepo    func()
	OnQuit         func()
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
	About            string
	AboutTooltip     string
	AboutAuthor         string
	AboutLicense        string
	AboutLicenseTooltip string
	AboutVersion        string
	AboutVersionTooltip string
	AboutRepo           string
	AboutRepoTooltip    string
	Quit                string
	QuitTooltip         string
}
