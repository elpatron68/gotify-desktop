# Gotify Desktop

Cross-platform Gotify desktop client built with [Wails v2](https://wails.io) and Go.

## Features

- Connect with Gotify username/password; the app creates a **client token** (password is not stored)
- Native desktop **notifications** via Gotify WebSocket `/stream`
- **System tray** with open / recent notifications / settings / start-at-login / quit (Linux/Windows; macOS uses the app menu because systray conflicts with Wails)
- **Notification history**: the last 100 notifications are kept in `history.json`; open **Recent notifications** from the tray (or the app menu on macOS) to browse them, copy an entry as Markdown, clear the list, or switch between toast-only and all messages (`history_mode`)
- **Start at login** via OS autostart (Windows Run key, Linux XDG autostart, macOS LaunchAgent)
- Window close minimizes to tray / background (quit via tray or app menu)
- Window size, position and maximised state are remembered. Known limitations: positions that are negative or on a monitor left of/above the primary one may be treated as unusable (the window then opens at the default position), and on Wayland the reported position may be 0,0
- Embedded Gotify **web UI** (management) in an in-app iframe

## Requirements

| Platform | Prerequisites |
|----------|----------------|
| Linux | Go 1.25+, Node.js 24+, `libgtk-3-dev`, `libwebkit2gtk-4.0-dev` or `libwebkit2gtk-4.1-dev`, `gcc`, `pkg-config` |
| Windows | Go 1.25+, Node.js 24+, WebView2 runtime |
| macOS | Go 1.25+, Node.js 24+, Xcode Command Line Tools |

Ubuntu 24.04 example (`libwebkit2gtk-4.1`):

```bash
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

### Linux system tray (AppIndicator)

On Linux the tray icon is provided by [`fyne.io/systray`](https://github.com/fyne-io/systray). Unlike Windows and macOS, most Linux desktops do **not** draw tray icons themselves. Instead the app registers a **Status Notifier Item** (SNI) / **AppIndicator** on D-Bus, and a panel applet (or extension) displays it.

If that host component is missing, the app can still run, but:

- no Gotify tray icon appears
- “minimize to tray” feels like the window simply vanished
- Quit / Settings / Open are only reachable via the window menu (where available)

#### Runtime package

On Debian/Ubuntu and derivatives, install the Ayatana AppIndicator library (used by many trays and by Go systray bindings):

```bash
sudo apt install libayatana-appindicator3-1
```

Related packages you may already have via the desktop:

| Package | Role |
|---------|------|
| `libayatana-appindicator3-1` | Runtime library the tray client talks to |
| `libayatana-appindicator3-dev` | Only needed if you **build** against AppIndicator headers |
| `gir1.2-ayatanaappindicator3-0.1` | GObject introspection (often pulled in automatically) |

Older docs mention `libappindicator3-1` (Unity-era). Prefer **Ayatana** on modern Ubuntu; it is the maintained fork.

Fedora / RHEL-like:

```bash
sudo dnf install libappindicator-gtk3
```

Arch:

```bash
sudo pacman -S libappindicator-gtk3
```

#### Desktop environment support

Having the library installed is necessary but not always sufficient. The shell must also **host** indicators:

| Desktop | Tray support |
|---------|----------------|
| **KDE Plasma** | Built-in system tray; works out of the box with SNI |
| **XFCE** | Panel → add “Status Tray Plugin” / “Indicator Plugin” |
| **MATE** | Panel → “Notification Area” / “Indicator Applet Complete” |
| **Cinnamon** | Usually works with the default panel tray |
| **GNOME (45+)** | No classic tray by default. Install an extension such as [AppIndicator and KStatusNotifierItem Support](https://extensions.gnome.org/extension/615/appindicator-support/) (`gnome-shell-extension-appindicator` on Ubuntu) and enable it |
| **WSL / headless** | Often **no** tray host at all; run under a full Linux desktop session or WSLg with a DE that provides one |

Ubuntu GNOME example:

```bash
sudo apt install libayatana-appindicator3-1 gnome-shell-extension-appindicator
```

Then enable the extension (Extensions app, or GNOME Tweaks / `gnome-extensions enable …`), and log out/in if the icon still does not show.

#### Build vs run

- **Building** Gotify Desktop needs GTK/WebKit **dev** packages (`libgtk-3-dev`, `libwebkit2gtk-4.1-dev`, …).
- **Running** a prebuilt binary on another machine needs the **runtime** stack: GTK 3, WebKitGTK, and for the tray typically `libayatana-appindicator3-1` plus a working panel indicator host.

#### Quick checks

```bash
# Library present?
ldconfig -p | grep -i appindicator

# Session bus available? (required for StatusNotifierItem)
echo "$DBUS_SESSION_BUS_ADDRESS"

# On GNOME: is the AppIndicator extension enabled?
gnome-extensions list 2>/dev/null | grep -i indicator || true
```

If the window opens but the tray icon never appears, install the Ayatana package, confirm your DE hosts indicators (especially on GNOME), then restart the app.

Install the Wails CLI (ensure `$HOME/go/bin` is on your `PATH`):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## Develop

```bash
cd frontend && npm install && cd ..
wails dev
```

On Ubuntu 24.04 with WebKit 4.1, `wails.json` sets `build:tags` to `webkit2_41`. If needed explicitly:

```bash
wails dev -tags webkit2_41
```

## Build

```bash
wails build
```

Binary: `build/bin/gotify-desktop`.

Binary output: `build/bin/gotify-desktop` (extension varies by OS).

Cross-compile examples (host toolchains / Docker may be required):

```bash
wails build -platform windows/amd64
wails build -platform darwin/arm64
wails build -platform linux/amd64
```

## CI/CD & packaging

GitHub Actions (`.github/workflows/ci.yml`) runs on pushes/PRs to `main` and on version tags:

1. Unit tests
2. Native builds for **Linux**, **Windows**, and **macOS** (arm64)
3. Installer / portable packages uploaded as artifacts (and attached to Releases on tags)

| Platform | Artifacts |
|----------|-----------|
| Windows | NSIS setup (`*-setup.exe`) + portable `.exe` |
| Linux | AppImage + raw binary (needs host WebKitGTK 4.1) |
| macOS | DMG + `.app` zip (unsigned) |

Local helpers:

```bash
# Windows (with NSIS on PATH)
wails build -platform windows/amd64 -nsis

# Linux AppImage (after wails build)
./packaging/linux/build-appimage.sh build/bin/gotify-desktop dist linux-amd64

# macOS DMG (after wails build)
./packaging/darwin/build-dmg.sh build/bin/gotify-desktop.app dist darwin-arm64
```

Pushing a version tag creates a GitHub Release:

```bash
git tag v0.1.9
git push origin v0.1.9
```

## Configuration

Settings are stored under the user config directory:

- Linux: `~/.config/gotify-desktop/config.json`
- macOS: `~/Library/Application Support/gotify-desktop/config.json`
- Windows: `%AppData%\gotify-desktop\config.json`

Fields: `server_url`, `client_token`, `min_priority` (default `0`), `last_message_id`, `autostart`, `locale` (optional BCP-47 tag such as `de` / `en`; empty = OS language), `history_mode` (`toasts` default, or `all`; unknown/empty → `toasts`), `window_width` / `window_height` (0 or missing = defaults 1100×800; clamped to min 640×480), `window_x` / `window_y` with `window_pos_set` (so `0,0` is valid), `window_maximised`. Notification history is stored as `history.json` in the same directory.

### Internationalization

UI strings (setup form, tray, app menu, status) live in `internal/i18n/locales/*.json`. **English is the fallback.** Bundled languages: `en`, `de`, `fr`, `es`.

Default: OS language if available, otherwise English. Change via **Language** in the tray (or app menu on macOS); the choice is stored as `locale` in `config.json`.

Notification diagnostics are appended to `notify.log` in the same directory (e.g. `%AppData%\gotify-desktop\notify.log` on Windows).

To verify only the Gotify WebSocket stream (from any machine with Go):

```bash
GOTIFY_URL=https://gotify.example.com GOTIFY_TOKEN=... go run ./cmd/streamcheck
```

## License

MIT — see [LICENSE](LICENSE).
