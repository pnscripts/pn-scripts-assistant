#!/usr/bin/env bash
#
# Locates the portable brain data root (wherever it currently lives — this drive,
# a different drive, a different computer entirely), points the app at it, and
# brings the brain up via Docker.
#
# Usage:
#   scripts/start-brain.sh                 # find/create the data root, then start
#   scripts/start-brain.sh --data <path>   # pin the data root explicitly
#   scripts/start-brain.sh --down          # stop the brain
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$PROJECT_ROOT/.env"
MARKER_NAME=".brain-root.json"

# Where to look for an existing data root. Extend BRAIN_SEARCH_PATHS to add more
# locations (e.g. a NAS mount) without touching this script.
DEFAULT_SEARCH_ROOTS=(/media/*/* /mnt/* /Volumes/*)
SEARCH_ROOTS=("${DEFAULT_SEARCH_ROOTS[@]}" ${BRAIN_SEARCH_PATHS:-})

log() { echo "[start-brain] $*"; }

find_data_root() {
    local candidate
    for pattern in "${SEARCH_ROOTS[@]}"; do
        for dir in $pattern; do
            [ -d "$dir" ] || continue
            candidate="$dir/AI-BRAIN-DATA"
            if [ -f "$candidate/$MARKER_NAME" ]; then
                echo "$candidate"
                return 0
            fi
        done
    done
    return 1
}

set_env_var() {
    local key="$1" value="$2"
    local escaped
    escaped=$(printf '%s' "$value" | sed 's/[&/\]/\\&/g')
    if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
        sed -i "s#^${key}=.*#${key}=${escaped}#" "$ENV_FILE"
    else
        echo "${key}=${value}" >> "$ENV_FILE"
    fi
}

DATA_ROOT=""
ACTION="up"

while [ $# -gt 0 ]; do
    case "$1" in
        --data) DATA_ROOT="$2"; shift 2 ;;
        --down) ACTION="down"; shift ;;
        *) log "Unknown argument: $1"; exit 1 ;;
    esac
done

[ -f "$ENV_FILE" ] || cp "$PROJECT_ROOT/.env.example" "$ENV_FILE"

if [ "$ACTION" = "down" ]; then
    ( cd "$PROJECT_ROOT" && docker compose down )
    exit 0
fi

if [ -z "$DATA_ROOT" ]; then
    if DATA_ROOT="$(find_data_root)"; then
        log "Found existing brain data at: $DATA_ROOT"
    else
        log "No existing brain data root found on any connected drive."
        echo "Where should a new one be created? Enter a drive/folder path:"
        read -r base
        [ -d "$base" ] || { log "Path does not exist: $base"; exit 1; }
        DATA_ROOT="$base/AI-BRAIN-DATA"
        mkdir -p "$DATA_ROOT/postgres" "$DATA_ROOT/redis" "$DATA_ROOT/logs"
        brain_id="$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen)"
        cat > "$DATA_ROOT/$MARKER_NAME" <<EOF
{
  "id": "$brain_id",
  "name": "petar-brain",
  "schema_version": 1,
  "created_at": "$(date -Iseconds)"
}
EOF
        log "Created new brain data root: $DATA_ROOT"
    fi
fi

mkdir -p "$DATA_ROOT/postgres" "$DATA_ROOT/redis" "$DATA_ROOT/logs"
set_env_var "BRAIN_DATA_ROOT" "$DATA_ROOT"
log "BRAIN_DATA_ROOT=$DATA_ROOT"

( cd "$PROJECT_ROOT" && docker compose up -d )
log "Brain is starting. Run 'docker compose logs -f' from $PROJECT_ROOT to watch it."
