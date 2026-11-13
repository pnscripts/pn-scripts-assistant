#!/usr/bin/env bash
#
# load-check.sh — what the assistant costs the machine while nobody uses it.
#
#   scripts/load-check.sh [program] [budget-resting-%] [budget-hidden-%]
#
# The CPU budgets depend on the machine; the third check does not: while the
# page rests, WebKit's network process (every request goes through it) makes
# almost no system calls (LOAD_CHECK_CALLS, default 200 in 30 s; one poller
# alone makes thousands).
#
# Opens the real window (WebKitGTK) on a scratch brain, on its own virtual
# display, leaves it untouched until the page rests (pace.js: a minute), then
# measures the program, its page and network processes AND the display server
# for 30 seconds; then unmaps the window, as minimising does, and measures
# again. Fails when either total is over its budget, in percent of one core.
#
# Measured 08.11.2026 on four cores without a graphics card: before pace.js
# the idle window cost 45 % of a core plus 86 % in the display server; resting
# it costs about 2 % and 1 %, hidden about 0.5 % and nothing. The budgets
# leave room for a noisy CI runner and still catch any poller or endless
# animation that stops resting.
#
# Touches nothing outside its scratch folder: own HOME and XDG folders, a
# key file instead of the keyring, no sound, no microphone, no model, no
# D-Bus. Everything it starts is stopped by pid. Needs Xvfb and xdotool.
set -euo pipefail

PROGRAM="${1:-build/pn-scripts-assistant}"
BUDGET_RESTING="${2:-6}"
BUDGET_HIDDEN="${3:-3}"
BUDGET_CALLS="${LOAD_CHECK_CALLS:-200}"
PORT="${LOAD_CHECK_PORT:-8871}"
SETTLE="${LOAD_CHECK_SETTLE:-80}"
SECONDS_SAMPLED="${LOAD_CHECK_SECONDS:-30}"

PROGRAM="$(command -v "$PROGRAM" || realpath "$PROGRAM")"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/pnsa-load-check.XXXXXX")"
mkdir -p "$WORK"/{home,config,DATA,run}
chmod 700 "$WORK/run"
echo '{"id":"load-check","schema":1}' > "$WORK/DATA/.brain-root.json"
# The terms of use accepted, as its owner does once on the first start: what
# is measured is the console, not the agreement shown before it. The version
# in force is read from this tree (clients/internal/brain/terms).
TERMS_VERSION="$(grep -oE '^const Version = [0-9]+' "$(dirname "$0")/../clients/internal/brain/terms/terms.go" | grep -oE '[0-9]+$')"
[ -n "$TERMS_VERSION" ] || { echo "cannot read the terms version from this tree"; exit 1; }
printf '{"version": %s, "accepted_at": "2026-11-13T00:00:00Z", "language": "en"}\n' "$TERMS_VERSION" > "$WORK/DATA/terms-accepted.json"

XVFB_PID=""
APP_PID=""

# Asks a process to stop, waits up to the given seconds and kills it if it is
# still there, so nothing outlives the check (an Xvfb still starting up when
# the check gave up was left for the runner to clean up, 09.11.2026).
# shellcheck disable=SC2317 # reached through stop_all, which the EXIT trap calls
stop_pid() {
    local pid="$1" seconds="$2"
    [ -n "$pid" ] || return 0
    kill "$pid" 2> /dev/null || return 0
    for _ in $(seq 1 $((seconds * 2))); do
        kill -0 "$pid" 2> /dev/null || return 0
        sleep 0.5
    done
    kill -KILL "$pid" 2> /dev/null || true
}

# shellcheck disable=SC2317 # called by the EXIT trap
stop_all() {
    stop_pid "$APP_PID" 10
    stop_pid "$XVFB_PID" 5
    rm -rf "$WORK"
}
trap stop_all EXIT

# A display of its own; -displayfd says which number it took. On a busy CI
# runner that took more than 5 s (dev run 42631481184, 09.11.2026), so wait
# up to LOAD_CHECK_XVFB_WAIT seconds, and stop waiting at once if Xvfb died.
XVFB_WAIT="${LOAD_CHECK_XVFB_WAIT:-30}"
exec 9> "$WORK/display"
# shellcheck disable=SC2086 # extra Xvfb arguments are words on purpose
Xvfb -displayfd 9 -screen 0 1366x768x24 -nolisten tcp ${LOAD_CHECK_XVFB_ARGS:-} > "$WORK/xvfb.log" 2>&1 &
XVFB_PID=$!
waited=0
for _ in $(seq 1 $((XVFB_WAIT * 10))); do
    [ -s "$WORK/display" ] && break
    kill -0 "$XVFB_PID" 2> /dev/null || break
    sleep 0.1
    waited=$((waited + 1))
done
DISPLAY_NUMBER="$(head -1 "$WORK/display")"
if [ -z "$DISPLAY_NUMBER" ]; then
    if kill -0 "$XVFB_PID" 2> /dev/null; then
        echo "Xvfb did not report a display within ${XVFB_WAIT}s"
    else
        status=0
        wait "$XVFB_PID" || status=$?
        XVFB_PID=""
        echo "Xvfb exited with status $status before reporting a display"
    fi
    echo "--- Xvfb's log ($WORK/xvfb.log):"
    cat "$WORK/xvfb.log"
    exit 1
fi
echo "→ Xvfb gave display :$DISPLAY_NUMBER after about $((waited / 10)).$((waited % 10))s"
export DISPLAY=":$DISPLAY_NUMBER"

env -i HOME="$WORK/home" XDG_CONFIG_HOME="$WORK/config" XDG_DATA_HOME="$WORK/home/.local/share" \
    XDG_CACHE_HOME="$WORK/home/.cache" XDG_RUNTIME_DIR="$WORK/run" PN_SCRIPTS_ASSISTANT_DATA_ROOT="$WORK/DATA" \
    PN_SCRIPTS_ASSISTANT_KEYRING=file PN_SCRIPTS_ASSISTANT_SILENT=1 BRAIN_CANCEL_ROOM=false \
    BRAIN_ALWAYS_LISTEN=false BRAIN_ALWAYS_SPEAK=false BRAIN_SETUP_DONE=true \
    OLLAMA_BASE_URL=http://127.0.0.1:9 BRAIN_ADDR="127.0.0.1:$PORT" PATH=/usr/bin:/bin \
    DBUS_SESSION_BUS_ADDRESS=disabled: NO_AT_BRIDGE=1 DISPLAY="$DISPLAY" \
    "$PROGRAM" app --skip-setup > "$WORK/app.log" 2>&1 < /dev/null &
APP_PID=$!

for _ in $(seq 1 60); do curl -fs -m 2 "http://127.0.0.1:$PORT/health" > /dev/null && break; sleep 1; done
curl -fs -m 2 "http://127.0.0.1:$PORT/health" > /dev/null || { echo "the program did not start"; tail -20 "$WORK/app.log"; exit 1; }

# The window is up when its page process is.
for _ in $(seq 1 30); do pgrep -P "$APP_PID" WebKitWebProces > /dev/null && break; sleep 1; done
pgrep -P "$APP_PID" WebKitWebProces > /dev/null || { echo "no window: the page process never started"; tail -20 "$WORK/app.log"; exit 1; }

echo "→ window open on $DISPLAY; leaving it untouched for ${SETTLE}s so the page rests"
sleep "$SETTLE"

# CPU of the program, everything under it, and the display server, in percent
# of one core over the given seconds.
measure() {
    python3 - "$APP_PID" "$XVFB_PID" "$1" << 'PY'
import os, sys, time
HZ = os.sysconf('SC_CLK_TCK')
def tree(root):
    out, todo = [], [root]
    while todo:
        p = todo.pop(); out.append(p)
        for t in os.listdir(f'/proc/{p}/task') if os.path.isdir(f'/proc/{p}/task') else []:
            try: todo += [int(c) for c in open(f'/proc/{p}/task/{t}/children').read().split()]
            except OSError: pass
    return out
def cpu(p):
    try:
        s = open(f'/proc/{p}/stat').read(); f = s[s.rindex(')') + 2:].split()
        return s[s.index('(') + 1:s.rindex(')')], int(f[11]) + int(f[12])
    except OSError: return None, None
def threads(p):
    out = {}
    for t in os.listdir(f'/proc/{p}/task') if os.path.isdir(f'/proc/{p}/task') else []:
        try:
            s = open(f'/proc/{p}/task/{t}/stat').read(); f = s[s.rindex(')') + 2:].split()
            out[t] = (s[s.index('(') + 1:s.rindex(')')], int(f[11]) + int(f[12]))
        except OSError: pass
    return out
app, xvfb, secs = int(sys.argv[1]), int(sys.argv[2]), float(sys.argv[3])
pids = tree(app) + [xvfb]
ta = {p: threads(p) for p in pids}
a = {p: cpu(p) for p in pids}; t0 = time.time(); time.sleep(secs); dt = time.time() - t0
tb = {p: threads(p) for p in pids}
total = 0.0
for p in pids:
    name, c = cpu(p)
    if c is None or a[p][1] is None: continue
    used = (c - a[p][1]) / HZ / dt * 100; total += used
    print(f'    {name:<18} {used:6.2f} %', file=sys.stderr)
    # The busiest threads of anything that costs more than a trace, so a
    # failure says where the time went (a JavaScript timer, the garbage
    # collector, the compositor, software GL) without another run.
    if used >= 0.5:
        busy = sorted(((tb[p][t][1] - ta[p][t][1]) / HZ / dt * 100, tb[p][t][0]) for t in tb[p] if t in ta.get(p, {}))
        for share, thread in reversed(busy[-4:]):
            if share >= 0.2:
                print(f'        thread {thread:<22} {share:6.2f} %', file=sys.stderr)
print(f'{total:.2f}')
PY
}

over() { python3 -c "import sys; sys.exit(0 if float(sys.argv[1]) > float(sys.argv[2]) else 1)" "$1" "$2"; }

# What the page is told: busy keeps it awake, by design.
echo "  pulse: $(curl -sN -m 2 "http://127.0.0.1:$PORT/api/pulse" 2> /dev/null | grep -m1 '^data:' || echo none)"
echo "  doing: $(curl -s -m 2 "http://127.0.0.1:$PORT/api/progress" 2> /dev/null | head -c 300)"

# Every request the page makes passes through WebKit's network process, so
# the system calls it makes while the page rests count them, exactly and
# without depending on how fast the machine is: a resting page makes none.
network="$(pgrep -P "$APP_PID" WebKitNetworkPr | head -1)"
calls() { awk '/^sysc[rw]:/ { n += $2 } END { print n + 0 }' "/proc/$network/io"; }
calls_before="$(calls)"

echo "→ resting, ${SECONDS_SAMPLED}s:"
resting="$(measure "$SECONDS_SAMPLED")"
network_calls=$(( $(calls) - calls_before ))
echo "  page requests while resting: $network_calls network system calls (budget $BUDGET_CALLS)"
echo "  resting: $resting % of one core (budget $BUDGET_RESTING %)"

window="$(xdotool search --pid "$APP_PID" 2> /dev/null | tail -1 || true)"
[ -n "$window" ] || window="$(xdotool search --onlyvisible --name . 2> /dev/null | tail -1 || true)"
[ -n "$window" ] || { echo "could not find the window to hide it"; exit 1; }
xdotool windowunmap "$window"
sleep 10

echo "→ hidden, ${SECONDS_SAMPLED}s:"
hidden="$(measure "$SECONDS_SAMPLED")"
echo "  hidden: $hidden % of one core (budget $BUDGET_HIDDEN %)"

failed=0
if over "$resting" "$BUDGET_RESTING"; then echo "FAIL: the resting window costs $resting %, over $BUDGET_RESTING %"; failed=1; fi
if over "$hidden" "$BUDGET_HIDDEN"; then echo "FAIL: the hidden window costs $hidden %, over $BUDGET_HIDDEN %"; failed=1; fi
if [ "$network_calls" -gt "$BUDGET_CALLS" ]; then echo "FAIL: the resting page is still asking the program for things ($network_calls system calls in ${SECONDS_SAMPLED}s)"; failed=1; fi

[ "$failed" -eq 0 ] && echo "→ load check OK: resting $resting %, hidden $hidden % of one core"
exit "$failed"
