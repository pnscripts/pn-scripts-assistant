#!/usr/bin/env bash
#
# The apt repository: the place `sudo apt upgrade` and Software Updater get
# new versions of PN Scripts Assistant from.
#
#   scripts/build-apt-repo.sh <folder with .deb files> <output folder> [URI]
#
# The signing key comes from the environment, never from a file in the
# repository: APT_SIGNING_KEY holds the armored private key (a GitHub Actions
# secret in release.yml). URI is where the repository will be served; it
# defaults to the published one, and a test passes its own.
#
# Plain tools only — dpkg-scanpackages and apt-ftparchive, both on every
# Ubuntu — rather than a repository manager: the repository is rebuilt from
# the release files every time, so there is no state to manage.
#
# Layout (what apt asks for):
#   <out>/dists/stable/{Release,InRelease,Release.gpg}
#   <out>/dists/stable/main/binary-amd64/{Packages,Packages.gz}
#   <out>/pool/main/p/pn-scripts-assistant/*.deb
#   <out>/pn-scripts-assistant.sources        the one file a person adds
#   <out>/pn-scripts-assistant-archive-keyring.asc

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PUBLISHED_URI="https://pnscripts.github.io/pn-scripts-assistant/apt"

debs="${1:?folder with .deb files}"
out="${2:?output folder}"
uri="${3:-$PUBLISHED_URI}"

die() { echo "build-apt-repo: $*" >&2; exit 1; }

[ -n "${APT_SIGNING_KEY:-}" ] || die "APT_SIGNING_KEY is empty: the repository would be unsigned and apt refuses those"

command -v dpkg-scanpackages >/dev/null || die "dpkg-scanpackages is missing (apt install dpkg-dev)"
command -v apt-ftparchive >/dev/null || die "apt-ftparchive is missing (apt install apt-utils)"

ls "$debs"/pn-scripts-assistant_*_amd64.deb >/dev/null 2>&1 || die "no pn-scripts-assistant .deb in $debs"

# A keyring of its own, gone when this ends, so the key never lands in
# anybody's ~/.gnupg.
GNUPGHOME="$(mktemp -d)"
export GNUPGHOME
trap 'rm -rf "$GNUPGHOME"' EXIT

printf '%s\n' "$APT_SIGNING_KEY" | gpg --batch --quiet --import 2>/dev/null || die "the signing key could not be read"

fpr="$(gpg --batch --with-colons --list-secret-keys | awk -F: '/^fpr/ { print $10; exit }')"
[ -n "$fpr" ] || die "the signing key has no secret part"

public="$(gpg --batch --armor --export "$fpr")"

# The published repository must be signed by the key every installed copy
# already trusts — the one in packaging/apt. A different key would be a
# repository that every existing user's apt refuses, silently, forever.
if [ "$uri" = "$PUBLISHED_URI" ]; then
    want="$(gpg --batch --with-colons --show-keys "$ROOT/packaging/apt/pn-scripts-assistant-archive-keyring.asc" |
        awk -F: '/^fpr/ { print $10; exit }')"

    [ "$fpr" = "$want" ] || die "APT_SIGNING_KEY ($fpr) is not the key the package trusts ($want)"
fi

rm -rf "$out"
mkdir -p "$out/pool/main/p/pn-scripts-assistant" "$out/dists/stable/main/binary-amd64"

cp "$debs"/pn-scripts-assistant_*_amd64.deb "$out/pool/main/p/pn-scripts-assistant/"

(
    cd "$out"

    dpkg-scanpackages --multiversion --arch amd64 pool > dists/stable/main/binary-amd64/Packages
    gzip -9nk dists/stable/main/binary-amd64/Packages

    apt-ftparchive \
        -o APT::FTPArchive::Release::Origin="PN Scripts" \
        -o APT::FTPArchive::Release::Label="PN Scripts Assistant" \
        -o APT::FTPArchive::Release::Suite=stable \
        -o APT::FTPArchive::Release::Codename=stable \
        -o APT::FTPArchive::Release::Architectures=amd64 \
        -o APT::FTPArchive::Release::Components=main \
        -o APT::FTPArchive::Release::Description="PN Scripts Assistant, a private assistant that runs on your own computer" \
        release dists/stable > dists/stable/Release.tmp

    mv dists/stable/Release.tmp dists/stable/Release

    gpg --batch --yes --local-user "$fpr" --digest-algo SHA512 --clearsign \
        -o dists/stable/InRelease dists/stable/Release
    gpg --batch --yes --local-user "$fpr" --digest-algo SHA512 --armor --detach-sign \
        -o dists/stable/Release.gpg dists/stable/Release
)

printf '%s\n' "$public" > "$out/pn-scripts-assistant-archive-keyring.asc"

# The file a person adds by hand: the published one is the very file the
# package installs, byte for byte, so dpkg takes it over without asking when
# the package arrives afterwards.
if [ "$uri" = "$PUBLISHED_URI" ]; then
    cp "$ROOT/packaging/apt/pn-scripts-assistant.sources" "$out/pn-scripts-assistant.sources"
else
    {
        printf 'Types: deb\nURIs: %s\nSuites: stable\nComponents: main\nArchitectures: amd64\nSigned-By:\n' "$uri"
        printf '%s\n' "$public" | sed -e 's/^$/./' -e 's/^/ /'
    } > "$out/pn-scripts-assistant.sources"
fi

# The repository checks itself before it is published: the signature is good
# and every package in the index is a file that is really there.
gpg --batch --verify "$out/dists/stable/InRelease" 2>/dev/null || die "InRelease does not verify"

while read -r file; do
    [ -f "$out/$file" ] || die "the index names $file, which is not there"
done < <(awk '/^Filename:/ { print $2 }' "$out/dists/stable/main/binary-amd64/Packages")

echo "apt repository in $out:"
awk '/^Version:/ { print "  " $2 }' "$out/dists/stable/main/binary-amd64/Packages"
