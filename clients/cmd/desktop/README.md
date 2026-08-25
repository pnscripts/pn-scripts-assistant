# pn-brain-desktop (Go)

The native desktop application. Starts PN Brain if it isn't running, then opens it
in a real application window drawn by the operating system's own web engine —
WebKitGTK on Linux. Not a browser: no tabs, no address bar, no Chrome.

## Build

Linux needs the web engine's development headers once:

```bash
sudo apt install -y libwebkit2gtk-4.1-dev libgtk-3-dev
```

Then:

```bash
CGO_ENABLED=1 go build -o pn-brain-desktop .
```

The runtime libraries (`libwebkit2gtk-4.1-0`, `libgtk-3-0`) ship with any GTK
desktop, so only building needs the headers — running does not.

## Why a hand-written binding

The window is about eighty lines of cgo in `window_linux.go` rather than a
library, because the maintained Go webview bindings still `pkg-config` against
`webkit2gtk-4.0`, and Ubuntu 24.04 removed 4.0 entirely — only 4.1 exists, so
they cannot build at all. PN Brain needs one window showing one URL, which is
short enough to own outright.

| Alternative | Why not |
|---|---|
| `webview_go` / Wails | pin webkit2gtk-4.0, unavailable on Ubuntu 24.04 |
| Electron / NativePHP | ~150MB per platform, and NativePHP needs Laravel ≤12 |
| Tauri | Rust toolchain (~1GB) |
| Chromium `--app=` | works, but it is a browser in a costume |

## Platform support

Linux is implemented. Windows (WebView2) and macOS (WKWebView) each need their
own native implementation; until then the binary builds there but exits with a
message pointing at the URL. It does **not** quietly fall back to launching a
browser — silently opening Chrome is precisely what this client exists to avoid.

cgo is the cost of a real window: it cannot be cross-compiled without a full
cross toolchain, so each platform builds on its own CI runner. The CLI, being
pure Go, still cross-compiles everywhere from one machine.

## Usage

```bash
pn-brain-desktop                  # start PN Brain if needed, open the window
pn-brain-desktop --no-start       # fail instead of starting it
pn-brain-desktop --url http://…   # point at a different instance
```

Also installed as a desktop entry, so "PN Brain" appears in the applications menu.
