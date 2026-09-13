#!/usr/bin/env bash
#
# Builds the thing somebody downloads, for each system they might be on.
#
#   Ubuntu / Debian   a .deb, so it installs and updates like anything else
#   macOS             a .app bundle, so it is dragged to Applications
#   Windows           a folder with the .exe, so it runs from anywhere
#
# One script rather than three, because the differences between these are
# almost entirely in the wrapping: the same Go binary is inside all of them.
# Keeping them together is what stops one of them quietly rotting — a packaging
# script nobody runs is a packaging script that has already broken.
#
# What deliberately stays outside every package: Ollama and the language model.
# Ollama has its own installer and update cycle, and the model is several
# gigabytes that most people will either already have or will want to choose
# for themselves. Bundling either would double the download and go stale
# immediately, so setup checks for them on first run and says what is missing.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/build/packages"
# ---------------------------------------------------------------------------
# What to call this build
# ---------------------------------------------------------------------------
#
# A tag when there is one, and something derived when there is not. It cannot
# simply be the commit hash: dpkg refuses a version that does not begin with a
# digit, and more importantly a hash does not sort — so "upgrade" and
# "downgrade" become meaningless to the package manager and it will happily
# install an older build over a newer one.
#
# The date sorts, and the hash after it says exactly which build it was.
derive_version() {
    if [ -n "${VERSION:-}" ]; then
        printf '%s' "${VERSION#v}"

        return
    fi

    local tag
    tag="$(git -C "$ROOT" describe --tags --exact-match 2>/dev/null || true)"

    if [ -n "$tag" ]; then
        printf '%s' "${tag#v}"

        return
    fi

    local when hash
    when="$(git -C "$ROOT" log -1 --format=%cd --date=format:%Y%m%d 2>/dev/null || date +%Y%m%d)"
    hash="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"

    printf '0.1.0~git%s.%s' "$when" "$hash"
}

VERSION="$(derive_version)"

log()  { printf '\033[36m→\033[0m %s\n' "$1"; }
warn() { printf '\033[33m!\033[0m %s\n' "$1"; }
die()  { printf '\033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }

want="${1:-all}"

rm -rf "$OUT"
mkdir -p "$OUT"

# ---------------------------------------------------------------------------
# Ubuntu and Debian
# ---------------------------------------------------------------------------
#
# Built with cgo so the native window is real. That is the one package where
# the window exists, and a .deb can declare the libraries it needs rather than
# discovering them missing at runtime — which is the entire advantage of using
# the system's own package manager, so it is worth using properly.
build_deb() {
    command -v dpkg-deb >/dev/null || { warn "dpkg-deb missing; skipping the .deb"; return; }

    local stage="$ROOT/build/deb/pn-scripts-assistant_${VERSION}_amd64"

    log "Ubuntu/Debian package $VERSION"

    if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.1 2>/dev/null; then
        warn "libwebkit2gtk-4.1-dev and libgtk-3-dev are missing."
        warn "The .deb would have no native window. Install them and run again:"
        warn "  sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev"

        return
    fi

    rm -rf "$stage"
    mkdir -p "$stage/DEBIAN" "$stage/usr/bin" "$stage/usr/share/applications"

    for size in 48 64 128 256 512; do
        mkdir -p "$stage/usr/share/icons/hicolor/${size}x${size}/apps"

        local icon="$ROOT/clients/internal/brain/desktop/icons/pn-scripts-assistant-${size}.png"

        [ -f "$icon" ] && install -m 644 "$icon" \
            "$stage/usr/share/icons/hicolor/${size}x${size}/apps/pn-scripts-assistant.png"
    done

    CGO_ENABLED=1 go build -C "$ROOT/clients" -trimpath -ldflags="-s -w" \
        -o "$stage/usr/bin/pn-scripts-assistant" ./cmd/pn-scripts-assistant || die "build failed"

    chmod 755 "$stage/usr/bin/pn-scripts-assistant"

    # Depends rather than Recommends for the window libraries: without them the
    # program runs but cannot open its own window, and somebody who installed a
    # desktop application has not asked for a thing that only serves a port.
    cat > "$stage/DEBIAN/control" <<CONTROL
Package: pn-scripts-assistant
Version: $VERSION
Section: utils
Priority: optional
Architecture: amd64
Depends: libwebkit2gtk-4.1-0, libgtk-3-0t64 | libgtk-3-0
Recommends: xdotool, espeak-ng
Maintainer: PN Scripts Assistant <noreply@localhost>
Description: A private assistant that runs entirely on this machine
 PN Scripts Assistant is a personal, self-learning assistant. It keeps everything it
 learns in one folder on your own computer, runs its language model locally,
 and by default sends nothing anywhere.
 .
 Ollama and a language model are not included: setup checks for them on first
 run and installs them for you.
CONTROL

    cat > "$stage/usr/share/applications/pn-scripts-assistant.desktop" <<'DESKTOP'
[Desktop Entry]
Type=Application
Version=1.0
Name=PN Scripts Assistant
GenericName=AI Assistant
Comment=A private assistant that runs entirely on this machine
Exec=pn-scripts-assistant
Icon=pn-scripts-assistant
Terminal=false
Categories=Utility;
Keywords=assistant;brain;voice;memory;
StartupNotify=true
StartupWMClass=pn-scripts-assistant
Actions=setup;

[Desktop Action setup]
Name=Set up again
Exec=pn-scripts-assistant setup
DESKTOP

    fakeroot dpkg-deb --build "$stage" "$OUT/pn-scripts-assistant_${VERSION}_amd64.deb" >/dev/null \
        || die "dpkg-deb failed"

    log "  $OUT/pn-scripts-assistant_${VERSION}_amd64.deb"
}

# ---------------------------------------------------------------------------
# What goes in the box beside the program
# ---------------------------------------------------------------------------
#
# Neither the macOS bundle nor the Windows executable is signed, and both
# systems react to that with a dialog that reads like a virus warning. Somebody
# meeting it with no explanation reasonably concludes the download is unsafe
# and deletes it, so the explanation ships in the package rather than living on
# a website they would have to find.
prepare_notes() {
    mkdir -p "$ROOT/build/macos" "$ROOT/build/windows"

    cat > "$ROOT/build/macos/README.txt" <<'MACNOTE'
PN Scripts Assistant for macOS
==================

Drag "PN Scripts Assistant.app" to your Applications folder, then open it.

The first time, macOS will refuse
---------------------------------
It will say the app "cannot be opened because it is from an unidentified
developer". That is not a fault and not a warning about this program in
particular: it means the app has not been signed with an Apple Developer
certificate, which costs money and identifies a company.

To open it anyway: right-click (or Control-click) the app and choose Open,
then Open again in the dialog. You only do this once.

What happens when it starts
---------------------------
PN Scripts Assistant checks what your machine has and opens setup in your browser. It
will ask you to install Ollama, which it cannot install for you on macOS —
get it from https://ollama.com/download, then continue. Everything after
that, including the language model, it does itself.

There is no separate window on macOS yet. PN Scripts Assistant runs and shows its
interface in your browser.

Where your data goes
--------------------
Everything it learns lives in one folder, and setup lets you choose which.
Nothing is sent anywhere unless you turn that on. The language models are
kept by Ollama in ~/.ollama/models, which is separate from that folder.
MACNOTE

    cat > "$ROOT/build/windows/README.txt" <<'WINNOTE'
PN Scripts Assistant for Windows
====================

Put this folder anywhere you like and run pn-scripts-assistant.exe.

The first time, Windows will refuse
-----------------------------------
SmartScreen will say "Windows protected your PC". That is not a fault and
not a warning about this program in particular: it means the file has not
been signed with a code-signing certificate, which costs money and
identifies a company.

To run it anyway: click "More info", then "Run anyway". You only do this
once.

What happens when it starts
---------------------------
PN Scripts Assistant checks what your machine has and opens setup in your browser. It
will ask you to install Ollama, which it cannot install for you on Windows
-- get it from https://ollama.com/download, then continue. Everything after
that, including the language model, it does itself.

There is no separate window on Windows yet. PN Scripts Assistant runs and shows its
interface in your browser.

Where your data goes
--------------------
Everything it learns lives in one folder, and setup lets you choose which.
Nothing is sent anywhere unless you turn that on. The language models are
kept by Ollama in its own folder, which is separate from that one.
WINNOTE
}

# ---------------------------------------------------------------------------
# macOS
# ---------------------------------------------------------------------------
#
# A .app is a directory with a particular shape, so it can be assembled from
# here even though it can only be run over there. Both architectures, because
# an Intel-only build on Apple silicon runs under translation and an
# arm64-only one does not run at all on the older machines.
#
# Not signed and not notarised. That needs an Apple Developer account and the
# key belonging to whoever publishes this, so the first launch requires
# right-click → Open. Said plainly in the README beside it rather than left to
# be discovered as a scary dialog.
build_macos() {
    local arch
    for arch in amd64 arm64; do
        local app="$ROOT/build/macos/$arch/PN Scripts Assistant.app"

        log "macOS bundle ($arch)"

        rm -rf "$app"
        mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

        # CGO off: the window layer is Linux-only, so this serves its interface
        # and opens the system browser. Cross-compiling cgo would need an Apple
        # SDK on this machine and would still not add a window.
        GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 \
            go build -C "$ROOT/clients" -trimpath -ldflags="-s -w" \
            -o "$app/Contents/MacOS/pn-scripts-assistant" ./cmd/pn-scripts-assistant || die "darwin/$arch build failed"

        chmod 755 "$app/Contents/MacOS/pn-scripts-assistant"

        cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>PN Scripts Assistant</string>
  <key>CFBundleDisplayName</key><string>PN Scripts Assistant</string>
  <key>CFBundleIdentifier</key><string>com.pnscripts.pn-scripts-assistant</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>pn-scripts-assistant</string>
  <key>CFBundleIconFile</key><string>pn-scripts-assistant</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <!-- It serves its interface to a browser rather than drawing a window, so
       it has no dock icon of its own to manage. -->
  <key>LSBackgroundOnly</key><false/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

        [ -f "$ROOT/assets/pn-scripts-assistant.png" ] &&
            cp "$ROOT/assets/pn-scripts-assistant.png" "$app/Contents/Resources/pn-scripts-assistant.png"

        cp "$ROOT/build/macos/README.txt" "$ROOT/build/macos/$arch/" 2>/dev/null || true

        tar -czf "$OUT/pn-scripts-assistant-${VERSION}-macos-${arch}.tar.gz" \
            -C "$ROOT/build/macos/$arch" . || die "packaging darwin/$arch failed"

        log "  $OUT/pn-scripts-assistant-${VERSION}-macos-${arch}.tar.gz"
    done
}

# ---------------------------------------------------------------------------
# Windows
# ---------------------------------------------------------------------------
#
# A folder with the .exe in it, zipped. Not an installer: a real one wants NSIS
# or WiX and a code-signing certificate, and without the certificate an
# installer is more alarming than a plain executable, not less — it asks for
# more trust while offering the same unsigned binary.
build_windows() {
    local dir="$ROOT/build/windows/PN-Scripts-Assistant"

    log "Windows package"

    rm -rf "$dir"
    mkdir -p "$dir"

    # -H windowsgui: linked as a GUI program, so double-clicking it does not
    # put a black console window behind the browser on every single launch.
    # The cost is that stderr goes nowhere, which is why the one message that
    # must not be lost — setup could not be opened — is a message box instead.
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
        go build -C "$ROOT/clients" -trimpath -ldflags="-s -w -H windowsgui" \
        -o "$dir/pn-scripts-assistant.exe" ./cmd/pn-scripts-assistant || die "windows build failed"

    cp "$ROOT/build/windows/README.txt" "$dir/" 2>/dev/null || true

    ( cd "$ROOT/build/windows" && zip -qr "$OUT/pn-scripts-assistant-${VERSION}-windows-amd64.zip" "PN-Scripts-Assistant" ) \
        || die "zip failed"

    log "  $OUT/pn-scripts-assistant-${VERSION}-windows-amd64.zip"
}

case "$want" in
    deb)     build_deb ;;
    macos)   prepare_notes; build_macos ;;
    windows) prepare_notes; build_windows ;;
    all)     build_deb; prepare_notes; build_macos; build_windows ;;
    *)       die "unknown target: $want (deb, macos, windows, all)" ;;
esac

log "Done. Packages in $OUT"
ls -lh "$OUT" 2>/dev/null | tail -n +2 | awk '{printf "    %-52s %s\n", $9, $5}'
