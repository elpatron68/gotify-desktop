# Persistent Window Geometry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist native window size, position, and maximised state across restarts and hide-to-tray.

**Architecture:** Store geometry in `config.json`. Pure helpers in `internal/config` for validation/clamping; `App.RestoreWindowState` / `SaveWindowState` use Wails runtime; frontend debounces resize/move into `SaveWindowState`; also save on beforeClose/shutdown.

**Tech Stack:** Go, Wails v2 runtime window APIs, existing `config.Save`/`Load`, vanilla frontend.

## Global Constraints

- Fields: `window_width`, `window_height`, `window_x`, `window_y`, `window_pos_set`, `window_maximised`
- Width/height `<=0` = unset → defaults 1100×800; clamp to min 640×480
- Position: only apply if `window_pos_set`; simple off-screen skip
- Debounce ~400ms while resizing/moving; also save on hide-to-tray and quit
- Do not force-show window on restore (`StartHidden` preserved)
- Save errors: log only
- README documents new fields

---

### Task 1: Config geometry fields + pure helpers

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `README.md` (fields line only is OK here or Task 4)

**Interfaces:**
- Produces on `Config`:
  - `WindowWidth, WindowHeight, WindowX, WindowY int`
  - `WindowPosSet, WindowMaximised bool`
- Produces:
  - `const DefaultWindowWidth = 1100`, `DefaultWindowHeight = 800`, `MinWindowWidth = 640`, `MinWindowHeight = 480`
  - `func (c Config) ResolvedWindowSize() (w, h int)` — defaults + clamp
  - `func ClampWindowSize(w, h int) (int, int)`
  - `func WindowPositionUsable(x, y, w, h int) bool` — false if clearly off primary heuristics: reject if `x+w < 50 || y+h < 50 || x > 10000 || y > 10000` (simple guard; not full multi-monitor)

- [ ] **Step 1: Failing tests**

```go
func TestResolvedWindowSizeDefaults(t *testing.T) {
	c := Default()
	w, h := c.ResolvedWindowSize()
	if w != DefaultWindowWidth || h != DefaultWindowHeight {
		t.Fatalf("%d×%d", w, h)
	}
}

func TestResolvedWindowSizeClamps(t *testing.T) {
	c := Config{WindowWidth: 100, WindowHeight: 50}
	w, h := c.ResolvedWindowSize()
	if w != MinWindowWidth || h != MinWindowHeight {
		t.Fatalf("%d×%d", w, h)
	}
}

func TestWindowPositionUsable(t *testing.T) {
	if !WindowPositionUsable(100, 100, 800, 600) {
		t.Fatal("expected usable")
	}
	if WindowPositionUsable(-9000, -9000, 800, 600) {
		t.Fatal("expected unusable")
	}
}
```

- [ ] **Step 2:** `go test ./internal/config/ -run 'Window|Resolved' -count=1` → FAIL

- [ ] **Step 3: Implement fields + helpers**

```go
const (
	DefaultWindowWidth  = 1100
	DefaultWindowHeight = 800
	MinWindowWidth      = 640
	MinWindowHeight     = 480
)

// on Config:
WindowWidth     int  `json:"window_width,omitempty"`
WindowHeight    int  `json:"window_height,omitempty"`
WindowX         int  `json:"window_x,omitempty"`
WindowY         int  `json:"window_y,omitempty"`
WindowPosSet    bool `json:"window_pos_set,omitempty"`
WindowMaximised bool `json:"window_maximised,omitempty"`

func ClampWindowSize(w, h int) (int, int) {
	if w < MinWindowWidth {
		w = MinWindowWidth
	}
	if h < MinWindowHeight {
		h = MinWindowHeight
	}
	return w, h
}

func (c Config) ResolvedWindowSize() (int, int) {
	w, h := c.WindowWidth, c.WindowHeight
	if w <= 0 {
		w = DefaultWindowWidth
	}
	if h <= 0 {
		h = DefaultWindowHeight
	}
	return ClampWindowSize(w, h)
}

func WindowPositionUsable(x, y, w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if x+w < 50 || y+h < 50 {
		return false
	}
	if x > 10000 || y > 10000 {
		return false
	}
	return true
}
```

- [ ] **Step 4:** tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/ README.md
git commit -m "$(cat <<'EOF'
Add config fields for persistent window geometry.

EOF
)"
```

---

### Task 2: App restore + save via Wails

**Files:**
- Modify: `app.go`
- Modify: `frontend/wailsjs/go/main/App.js`
- Modify: `frontend/wailsjs/go/main/App.d.ts`
- Optionally: `main.go` — use `config.DefaultWindowWidth` constants for Width/Height/Min* (keep numbers in sync)

**Interfaces:**
- Consumes: config helpers above; `runtime.WindowGetSize/SetSize/GetPosition/SetPosition/WindowIsMaximised/WindowMaximise`
- Produces:
  - `func (a *App) RestoreWindowState()`
  - `func (a *App) SaveWindowState()`
  - Call restore at end of `startup` (ctx set) before tray work is fine
  - Call `SaveWindowState` at start of `beforeClose` and `shutdown` (and existing Quit path if distinct)

- [ ] **Step 1: Implement RestoreWindowState**

```go
func (a *App) RestoreWindowState() {
	if a.ctx == nil {
		return
	}
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	w, h := cfg.ResolvedWindowSize()
	runtime.WindowSetSize(a.ctx, w, h)
	if cfg.WindowPosSet && config.WindowPositionUsable(cfg.WindowX, cfg.WindowY, w, h) {
		runtime.WindowSetPosition(a.ctx, cfg.WindowX, cfg.WindowY)
	}
	if cfg.WindowMaximised {
		runtime.WindowMaximise(a.ctx)
	}
}
```

Call from `startup` after config load (ctx already assigned at start of startup).

- [ ] **Step 2: Implement SaveWindowState**

```go
func (a *App) SaveWindowState() {
	if a.ctx == nil {
		return
	}
	w, h := runtime.WindowGetSize(a.ctx)
	x, y := runtime.WindowGetPosition(a.ctx)
	maxed := runtime.WindowIsMaximised(a.ctx)
	w, h = config.ClampWindowSize(w, h)

	a.mu.Lock()
	cfg := a.cfg
	if maxed {
		// Keep prior normal size if we already have one; still update maximised flag.
		// If previous width unset, store current get (may be screen-sized on some platforms).
		if cfg.WindowWidth <= 0 || cfg.WindowHeight <= 0 {
			cfg.WindowWidth, cfg.WindowHeight = w, h
		}
		if !cfg.WindowPosSet {
			cfg.WindowX, cfg.WindowY = x, y
			cfg.WindowPosSet = true
		}
		cfg.WindowMaximised = true
	} else {
		cfg.WindowWidth, cfg.WindowHeight = w, h
		cfg.WindowX, cfg.WindowY = x, y
		cfg.WindowPosSet = true
		cfg.WindowMaximised = false
	}
	changed := cfg.WindowWidth != a.cfg.WindowWidth ||
		cfg.WindowHeight != a.cfg.WindowHeight ||
		cfg.WindowX != a.cfg.WindowX ||
		cfg.WindowY != a.cfg.WindowY ||
		cfg.WindowPosSet != a.cfg.WindowPosSet ||
		cfg.WindowMaximised != a.cfg.WindowMaximised
	if !changed {
		a.mu.Unlock()
		return
	}
	a.cfg = cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		applog.Printf("window geometry save: %v", err)
	}
}
```

Improve maximised path: when `maxed` and we already have normal bounds in cfg, **do not** overwrite width/height/x/y with maximised screen size — only set `WindowMaximised=true`. When not maxed, always write current bounds.

```go
if maxed {
	cfg.WindowMaximised = true
	// leave width/height/x/y as last known normal bounds if already set
	if cfg.WindowWidth <= 0 || cfg.WindowHeight <= 0 {
		cfg.WindowWidth, cfg.WindowHeight = w, h
	}
	if !cfg.WindowPosSet {
		cfg.WindowX, cfg.WindowY = x, y
		cfg.WindowPosSet = true
	}
} else {
	cfg.WindowWidth, cfg.WindowHeight = w, h
	cfg.WindowX, cfg.WindowY = x, y
	cfg.WindowPosSet = true
	cfg.WindowMaximised = false
}
```

- [ ] **Step 3: Wire beforeClose + shutdown**

```go
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	a.SaveWindowState()
	// ... existing quit/hide logic
}

func (a *App) shutdown(ctx context.Context) {
	a.SaveWindowState()
	// ... existing cleanup
}
```

- [ ] **Step 4: Export bindings** in `App.js` / `App.d.ts` for `SaveWindowState` (Restore is Go-only).

- [ ] **Step 5:** `go test ./... -count=1` && `go build -o /dev/null .`

- [ ] **Step 6: Commit**

```bash
git commit -m "$(cat <<'EOF'
Wire window geometry restore and save through Wails.

EOF
)"
```

---

### Task 3: Frontend debounce caller

**Files:**
- Modify: `frontend/src/main.js`
- Bindings already updated in Task 2

**Interfaces:**
- Consumes: `SaveWindowState` from wailsjs
- Debounce 400ms on `window` `resize`; also `setInterval` every 1s while document visible comparing last saved fingerprint is optional — prefer: resize listener + call `SaveWindowState` on `visibilitychange` when hidden

- [ ] **Step 1: Add debounce helper and listeners** near bootstrap:

```js
import { ..., SaveWindowState } from '../wailsjs/go/main/App';

let saveGeoTimer = null;
function scheduleSaveWindowState() {
  clearTimeout(saveGeoTimer);
  saveGeoTimer = setTimeout(() => {
    SaveWindowState().catch((err) => console.warn('SaveWindowState', err));
  }, 400);
}

window.addEventListener('resize', scheduleSaveWindowState);
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'hidden') {
    clearTimeout(saveGeoTimer);
    SaveWindowState().catch(() => {});
  }
});

// Lightweight move detection: poll position via SaveWindowState every 2s while focused
// (Wails has no window-moved DOM event). Cheap: only calls Go which no-ops if unchanged.
setInterval(() => {
  if (document.visibilityState === 'visible') {
    SaveWindowState().catch(() => {});
  }
}, 2000);
```

Note: 2s poll + change detection in Go avoids disk spam.

- [ ] **Step 2:** `cd frontend && npm run build`

- [ ] **Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
Debounce window geometry saves from the frontend.

EOF
)"
```

---

### Task 4: README + verify

**Files:**
- Modify: `README.md` if not done in Task 1

- [ ] **Step 1:** Document `window_*` fields next to other config fields.

- [ ] **Step 2:** `go test ./... -count=1` && `cd frontend && npm run build`

- [ ] **Step 3:** Manual checklist (note if GUI unavailable): resize → hide tray → show; maximise → quit → restore; off-screen config skipped.

- [ ] **Step 4: Commit** if README changed

```bash
git commit -m "$(cat <<'EOF'
Document window geometry config fields.

EOF
)"
```

---

## Spec coverage

| Spec item | Task |
| --- | --- |
| config fields + pos flag | 1 |
| restore size/pos/max | 2 |
| save debounced + hide/quit | 2, 3 |
| off-screen guard | 1, 2 |
| README | 1 or 4 |
| no force-show on restore | 2 |

## Self-review

- Maximised must not overwrite normal bounds with fullscreen size when bounds already known.
- `SaveWindowState` is idempotent when unchanged (no config rewrite storm from 2s poll).
