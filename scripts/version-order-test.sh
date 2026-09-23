#!/usr/bin/env bash
#
# A newer build has to look newer to apt.
#
# The version carried a commit hash and nothing else that moved: dpkg compares
# a version in runs of digits and letters, so 20260923.983e04f against
# 20260923.91b8207 came down to 983 against 91 — and the newer build lost.
# apt refused to install it over the older one, calling it a downgrade. Found
# by doing it rather than by reading it.
#
# Run by `make check`. Needs dpkg, which every machine that can build the
# package has.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
declared="$(tr -d ' \t\n\r' < "$ROOT/VERSION")"

fail() { printf '✗ %s\n' "$1" >&2; exit 1; }

# Two builds a second apart, the later one with a hash that sorts lower.
older="${declared}~git20260923163746.983e04f"
newer="${declared}~git20260923163747.91b8207"

dpkg --compare-versions "$newer" gt "$older" ||
    fail "a later build does not sort after an earlier one: $newer ≯ $older"

# And the release itself is newer than every snapshot leading to it, which is
# what the tilde is for.
dpkg --compare-versions "$declared" gt "$newer" ||
    fail "the release $declared does not sort after its snapshots"

# The version this tree would actually produce is a real one.
produced="$(bash "$ROOT/scripts/build-packages.sh" --print-version)"

case "$produced" in
    "$declared"|"$declared"~git*) ;;
    *) fail "the version this tree produces is $produced, which is not $declared or a snapshot of it" ;;
esac

printf '→ versions order correctly (%s)\n' "$produced"
