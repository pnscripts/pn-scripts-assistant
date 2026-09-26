#!/usr/bin/env bash
#
# Points every local clone at the correct GitHub URL, in lowercase.
#
# Run this after renaming an account or organisation on GitHub. Renames cannot
# be done through the API — GitHub gates them behind interactive confirmation —
# so the rename is manual and this handles everything downstream.
#
# Old URLs keep working through GitHub's redirect, which is precisely why this
# is worth running: a redirect hides the fact that a remote is stale, until the
# day the old name is claimed by someone else and the redirect stops.
#
#   scripts/sync-git-remotes.sh            # report only
#   scripts/sync-git-remotes.sh --apply    # rewrite them
set -uo pipefail

APPLY=false
[ "${1:-}" = "--apply" ] && APPLY=true

# Where clones live. Named roots rather than scanning the whole disk, which on
# a machine with a media drive takes minutes and finds other people's code.
#
# From the environment, so this works for whoever is running it: it used to
# carry one person's drive, spelled out with its filesystem UUID, which was
# fine while this was one person's script and is not something a public
# repository should be handing to everybody who clones it.
#
#   PN_CLONE_ROOTS=/data/code:/srv/work scripts/sync-git-remotes.sh
#
# Unset, it looks where this checkout already is — whatever directory holds
# it — and in ~/Projects.
if [ -n "${PN_CLONE_ROOTS:-}" ]; then
    IFS=':' read -r -a SEARCH_ROOTS <<< "$PN_CLONE_ROOTS"
else
    here="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

    SEARCH_ROOTS=(
        "$here"
        "$HOME/Projects"
    )
fi

changed=0
already=0
missing=0

lower() { tr 'A-Z' 'a-z'; }

for root in "${SEARCH_ROOTS[@]}"; do
    [ -d "$root" ] || continue

    # Two levels deep covers project/repo and project/group/repo layouts.
    for dir in "$root"/*/ "$root"/*/*/; do
        [ -d "$dir/.git" ] || continue

        url="$(git -C "$dir" remote get-url origin 2>/dev/null)" || continue
        case "$url" in *github.com*) ;; *) continue ;; esac

        owner="$(printf '%s' "$url" | sed -E 's#.*github\.com[:/]([^/]+)/.*#\1#')"
        repo="$(printf '%s' "$url" | sed -E 's#.*github\.com[:/][^/]+/(.*)$#\1#; s#\.git$##')"

        target="git@github.com:$(printf '%s' "$owner" | lower)/$(printf '%s' "$repo" | lower).git"

        if [ "$url" = "$target" ]; then
            already=$((already + 1))
            continue
        fi

        # Only rewrite once the destination is confirmed to exist. Rewriting a
        # remote to a repository that was never created would replace a URL that
        # at least redirects with one that simply fails.
        if ! gh api "repos/$(printf '%s' "$owner" | lower)/$(printf '%s' "$repo" | lower)" \
                --jq '.full_name' >/dev/null 2>&1; then
            printf '  \033[33m!\033[0m %-30s no such repository: %s\n' "$(basename "$dir")" "$repo"
            missing=$((missing + 1))
            continue
        fi

        if [ "$APPLY" = true ]; then
            git -C "$dir" remote set-url origin "$target"
            printf '  \033[32m✓\033[0m %-30s %s\n' "$(basename "$dir")" "$target"
        else
            printf '  \033[2m→\033[0m %-30s would become %s\n' "$(basename "$dir")" "$target"
        fi

        changed=$((changed + 1))
    done
done

echo
if [ "$APPLY" = true ]; then
    echo "  rewritten: $changed   already correct: $already   pointing at nothing: $missing"
else
    echo "  would rewrite: $changed   already correct: $already   pointing at nothing: $missing"
    echo "  Re-run with --apply to make the changes."
fi

[ "$missing" -gt 0 ] && echo "  The ones marked ! have no repository on GitHub — create them or fix the remote."

exit 0
