# Notification History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a tray/menu-reachable in-app history of the last 100 notifications (persisted), with toasts/all mode toggle and Markdown clipboard copy.

**Architecture:** New `internal/history` ring-buffer store (`history.json` beside config). `App.handleMessage` appends per capture rules; Wails bindings + `history` frontend view; tray/app-menu entry opens the view.

**Tech Stack:** Go 1.x, Wails v2, existing `internal/config` / `internal/i18n` / tray, vanilla frontend (Vite).

## Global Constraints

- Max **100** entries (newest first); fixed cap, not user-configurable
- Default `history_mode` = `toasts`; allowed values `toasts` | `all`
- Markdown copy: `**{title}**\n{body}\n\n_{timestamp}_` (omit title line if empty); local timestamp
- Capture: dedup (`last_message_id`) first; toast uses `min_priority`; `all` still stores below-priority with `notified=false`
- No Gotify REST history; do not parse `notify.log`
- i18n: add keys to `en`/`de`/`fr`/`es` (EN fallback via merge)
- Save/IO errors: log only; never break the stream

## File map

| Path | Role |
| --- | --- |
| `internal/history/history.go` | Entry, Store, Append, List, Clear, Markdown, Load/Save |
| `internal/history/history_test.go` | Unit tests |
| `internal/config/config.go` | `HistoryMode` field + normalizer |
| `app.go` | Wire store, capture, bindings, menu, events |
| `internal/tray/types.go` | `OnHistory` callback + label fields |
| `internal/tray/tray_systray.go` | Menu item |
| `internal/tray/tray_darwin.go` | No-op stubs if needed |
| `frontend/index.html` | History section markup |
| `frontend/src/main.js` | View switching, list render, copy, events |
| `frontend/src/style.css` | History layout |
| `frontend/src/i18n.js` | unchanged (catalog-driven) |
| `internal/i18n/locales/*.json` | New strings |
| `README.md` | Short history note |
| `frontend/wailsjs/go/main/App.js` + `.d.ts` | Regenerated or hand-updated bindings |

---

### Task 1: `internal/history` store + Markdown helper

**Files:**
- Create: `internal/history/history.go`
- Create: `internal/history/history_test.go`

**Interfaces:**
- Consumes: `config.Dir()` for default path (or inject path in tests)
- Produces:
  - `const MaxEntries = 100`
  - `type Entry struct { ID, Title, Body string; Priority int; AppID uint; ReceivedAt time.Time; Notified bool }` with JSON tags
  - `type Store struct` with `Load(path string) error`, `Save() error`, `Append(Entry) error`, `Clear() error`, `List(toastsOnly bool) []Entry`, `Path() string`
  - `func FormatMarkdown(e Entry, loc *time.Location) string`
  - `func DefaultPath() (string, error)` → `filepath.Join(config.Dir(), "history.json")`

- [ ] **Step 1: Write failing tests**

```go
package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendTrimsToMax(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.json"))
	for i := 0; i < MaxEntries+5; i++ {
		e := Entry{ID: fmt.Sprintf("%d", i), Title: "t", Body: "b", ReceivedAt: time.Now(), Notified: true}
		if err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	all := s.List(false)
	if len(all) != MaxEntries {
		t.Fatalf("len=%d", len(all))
	}
}

func TestListToastsOnly(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "h.json"))
	_ = s.Append(Entry{ID: "1", Title: "a", Notified: true, ReceivedAt: time.Now()})
	_ = s.Append(Entry{ID: "2", Title: "b", Notified: false, ReceivedAt: time.Now()})
	// Newest first → [2, 1]; toastsOnly keeps notified=true → [1]
	got := s.List(true)
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("%+v", got)
	}
	if len(s.List(false)) != 2 {
		t.Fatalf("%+v", s.List(false))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s := New(path)
	ts := time.Date(2026, 10, 31, 10, 48, 0, 0, time.Local)
	_ = s.Append(Entry{ID: "9", Title: "Hi", Body: "Body", Priority: 5, AppID: 1, ReceivedAt: ts, Notified: true})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got := s2.List(false)
	if len(got) != 1 || got[0].Title != "Hi" || !got[0].Notified {
		t.Fatalf("%+v", got)
	}
}

func TestLoadMissingOrCorrupt(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "missing.json"))
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if len(s.List(false)) != 0 {
		t.Fatal("expected empty")
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if len(s2.List(false)) != 0 {
		t.Fatal("corrupt should yield empty")
	}
}

func TestFormatMarkdown(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	ts := time.Date(2026, 10, 31, 10, 48, 0, 0, loc)
	got := FormatMarkdown(Entry{Title: "Alert", Body: "Disk full", ReceivedAt: ts}, loc)
	want := "**Alert**\nDisk full\n\n_31.10.2026 10:48_"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got2 := FormatMarkdown(Entry{Body: "only body", ReceivedAt: ts}, loc)
	want2 := "only body\n\n_31.10.2026 10:48_"
	if got2 != want2 {
		t.Fatalf("got %q want %q", got2, want2)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

Run: `go test ./internal/history/ -count=1`
Expected: FAIL (package or symbols missing)

- [ ] **Step 3: Implement store**

```go
package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elpatron68/gotify-desktop/internal/config"
)

const MaxEntries = 100

type Entry struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Priority   int       `json:"priority"`
	AppID      uint      `json:"app_id"`
	ReceivedAt time.Time `json:"received_at"`
	Notified   bool      `json:"notified"`
}

type fileShape struct {
	Entries []Entry `json:"entries"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	entries []Entry // newest first
}

func New(path string) *Store {
	return &Store{path: path}
}

func DefaultPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.json"), nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.entries = nil
		return nil
	}
	if err != nil {
		return err
	}
	var f fileShape
	if err := json.Unmarshal(data, &f); err != nil {
		s.entries = nil
		return nil // corrupt → empty
	}
	s.entries = f.Entries
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[:MaxEntries]
	}
	return nil
}

func (s *Store) Save() error {
	s.mu.RLock()
	f := fileShape{Entries: append([]Entry(nil), s.entries...)}
	path := s.path
	s.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) Append(e Entry) error {
	s.mu.Lock()
	s.entries = append([]Entry{e}, s.entries...)
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[:MaxEntries]
	}
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) Clear() error {
	s.mu.Lock()
	s.entries = nil
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) List(toastsOnly bool) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if toastsOnly && !e.Notified {
			continue
		}
		out = append(out, e)
	}
	return out
}

func FormatMarkdown(e Entry, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	ts := e.ReceivedAt.In(loc).Format("02.01.2006 15:04")
	body := e.Body
	title := strings.TrimSpace(e.Title)
	if title == "" {
		return body + "\n\n_" + ts + "_"
	}
	return "**" + title + "**\n" + body + "\n\n_" + ts + "_"
}
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./internal/history/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/history/
git commit -m "$(cat <<'EOF'
Add persistent notification history store.

Ring buffer of 100 entries with Markdown formatting for clipboard copy.
EOF
)"
```

---

### Task 2: Config `history_mode`

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go` (add cases)
- Modify: `README.md` (fields line)

**Interfaces:**
- Consumes: existing `Config` / `Default` / `Load`
- Produces:
  - `HistoryMode string \`json:"history_mode,omitempty"\``
  - `const HistoryModeToasts = "toasts"`, `HistoryModeAll = "all"`
  - `func NormalizeHistoryMode(m string) string` → unknown/empty → `toasts`

- [ ] **Step 1: Failing test**

```go
func TestNormalizeHistoryMode(t *testing.T) {
	if NormalizeHistoryMode("") != HistoryModeToasts {
		t.Fatal()
	}
	if NormalizeHistoryMode("ALL") != HistoryModeAll {
		t.Fatal()
	}
	if NormalizeHistoryMode("nope") != HistoryModeToasts {
		t.Fatal()
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/config/ -run TestNormalizeHistoryMode -count=1`

- [ ] **Step 3: Implement**

In `config.go`:

```go
const (
	HistoryModeToasts = "toasts"
	HistoryModeAll    = "all"
)

// on Config:
HistoryMode string `json:"history_mode,omitempty"`

func NormalizeHistoryMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case HistoryModeAll:
		return HistoryModeAll
	default:
		return HistoryModeToasts
	}
}
```

Call `cfg.HistoryMode = NormalizeHistoryMode(cfg.HistoryMode)` at end of successful `Load` (and optionally in `Save` before write).

README: mention `history_mode` and `history.json`.

- [ ] **Step 4: Tests PASS**

Run: `go test ./internal/config/ -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/config/ README.md
git commit -m "$(cat <<'EOF'
Add history_mode config for toast vs all capture.

EOF
)"
```

---

### Task 3: Wire capture + Wails API in `app.go`

**Files:**
- Modify: `app.go`
- Modify: `frontend/wailsjs/go/main/App.js`
- Modify: `frontend/wailsjs/go/main/App.d.ts`

**Interfaces:**
- Consumes: `history.Store`, `config.NormalizeHistoryMode`, `history.FormatMarkdown`
- Produces (exported on `App`):
  - `GetNotificationHistory() []history.Entry` — filtered by current mode
  - `GetHistoryMode() string`
  - `SetHistoryMode(mode string)`
  - `ClearNotificationHistory()`
  - `OpenNotificationHistory()` — show window, set `view=history`, `emitStatus` + emit `show-history`
  - `LeaveNotificationHistory()` — restore `gotify` if configured else `setup`
  - Event `history-updated` after append/clear

- [ ] **Step 1: Add `history *history.Store` on `App`; load in `startup` after config**

```go
path, err := history.DefaultPath()
if err == nil {
    a.history = history.New(path)
    if err := a.history.Load(); err != nil {
        applog.Printf("history load: %v", err)
    }
}
a.cfg.HistoryMode = config.NormalizeHistoryMode(a.cfg.HistoryMode)
```

- [ ] **Step 2: Refactor `handleMessage` capture order**

Pseudo-order (keep logging/toast side effects):

```go
func (a *App) handleMessage(msg stream.Message) {
	a.mu.Lock()
	minP := a.cfg.MinPriority
	lastID := a.cfg.LastMessageID
	mode := config.NormalizeHistoryMode(a.cfg.HistoryMode)
	a.mu.Unlock()

	if msg.ID != 0 && lastID != 0 && msg.ID <= lastID {
		return
	}

	eligible := config.ShouldNotify(minP, msg.Priority)
	id := strconv.FormatUint(uint64(msg.ID), 10)
	if id == "0" {
		id = fmt.Sprintf("msg-%d", msg.AppID)
	}
	entry := history.Entry{
		ID: id, Title: msg.Title, Body: msg.Message,
		Priority: msg.Priority, AppID: msg.AppID,
		ReceivedAt: time.Now(), Notified: eligible,
	}

	if eligible {
		// existing EventsEmit gotify-message + notify.Show + tray status + LastMessageID save
		a.recordHistory(entry)
	} else if mode == config.HistoryModeAll {
		a.recordHistory(entry)
	}
}

func (a *App) recordHistory(e history.Entry) {
	if a.history == nil {
		return
	}
	if err := a.history.Append(e); err != nil {
		applog.Printf("history append: %v", err)
		return
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "history-updated", nil)
	}
}
```

Move existing toast body into the `eligible` branch unchanged (including `LastMessageID` update only when eligible).

- [ ] **Step 3: `TestNotification` also `recordHistory` with synthetic id `test-<unix>`**

- [ ] **Step 4: Bindings**

```go
func (a *App) GetHistoryMode() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return config.NormalizeHistoryMode(a.cfg.HistoryMode)
}

func (a *App) SetHistoryMode(mode string) {
	mode = config.NormalizeHistoryMode(mode)
	a.mu.Lock()
	a.cfg.HistoryMode = mode
	cfg := a.cfg
	a.mu.Unlock()
	_ = config.Save(cfg)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "history-updated", nil)
	}
}

func (a *App) GetNotificationHistory() []history.Entry {
	if a.history == nil {
		return nil
	}
	toastsOnly := a.GetHistoryMode() == config.HistoryModeToasts
	return a.history.List(toastsOnly)
}

func (a *App) ClearNotificationHistory() {
	if a.history == nil {
		return
	}
	if err := a.history.Clear(); err != nil {
		applog.Printf("history clear: %v", err)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "history-updated", nil)
	}
}

func (a *App) OpenNotificationHistory() {
	a.mu.Lock()
	a.view = "history"
	a.mu.Unlock()
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		runtime.EventsEmit(a.ctx, "show-history", nil)
	}
	a.emitStatus()
}

func (a *App) LeaveNotificationHistory() {
	a.mu.Lock()
	if a.cfg.Configured() {
		a.view = "gotify"
	} else {
		a.view = "setup"
	}
	view := a.view
	a.mu.Unlock()
	if a.ctx != nil {
		if view == "gotify" {
			runtime.EventsEmit(a.ctx, "show-gotify", map[string]string{
				"token":  a.GetClientToken(),
				"uiPath": a.GetUIPath(),
			})
		} else {
			runtime.EventsEmit(a.ctx, "show-setup", nil)
		}
	}
	a.emitStatus()
}
```

Ensure `GetStatus` can return `view: "history"`. Update `AppStatus.View` comment.

Update `frontend/wailsjs/go/main/App.js` / `.d.ts` with the new exports (same style as existing wrappers).

- [ ] **Step 5: Compile**

Run: `go test ./... -count=1` and `go build -o /dev/null .`
Expected: PASS / success

- [ ] **Step 6: Commit**

```bash
git add app.go frontend/wailsjs/go/main/App.js frontend/wailsjs/go/main/App.d.ts
git commit -m "$(cat <<'EOF'
Wire notification history into stream handling and Wails API.

EOF
)"
```

---

### Task 4: Tray + app menu entry

**Files:**
- Modify: `internal/tray/types.go`
- Modify: `internal/tray/tray_systray.go`
- Modify: `internal/tray/tray_darwin.go` (if Labels need no-op already covered)
- Modify: `app.go` (`buildMenu`, `trayLabels`, callbacks)

**Interfaces:**
- Consumes: `OpenNotificationHistory`
- Produces: tray item + menu item calling it; labels `tray.history` / `menu.recent_notifications`

- [ ] **Step 1: Extend `Callbacks` with `OnHistory func()`; `Labels` with `History`, `HistoryTooltip`**

- [ ] **Step 2: In `onReady`, add menu item after Settings (before autostart):**

```go
historyItem := systray.AddMenuItem(or(l.History, "Recent notifications"), or(l.HistoryTooltip, "Show recent notifications"))
// store on controller; in select loop:
case <-historyItem.ClickedCh:
    if c.cb.OnHistory != nil {
        c.cb.OnHistory()
    }
```

Update `SetLabels` to refresh the history title.

- [ ] **Step 3: `buildMenu` add `menu.Text(i18n.T("menu.recent_notifications"), nil, func(...){ a.OpenNotificationHistory() })` near settings**

- [ ] **Step 4: Wire `OnHistory: a.OpenNotificationHistory` in `startup`**

- [ ] **Step 5: Add i18n keys (en/de/fr/es)** — can be completed in Task 5 if preferred; minimum English keys here so menu is not raw keys:

```json
"menu.recent_notifications": "Recent Notifications",
"tray.history": "Recent notifications",
"tray.history_tooltip": "Show recent notifications"
```

(de/fr/es in Task 5)

- [ ] **Step 6: `go test ./...` + commit**

```bash
git commit -m "$(cat <<'EOF'
Add tray and app menu entry for notification history.

EOF
)"
```

---

### Task 5: Frontend history view + i18n + copy

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/src/main.js`
- Modify: `frontend/src/style.css`
- Modify: `internal/i18n/locales/en.json`, `de.json`, `fr.json`, `es.json`

**Interfaces:**
- Consumes: `GetNotificationHistory`, `GetHistoryMode`, `SetHistoryMode`, `ClearNotificationHistory`, `LeaveNotificationHistory`, events `show-history`, `history-updated`
- Produces: scrollable list UI with toggle, clear, back, copy buttons

- [ ] **Step 1: HTML** — add section after gotify-view:

```html
<section id="history-view" class="view" hidden>
  <header class="history-header">
    <button type="button" id="history-back" data-i18n="history.back">Back</button>
    <h1 data-i18n="history.title">Recent notifications</h1>
    <div class="history-actions">
      <label class="history-mode">
        <input type="checkbox" id="history-mode-all"/>
        <span data-i18n="history.mode_all">All messages</span>
      </label>
      <button type="button" id="history-clear" data-i18n="history.clear">Clear</button>
    </div>
  </header>
  <p id="history-empty" class="history-empty" data-i18n="history.empty" hidden>No notifications yet.</p>
  <ul id="history-list" class="history-list"></ul>
  <p id="history-toast" class="history-toast" hidden></p>
</section>
```

Checkbox unchecked = toasts mode (default); checked = all.

- [ ] **Step 2: CSS** — header sticky, list `overflow-y: auto; height: calc(100% - header)`; row with meta + copy button using existing CSS variables (`--panel`, `--accent`, etc.). Keep visual language consistent with setup card (no new design system).

- [ ] **Step 3: JS**

- Import new Go bindings
- `showHistory()` hides setup/gotify, shows history, loads mode + list
- `renderHistory(entries)` builds `<li>` with time (`toLocaleString`), title, body, copy button
- Copy: build Markdown client-side matching Go helper (or call a tiny exported `FormatNotificationMarkdown` — prefer **duplicate format in JS** matching spec to avoid extra round-trip; keep identical format string)
- Prefer matching Go: export `FormatHistoryMarkdown(entry)` from Go if easier — **use Go binding** `FormatHistoryEntryMarkdown(id string) (string, error)` looking up entry by id in unfiltered store to guarantee one format. Add to Task 3 if missing:

```go
func (a *App) FormatHistoryEntryMarkdown(id string) string {
	if a.history == nil {
		return ""
	}
	for _, e := range a.history.List(false) {
		if e.ID == id {
			return history.FormatMarkdown(e, time.Local)
		}
	}
	return ""
}
```

(If duplicate IDs possible, match first newest.)

- `navigator.clipboard.writeText(md)` then show `t('history.copied')` for ~1.5s; on failure `t('history.copy_failed')`
- Events: `show-history` → `showHistory()`; `history-updated` → refresh if `currentView==='history'`
- `status` handler: if `view==='history'` keep history visible (do not bounce to setup/gotify)
- Mode checkbox → `SetHistoryMode(checked ? 'all' : 'toasts')` then re-fetch list
- Clear → `ClearNotificationHistory()`
- Back → `LeaveNotificationHistory()`

- [ ] **Step 4: i18n keys** (all four locale files; EN text as source):

| Key | en |
| --- | --- |
| `menu.recent_notifications` | Recent Notifications |
| `tray.history` | Recent notifications |
| `tray.history_tooltip` | Show recent notifications |
| `history.title` | Recent notifications |
| `history.back` | Back |
| `history.mode_all` | All messages |
| `history.mode_toasts_hint` | (optional aria) Show only toast notifications |
| `history.clear` | Clear |
| `history.empty` | No notifications yet. |
| `history.copy` | Copy |
| `history.copied` | Copied |
| `history.copy_failed` | Copy failed |

Translate de/fr/es appropriately.

- [ ] **Step 5: Build frontend**

Run: `cd frontend && npm run build`
Expected: success

- [ ] **Step 6: Commit**

```bash
git add frontend/ internal/i18n/locales/ app.go frontend/wailsjs/
git commit -m "$(cat <<'EOF'
Add history view with mode toggle and Markdown copy.

EOF
)"
```

---

### Task 6: Verify end-to-end locally

**Files:** none (verification only)

- [ ] **Step 1:** `go test ./... -count=1`
- [ ] **Step 2:** `cd frontend && npm run build`
- [ ] **Step 3:** Manual checklist (document results in commit message or leave for PR):
  - Tray → Recent notifications opens list
  - Send Gotify message → appears with timestamp after toast
  - Toggle All → below-min-priority messages appear when mode was all at receive time
  - Restart → entries persist
  - Copy → Markdown clipboard matches format
  - Clear empties list
  - macOS: app menu item works (if available)

- [ ] **Step 4:** If README still incomplete, finalize history section; commit docs if needed

```bash
git add README.md
git commit -m "$(cat <<'EOF'
Document notification history in README.

EOF
)"
```

---

## Spec coverage checklist

| Spec item | Task |
| --- | --- |
| Dedicated history view | 5 |
| Persist max 100 | 1 |
| Default toasts mode | 2, 3 |
| Toggle in history UI | 5 |
| Capture rules / dedup / min_priority | 3 |
| TestNotification in history | 3 |
| Tray + macOS menu | 4 |
| Markdown copy | 1, 5 |
| history-updated event | 3, 5 |
| i18n en/de/fr/es | 5 |
| Corrupt file → empty | 1 |
| README | 2, 6 |

## Self-review notes

- `FormatHistoryEntryMarkdown` is required by Task 5; implement in Task 3 (or fold into Task 5 commit) — do not leave unimplemented.
- `TestListToastsOnly` in Task 1 must use clean assertions only (no contradictory comments).
- `OpenSettings` should not fight history view; leaving history via Back is enough (YAGNI).
