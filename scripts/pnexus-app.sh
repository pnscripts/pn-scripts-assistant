#!/usr/bin/env bash
#
# Launcher: brings Pnexus up if it isn't already, waits for it to answer, then
# opens the UI. This is what the desktop entry runs, so starting the brain is a
# single click rather than a sequence of docker commands.
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
URL="http://localhost:8090"

notify() {
    command -v notify-send >/dev/null 2>&1 && notify-send "Pnexus" "$1" || echo "[pnexus] $1"
}

is_up() {
    curl -sf -o /dev/null --max-time 2 "$URL/api/brain"
}

if ! is_up; then
    notify "Starting up..."

    if ! "$PROJECT_ROOT/scripts/start-brain.sh" >/tmp/pnexus-start.log 2>&1; then
        notify "Failed to start. See /tmp/pnexus-start.log"
        exit 1
    fi

    # Postgres and the app container need a moment before the API answers.
    for _ in $(seq 1 60); do
        is_up && break
        sleep 1
    done
fi

if ! is_up; then
    notify "Started, but the API is not responding. See /tmp/pnexus-start.log"
    exit 1
fi

notify "Online."
xdg-open "$URL" >/dev/null 2>&1 &
