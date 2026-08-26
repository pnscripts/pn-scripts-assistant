#!/usr/bin/env bash
#
# Builds the single file a user downloads.
#
# The goal is that someone can download one thing, run it, and never open a
# terminal or install a database. An AppImage is how Linux does that: one
# executable file, no installation, no package manager, no root.
#
# What ends up inside:
#   pn-brain-brain     the whole Laravel application, PHP and a web server,
#                      compiled into one static binary (see static-build.Dockerfile)
#   pn-brain-desktop   the native window
#   pn-brain-doctor    the requirement checker the setup screen drives
#
# What deliberately stays outside: Ollama and the language model. Ollama is a
# system service with its own installer and update cycle, and the model is
# several gigabytes that most users will already have or will want to choose.
# Bundling either would double the download and go stale immediately, so the app
# installs them on first run instead, with the user watching and consenting.
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="$PROJECT_ROOT/build"
APPDIR="$BUILD_DIR/PN-Brain.AppDir"
OUTPUT="$BUILD_DIR/PN-Brain-x86_64.AppImage"

log() { printf '\033[36m→\033[0m %s\n' "$1"; }
die() { printf '\033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Ingredients
# ---------------------------------------------------------------------------

BRAIN_BIN="$PROJECT_ROOT/dist/pn-brain-brain"

if [ ! -x "$BRAIN_BIN" ]; then
    die "The brain binary is missing.

Build it first:
  docker build -f static-build.Dockerfile -t pn-brain-static .
  id=\$(docker create pn-brain-static)
  docker cp \"\$id:/go/src/app/dist/frankenphp-linux-x86_64\" dist/pn-brain-brain
  docker rm \"\$id\""
fi

log "Building the Go clients"
(
    cd "$PROJECT_ROOT/clients"
    CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o "$PROJECT_ROOT/dist/pn-brain-desktop" ./cmd/desktop
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$PROJECT_ROOT/dist/pn-brain-doctor" ./cmd/doctor
) || die "Client build failed. The window needs libwebkit2gtk-4.1-dev and libgtk-3-dev to compile."

# ---------------------------------------------------------------------------
# Lay out the AppDir
# ---------------------------------------------------------------------------

log "Assembling the application directory"
rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/256x256/apps"

install -m 755 "$BRAIN_BIN" "$APPDIR/usr/bin/pn-brain-brain"
install -m 755 "$PROJECT_ROOT/dist/pn-brain-desktop" "$APPDIR/usr/bin/pn-brain-desktop"
install -m 755 "$PROJECT_ROOT/dist/pn-brain-doctor" "$APPDIR/usr/bin/pn-brain-doctor"

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

# A plain generated icon rather than a missing one: an AppImage without an icon
# shows as a blank square in every launcher and looks broken before it has run.
if command -v convert >/dev/null 2>&1; then
    convert -size 256x256 xc:'#070a0f' \
        -fill '#4dd0e1' -draw "circle 128,128 128,58" \
        -fill '#070a0f' -draw "circle 128,128 128,84" \
        "$APPDIR/pn-brain.png" 2>/dev/null || true
fi

if [ ! -f "$APPDIR/pn-brain.png" ]; then
    # A 1x1 PNG is enough to satisfy the format; better a dull icon than none.
    printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82' \
        > "$APPDIR/pn-brain.png"
fi
cp "$APPDIR/pn-brain.png" "$APPDIR/usr/share/icons/hicolor/256x256/apps/" 2>/dev/null || true

# ---------------------------------------------------------------------------
# AppRun: what actually happens on a double click
# ---------------------------------------------------------------------------

cat > "$APPDIR/AppRun" <<'APPRUN'
#!/usr/bin/env bash
# Entry point for the AppImage.
#
# An AppImage is mounted read-only at a fresh path on every run, so nothing may
# be written inside it and no path within it survives. Data therefore lives in
# the user's own directory, and the brain binary is told where to find it.
set -uo pipefail

HERE="$(dirname "$(readlink -f "${0}")")"
export PATH="$HERE/usr/bin:$PATH"

DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/pn-brain"
mkdir -p "$DATA_HOME/storage/framework/views" \
         "$DATA_HOME/storage/framework/cache/data" \
         "$DATA_HOME/storage/framework/sessions" \
         "$DATA_HOME/storage/logs" \
         "$DATA_HOME/storage/app"

export PN_BRAIN_ROOT="$DATA_HOME"
export PN_BRAIN_STORAGE_PATH="$DATA_HOME/storage"
export PN_BRAIN_BRAIN_BIN="$HERE/usr/bin/pn-brain-brain"

# First run: write a configuration the user can later edit by hand, and give the
# application a key. Without a key Laravel refuses to serve anything.
if [ ! -f "$DATA_HOME/.env" ]; then
    cat > "$DATA_HOME/.env" <<ENV
APP_NAME="PN Brain"
APP_ENV=production
APP_DEBUG=false
APP_URL=http://127.0.0.1:8790

DB_CONNECTION=sqlite
DB_DATABASE=$DATA_HOME/pn-brain.sqlite

PN_BRAIN_STORAGE_PATH=$DATA_HOME/storage
QUEUE_CONNECTION=database
CACHE_STORE=database
SESSION_DRIVER=database

BRAIN_PRIVACY=private
BRAIN_NAME="PN Brain"
BRAIN_OWNER=

LLM_DEFAULT_PROVIDER=ollama
OLLAMA_BASE_URL=http://127.0.0.1:11434
OLLAMA_DEFAULT_MODEL=qwen2.5-coder:7b
EMBEDDING_MODEL=nomic-embed-text
ANTHROPIC_API_KEY=
ENV
    chmod 600 "$DATA_HOME/.env"
    touch "$DATA_HOME/pn-brain.sqlite"
    "$HERE/usr/bin/pn-brain-brain" php-cli artisan key:generate --force >/dev/null 2>&1 || true
fi

# Migrations run every launch. They are idempotent, and doing it here means an
# update never needs the user to be told to run anything.
"$HERE/usr/bin/pn-brain-brain" php-cli artisan migrate --force >/dev/null 2>&1 || true

exec "$HERE/usr/bin/pn-brain-desktop" "$@"
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
        || die "Could not download appimagetool"
    chmod +x "$TOOL"
fi

log "Packaging"
# --appimage-extract-and-run avoids needing FUSE, which is absent in containers
# and on some desktops.
ARCH=x86_64 "$TOOL" --appimage-extract-and-run "$APPDIR" "$OUTPUT" >/dev/null 2>&1 \
    || die "appimagetool failed"

printf '\033[32m✓\033[0m %s (%s)\n' "$OUTPUT" "$(du -h "$OUTPUT" | cut -f1)"
echo
echo "  One file. Users download it, make it executable, and run it."
echo "  Ollama and the model are installed by the app on first run."
