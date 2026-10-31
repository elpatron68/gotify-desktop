# Notification history — design

Date: 2026-10-31  
Status: approved for planning

## Goal

Users can open a scrollable list of recent notifications from the tray (or macOS app menu), with timestamps. Entries persist across restarts. Each entry can be copied to the clipboard as Markdown.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Presentation | Dedicated in-app view (`history`), not tray submenu items |
| Persistence | On disk next to config; ring buffer max **100** |
| Default capture mode | **Toasts only** (`history_mode=toasts`) |
| Mode control | Toggle in the history view only (not setup form) |
| Modes | `toasts` = only toast-path messages; `all` = stream messages (see capture rules) |
| Copy format | Markdown: `**{title}**\n{body}\n\n_{timestamp}_` (omit title line if empty); local timestamp |
| Approach | Local ring buffer + JSON file + Wails UI (not Gotify REST, not `notify.log`) |

## Architecture

### Package `internal/history`

Responsibilities:

- In-memory list (newest first) capped at 100
- Load/save `history.json` in the same directory as `config.json`
- Thread-safe append + trim
- Pure helper to format an entry as Markdown for clipboard

Entry fields:

- `id` (string; Gotify message id or synthetic)
- `title`, `body` (string)
- `priority` (int)
- `app_id` (uint)
- `received_at` (RFC3339, local receive time)
- `notified` (bool — `true` if a desktop toast was attempted/shown for this entry)

Corrupt or missing file → treat as empty list and recreate on next save. Save errors are logged; they must not break the stream.

### Config

Add to `config.Config`:

- `history_mode` — `"toasts"` (default) or `"all"`

Persisted when the user toggles the control in the history UI.

### Capture rules (`handleMessage`)

Order inside `handleMessage`:

1. **Dedup** via `last_message_id` (unchanged). Replays never enter history or toast.
2. Decide toast eligibility with existing `min_priority` (and any other toast gates).
3. If eligible: show toast as today; append history with `notified=true`.
4. If not eligible for toast:
   - `history_mode=all` → append with `notified=false` (still recorded; below-priority messages are included)
   - `history_mode=toasts` → do not append

`TestNotification` appends a history entry with `notified=true`.

**Display filter:** UI shows all stored rows when mode is `all`, and only `notified=true` when mode is `toasts`. Changing mode does not rewrite old rows; it changes future capture and the filter used when reading for display.

### App / tray wiring

- Tray + app menu: **Recent notifications** / localized equivalent → `ShowWindow` + switch view to `history`
- Wails bindings:
  - `GetNotificationHistory() []HistoryEntry` (already filtered for current mode)
  - `GetHistoryMode() string` / `SetHistoryMode(mode string)`
  - `ClearNotificationHistory()`
  - Optional: `FormatNotificationMarkdown(id)` if clipboard is done in Go; otherwise format in frontend from entry fields
- Event `history-updated` after append/clear so an open history view refreshes
- View enum extended: `"setup" | "gotify" | "history"`

Clipboard: prefer frontend `navigator.clipboard.writeText` with the agreed Markdown; brief “Copied” feedback. Fallback to Wails clipboard API only if the webview blocks clipboard.

## UI

- Header: back (to gotify if configured, else setup), mode toggle (toasts / all), clear action
- Scrollable list, newest first
- Row: local timestamp, title, body (readable wrap), small copy button
- Empty state when no matching entries
- i18n keys in `en` / `de` / `fr` / `es` for menu, tray, headings, toggle, clear, copy, empty, copied

macOS: same entry in the native application menu (systray disabled).

## Error handling

- History I/O failures: log only
- Unknown `history_mode` on load: treat as `toasts`
- Clipboard failure: show short error in UI; do not crash

## Testing

- Ring buffer trims to 100 (oldest dropped)
- Load/save round-trip
- Display filter: `toasts` vs `all` with mixed `notified` flags
- Markdown helper matches the locked format
- Mode persistence via config (unit or thin app-level test where practical)

## Out of scope

- Fetching history from Gotify HTTP API
- Click-to-open deep link into Gotify message detail
- Syncing history across machines
- Unlimited retention or user-configurable cap (fixed 100)

## Success criteria

- Tray/menu opens a scrollable history view with timestamps
- Up to 100 entries survive restart
- Toggle switches toasts-only vs all (default toasts)
- Each row copies Markdown in the agreed format
- Existing toast/stream behavior unchanged aside from history side effects
