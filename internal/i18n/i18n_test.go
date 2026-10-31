package i18n

import (
	"encoding/json"
	"testing"
)

func TestInitEnglishAndGerman(t *testing.T) {
	if got := Init("en"); got != "en" {
		t.Fatalf("Init(en)=%q", got)
	}
	if T("menu.quit") != "Quit" {
		t.Fatalf("en quit=%q", T("menu.quit"))
	}
	if got := Init("de-DE"); got != "de" {
		t.Fatalf("Init(de-DE)=%q want de", got)
	}
	if T("menu.quit") != "Beenden" {
		t.Fatalf("de quit=%q", T("menu.quit"))
	}
	if T("menu.settings") != "Einstellungen" {
		t.Fatalf("de settings=%q", T("menu.settings"))
	}
}

func TestFallbackUnknownLocale(t *testing.T) {
	Init("zz")
	if Locale() != "en" {
		t.Fatalf("locale=%q", Locale())
	}
	if T("app.name") != "Gotify Desktop" {
		t.Fatal(T("app.name"))
	}
}

func TestTf(t *testing.T) {
	Init("en")
	got := Tf("status.notify_error", map[string]string{"Error": "boom"})
	if got != "Notify error: boom" {
		t.Fatalf("got %q", got)
	}
}

func TestCatalogJSON(t *testing.T) {
	Init("de")
	raw, err := CatalogJSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["setup.connect"] != "Verbinden" {
		t.Fatalf("%v", m["setup.connect"])
	}
}

func TestSupported(t *testing.T) {
	s := Supported()
	if len(s) < 4 {
		t.Fatalf("supported=%v", s)
	}
	langs := Languages()
	if len(langs) != len(s) {
		t.Fatalf("languages=%v supported=%v", langs, s)
	}
}

func TestFrenchSpanish(t *testing.T) {
	Init("fr")
	if T("menu.quit") != "Quitter" {
		t.Fatalf("fr quit=%q", T("menu.quit"))
	}
	Init("es")
	if T("menu.quit") != "Salir" {
		t.Fatalf("es quit=%q", T("menu.quit"))
	}
}
