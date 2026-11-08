#!/usr/bin/env bash
#
# The winget manifests for one published release.
#
#   scripts/winget-manifests.sh <version> <output folder>
#
# Writes <output>/manifests/p/PNScripts/PNScriptsAssistant/<version>/ with the
# three files microsoft/winget-pkgs takes: the version, the installer (the
# published setup.exe, its SHA-256 read from the release's own SHA256SUMS)
# and the default locale, plus Bulgarian. The installer URL is the release
# download itself — winget's rule is that it comes from the publisher's own
# release location, and ours is GitHub Releases.
#
# The same folder is what `winget validate` checks and `winget install
# --manifest` installs from in CI, and what goes into the pull request to
# microsoft/winget-pkgs.

set -euo pipefail

version="${1:?version, e.g. 0.12.0}"
out="${2:?output folder}"
version="${version#v}"

repo="pnscripts/pn-scripts-assistant"
id="PNScripts.PNScriptsAssistant"
setup="pn-scripts-assistant-${version}-windows-setup.exe"
url="https://github.com/${repo}/releases/download/v${version}/${setup}"

sums="$(curl -fsSL "https://github.com/${repo}/releases/download/v${version}/SHA256SUMS")"
sha="$(printf '%s\n' "$sums" | awk -v f="$setup" '$2 == f || $2 == "*" f { print toupper($1) }')"

[ ${#sha} -eq 64 ] || { echo "no SHA-256 for $setup in the v$version SHA256SUMS" >&2; exit 1; }

date="$(curl -fsSL "https://api.github.com/repos/${repo}/releases/tags/v${version}" |
    sed -n 's/.*"published_at": *"\([0-9-]*\)T.*/\1/p' | head -1)"

dir="$out/manifests/p/PNScripts/PNScriptsAssistant/$version"
mkdir -p "$dir"

cat > "$dir/$id.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.version.1.10.0.schema.json

PackageIdentifier: $id
PackageVersion: $version
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.10.0
EOF

cat > "$dir/$id.installer.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.installer.1.10.0.schema.json

PackageIdentifier: $id
PackageVersion: $version
InstallerType: inno
Scope: user
UpgradeBehavior: install
ProductCode: '{7F3B6A1E-6C2C-4E0B-9E5F-2B7A6F0C9D41}_is1'
ReleaseDate: ${date:-$(date -u +%Y-%m-%d)}
InstallationMetadata:
  DefaultInstallLocation: '%LocalAppData%\Programs\PN Scripts Assistant'
Installers:
- Architecture: x64
  InstallerUrl: $url
  InstallerSha256: $sha
ManifestType: installer
ManifestVersion: 1.10.0
EOF

cat > "$dir/$id.locale.en-US.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.defaultLocale.1.10.0.schema.json

PackageIdentifier: $id
PackageVersion: $version
PackageLocale: en-US
Publisher: PN Scripts
PublisherUrl: https://pnscripts.com
PublisherSupportUrl: https://github.com/${repo}/issues
Author: PN Scripts
PackageName: PN Scripts Assistant
PackageUrl: https://github.com/${repo}
License: MIT
LicenseUrl: https://github.com/${repo}/blob/main/LICENSE
Copyright: Copyright (c) PN Scripts
ShortDescription: A private, self-learning assistant that runs on your own computer.
Description: |-
  PN Scripts Assistant is a personal assistant that keeps everything it learns in one folder on your own computer and runs its language model locally through Ollama. By default it sends nothing anywhere.
  It reads your projects and documents, answers with sources, writes and translates in English and Bulgarian, and asks before it changes anything.
Moniker: pn-scripts-assistant
Tags:
- ai
- assistant
- local-ai
- ollama
- privacy
ReleaseNotesUrl: https://github.com/${repo}/releases/tag/v${version}
ManifestType: defaultLocale
ManifestVersion: 1.10.0
EOF

cat > "$dir/$id.locale.bg-BG.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.locale.1.10.0.schema.json

PackageIdentifier: $id
PackageVersion: $version
PackageLocale: bg-BG
Publisher: PN Scripts
PackageName: PN Scripts Assistant
ShortDescription: Личен самообучаващ се асистент, който работи на вашия компютър.
Description: |-
  PN Scripts Assistant пази всичко научено в една папка на вашия компютър и работи с локален езиков модел чрез Ollama. По подразбиране не изпраща нищо никъде.
  Чете проектите и документите ви, отговаря с източници, пише и превежда на български и английски и пита, преди да промени нещо.
ManifestType: locale
ManifestVersion: 1.10.0
EOF

echo "$dir"
ls "$dir"
