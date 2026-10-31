package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"

	"embed"
)

//go:embed locales/*.json
var localeFS embed.FS

// Language is a selectable UI locale.
type Language struct {
	Code string // e.g. "de"
	Name string // native endonym, e.g. "Deutsch"
}

// preferredOrder is the menu order for known languages.
var preferredOrder = []string{"en", "de", "fr", "es"}

// endonyms are fixed native names (not translated).
var endonyms = map[string]string{
	"en": "English",
	"de": "Deutsch",
	"fr": "Français",
	"es": "Español",
}

func load(lang string) (Catalog, error) {
	name := "locales/" + lang + ".json"
	data, err := localeFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, err
	}
	return cat, nil
}

// Supported returns language codes that have a catalog file.
func Supported() []string {
	entries, err := fs.ReadDir(localeFS, "locales")
	if err != nil {
		return []string{"en"}
	}
	have := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if len(name) > 5 && name[len(name)-5:] == ".json" {
			have[name[:len(name)-5]] = true
		}
	}
	var out []string
	for _, code := range preferredOrder {
		if have[code] {
			out = append(out, code)
			delete(have, code)
		}
	}
	var rest []string
	for code := range have {
		rest = append(rest, code)
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// Languages returns supported locales with native display names for menus.
func Languages() []Language {
	codes := Supported()
	out := make([]Language, 0, len(codes))
	for _, code := range codes {
		name := endonyms[code]
		if name == "" {
			name = code
		}
		out = append(out, Language{Code: code, Name: name})
	}
	return out
}
