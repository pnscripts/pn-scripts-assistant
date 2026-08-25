# pnexus-desktop (Go)

The cross-platform desktop app. Starts Pnexus if it isn't running, then opens it
in a chromeless window with its own profile and taskbar entry.

## Build

```bash
go build -o pnexus-desktop .
```

Cross-compiling needs no toolchain beyond Go itself:

```bash
GOOS=windows GOARCH=amd64 go build -o pnexus-desktop.exe .
GOOS=darwin  GOARCH=arm64 go build -o pnexus-desktop-mac .
```

All targets build to roughly 8MB.

## Why not Electron, Tauri, or a native webview

| Option | Blocker |
|---|---|
| NativePHP / Electron | Requires `illuminate/contracts ^10\|^11\|^12` — would force a **Laravel 13 downgrade**, plus ~150MB per platform |
| Tauri | Needs a Rust toolchain (~1GB) that isn't installed |
| `webview_go` / Wails | Needs `libwebkit2gtk-4.1-dev` + GTK3 headers installed as root |

Every desktop already has a Chromium-family browser, and all of them support
`--app=`, which opens a window with no tabs and no URL bar. Driving that from Go
gives a real app window with no new dependencies.

**The trade-off:** this needs a Chromium-family browser present. Guaranteed on
Windows (Edge ships with the OS), near-universal elsewhere. If none is found the
app says so rather than quietly opening a normal browser tab. Swapping in a true
native webview later means changing this one file — nothing else depends on how
the window is made.

## Usage

```bash
pnexus-desktop                  # start Pnexus if needed, open the window
pnexus-desktop --no-start       # fail instead of starting it
pnexus-desktop --url http://…   # point at a different instance
```

Also installed as a desktop entry, so "Pnexus" appears in the applications menu.
