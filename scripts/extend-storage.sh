#!/usr/bin/env bash
#
# Asks where to put the brain when its drive is filling up, and moves it there.
#
# This lives on the host rather than in the application because only the host
# can see drives. The container knows it is running out of room; it has no idea
# what else is plugged in, and cannot mount anything.
#
# Graphical by default (zenity), with a terminal fallback for headless use.
#
# Moving rather than splitting is deliberate. Postgres can be spread across
# drives with tablespaces, but then the brain only works when every drive is
# attached, and a missing one is a corrupt database rather than a smaller one.
# A brain that lives on exactly one drive at a time can be carried, backed up
# and reasoned about; that property is worth more than squeezing two drives
# together.
set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$PROJECT_ROOT/.env"
MARKER=".brain-root.json"

have() { command -v "$1" >/dev/null 2>&1; }
graphical() { have zenity && [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; }

info()  { graphical && zenity --info  --title="Pnexus storage" --width=430 --text="$1" || echo "[pnexus] $1"; }
error() { graphical && zenity --error --title="Pnexus storage" --width=430 --text="$1" || echo "[pnexus] error: $1" >&2; }

current_root() {
    grep -E "^BRAIN_DATA_ROOT=" "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-
}

human() { numfmt --to=iec --suffix=B "$1" 2>/dev/null || echo "$1"; }

# Every writable mounted drive other than the one already in use, with room to
# spare. Root and system mounts are excluded: filling those breaks the computer,
# not just Pnexus.
candidates() {
    local current="$1"
    df -PB1 --output=target,avail 2>/dev/null | tail -n +2 | while read -r target avail; do
        case "$target" in
            /media/*|/mnt/*|/run/media/*) ;;
            *) continue ;;
        esac

        [ "$target" = "$current" ] && continue
        [ -w "$target" ] || continue
        [ "$avail" -lt 5368709120 ] && continue   # under 5GB is not worth moving to

        echo "$target|$avail"
    done
}

main() {
    local current
    current="$(current_root)"

    if [ -z "$current" ] || [ ! -d "$current" ]; then
        error "Pnexus does not have a data folder yet. Start Pnexus first."
        exit 1
    fi

    local drive_of_current
    drive_of_current="$(df -P "$current" | tail -1 | awk '{print $6}')"

    mapfile -t options < <(candidates "$drive_of_current")

    if [ ${#options[@]} -eq 0 ]; then
        info "No other drive is available.\n\nConnect an external drive, then run this again. Pnexus is currently using:\n$current"
        exit 0
    fi

    # Build the picker rows: what a person needs is the drive and how much room
    # it has, not a device node.
    local rows=()
    for entry in "${options[@]}"; do
        local target="${entry%%|*}"
        local avail="${entry##*|}"
        rows+=("$target" "$(human "$avail") free")
    done

    local chosen
    if graphical; then
        chosen="$(zenity --list --title="Pnexus storage" --width=520 --height=300 \
            --text="Pnexus is using:\n$current\n\nWhere should the brain move to?" \
            --column="Drive" --column="Space" "${rows[@]}" 2>/dev/null)"
    else
        echo "Pnexus is using: $current"
        echo "Available drives:"
        local i=1
        for entry in "${options[@]}"; do
            echo "  $i) ${entry%%|*}  (${entry##*|} bytes free)"
            i=$((i + 1))
        done
        echo -n "Number (or blank to cancel): "
        read -r pick
        [ -n "$pick" ] && chosen="${options[$((pick - 1))]%%|*}"
    fi

    [ -z "${chosen:-}" ] && exit 0

    local destination="$chosen/PNEXUS-DATA"

    if [ -e "$destination" ]; then
        error "There is already a PNEXUS-DATA folder on that drive.\n\n$destination\n\nMove or rename it first — refusing to overwrite a brain."
        exit 1
    fi

    # Stop before copying. Postgres files copied while running are not a
    # database, they are a corrupt one.
    if ! "$PROJECT_ROOT/scripts/start-brain.sh" --down >/dev/null 2>&1; then
        error "Could not stop Pnexus cleanly. Nothing has been moved."
        exit 1
    fi

    local moved=false
    if graphical; then
        (
            cp -a "$current" "$destination" && moved=true
            echo 100
        ) | zenity --progress --pulsate --auto-close --no-cancel \
                   --title="Pnexus storage" --width=430 \
                   --text="Moving the brain to $chosen…\n\nThis can take a while. Do not unplug either drive."
        [ -d "$destination/$MARKER" ] || [ -f "$destination/$MARKER" ] && moved=true
    else
        echo "[pnexus] copying $current -> $destination"
        cp -a "$current" "$destination" && moved=true
    fi

    if [ "$moved" != true ] || [ ! -f "$destination/$MARKER" ]; then
        error "The move did not finish. Your original brain is untouched at:\n$current"
        rm -rf "$destination"
        exit 1
    fi

    # Point at the copy and start again. The original is deliberately left in
    # place: it is the only backup that exists at this moment, and deleting it
    # automatically would make a bad copy unrecoverable.
    sed -i "s#^BRAIN_DATA_ROOT=.*#BRAIN_DATA_ROOT=$destination#" "$ENV_FILE"

    if ! "$PROJECT_ROOT/scripts/start-brain.sh" >/dev/null 2>&1; then
        error "Moved, but Pnexus did not restart. Your data is safe in both places."
        exit 1
    fi

    info "Pnexus now runs from:\n$destination\n\nThe old copy is still at:\n$current\n\nCheck everything works, then delete the old one yourself when you are satisfied."
}

main "$@"
