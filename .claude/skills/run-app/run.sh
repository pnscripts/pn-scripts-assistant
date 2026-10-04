#!/usr/bin/env bash
# Build PN Scripts Assistant from this checkout and serve it on loopback with a
# throwaway brain, so it can be driven in a browser without touching the
# owner's real data root or an instance they already have running.
#
#   run.sh start   build, prepare the scratch brain, serve, wait until it answers
#   run.sh stop    stop the server this script started (by PID, never pkill)
#   run.sh status  say whether it is running and where
#
# RUN_DIR (default ${TMPDIR:-/tmp}/pnsa-run) holds the binary, the brain and
# the log. PORT (default 8799) must not be the one a real instance uses.
set -euo pipefail

repo=$(cd "$(dirname "$0")/../../.." && pwd)
dir=${RUN_DIR:-${TMPDIR:-/tmp}/pnsa-run}
port=${PORT:-8799}
bin=$dir/pnsa
root=$dir/root
pidfile=$dir/serve.pid
log=$dir/serve.log

running() {
	[ -f "$pidfile" ] || return 1
	pid=$(cat "$pidfile")
	# Ours only if that PID is still this binary: a stale file must never get
	# somebody else's process killed.
	[ "$(readlink "/proc/$pid/exe" 2>/dev/null || true)" = "$bin" ]
}

case ${1:-start} in
start)
	if running; then
		echo "already running: http://127.0.0.1:$port (pid $(cat "$pidfile"))"
		exit 0
	fi

	if ss -ltn "sport = :$port" | grep -q LISTEN; then
		echo "port $port is taken by something else; set PORT=..." >&2
		exit 1
	fi

	mkdir -p "$root"
	echo "building..."
	CGO_ENABLED=1 go build -C "$repo/clients" -o "$bin" ./cmd/pn-scripts-assistant

	# The marker makes the folder a brain; the program refuses a data root
	# without one. Same shape paths.Create writes.
	if [ ! -f "$root/.brain-root.json" ]; then
		printf '{\n  "id": "%s",\n  "schema": 1,\n  "created": "%s"\n}\n' \
			"$(date +%s%N)" "$(date -u +%FT%TZ)" > "$root/.brain-root.json"
	fi

	# A settings file means it is not a first run, so the "Before we start"
	# naming card does not cover the page. The owner is left empty on purpose.
	if [ ! -f "$root/brain.conf" ]; then
		printf 'BRAIN_NAME=Assistant\nBRAIN_OWNER=\n' > "$root/brain.conf"
	fi

	PN_SCRIPTS_ASSISTANT_DATA_ROOT=$root setsid "$bin" serve --addr "127.0.0.1:$port" > "$log" 2>&1 < /dev/null &
	echo $! > "$pidfile"

	for _ in $(seq 1 60); do
		if [ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/" || true)" = 200 ]; then
			echo "ready: http://127.0.0.1:$port (pid $(cat "$pidfile"), log $log)"
			exit 0
		fi

		if ! running; then
			echo "it exited before answering:" >&2
			tail -20 "$log" >&2
			exit 1
		fi

		sleep 1
	done

	echo "not answering after 60s; see $log" >&2
	exit 1
	;;
stop)
	if running; then
		kill "$(cat "$pidfile")"
		rm -f "$pidfile"
		echo stopped
	else
		rm -f "$pidfile"
		echo "not running"
	fi
	;;
status)
	if running; then
		echo "running: http://127.0.0.1:$port (pid $(cat "$pidfile"))"
	else
		echo "not running"
	fi
	;;
*)
	echo "usage: $0 start|stop|status" >&2
	exit 2
	;;
esac
