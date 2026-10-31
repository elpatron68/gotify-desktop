# Gotify Desktop

[![CI](https://github.com/elpatron68/gotify-desktop/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/elpatron68/gotify-desktop/actions/workflows/ci.yml)
[![Build](https://img.shields.io/github/actions/workflow/status/elpatron68/gotify-desktop/ci.yml?branch=main&label=build)](https://github.com/elpatron68/gotify-desktop/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/elpatron68/gotify-desktop)](https://github.com/elpatron68/gotify-desktop/releases)
[![Wails](https://img.shields.io/badge/Wails-v3.0.0-red)](https://v3.wails.io)
[![Go](https://img.shields.io/github/go-mod/go-version/elpatron68/gotify-desktop?logo=go&logoColor=white)](https://github.com/elpatron68/gotify-desktop/blob/main/go.mod)
[![Node.js](https://img.shields.io/badge/Node.js-24-339933?logo=nodedotjs&logoColor=white)](https://nodejs.org/)
[![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20Windows%20%7C%20macOS-blue)](#requirements)

Cross-platform Gotify desktop client built with [Wails v3](https://v3.wails.io) and Go.

## Features

- Connect with Gotify username/password; the app creates a **client token** (password is not stored)
- Native desktop **notifications** via Gotify WebSocket `/stream`
- **System tray** with open / recent notifications / settings / start-at-login / quit on Linux, Windows and macOS (Wails built-in tray; macOS also keeps the application menu)
- **Notification history**: the last 100 notifications are kept in `history.json`; open **Recent notifications** from the tray (or the app menu on macOS) to browse them, copy an entry as Markdown, clear the list, or switch between toast-only and all messages (`history_mode`)
- **Send message**: open **Send message** from the tray (or the app menu on macOS), pick one of your Gotify applications, enter title / message / priority, and push to that app channel (app token stays in the Go backend)
- **Automatic updates**: on startup (at most every 24 hours) and via **Check for Updates** in About, the app queries GitHub Releases, asks before download/install, verifies `SHA256SUMS`, then replaces the running package
- **Start at login** via OS autostart (Windows Run key, Linux XDG autostart, macOS LaunchAgent)
- Window close minimizes to tray / background (quit via tray or app menu)
- Window size, position and maximised state are remembered. Known limitations: positions that are negative or on a monitor left of/above the primary one may be treated as unusable (the window then opens at the default position); under **Wayland** the reported position is often `0,0` (see below)
- Embedded Gotify **web UI** (management) in an in-app iframe

## Requirements

| Platform | Prerequisites |
|----------|----------------|
| Linux | Go 1.26+, Node.js 24+, `libgtk-4-dev`, `libwebkitgtk-6.0-dev`, `gcc`, `pkg-config` |
| Windows | Go 1.26+, Node.js 24+, WebView2 runtime |
| macOS | Go 1.26+, Node.js 24+, Xcode Command Line Tools |

Ubuntu 24.04 example (GTK4 / WebKitGTK 6.0):

```bash
sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
```

### Linux system tray (AppIndicator)

On Linux the tray icon is provided by the Wails v3 system tray (Status Notifier Item / AppIndicator on D-Bus). Unlike Windows and macOS, most Linux desktops do **not** draw tray icons themselves — a panel applet (or extension) must host them.

If that host component is missing, the app can still run, but:

- no Gotify tray icon appears
- “minimize to tray” feels like the window simply vanished
- Quit / Settings / Open are only reachable via the window menu (where available)

#### Runtime package

On Debian/Ubuntu and derivatives, install the Ayatana AppIndicator library (used by many trays):

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
sudo dnf install libappindicator-gtk3 gnome-shell-extension-appindicator
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
| **GNOME (45+)** | No classic tray by default. Install an extension such as [AppIndicator and KStatusNotifierItem Support](https://extensions.gnome.org/extension/615/appindicator-support/) (`gnome-shell-extension-appindicator` on Ubuntu/Fedora) and **enable** it |
| **Hyprland / Sway / wlroots** | No tray unless a panel hosts SNI (e.g. Waybar `tray` module). Same “vanished window” symptom without it |
| **WSL / headless** | Often **no** tray host at all; run under a full Linux desktop session or WSLg with a DE that provides one |

Ubuntu GNOME example:

```bash
sudo apt install libayatana-appindicator3-1 gnome-shell-extension-appindicator
```

Then enable the extension (Extensions app, or GNOME Tweaks / `gnome-extensions enable …`), and log out/in if the icon still does not show.

#### Fedora / GNOME: “app vanished after login”

On a successful first connect (and whenever a saved session starts), Gotify Desktop **hides the main window and keeps running in the tray**. That is intentional — not a crash.

Under stock **Fedora Workstation (GNOME)** there is no classic system tray. Without an indicator host the window simply disappears and there is nowhere obvious to reopen it.

What to do:

1. Install runtime + extension packages (see `dnf` above).
2. Open **Extensions** (`gnome-extensions-app`) and enable **AppIndicator and KStatusNotifierItem Support**.
3. If the toggle stays greyed out, user extensions may be disabled globally — turn them on, then enable the extension:

```bash
gsettings set org.gnome.shell disable-user-extensions false
gnome-extensions enable appindicatorsupport@rgcjonas.gmail.com
```

4. Restart Gotify Desktop. The tray icon appears in the **GNOME top bar** (top-right, near network/volume/clock) — not in the Activities app grid.
5. Use the tray menu (**Open window** / **Quit**). To force a restart from a terminal:

```bash
pkill -f gotify-desktop; gotify-desktop &
```

Log out/in once if the icon still does not appear after enabling the extension.

#### Wayland (e.g. CachyOS, Fedora Workstation, Plasma Wayland)

Gotify Desktop is a GTK4 / WebKitGTK 6 / Wails v3 app. It generally runs under Wayland, but a few behaviours differ from X11:

| Area | What to expect |
|------|----------------|
| **Window position** | Wayland often does not expose global coordinates to clients (`gtk_window_get_position` may always report `0,0`). **Size** and **maximised** still persist; restoring a previous screen position may be ignored or land at the compositor default. |
| **Tray** | Same rules as above — the compositor/shell must host Status Notifier / AppIndicator. Plasma Wayland usually works out of the box; GNOME needs the AppIndicator extension; Hyprland/Sway need a panel with a tray module. |
| **Notifications** | Desktop toasts go through `xdg-desktop-portal` and a session backend. Plasma/GNOME usually provide one; a minimal Hyprland setup may need an extra portal package or show `Notify: Fallback` / silent toasts until portals are configured. |
| **Autostart** | XDG autostart still works, but starting before the session bus / portals are ready can delay tray or notifications once — restarting the app after login fixes that. |
| **Clipboard / focus** | Occasional focus or clipboard quirks in the embedded UI (e.g. History → Copy) are possible; retry or refocus the window if a copy fails. |

Quick environment check:

```bash
echo "$XDG_SESSION_TYPE"   # expect "wayland"
echo "$WAYLAND_DISPLAY"
echo "$XDG_CURRENT_DESKTOP"
```

Arch / CachyOS tray library (if needed):

```bash
sudo pacman -S libappindicator-gtk3
```

#### Build vs run

- **Building** Gotify Desktop needs GTK/WebKit **dev** packages (`libgtk-4-dev`, `libwebkitgtk-6.0-dev`, …).
- **Running** a prebuilt binary on another machine needs the **runtime** stack: GTK 4, WebKitGTK 6.0, and for the tray typically `libayatana-appindicator3-1` plus a working panel indicator host.

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

Install the Wails v3 CLI (ensure `$HOME/go/bin` is on your `PATH`):

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
```

## Develop

```bash
cd frontend && npm install && cd ..
wails3 dev
```

## Build

```bash
wails3 build
```

Binary: `bin/gotify-desktop` (extension / `.app` bundle varies by OS).

Cross-compile examples (host toolchains / Docker may be required):

```bash
wails3 build GOOS=windows GOARCH=amd64
wails3 build GOOS=darwin GOARCH=arm64
wails3 build GOOS=linux GOARCH=amd64
```

## CI/CD & packaging

GitHub Actions (`.github/workflows/ci.yml`) runs on pushes/PRs to `main` and on version tags:

1. Unit tests
2. Native builds for **Linux**, **Windows**, and **macOS** (arm64)
3. Installer / portable packages uploaded as artifacts (and attached to Releases on tags)

| Platform | Artifacts |
|----------|-----------|
| Windows | NSIS setup (`*-setup.exe`) + portable `.exe` |
| Linux | AppImage + raw binary (needs host WebKitGTK 6.0) |
| macOS | DMG + `.app` zip (unsigned) |
| All | `SHA256SUMS` (GNU sha256sum) attached to each GitHub Release |

### Automatic updates

Gotify Desktop checks `releases/latest` over HTTPS (User-Agent `gotify-desktop/<version>`). If a newer `major.minor.patch` is published, a dialog asks to install. After confirmation the matching asset and `SHA256SUMS` are downloaded, the hash is verified, and a helper process replaces the running install then relaunches.

| Running as | Update package |
|------------|----------------|
| Linux AppImage (`APPIMAGE` set) | `gotify-desktop-linux-<arch>.AppImage` |
| Linux binary | `gotify-desktop-linux-<arch>` |
| Windows EXE | `gotify-desktop-windows-<arch>.exe` (not the NSIS setup) |
| macOS `.app` | `gotify-desktop-darwin-<arch>.zip` |

Development builds with version `0.0.0` skip checks. Releases without a matching asset show an error and the releases URL. There is no code signing yet; integrity is HTTPS plus the release checksum file. Manual check: **About → Check for Updates** (tray or app menu).

Local helpers:

```bash
# Windows (with NSIS on PATH)
wails3 package GOOS=windows GOARCH=amd64

# Linux AppImage (after wails3 build)
./packaging/linux/build-appimage.sh bin/gotify-desktop dist linux-amd64

# macOS DMG (after wails3 build)
./packaging/darwin/build-dmg.sh bin/gotify-desktop.app dist darwin-arm64
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

Fields: `server_url`, `client_token`, `min_priority` (default `0`), `last_message_id`, `autostart`, `locale` (optional BCP-47 tag such as `de` / `en`; empty = OS language), `history_mode` (`toasts` default, or `all`; unknown/empty → `toasts`), `window_width` / `window_height` (0 or missing = defaults 1100×800; clamped to min 640×480), `window_x` / `window_y` with `window_pos_set` (so `0,0` is valid), `window_maximised`, `update_checked_unix` (last automatic update check). Notification history is stored as `history.json` in the same directory.

### Internationalization

UI strings (setup form, tray, app menu, status) live in `internal/i18n/locales/*.json`. **English is the fallback.** Bundled languages: `en`, `de`, `fr`, `es`.

Default: OS language if available, otherwise English. Change via **Language** in the tray or app menu; the choice is stored as `locale` in `config.json`.

Notification diagnostics are appended to `notify.log` in the same directory (e.g. `%AppData%\gotify-desktop\notify.log` on Windows).

To verify only the Gotify WebSocket stream (from any machine with Go):

```bash
GOTIFY_URL=https://gotify.example.com GOTIFY_TOKEN=... go run ./cmd/streamcheck
```

## License

MIT — see [LICENSE](LICENSE).
