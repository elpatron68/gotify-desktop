# Persistent window geometry — design

Date: 2026-10-31  
Status: approved for planning

## Goal

The native Gotify Desktop window remembers **size**, **position**, and **maximised** state across restarts and hide-to-tray cycles.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| What | Native app window (not the Gotify iframe) |
| Fields | Width, height, x, y, maximised |
| Storage | `config.json` (same file as session settings) |
| When to save | Debounced while moving/resizing **and** on hide-to-tray / quit |
| First run | Current defaults: 1100×800, OS default position, not maximised |
| Approach | Wails runtime get/set + config fields |

## Config fields

Optional keys on `config.Config` (JSON):

- `window_width` / `window_height` (int; 0 or missing = use app defaults)
- `window_x` / `window_y` (int; use a presence flag or sentinel so `0,0` is valid — prefer explicit `window_geometry_set bool` or store only when user has moved once; **implementation:** treat width/height `<=0` as unset; for position use `window_pos_set bool` `json:"window_pos_set,omitempty"` so `0,0` can be saved)
- `window_maximised` (bool)

Defaults in code remain `Width: 1100`, `Height: 800`, `MinWidth: 640`, `MinHeight: 480` in `main.go`.

## Restore

After Wails context is available (`startup` or early `domReady`):

1. If saved width/height valid (≥ mins): `WindowSetSize`
2. If `window_pos_set`: `WindowSetPosition` after a simple on-screen clamp (if clearly off-screen, skip position and let OS place)
3. If `window_maximised`: `WindowMaximise`
4. Do not force-show the window here if `StartHidden` / tray flow expects hidden start — apply geometry while hidden is OK

## Save

- Exported `SaveWindowState()` on `App`:
  - Read size, position, maximised via Wails runtime
  - If maximised: keep previously stored normal bounds when the platform reports maximised size as the full screen; if platform returns restore bounds already, persist those
  - Clamp width/height to ≥ min sizes
  - Set `window_pos_set = true`
  - Persist only if something changed
  - Log save errors; never fail the UI path
- Debounce ~400ms from the frontend (window `resize` + lightweight position polling or move-related events if available) calling `SaveWindowState`
- Also call from `beforeClose` (hide-to-tray) and shutdown/quit paths

## Out of scope

- Per-monitor DPI profile databases
- Fullscreen state (distinct from maximised)
- Remembering which virtual desktop / workspace

## Success criteria

- Resize, move, maximise → quit or hide → reopen → same geometry
- Debounced saves during interactive resize/move without noticeable UI jank
- Invalid/off-screen saved position does not leave the window unreachable
- README mentions the new config fields
