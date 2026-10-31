#!/bin/bash
#
# Upgrade check: a brain written by an earlier release, opened by this tree.
#
#   scripts/upgrade-check.sh [old-tag]        (default v0.8.1)
#
# 1. Builds the old release and this tree (CGO, as released).
# 2. Lets the old program create a sealed brain in a scratch folder — its own
#    HOME, settings folder and key file, no display, no D-Bus, no microphone,
#    no sound, no Ollama — then fills every kind of record through the old
#    store (scripts/upgrade-check/seed.go.txt).
# 3. Starts this tree's program on that brain (with the old run's marker still
#    there, as after a crash) and asks its API for the records.
# 4. Reads every record back through this tree's store, checks the schema is at
#    the newest migration and that no seeded word is in the clear on disk
#    (scripts/upgrade-check/verify.go.txt).
#
# Never touches a real brain: everything lives under a new temporary folder,
# both helpers refuse a root outside it, and the servers are stopped by pid.
# The seed uses the old store's API, so a release that changes it needs its
# own copy of seed.go.txt.
set -euo pipefail

OLD="${1:-v0.8.1}"
HERE="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/pnsa-upgrade-check.XXXXXX")/scratchpad"
PORT="${UPGRADE_CHECK_PORT:-8851}"

mkdir -p "$WORK"/{old,new,bin,home,config,DATA,pathbin}
echo "→ scratch: $WORK"

git -C "$HERE" archive "$OLD" | tar -x -C "$WORK/old"
git -C "$HERE" archive HEAD | tar -x -C "$WORK/new"

mkdir -p "$WORK/old/clients/internal/zzseed" "$WORK/new/clients/internal/zzverify"
cp "$HERE/scripts/upgrade-check/seed.go.txt" "$WORK/old/clients/internal/zzseed/main.go"
cp "$HERE/scripts/upgrade-check/verify.go.txt" "$WORK/new/clients/internal/zzverify/main.go"
printf 'package store\n\nfunc Migrations() int { return len(migrations) }\n' > "$WORK/new/clients/internal/brain/store/zz_export.go"

echo "→ building $OLD and this tree"
(cd "$WORK/old/clients" && CGO_ENABLED=1 go build -o "$WORK/bin/old" ./cmd/pn-scripts-assistant && CGO_ENABLED=1 go build -o "$WORK/bin/seed" ./internal/zzseed)
(cd "$WORK/new/clients" && CGO_ENABLED=1 go build -o "$WORK/bin/new" ./cmd/pn-scripts-assistant && CGO_ENABLED=1 go build -o "$WORK/bin/verify" ./internal/zzverify)

echo '{"id":"upgrade-check","schema":1}' > "$WORK/DATA/.brain-root.json"
for b in sh git node; do p="$(command -v "$b" || true)"; [ -n "$p" ] && ln -sf "$p" "$WORK/pathbin/$b"; done

run_env() {
    env -i HOME="$WORK/home" XDG_CONFIG_HOME="$WORK/config" XDG_DATA_HOME="$WORK/home/.local/share" \
        XDG_CACHE_HOME="$WORK/home/.cache" PN_SCRIPTS_ASSISTANT_DATA_ROOT="$WORK/DATA" \
        PN_SCRIPTS_ASSISTANT_KEYRING=file PN_SCRIPTS_ASSISTANT_SILENT=1 BRAIN_CANCEL_ROOM=false \
        BRAIN_ALWAYS_LISTEN=false BRAIN_ALWAYS_SPEAK=false BRAIN_SETUP_DONE=true \
        OLLAMA_BASE_URL=http://127.0.0.1:9 BRAIN_ADDR="127.0.0.1:$PORT" PATH="$WORK/pathbin" \
        DBUS_SESSION_BUS_ADDRESS=disabled: "$@"
}

serve() {
    run_env /usr/bin/setsid "$1" serve > "$2" 2>&1 < /dev/null &

    for _ in $(seq 1 60); do
        curl -s -m 2 "http://127.0.0.1:$PORT/health" > /dev/null && break
        sleep 1
    done

    curl -s -m 2 "http://127.0.0.1:$PORT/health" > /dev/null || { echo "did not start:"; tail -20 "$2"; exit 1; }
}

stop() {
    local pid
    pid="$(ss -ltnpH "sport = :$PORT" | grep -o 'pid=[0-9]*' | head -1 | cut -d= -f2)"
    [ -n "$pid" ] && kill "$pid"

    for _ in $(seq 1 20); do
        ss -ltnH "sport = :$PORT" | grep -q . || return 0
        sleep 0.5
    done
}

echo "→ $OLD creates the brain"
serve "$WORK/bin/old" "$WORK/old.log"
stop

echo "→ $OLD fills it"
run_env "$WORK/bin/seed"

echo "→ this tree opens it"
serve "$WORK/bin/new" "$WORK/new.log"

missing=0
for route in conversations conversations/1 mail/drafts todos study research care books goals deeds; do
    if ! curl -s -m 5 "http://127.0.0.1:$PORT/api/$route" | grep -q ZZUPGRADEMARK; then
        echo "  FAIL  /api/$route does not show the seeded record"
        missing=$((missing + 1))
    else
        echo "  ok    /api/$route"
    fi
done
stop

echo "→ reading every record back"
run_env "$WORK/bin/verify"

[ "$missing" -eq 0 ] || { echo "$missing routes failed"; exit 1; }

echo "→ upgrade from $OLD: OK (scratch kept in $WORK)"
