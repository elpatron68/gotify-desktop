package i18n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/template"
)

// Catalog is a flat key → message map.
type Catalog map[string]string

var (
	mu       sync.RWMutex
	locale   = "en"
	messages = Catalog{}
	fallback Catalog
)

// Init loads the catalog for lang (e.g. "de", "de-DE", "en"). Empty uses the OS locale.
// Unknown languages fall back to English. Safe to call multiple times.
func Init(lang string) string {
	mu.Lock()
	defer mu.Unlock()

	if fallback == nil {
		fallback = mustLoad("en")
	}
	if lang == "" {
		lang = detectOSLocale()
	}
	locale = normalize(lang)
	cat, err := load(locale)
	if err != nil {
		// Try language primary subtag (de-DE → de).
		if i := strings.IndexByte(locale, '-'); i > 0 {
			locale = locale[:i]
			cat, err = load(locale)
		}
	}
	if err != nil {
		locale = "en"
		messages = clone(fallback)
		return locale
	}
	messages = merge(fallback, cat)
	return locale
}

// Locale returns the active language code (e.g. "en", "de").
func Locale() string {
	mu.RLock()
	defer mu.RUnlock()
	return locale
}

// T returns the translated string for key, or the key itself if missing.
func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if s, ok := messages[key]; ok {
		return s
	}
	if s, ok := fallback[key]; ok {
		return s
	}
	return key
}

// Tf renders a translated template with data (e.g. {{.Title}}).
func Tf(key string, data any) string {
	raw := T(key)
	tmpl, err := template.New(key).Parse(raw)
	if err != nil {
		return raw
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return raw
	}
	return buf.String()
}

// CatalogJSON returns the active catalog as JSON for the frontend shell.
func CatalogJSON() ([]byte, error) {
	mu.RLock()
	defer mu.RUnlock()
	return json.Marshal(messages)
}

func normalize(lang string) string {
	lang = strings.TrimSpace(lang)
	lang = strings.ReplaceAll(lang, "_", "-")
	if lang == "" {
		return "en"
	}
	parts := strings.SplitN(lang, ".", 2) // en_US.UTF-8 → en_US already normalized
	lang = parts[0]
	return strings.ToLower(lang)
}

func detectOSLocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" && v != "C" && v != "POSIX" {
			return v
		}
	}
	return detectOSLocalePlatform()
}

func mustLoad(lang string) Catalog {
	c, err := load(lang)
	if err != nil {
		panic(fmt.Sprintf("i18n: load %s: %v", lang, err))
	}
	return c
}

func merge(base, over Catalog) Catalog {
	out := clone(base)
	for k, v := range over {
		out[k] = v
	}
	return out
}

func clone(c Catalog) Catalog {
	out := make(Catalog, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}
