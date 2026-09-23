# Installing it

Ubuntu 24.04 or newer, 64-bit. Nothing else has to be installed first — `apt`
brings in what the window needs.

## From the package

```bash
sudo apt install ./pn-scripts-assistant_0.1.0_amd64.deb
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

## What it needs, and when

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

## Building it instead

```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev dpkg-dev fakeroot lintian desktop-file-utils
make release
```

The package lands in `build/packages` with its `SHA256SUMS`. See
[releasing.md](releasing.md) for what `make release` does, step by step.

## Updating

A newer package installs over the older one:

```bash
sudo apt install ./pn-scripts-assistant_<newer>_amd64.deb
```

Nothing that was learned is touched: the brain lives in your home directory (or
on whichever drive you moved it to), and the package only replaces the program.
The database migrates itself on first start, forwards only.

## Removing it

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
