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
