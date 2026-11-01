# Build Directory

The build directory houses Wails v3 build assets (icons, platform metadata, Taskfiles).
Compiled binaries are written to the project-root `bin/` directory.

The structure is:

* `Taskfile.yml` / `config.yml` - shared build tasks and product metadata
* `darwin` - macOS specific files
* `linux` - Linux build tasks
* `windows` - Windows specific files (including `nsis/`)

## Mac

The `darwin` directory holds files specific to Mac builds.
These may be customised and used as part of the build. To return these files to the default state, regenerate with
`wails3 generate build-assets` / `wails3 update build-assets`.

The directory contains the following files:

- `Info.plist` - the main plist file used for Mac builds (`wails3 build`).
- `Info.dev.plist` - same as the main plist file but used when building using `wails3 dev`.

## Windows

The `windows` directory contains the manifest and packaging files used with `wails3 build` / `wails3 package`.

- `icon.ico` - The icon used for the application. If missing, a new `icon.ico` is created from `appicon.png`.
- `nsis/*` - The files used to create the Windows NSIS installer (`wails3 package`).
- `info.json` - Application details used for Windows builds.
- `wails.exe.manifest` - The main application manifest file.
