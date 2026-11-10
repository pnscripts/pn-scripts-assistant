#!/usr/bin/env bash
#
# Every program we publish knows which version it is.
#
#   scripts/check-versions.sh <version> <file or folder>...
#
# The standalone Linux download of 1.0.0 (and of 0.13.0 before it) was built
# without the version stamp, so it called itself "built from source" and its
# in-app update notice never fired: it had no version to compare with the
# newest one. The .deb and the Windows copy were stamped by the packaging
# script; the plain binaries were built by a line of their own in release.yml
# that nobody checked. This is the check.
#
# For every Go program found (in the folders, and inside each .deb and .zip):
#   * it must carry the searchable stamp PNSA-RELEASE[<version>] (see
#     release.Mark; -trimpath keeps -ldflags out of the build record that
#     `go version -m` reads, so the record says which program and which
#     system it is, and the stamp is found in the file itself);
#   * when it can run here, `<program> --version` must print <version> and
#     must not say "built from source".
# Files that are not Go programs (the Windows installer, sums, manifests) are
# skipped and named. Finding no program at all is a failure, not a pass.
set -euo pipefail

fail() { printf '✗ %s\n' "$1" >&2; exit 1; }

[ "$#" -ge 2 ] || fail "usage: $0 <version> <file or folder>..."

want="${1#v}"
shift

[ -n "$want" ] || fail "no version given"

stamp="PNSA-RELEASE[$want]"
host="$(go env GOOS)/$(go env GOARCH)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

checked=0
bad=0

check_program() {
    local file="$1" label="$2" info target out

    info="$(go version -m "$file" 2>/dev/null)" || return 1

    # Only our own programs: a Go file from somebody else would carry
    # somebody else's build record.
    grep -qE "^[[:space:]]+path[[:space:]]+pn-scripts-assistant/" <<<"$info" || return 1

    checked=$((checked + 1))

    if ! grep -aqF -- "$stamp" "$file"; then
        printf '✗ %s is not stamped with %s\n' "$label" "$want" >&2
        bad=$((bad + 1))

        return 0
    fi

    target="$(awk '$2 ~ /^GOOS=/ {sub("GOOS=", "", $2); os=$2} $2 ~ /^GOARCH=/ {sub("GOARCH=", "", $2); arch=$2} END {print os "/" arch}' <<<"$info")"

    if [ "$target" != "$host" ]; then
        printf '✓ %s says %s (built for %s, not run here)\n' "$label" "$want" "$target"

        return 0
    fi

    # A program linked against libraries this machine lacks (the window
    # needs WebKitGTK) cannot start here; its stamp was checked above, and the
    # job that built it runs this check where the libraries are.
    if command -v ldd >/dev/null && ldd "$file" 2>/dev/null | grep -q "not found"; then
        printf '✓ %s says %s (libraries missing here, not run)\n' "$label" "$want"

        return 0
    fi

    # A downloaded artifact loses its execute bit (actions/download-artifact
    # keeps no permissions): run a copy that has one, never change the file
    # that is about to be published.
    local run="$file"

    if [ ! -x "$file" ]; then
        run="$(mktemp "$work/run.XXXXXX")"
        cp "$file" "$run"
        chmod 755 "$run"
    fi

    out="$(PN_SCRIPTS_ASSISTANT_SILENT=1 timeout 30 "$run" --version 2>&1)" || {
        printf '✗ %s --version failed: %s\n' "$label" "$out" >&2
        bad=$((bad + 1))

        return 0
    }

    if ! grep -qF -- " $want" <<<"$out" || grep -q "built from source" <<<"$out"; then
        printf '✗ %s --version printed: %s\n' "$label" "$out" >&2
        bad=$((bad + 1))

        return 0
    fi

    printf '✓ %s --version: %s\n' "$label" "$out"
}

look_at() {
    local file="$1" label="$2" inside

    case "$file" in
        *.deb)
            inside="$(mktemp -d "$work/deb.XXXXXX")"
            dpkg-deb -x "$file" "$inside" || fail "cannot open $label"
            look_in "$inside" "$label:"
            ;;
        *.zip)
            inside="$(mktemp -d "$work/zip.XXXXXX")"
            unzip -q "$file" -d "$inside" || fail "cannot open $label"
            look_in "$inside" "$label:"
            ;;
        *)
            check_program "$file" "$label" || printf '  %s is not one of our programs, skipped\n' "$label"
            ;;
    esac
}

look_in() {
    local dir="$1" prefix="$2" file

    while IFS= read -r -d '' file; do
        look_at "$file" "$prefix${file#"$dir"/}"
    done < <(find "$dir" -type f -print0 | sort -z)
}

for path in "$@"; do
    if [ -d "$path" ]; then
        look_in "$path" ""
    elif [ -f "$path" ]; then
        look_at "$path" "$(basename "$path")"
    else
        fail "$path does not exist"
    fi
done

[ "$checked" -gt 0 ] || fail "no program of ours was found in: $*"
[ "$bad" -eq 0 ] || fail "$bad of $checked programs do not say they are $want"

printf '✓ all %d programs say they are %s\n' "$checked" "$want"
