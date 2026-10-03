# Installing it

Three systems, one program, but not the same on each. The native window is
Linux-only, so on macOS and Windows the assistant runs and shows its interface
in your browser. Memory, typed conversation and most tools work the same
everywhere. These parts are Linux-only:

- **Listening.** Microphone capture uses PipeWire (`pw-record`) or ALSA
  (`arecord`).
- **Echo cancellation**, which keeps the machine's own sound out of what it
  hears. It routes audio through PipeWire.
- **Desktop control**: typing and clicking on your behalf goes through
  `xdotool`.
- **Confining project work by the kernel**, with Landlock. On macOS and Windows
  a command still gets a cleaned environment and no shell, but the folder limit
  is not enforced by the kernel.
- **Telling your voice from others, and opening full web pages.** Both need
  the cgo build, and the macOS and Windows builds are made without cgo. Plain
  page fetching still works there.

| System | Download | What it does |
|---|---|---|
| Ubuntu / Debian | `pn-scripts-assistant_<version>_amd64.deb` | installs with `apt`, native window, menu entry |
| macOS 11 or newer | `...-macos-<arch>.dmg` | drag to Applications; opens in your browser |
| Windows 10 or newer, 64-bit | `...-windows-setup.exe` | installs for your account; opens in your browser |

Every file has its SHA-256 in the `SHA256SUMS` published beside it.

Nothing on any of them is signed: there is no Apple Developer account and no
Windows code-signing certificate behind this, both of which cost money every
year and identify a company. Each system says so in its own way when you first
open it, and the way past it is written below. That is the honest state of it
rather than something to work around.

## On Ubuntu

Ubuntu 24.04 or newer, 64-bit. Nothing else has to be installed first — `apt`
brings in what the window needs.

### From the package

```bash
sudo apt install ./pn-scripts-assistant_*_amd64.deb
```

Check first, if you like, that the file is the one that was built:

```bash
sha256sum -c SHA256SUMS
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

## On macOS

macOS 11 or newer, Intel or Apple silicon — take the `.dmg` whose name matches
(`amd64` for Intel, `arm64` for an M-series Mac). Open it and drag **PN Scripts
Assistant** onto the Applications shortcut beside it.

The first open will be refused: macOS says the app "cannot be opened because it
is from an unidentified developer", which means it is not signed, not that
anything is wrong with it. Right-click (or Control-click) the app in
Applications and choose **Open**, then **Open** again in the dialog. Once only —
after that it opens normally.

There is no separate window on macOS. It starts, opens your browser at its own
address, and everything happens there. Listening, echo cancellation, desktop
control and Landlock confinement are not available on a Mac; see the list at
the top of this page.

Ollama it cannot install for you on a Mac: get it from
[ollama.com/download](https://ollama.com/download) and then carry on with
setup, which does the rest including the model.

To remove it: drag the app to the Bin. What it learned stays in
`~/Library/Application Support/pn-scripts-assistant` — deliberately, the same
as everywhere else — and `rm -rf` on that folder is the deliberate way to take
it away too.

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

There is no separate window on Windows either; it opens your browser. As on a
Mac, listening, echo cancellation, desktop control and Landlock confinement are
Linux-only.

Ollama is the same as on a Mac: install it from
[ollama.com/download](https://ollama.com/download) first, and setup does
everything after that.

Removing the program leaves what it learned, in
`%LOCALAPPDATA%\pn-scripts-assistant`, for the same reason it does on Linux.
Delete that folder when you actually mean it.

## Building the packages yourself

```bash
make packages
```

That builds the `.deb`, both macOS bundles and their disk images, and the
Windows folder, into `build/packages`. The Windows *installer* is the one thing
it cannot produce here: Inno Setup only runs on Windows, so the script says so
and builds the rest. The release workflow builds each one on its own system.
