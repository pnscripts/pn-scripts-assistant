#!/usr/bin/env bash
#
# Builds the single file a user downloads.
#
# The goal is that somebody can download one thing, run it, and never open a
# terminal or install a database. An AppImage is how Linux does that: one
# executable file, no installation, no package manager, no root.
#
# This used to assemble three binaries and a compiled PHP runtime, because the
# brain needed a container stack beside it. It now packages one Go binary that
# serves the interface, holds the memory in a SQLite file and opens the window
# itself.
#
# What deliberately stays outside: Ollama and the language model. Ollama is a
# system service with its own installer and update cycle, and the model is
# several gigabytes most users will already have or will want to choose.
# Bundling either would double the download and go stale immediately, so the app
# checks for them on first run and says what is missing.
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="$PROJECT_ROOT/build"
APPDIR="$BUILD_DIR/PN-Brain.AppDir"
OUTPUT="$BUILD_DIR/PN-Brain-x86_64.AppImage"

log()  { printf '\033[36m→\033[0m %s\n' "$1"; }
warn() { printf '\033[33m!\033[0m %s\n' "$1"; }
die()  { printf '\033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

# cgo is what gets the native window. Without the WebKit headers the binary
# still works — it serves the interface and says what to install — so a missing
# header is a warning rather than a failure. Shipping an AppImage that cannot
# open its own window would be worse, hence the loud notice.
CGO=1

if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.1 2>/dev/null; then
    warn "libwebkit2gtk-4.1-dev and libgtk-3-dev are missing."
    warn "Building without a native window; the app will serve its interface and say so."
    warn "  sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev"
    CGO=0
fi

log "Building the brain (cgo=$CGO)"
mkdir -p "$PROJECT_ROOT/dist"

CGO_ENABLED=$CGO go build -C "$PROJECT_ROOT/clients" \
    -trimpath -ldflags="-s -w" \
    -o "$PROJECT_ROOT/dist/pn-brain" ./cmd/brain \
    || die "Build failed."

# ---------------------------------------------------------------------------
# Lay out the AppDir
# ---------------------------------------------------------------------------

log "Assembling the application directory"
rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" \
         "$APPDIR/usr/share/icons/hicolor/256x256/apps"

install -m 755 "$PROJECT_ROOT/dist/pn-brain" "$APPDIR/usr/bin/pn-brain"

# The accelerated visual, if one has been exported.
#
# Optional in the strictest sense: the brain looks for it beside itself and
# draws the core in the page when it is not there. That keeps the download at
# seven megabytes for anyone who does not want a game engine in it, and lets
# this file be built on a machine that has never seen Godot.
if [ -f "$PROJECT_ROOT/dist/pn-brain-visuals" ]; then
    install -m 755 "$PROJECT_ROOT/dist/pn-brain-visuals" "$APPDIR/usr/bin/pn-brain-visuals"
    log "Including the accelerated visual"
fi

cat > "$APPDIR/pn-brain.desktop" <<'DESKTOP'
[Desktop Entry]
Type=Application
Version=1.0
Name=PN Brain
GenericName=AI Assistant
Comment=Personal self-learning AI assistant
Exec=pn-brain
Icon=pn-brain
Terminal=false
Categories=Utility;
Keywords=ai;assistant;brain;chat;
DESKTOP
cp "$APPDIR/pn-brain.desktop" "$APPDIR/usr/share/applications/"

# An AppImage without an icon shows as a blank square in every launcher and
# looks broken before it has run.
if command -v convert >/dev/null 2>&1; then
    convert -size 256x256 xc:'#070a0f' \
        -fill '#4dd0e1' -draw "circle 128,128 128,58" \
        -fill '#070a0f' -draw "circle 128,128 128,84" \
        "$APPDIR/pn-brain.png" 2>/dev/null || true
fi

if [ ! -f "$APPDIR/pn-brain.png" ]; then
    printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82' \
        > "$APPDIR/pn-brain.png"
fi
cp "$APPDIR/pn-brain.png" "$APPDIR/usr/share/icons/hicolor/256x256/apps/" 2>/dev/null || true

# ---------------------------------------------------------------------------
# AppRun: what happens on a double click
# ---------------------------------------------------------------------------

cat > "$APPDIR/AppRun" <<'APPRUN'
#!/usr/bin/env bash
# An AppImage is mounted read-only at a fresh path on every run, so nothing may
# be written inside it and no path within it survives. The brain finds its own
# data root by looking for a marker file, so there is nothing to configure here
# — it will locate an existing brain on an external drive, or create one under
# the user's home directory if there is none.
set -uo pipefail

HERE="$(dirname "$(readlink -f "${0}")")"
exec "$HERE/usr/bin/pn-brain" "$@"
APPRUN
chmod +x "$APPDIR/AppRun"

# ---------------------------------------------------------------------------
# Package
# ---------------------------------------------------------------------------

TOOL="$BUILD_DIR/appimagetool"

if [ ! -x "$TOOL" ]; then
    log "Fetching appimagetool"
    mkdir -p "$BUILD_DIR"
    curl -fsSL -o "$TOOL" \
        "https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage" \
        || die "Could not download appimagetool."
    chmod +x "$TOOL"
fi

# The default AppImage runtime dlopens libfuse.so.2, which Ubuntu has not
# installed by default since 22.04 — so the finished file fails to start with
# "error loading libfuse.so.2" on the very machines it is aimed at. Telling
# users to apt-install libfuse2 first defeats the entire point of shipping one
# self-contained file, so a statically linked runtime is used instead.
RUNTIME="$BUILD_DIR/runtime-x86_64"

if [ ! -f "$RUNTIME" ]; then
    log "Fetching the static (FUSE-less) runtime"
    curl -fsSL -o "$RUNTIME" \
        "https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-x86_64" \
        || die "Could not download the static runtime."
fi

log "Packaging"
# --appimage-extract-and-run runs appimagetool itself without FUSE; the
# --runtime-file is what removes FUSE from the file it produces.
ARCH=x86_64 "$TOOL" --appimage-extract-and-run \
    --runtime-file "$RUNTIME" \
    "$APPDIR" "$OUTPUT" >/dev/null 2>&1 \
    || die "appimagetool failed."

printf '\033[32m✓\033[0m %s (%s)\n' "$OUTPUT" "$(du -h "$OUTPUT" | cut -f1)"
echo
echo "  One file. Download it, make it executable, run it."

if [ "$CGO" = "0" ]; then
    echo
    warn "This build has no native window. Install the WebKit headers and rebuild"
    warn "for the real desktop app."
fi
