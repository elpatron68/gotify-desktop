# Gotify Desktop

Cross-platform Gotify desktop client built with [Wails v2](https://wails.io) and Go.

## Features

- Connect with Gotify username/password; the app creates a **client token** (password is not stored)
- Native desktop **notifications** via Gotify WebSocket `/stream`
- **System tray** with open / settings / quit (Linux/Windows; macOS uses the app menu because systray conflicts with Wails)
- Window close minimizes to tray / background (quit via tray or app menu)
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

## CI/CD

GitHub Actions (`.github/workflows/ci.yml`) runs on pushes/PRs to `main`:

1. Unit tests
2. Native builds for **Linux**, **Windows**, and **macOS** (universal)
3. Artifacts uploaded for download from the Actions run

Pushing a version tag creates a GitHub Release with the binaries:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Configuration

Settings are stored under the user config directory:

- Linux: `~/.config/gotify-desktop/config.json`
- macOS: `~/Library/Application Support/gotify-desktop/config.json`
- Windows: `%AppData%\gotify-desktop\config.json`

Fields: `server_url`, `client_token`, `min_priority` (default `0`), `last_message_id`.

## License

MIT — see [LICENSE](LICENSE).
