#!/usr/bin/env bash
#
# One click / one command to start the global Brain.
#
# Rebuilds from this tree when the source is newer than the binary, then opens
# the window. If the brain is already running, it just brings the page up.
# Double-clicking the desktop icon lands here; so does `pn-scripts-assistant` on PATH.
set -euo pipefail

SCRIPT="$(readlink -f "${BASH_SOURCE[0]}")"
PROJECT_ROOT="$(cd "$(dirname "$SCRIPT")/.." && pwd)"
BIN="$PROJECT_ROOT/dist/pn-scripts-assistant"
CLIENTS="$PROJECT_ROOT/clients"
URL="http://127.0.0.1:8790"

export PATH="/usr/local/go/bin:$HOME/.local/bin:$PATH"

say() { printf '\033[36m→\033[0m %s\n' "$1"; }
fail() {
    printf '\033[31m✗\033[0m %s\n' "$1" >&2
    if [[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]]; then
        command -v notify-send >/dev/null && notify-send --urgency=critical "PN Scripts Assistant" "$1" || true
    fi
    exit 1
}

already_running() {
    command -v curl >/dev/null 2>&1 && curl -fsS --max-time 0.4 "$URL" >/dev/null 2>&1
}

needs_rebuild() {
    [[ -x "$BIN" ]] || return 0
    # Any newer Go or embedded asset means the running binary is stale.
    if find "$CLIENTS" \( -name '*.go' -o -name '*.js' -o -name '*.css' -o -name '*.html' \) \
        -newer "$BIN" -print -quit | grep -q .; then
        return 0
    fi
    return 1
}

rebuild() {
    command -v go >/dev/null 2>&1 || fail "Go is not on PATH. Install it, or add /usr/local/go/bin."
    mkdir -p "$PROJECT_ROOT/dist"

    local cgo=1
    if ! command -v pkg-config >/dev/null 2>&1 || ! pkg-config --exists gtk+-3.0 webkit2gtk-4.1 2>/dev/null; then
        cgo=0
        say "Building without a native window (WebKit headers missing)."
    fi

    say "Building the brain (cgo=$cgo)"
    CGO_ENABLED="$cgo" go build -C "$CLIENTS" \
        -trimpath \
        -o "$BIN" ./cmd/pn-scripts-assistant \
        || fail "Build failed. Run it from a terminal to see the compiler error."
}

install_shortcuts() {
    mkdir -p "$HOME/.local/bin" "$HOME/.local/share/applications"

    # The shortcuts from when the program was PN Brain point at a launcher that
    # no longer exists under that name. Removed only when they are this
    # project's own: a pn-brain command somebody made themselves is theirs.
    local old_link="$HOME/.local/bin/pn-brain"
    if [[ -L "$old_link" && "$(readlink "$old_link")" == "$PROJECT_ROOT/"* ]]; then
        rm -f "$old_link"
    fi

    local old_desktop="$HOME/.local/share/applications/pn-brain.desktop"
    if [[ -f "$old_desktop" ]] && grep -q "Exec=$PROJECT_ROOT/" "$old_desktop"; then
        rm -f "$old_desktop"
    fi
    ln -sfn "$PROJECT_ROOT/scripts/pn-scripts-assistant-launch.sh" "$HOME/.local/bin/pn-scripts-assistant"

    local desktop="$HOME/.local/share/applications/pn-scripts-assistant.desktop"
    cat > "$desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Version=1.0
Name=PN Scripts Assistant
GenericName=AI Assistant
Comment=Personal self-learning AI assistant
Exec=$PROJECT_ROOT/scripts/pn-scripts-assistant-launch.sh
Icon=applications-science
Terminal=false
StartupNotify=true
Categories=Utility;
Keywords=ai;assistant;brain;chat;pnscripts;
DESKTOP
    chmod +x "$desktop"
}

if already_running; then
    say "Already running at $URL"
    if [[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]]; then
        command -v notify-send >/dev/null && notify-send "PN Scripts Assistant" "Already running — opening $URL" || true
        command -v xdg-open >/dev/null && xdg-open "$URL" >/dev/null 2>&1 || true
    fi
    exit 0
fi

if needs_rebuild; then
    rebuild
else
    say "Binary is current"
fi

install_shortcuts

[[ -x "$BIN" ]] || fail "No binary at $BIN"

# Remaining args go to the brain itself: `pn-scripts-assistant serve`, `pn-scripts-assistant status`, …
exec "$BIN" "$@"
