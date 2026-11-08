# Installing it

Български: [installing.bg.md](installing.bg.md)

Two systems: Linux (Ubuntu and Debian) and Windows. Both get the assistant in
its own window (WebKitGTK on Linux, Microsoft Edge WebView2 on Windows).
Memory, typed conversation and most tools work the same on both. These parts
are Linux-only:

- **Listening.** Microphone capture uses PipeWire (`pw-record`) or ALSA
  (`arecord`).
- **Echo cancellation**, which keeps the machine's own sound out of what it
  hears. It routes audio through PipeWire.
- **Desktop control**: typing and clicking on your behalf goes through
  `xdotool`.
- **Confining project work by the kernel**, with Landlock. On Windows a command
  still gets a cleaned environment and no shell, but the folder limit is not
  enforced by the kernel — so project work does not run commands written into
  `cmd /c` or PowerShell there; it writes them to a script first.
- **Telling your voice from others, and opening full web pages.** Both need
  the cgo build, and the Windows build is made without cgo. Plain page
  fetching still works there.

| System | Download | What it does |
|---|---|---|
| Ubuntu / Debian | `pn-scripts-assistant_<version>_amd64.deb` | installs with `apt`, native window, menu entry |
| Windows 10 or newer, 64-bit | `...-windows-setup.exe` | installs for your account, Start menu entry, its own window |

Every file has its SHA-256 in the `SHA256SUMS` published beside it.

Nothing is signed: there is no Windows code-signing certificate behind this,
which costs money every year and identifies a company. Windows says so when
you first open it, and the way past it is written below. That is the honest
state of it rather than something to work around.

## On Ubuntu

Ubuntu 24.04 or newer, 64-bit. Nothing else has to be installed first — `apt`
brings in what the window needs.

### From the package

```bash
sudo apt install ./pn-scripts-assistant_*_amd64.deb
```

Check first, if you like, that the file is the one that was built
(`--ignore-missing` because `SHA256SUMS` lists every download and you have
one):

```bash
sha256sum --ignore-missing -c SHA256SUMS
```

What that puts on the machine, and nothing else:

| Path | What it is |
|---|---|
| `/usr/bin/pn-scripts-assistant` | the program, one file |
| `/usr/share/applications/pn-scripts-assistant.desktop` | the applications-menu entry |
| `/usr/share/icons/hicolor/*/apps/pn-scripts-assistant.png` | the icon, five sizes |
| `/usr/share/man/man1/pn-scripts-assistant.1.gz` | `man pn-scripts-assistant` |
| `/usr/share/doc/pn-scripts-assistant/` | the licence and the changelog |

It writes nothing into your home directory until you run it.

### What it needs, and when

The package depends on the GTK and WebKit libraries, so `apt` installs those
with it. Everything else is optional and checked on the first run:

- **[Ollama](https://ollama.com)** with a chat model and `nomic-embed-text` —
  without it the assistant starts, says it has no model, and offers to install
  one.
- **`espeak-ng`** or `speech-dispatcher` to read answers aloud.
- **whisper.cpp** (`whisper-cli` on the PATH) to listen.
- **`xdotool`** to type and click on your behalf.

`apt` lists the last two as *recommended*, so an ordinary `apt install` takes
them unless you said `--no-install-recommends`.

### Building it instead

```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev dpkg-dev fakeroot lintian desktop-file-utils
make release
```

The package lands in `build/packages` with its `SHA256SUMS`. See
[releasing.md](releasing.md) for what `make release` does, step by step.

### Updating

A newer package installs over the older one:

```bash
sudo apt install ./pn-scripts-assistant_<newer>_amd64.deb
```

Nothing that was learned is touched: the brain lives in your home directory (or
on whichever drive you moved it to), and the package only replaces the program.
The database migrates itself on first start, forwards only.

### Removing it

```bash
sudo apt remove pn-scripts-assistant
```

That takes away the program, the menu entry, the icons and the manual page, and
**leaves everything it learned**, deliberately: years of somebody's memory
should not disappear with an `apt remove`. When you do mean it:

```bash
pn-scripts-assistant status          # says where the brain is, first
rm -rf ~/.local/share/pn-scripts-assistant
rm -rf ~/.config/pn-scripts-assistant
rm -rf ~/.local/state/pn-scripts-assistant
```

If the brain was moved to another drive, `status` names that folder and the
first path above will not be it.

## On Windows

Windows 10 or newer, 64-bit. Two downloads, the same program inside:

- **`...-windows-setup.exe`** — the installer. It installs into your own
  account, so it never asks for an administrator password, puts the assistant
  in the Start menu, and can be removed again from *Settings → Apps*.
- **`...-windows-amd64.zip`** — the folder. Unzip it anywhere and run
  `pn-scripts-assistant.exe`. Nothing is installed.

SmartScreen will stop the first run with "Windows protected your PC". That is
the missing signature, not a verdict on the program: click **More info**, then
**Run anyway**. Once only.

It opens in its own window, drawn by Microsoft Edge WebView2 — part of
Windows 11, and already installed on almost every Windows 10. The window needs
`WebView2Loader.dll`, which is in both downloads beside the program. On a
machine without the WebView2 runtime the assistant opens in your browser
instead and works the same; installing the *Evergreen WebView2 Runtime* from
Microsoft's WebView2 download page gives it its window at the next start.
Links that open a new window (sources, downloads) open in your usual browser.

Listening, echo cancellation, desktop control and Landlock
confinement are Linux-only.

Ollama it cannot install for you on Windows: install it from
[ollama.com/download](https://ollama.com/download) first, and setup does
everything after that.

Removing the program leaves what it learned, in
`%USERPROFILE%\.local\share\pn-scripts-assistant` (the same place as on Linux,
under your user folder), for the same reason it does on Linux.
`pn-scripts-assistant status` names the folder. Delete it when you actually
mean it.

## Building the packages yourself

```bash
make packages
```

That builds the `.deb` and the Windows folder into `build/packages`.
(`bash scripts/build-packages.sh macos` still makes a macOS bundle for anybody
who wants to try one; it is not published, supported or tested.) The Windows *installer* is the one thing
it cannot produce here: Inno Setup only runs on Windows, so the script says so
and builds the rest. The release workflow builds each one on its own system.
