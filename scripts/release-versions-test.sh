#!/usr/bin/env bash
#
# Every program the release publishes is stamped with its version.
#
# 1.0.0 published pn-scripts-assistant-linux-amd64 built by a line of
# release.yml that left the stamp out: it said "built from source" and its
# update notice never fired. Two halves, both run by `make check`:
#
#   1. every `go build` in release.yml takes its -ldflags stamp from
#      build-packages.sh --print-ldflags, the one place that makes it;
#   2. scripts/check-versions.sh, the check release.yml runs on what it built,
#      passes a stamped program and refuses an unstamped one and one stamped
#      with another version.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
fail() { printf '✗ %s\n' "$1" >&2; exit 1; }

# 1. Every build line in the release workflow carries the stamp.
release="$ROOT/.github/workflows/release.yml"
builds="$(grep -nE '(^|[^[:alnum:]_])go build ' "$release" | grep -v '^[0-9]*:[[:space:]]*#' || true)"

[ -n "$builds" ] || fail "release.yml builds nothing with go build: has the release moved? Update this test."

unstamped="$(grep -v -- '--print-ldflags' <<<"$builds" || true)"
[ -z "$unstamped" ] || fail "release.yml builds a program without the version stamp:
$unstamped"

# And the stamp printed is the one the packages use.
stamp="$(bash "$ROOT/scripts/build-packages.sh" --print-ldflags)"
version="$(bash "$ROOT/scripts/build-packages.sh" --print-version)"
for want in "release.Version=$version " "release.Mark=PNSA-RELEASE[$version] "; do
    grep -qF -- "$want" <<<"$stamp " || fail "--print-ldflags does not stamp $want: $stamp"
done

# 2. The check itself, on a real program (doctor: no cgo, quick to build).
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/stamped" "$work/plain" "$work/other"

go build -C "$ROOT/clients" -trimpath -ldflags="-s -w $stamp" -o "$work/stamped/pn-scripts-assistant-doctor" ./cmd/doctor
go build -C "$ROOT/clients" -trimpath -ldflags="-s -w" -o "$work/plain/pn-scripts-assistant-doctor" ./cmd/doctor
go build -C "$ROOT/clients" -trimpath \
    -ldflags="-s -w -X pn-scripts-assistant/internal/brain/release.Version=0.0.1 -X pn-scripts-assistant/internal/brain/release.Mark=PNSA-RELEASE[0.0.1]" \
    -o "$work/other/pn-scripts-assistant-doctor" ./cmd/doctor

# The CLI too, which answers --version before it reaches the network.
go build -C "$ROOT/clients" -trimpath -ldflags="-s -w $stamp" -o "$work/stamped/pn-scripts-assistant-cli" ./cmd/cli

# A cross-built copy is checked by its build record alone.
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -C "$ROOT/clients" -trimpath \
    -ldflags="-s -w $stamp" -o "$work/stamped/pn-scripts-assistant-doctor.exe" ./cmd/doctor

# Something that is not one of our programs is skipped, not failed.
printf 'not a program\n' > "$work/stamped/SHA256SUMS"

# Downloaded artifacts lose their execute bit (actions/download-artifact):
# the check must still run them rather than fail on permission.
chmod -x "$work/stamped/pn-scripts-assistant-doctor"

out="$(bash "$ROOT/scripts/check-versions.sh" "$version" "$work/stamped")" ||
    fail "check-versions.sh refused programs stamped with $version"
grep -qF "all 3 programs say they are $version" <<<"$out" ||
    fail "check-versions.sh did not check all three programs: $out"
grep -qF "pn-scripts-assistant-doctor --version: " <<<"$out" ||
    fail "check-versions.sh did not run a program that lost its execute bit: $out"

# The same, inside a zip, the way the Windows download carries it.
(cd "$work/stamped" && zip -q "$work/stamped.zip" pn-scripts-assistant-doctor.exe) 2>/dev/null ||
    python3 -c 'import sys, zipfile; z = zipfile.ZipFile(sys.argv[1], "w"); z.write(sys.argv[2], "pn-scripts-assistant-doctor.exe")' \
        "$work/stamped.zip" "$work/stamped/pn-scripts-assistant-doctor.exe"
bash "$ROOT/scripts/check-versions.sh" "$version" "$work/stamped.zip" >/dev/null ||
    fail "check-versions.sh refused a stamped program inside a zip"

if bash "$ROOT/scripts/check-versions.sh" "$version" "$work/plain" >/dev/null 2>&1; then
    fail "check-versions.sh passed a program built without the stamp"
fi

if bash "$ROOT/scripts/check-versions.sh" "$version" "$work/other" >/dev/null 2>&1; then
    fail "check-versions.sh passed a program stamped with another version"
fi

if bash "$ROOT/scripts/check-versions.sh" "$version" "$work/stamped/SHA256SUMS" >/dev/null 2>&1; then
    fail "check-versions.sh passed when it found no program at all"
fi

printf '✓ every released program carries its version, and the check refuses one that does not\n'
