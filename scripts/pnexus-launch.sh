#!/usr/bin/env bash
#
# What the applications menu runs. Everything it does is graphical: dialogs via
# zenity, password prompts via pkexec. No terminal at any point.
#
# It exists to break a circular dependency. The desktop application draws its
# own window using the system web engine, so it cannot be built until that
# engine's development headers are installed — and asking someone to install
# them from a shell is exactly what this project is trying to avoid. This script
# needs neither: zenity and pkexec are part of the desktop session already.
#
# The terminal path remains for anyone who prefers it (pnexus-doctor --install),
# but it is an option, not the route.
set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCTOR="$PROJECT_ROOT/dist/pnexus-doctor"
DESKTOP_BIN="$PROJECT_ROOT/dist/pnexus-desktop"
CLIENTS_DIR="$PROJECT_ROOT/clients"

have() { command -v "$1" >/dev/null 2>&1; }

# Falls back to the terminal only when there is genuinely no desktop session to
# draw a dialog on — a server, or SSH.
graphical() { have zenity && [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; }

say_error() {
    if graphical; then
        zenity --error --title="Pnexus" --width=420 --text="$1"
    else
        echo "[pnexus] error: $1" >&2
    fi
}

ask() {
    if graphical; then
        zenity --question --title="Pnexus" --width=440 --text="$1"
    else
        echo "[pnexus] $1 [y/N]"
        read -r reply
        [ "$reply" = "y" ] || [ "$reply" = "Y" ]
    fi
}

# Runs a long job behind an indeterminate progress dialog. The job's output is
# discarded from view deliberately: a wall of apt or compiler output in a
# progress window tells a non-technical person nothing, and the log file is
# there for when it fails.
with_progress() {
    local message="$1"
    shift

    if ! graphical; then
        echo "[pnexus] $message"
        "$@" >>"$LOG" 2>&1
        return $?
    fi

    (
        "$@" >>"$LOG" 2>&1
        echo $? >"$STATUS_FILE"
    ) &
    local job=$!

    (
        while kill -0 "$job" 2>/dev/null; do
            echo "#$message"
            sleep 0.5
        done
    ) | zenity --progress --pulsate --auto-close --no-cancel \
               --title="Pnexus" --width=420 --text="$message"

    wait "$job"

    return "$(cat "$STATUS_FILE" 2>/dev/null || echo 1)"
}

LOG="${TMPDIR:-/tmp}/pnexus-setup.log"
STATUS_FILE="${TMPDIR:-/tmp}/pnexus-setup.status"
: >"$LOG"

# ---------------------------------------------------------------------------
# 1. The doctor itself is a Go binary; without Go we cannot even check.
# ---------------------------------------------------------------------------

if [ ! -x "$DOCTOR" ]; then
    if ! have go; then
        say_error "Pnexus needs the Go toolchain to build its own tools.\n\nInstall Go, then start Pnexus again."
        exit 1
    fi

    with_progress "Preparing Pnexus…" \
        env -C "$CLIENTS_DIR" go build -o "$PROJECT_ROOT/dist/pnexus-doctor" ./cmd/doctor \
        || { say_error "Could not prepare Pnexus.\n\nDetails: $LOG"; exit 1; }
fi

# ---------------------------------------------------------------------------
# 2. Check requirements, and offer to install what is missing — graphically.
# ---------------------------------------------------------------------------

if ! "$DOCTOR" --quiet >>"$LOG" 2>&1; then
    summary="$("$DOCTOR" 2>&1 | sed -e 's/\x1b\[[0-9;]*m//g')"

    if ! ask "Pnexus needs a few things from this computer.\n\n$summary\n\nInstall them now?"; then
        exit 0
    fi

    # pkexec gives a graphical password prompt; the doctor picks it over sudo
    # automatically when there is no terminal to type into.
    with_progress "Installing what Pnexus needs…" "$DOCTOR" --install --yes \
        || { say_error "Some things could not be installed.\n\nDetails: $LOG"; exit 1; }
fi

# ---------------------------------------------------------------------------
# 3. Build the desktop app now that the web engine headers exist.
# ---------------------------------------------------------------------------

needs_build=false
[ -x "$DESKTOP_BIN" ] || needs_build=true

if [ "$needs_build" = true ]; then
    if ! have go; then
        say_error "Pnexus needs the Go toolchain to build its window.\n\nInstall Go, then start Pnexus again."
        exit 1
    fi

    # The web engine headers are marked optional overall — someone running a
    # prebuilt binary genuinely does not need them — but building the window
    # here does. Checking the general case would pass and then fail at the
    # compiler, so ask specifically.
    if ! pkg-config --exists webkit2gtk-4.1 gtk+-3.0 2>/dev/null; then
        if ! ask "Pnexus needs the system web engine to draw its window.\n\nInstall it now? This asks for your password."; then
            exit 0
        fi

        with_progress "Installing the web engine…" \
            pkexec apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev \
            || { say_error "Could not install the web engine.\n\nDetails: $LOG"; exit 1; }
    fi

    with_progress "Building the Pnexus window…" \
        env -C "$CLIENTS_DIR" CGO_ENABLED=1 go build -o "$DESKTOP_BIN" ./cmd/desktop \
        || {
            say_error "Could not build the Pnexus window.\n\nDetails: $LOG"
            exit 1
        }
fi

# ---------------------------------------------------------------------------
# 4. Hand over. From here the application takes care of itself, including
#    first-run setup, which it shows in its own window.
# ---------------------------------------------------------------------------

exec "$DESKTOP_BIN" "$@"
