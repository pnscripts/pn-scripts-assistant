#!/usr/bin/env bash
#
# Builds the thing somebody downloads, for each system they might be on.
#
#   Ubuntu / Debian   a .deb, so it installs and updates like anything else
#   macOS             a .app bundle, so it is dragged to Applications
#   Windows           a folder with the .exe, so it runs from anywhere
#
# One script rather than three, because the differences between these are
# almost entirely in the wrapping: the same Go binary is inside all of them.
# Keeping them together is what stops one of them quietly rotting — a packaging
# script nobody runs is a packaging script that has already broken.
#
# What deliberately stays outside every package: Ollama and the language model.
# Ollama has its own installer and update cycle, and the model is several
# gigabytes that most people will either already have or will want to choose
# for themselves. Bundling either would double the download and go stale
# immediately, so setup checks for them on first run and says what is missing.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/build/packages"
# ---------------------------------------------------------------------------
# What to call this build
# ---------------------------------------------------------------------------
#
# A tag when there is one, and something derived when there is not. It cannot
# simply be the commit hash: dpkg refuses a version that does not begin with a
# digit, and more importantly a hash does not sort — so "upgrade" and
# "downgrade" become meaningless to the package manager and it will happily
# install an older build over a newer one.
#
# The date sorts, and the hash after it says exactly which build it was.
# Which version this is, from the one place that says so.
#
# The VERSION file at the top of the repository. It used to be three places
# that disagreed: a number hard-coded here, a commit hash the program reported
# about itself, and nothing at all in the desktop entry.
#
# A build that is not exactly a release says so in the version rather than
# pretending: 0.1.0 at the tag, 0.1.0~git20260923.e9b80e0 anywhere else. The
# tilde sorts *before* the plain number in dpkg's ordering, which is the right
# way round — a build from the middle of the work is older than the release it
# is working towards.
released_version() {
    local declared
    declared="$(tr -d " \t\n\r" < "$ROOT/VERSION" 2>/dev/null)"

    printf '%s' "${declared:-0.0.0}"
}

derive_version() {
    if [ -n "${VERSION:-}" ]; then
        printf '%s' "${VERSION#v}"

        return
    fi

    local declared tag
    declared="$(released_version)"
    tag="$(git -C "$ROOT" describe --tags --exact-match 2>/dev/null || true)"

    if [ "${tag#v}" = "$declared" ] && [ -n "$tag" ]; then
        printf '%s' "$declared"

        return
    fi

    # The time, not only the day, and the hash last.
    #
    # dpkg compares a version in runs of digits and letters: 20260923.983e04f
    # against 20260923.91b8207 comes down to 983 against 91, and the newer
    # build loses. Found by installing one over the other — apt refused it as
    # a downgrade. A commit hash cannot order anything; the second it was
    # committed can, so that is what carries the ordering and the hash is left
    # as the label it always was.
    local when hash
    when="$(git -C "$ROOT" log -1 --format=%cd --date=format:%Y%m%d%H%M%S 2>/dev/null || date -u +%Y%m%d%H%M%S)"
    hash="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"

    printf '%s~git%s.%s' "$declared" "$when" "$hash"
}

# What the program says when it is asked which copy it is. Empty parts are
# left empty rather than guessed at; see internal/brain/release.
version_stamp() {
    local pkg="pn-scripts-assistant/internal/brain/release"
    local hash when

    hash="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || true)"
    when="$(git -C "$ROOT" log -1 --format=%cd --date=format:%Y-%m-%d 2>/dev/null || date +%Y-%m-%d)"

    printf -- "-X %s.Version=%s -X %s.Commit=%s -X %s.Built=%s" \
        "$pkg" "$VERSION" "$pkg" "$hash" "$pkg" "$when"
}

VERSION="$(derive_version)"

# When this build says it happened: the commit's own date, never now.
#
# Reproducibility is the point. A package built twice from one commit should be
# the same package, and it was not: the changelog carried `date -R` — the
# moment the script ran — and dpkg-deb wrote each file's mtime, which is
# whenever the staging directory happened to be created. Two builds of one
# commit therefore had two different checksums, which makes "check what you
# downloaded is what was built" a much weaker statement than it sounds.
#
# SOURCE_DATE_EPOCH is the convention for this and dpkg-deb honours it: it
# clamps every timestamp it writes to that second. Everything else that records
# a time reads it too. Falling back to now when there is no git, because a
# build from a tarball still has to work — it is simply not reproducible, which
# is true of anything built outside the repository.
export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || date -u +%s)}"

log()  { printf '\033[36m→\033[0m %s\n' "$1"; }
warn() { printf '\033[33m!\033[0m %s\n' "$1"; }
die()  { printf '\033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }

want="${1:-all}"

# Asked what it would call this build, without building it. The Makefile and
# CI both need the answer, and working it out twice is how two names for one
# build happen.
if [ "$want" = "--print-version" ]; then
    printf '%s\n' "$VERSION"

    exit 0
fi

mkdir -p "$OUT"

# Each target clears its own files, and nothing clears the folder.
#
# It used to be `rm -rf "$OUT"` at the top, which is right for a machine that
# builds everything at once and wrong for the one that does not: asking for
# just the Windows download deleted the .deb that had been built, checked with
# lintian and summed ten minutes earlier. Silently, with an empty folder as
# the only evidence — and it is exactly the sequence somebody follows on the
# evening of a release.
clear_old() {
    local pattern

    for pattern in "$@"; do
        # Unquoted on purpose so the glob expands; rm -f says nothing when a
        # pattern matches nothing, which is the usual case on a clean machine.
        # shellcheck disable=SC2086
        rm -f "$OUT"/$pattern
    done
}

# ---------------------------------------------------------------------------
# Ubuntu and Debian
# ---------------------------------------------------------------------------
#
# Built with cgo so the native window is real. That is the one package where
# the window exists, and a .deb can declare the libraries it needs rather than
# discovering them missing at runtime — which is the entire advantage of using
# the system's own package manager, so it is worth using properly.
# The two files Debian policy asks every package to carry.
#
# Neither is decoration. copyright is required — §12.5 — and a package without
# one cannot go anywhere that checks; the changelog is what somebody reads to
# find out what they just installed, and apt shows it. Both belong in
# /usr/share/doc/<package>, and the changelog is gzipped because policy says so
# and because lintian says so twice.
deb_documents() {
    local stage="$1" doc="$1/usr/share/doc/pn-scripts-assistant"

    mkdir -p "$doc"

    cat > "$doc/copyright" <<'COPYRIGHT'
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: pn-scripts-assistant
Source: https://github.com/pnscripts/pn-scripts-assistant

Files: *
Copyright: 2026 Petar Nikolov
License: MIT

License: MIT
 Permission is hereby granted, free of charge, to any person obtaining a copy
 of this software and associated documentation files (the "Software"), to deal
 in the Software without restriction, including without limitation the rights
 to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 copies of the Software, and to permit persons to whom the Software is
 furnished to do so, subject to the following conditions:
 .
 The above copyright notice and this permission notice shall be included in
 all copies or substantial portions of the Software.
 .
 THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 THE SOFTWARE.
COPYRIGHT

    cat > "$doc/changelog" <<CHANGELOG
pn-scripts-assistant ($VERSION) unstable; urgency=medium

  * Built from $( git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo "an unknown revision" ).
  * The release notes for this build are in the repository, under docs/.

 -- Petar Nikolov <petar.v.nikolov@gmail.com>  $(date -R -u -d "@$SOURCE_DATE_EPOCH" 2>/dev/null || date -R)
CHANGELOG

    # -n so the name and timestamp are not written into the gzip header, which
    # is one of the things that would otherwise make two builds of the same
    # source differ. See the release script.
    gzip -9n -f "$doc/changelog"

    chmod 644 "$doc/copyright" "$doc/changelog.gz"
}

build_deb() {
    # The old package and the checksums that describe it, which would otherwise
    # go on describing a file that is no longer here.
    clear_old 'pn-scripts-assistant_*.deb' 'SHA256SUMS'

    command -v dpkg-deb >/dev/null || { warn "dpkg-deb missing; skipping the .deb"; return; }

    local stage="$ROOT/build/deb/pn-scripts-assistant_${VERSION}_amd64"

    log "Ubuntu/Debian package $VERSION"

    if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.1 2>/dev/null; then
        warn "libwebkit2gtk-4.1-dev and libgtk-3-dev are missing."
        warn "The .deb would have no native window. Install them and run again:"
        warn "  sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev"

        return
    fi

    rm -rf "$stage"
    mkdir -p "$stage/DEBIAN" "$stage/usr/bin" "$stage/usr/share/applications"

    for size in 48 64 128 256 512; do
        mkdir -p "$stage/usr/share/icons/hicolor/${size}x${size}/apps"

        local icon="$ROOT/clients/internal/brain/desktop/icons/pn-scripts-assistant-${size}.png"

        [ -f "$icon" ] && install -m 644 "$icon" \
            "$stage/usr/share/icons/hicolor/${size}x${size}/apps/pn-scripts-assistant.png"
    done

    # -buildmode=pie so the program is laid out somewhere different on every
    # run, which is what every other program Ubuntu ships does and the one
    # hardening measure that costs nothing to turn on.
    CGO_ENABLED=1 go build -C "$ROOT/clients" -trimpath -buildmode=pie -ldflags="-s -w $(version_stamp)" \
        -o "$stage/usr/bin/pn-scripts-assistant" ./cmd/pn-scripts-assistant || die "build failed"

    chmod 755 "$stage/usr/bin/pn-scripts-assistant"

    # What it actually links against, asked of the system rather than
    # remembered by hand.
    #
    # The list was written out once and would have been wrong the first time
    # anything changed — a program that links about sixty libraries
    # transitively cannot have its dependencies maintained by memory.
    # dpkg-shlibdeps reads the binary and names the packages that own the
    # libraries it needs; the hand-written pair stays as the fallback for a
    # machine without it, and both are recorded so the difference is visible.
    local depends="libwebkit2gtk-4.1-0, libgtk-3-0t64 | libgtk-3-0"

    if command -v dpkg-shlibdeps >/dev/null; then
        local computed
        computed="$(cd "$stage" && mkdir -p debian && : > debian/control &&
            dpkg-shlibdeps -O --ignore-missing-info "usr/bin/pn-scripts-assistant" 2>/dev/null |
            sed 's/^shlibs:Depends=//')"

        rm -rf "$stage/debian"

        if [ -n "$computed" ]; then
            depends="$computed"
            log "  dependencies computed from the binary"
        else
            warn "dpkg-shlibdeps said nothing; keeping the written-out dependencies"
        fi
    fi

    # Depends rather than Recommends for the window libraries: without them the
    # program runs but cannot open its own window, and somebody who installed a
    # desktop application has not asked for a thing that only serves a port.
    cat > "$stage/DEBIAN/control" <<CONTROL
Package: pn-scripts-assistant
Version: $VERSION
Section: utils
Priority: optional
Architecture: amd64
Depends: $depends
Recommends: xdotool, espeak-ng
Maintainer: Petar Nikolov <petar.v.nikolov@gmail.com>
Homepage: https://github.com/pnscripts/pn-scripts-assistant
Installed-Size: $(du -sk "$stage" | cut -f1)
Description: Private assistant that runs entirely on this machine
 PN Scripts Assistant is a personal, self-learning assistant. It keeps
 everything it learns in one folder on your own computer, runs its language
 model locally, and by default sends nothing anywhere.
 .
 Ollama and a language model are not included: setup checks for them on first
 run and installs them for you.
CONTROL

    cat > "$stage/usr/share/applications/pn-scripts-assistant.desktop" <<'DESKTOP'
[Desktop Entry]
Type=Application
Version=1.0
Name=PN Scripts Assistant
GenericName=AI Assistant
Comment=A private assistant that runs entirely on this machine
Exec=pn-scripts-assistant
Icon=pn-scripts-assistant
Terminal=false
Categories=Utility;
Keywords=assistant;brain;voice;memory;
StartupNotify=true
StartupWMClass=pn-scripts-assistant
Actions=setup;

[Desktop Action setup]
Name=Set up again
Exec=pn-scripts-assistant setup
DESKTOP

    # 0644, not whatever the umask left: a desktop entry is read by the
    # desktop for every user on the machine, and group-writable is a finding
    # in every checker there is.
    chmod 644 "$stage/usr/share/applications/pn-scripts-assistant.desktop"

    if command -v desktop-file-validate >/dev/null; then
        desktop-file-validate "$stage/usr/share/applications/pn-scripts-assistant.desktop" \
            || die "the desktop entry is not valid"
    fi

    deb_documents "$stage"

    mkdir -p "$stage/usr/share/man/man1"
    gzip -9nc "$ROOT/packaging/pn-scripts-assistant.1" \
        > "$stage/usr/share/man/man1/pn-scripts-assistant.1.gz"
    chmod 644 "$stage/usr/share/man/man1/pn-scripts-assistant.1.gz"

    # The umask on this machine leaves directories group-writable, and what
    # the archive carries is what lands on somebody else's machine.
    find "$stage/usr" -type d -exec chmod 755 {} +

    # md5sums, so dpkg can say afterwards which installed file has been
    # changed. Every package built by the usual tools has one; this was built
    # by hand and did not.
    ( cd "$stage" && find usr -type f -print0 | sort -z |
        xargs -0 md5sum > DEBIAN/md5sums ) 2>/dev/null || true

    fakeroot dpkg-deb --build "$stage" "$OUT/pn-scripts-assistant_${VERSION}_amd64.deb" >/dev/null \
        || die "dpkg-deb failed"

    log "  $OUT/pn-scripts-assistant_${VERSION}_amd64.deb"

    if command -v lintian >/dev/null; then
        log "  lintian:"
        lintian --tag-display-limit 0 "$OUT/pn-scripts-assistant_${VERSION}_amd64.deb" 2>&1 |
            sed 's/^/    /' || true
    else
        warn "  lintian is not installed; the package was not checked"
    fi
}

# ---------------------------------------------------------------------------
# What goes in the box beside the program
# ---------------------------------------------------------------------------
#
# Neither the macOS bundle nor the Windows executable is signed, and both
# systems react to that with a dialog that reads like a virus warning. Somebody
# meeting it with no explanation reasonably concludes the download is unsafe
# and deletes it, so the explanation ships in the package rather than living on
# a website they would have to find.
prepare_notes() {
    mkdir -p "$ROOT/build/macos" "$ROOT/build/windows"

    cat > "$ROOT/build/macos/README.txt" <<'MACNOTE'
PN Scripts Assistant for macOS
==================

Open the .dmg and drag "PN Scripts Assistant.app" onto the Applications
shortcut beside it, then open it from Applications.

The .tar.gz beside it holds the same app for anybody who would rather not
mount a disk image: unpack it and move the app into Applications yourself.

The first time, macOS will refuse
---------------------------------
It will say the app "cannot be opened because it is from an unidentified
developer". That is not a fault and not a warning about this program in
particular: it means the app has not been signed with an Apple Developer
certificate, which costs money and identifies a company.

To open it anyway: right-click (or Control-click) the app and choose Open,
then Open again in the dialog. You only do this once.

What happens when it starts
---------------------------
PN Scripts Assistant checks what your machine has and opens setup in your browser. It
will ask you to install Ollama, which it cannot install for you on macOS —
get it from https://ollama.com/download, then continue. Everything after
that, including the language model, it does itself.

There is no separate window on macOS yet. PN Scripts Assistant runs and shows its
interface in your browser.

Where your data goes
--------------------
Everything it learns lives in one folder, and setup lets you choose which.
Nothing is sent anywhere unless you turn that on. The language models are
kept by Ollama in ~/.ollama/models, which is separate from that folder.
MACNOTE

    cat > "$ROOT/build/windows/README.txt" <<'WINNOTE'
PN Scripts Assistant for Windows
====================

There are two downloads and they hold the same program.

  ...-windows-setup.exe   the installer. It puts the program in your own
                          account, adds it to the Start menu, asks for no
                          administrator password, and can remove itself
                          again from Settings.

  ...-windows-amd64.zip   this folder. Put it anywhere you like and run
                          pn-scripts-assistant.exe. Nothing is installed and
                          nothing is written outside the brain's own folder.

The first time, Windows will refuse
-----------------------------------
SmartScreen will say "Windows protected your PC". That is not a fault and
not a warning about this program in particular: it means the file has not
been signed with a code-signing certificate, which costs money and
identifies a company.

To run it anyway: click "More info", then "Run anyway". You only do this
once.

What happens when it starts
---------------------------
PN Scripts Assistant checks what your machine has and opens setup in your browser. It
will ask you to install Ollama, which it cannot install for you on Windows
-- get it from https://ollama.com/download, then continue. Everything after
that, including the language model, it does itself.

There is no separate window on Windows yet. PN Scripts Assistant runs and shows its
interface in your browser.

Where your data goes
--------------------
Everything it learns lives in one folder, and setup lets you choose which.
Nothing is sent anywhere unless you turn that on. The language models are
kept by Ollama in its own folder, which is separate from that one.
WINNOTE
}

# ---------------------------------------------------------------------------
# macOS
# ---------------------------------------------------------------------------
#
# A .app is a directory with a particular shape, so it can be assembled from
# here even though it can only be run over there. Both architectures, because
# an Intel-only build on Apple silicon runs under translation and an
# arm64-only one does not run at all on the older machines.
#
# Not signed and not notarised. That needs an Apple Developer account and the
# key belonging to whoever publishes this, so the first launch requires
# right-click → Open. Said plainly in the README beside it rather than left to
# be discovered as a scary dialog.
build_macos() {
    clear_old '*-macos-*'

    local arch
    for arch in amd64 arm64; do
        local app="$ROOT/build/macos/$arch/PN Scripts Assistant.app"

        log "macOS bundle ($arch)"

        # The whole staging folder, not just the bundle inside it.
        #
        # The disk image drops an Applications shortcut in here — that is what
        # makes it an install rather than a download — and clearing only the
        # .app left the shortcut behind for the next build to find. The tarball
        # is made from this folder before the image is, so a second build of
        # the same commit produced a .tar.gz with an absolute symlink to
        # /Applications in it that the first one did not. A build that differs
        # from itself is exactly what a reproducible release is checked for.
        rm -rf "$ROOT/build/macos/$arch"
        mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

        # CGO off: the window layer is Linux-only, so this serves its interface
        # and opens the system browser. Cross-compiling cgo would need an Apple
        # SDK on this machine and would still not add a window.
        GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 \
            go build -C "$ROOT/clients" -trimpath -ldflags="-s -w $(version_stamp)" \
            -o "$app/Contents/MacOS/pn-scripts-assistant" ./cmd/pn-scripts-assistant || die "darwin/$arch build failed"

        chmod 755 "$app/Contents/MacOS/pn-scripts-assistant"

        cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>PN Scripts Assistant</string>
  <key>CFBundleDisplayName</key><string>PN Scripts Assistant</string>
  <key>CFBundleIdentifier</key><string>com.pnscripts.pn-scripts-assistant</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>pn-scripts-assistant</string>
  <key>CFBundleIconFile</key><string>pn-scripts-assistant</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <!-- It serves its interface to a browser rather than drawing a window, so
       it has no dock icon of its own to manage. -->
  <key>LSBackgroundOnly</key><false/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

        [ -f "$ROOT/assets/pn-scripts-assistant.png" ] &&
            cp "$ROOT/assets/pn-scripts-assistant.png" "$app/Contents/Resources/pn-scripts-assistant.png"

        cp "$ROOT/build/macos/README.txt" "$ROOT/build/macos/$arch/" 2>/dev/null || true

        tar -czf "$OUT/pn-scripts-assistant-${VERSION}-macos-${arch}.tar.gz" \
            -C "$ROOT/build/macos/$arch" . || die "packaging darwin/$arch failed"

        log "  $OUT/pn-scripts-assistant-${VERSION}-macos-${arch}.tar.gz"

        disk_image "$arch"
    done
}

# ---------------------------------------------------------------------------
# The disk image, which is how a program arrives on a Mac
# ---------------------------------------------------------------------------
#
# A .tar.gz is a developer's answer. What somebody expects is a window with
# the application on one side and a shortcut to Applications on the other, and
# dragging one onto the other. So: a .dmg containing the bundle and that
# shortcut.
#
# Built with hdiutil on a Mac and with xorriso anywhere else. The second is
# not a lesser image — a .dmg is a disk image and macOS mounts a plain ISO9660
# one perfectly well — but it is worth saying which made it, because if a Mac
# ever refuses one of these that is the first thing to know.
#
# Nothing here is signed or notarised. macOS will say so: Gatekeeper refuses
# an unsigned application on first open and it takes a right-click and Open to
# pass. That is the honest state of things — notarisation needs a paid Apple
# developer account — and the documentation says the same rather than leaving
# somebody to meet it alone.
disk_image() {
    local arch="$1"
    local staged="$ROOT/build/macos/$arch"
    local dmg="$OUT/pn-scripts-assistant-${VERSION}-macos-${arch}.dmg"

    # The shortcut that makes it an install rather than a download.
    ln -sfn /Applications "$staged/Applications" 2>/dev/null || true

    if command -v hdiutil >/dev/null; then
        rm -f "$dmg"

        hdiutil create -volname "PN Scripts Assistant" -srcfolder "$staged" \
            -ov -format UDZO "$dmg" >/dev/null || die "hdiutil failed"

        log "  $dmg (hdiutil)"

        return
    fi

    if command -v xorriso >/dev/null; then
        rm -f "$dmg"

        xorriso -as mkisofs -quiet -V "PN Scripts Assistant" -r -J \
            -o "$dmg" "$staged" || die "xorriso failed"

        log "  $dmg (xorriso; built off a Mac)"

        return
    fi

    warn "  no hdiutil and no xorriso: the macOS disk image was not built"
}

# archive_zip puts a folder into a zip, with whatever this machine has.
#
# Three ways of doing it because the Windows runner is the one place this has
# to work and is the one place `zip` may not exist: Git Bash does not ship it,
# and the build that produces the Windows download failing for want of a
# Unix archiver would be a silly way to lose a release. 7-Zip is on every
# Windows image, and PowerShell's own Compress-Archive is on every Windows.
archive_zip() {
    local parent="$1" folder="$2" into="$3"

    if command -v zip >/dev/null; then
        ( cd "$parent" && zip -qr "$into" "$folder" )

        return
    fi

    if command -v 7z >/dev/null; then
        ( cd "$parent" && 7z a -tzip -bso0 -bsp0 "$into" "$folder" >/dev/null )

        return
    fi

    if command -v powershell >/dev/null; then
        local from="$parent/$folder" to="$into"

        if command -v cygpath >/dev/null; then
            from="$(cygpath -w "$from")"
            to="$(cygpath -w "$to")"
        fi

        powershell -NoProfile -Command \
            "Compress-Archive -Path '$from' -DestinationPath '$to' -Force"

        return
    fi

    warn "  nothing here can make a zip (zip, 7z, powershell)"

    return 1
}

# ---------------------------------------------------------------------------
# Windows
# ---------------------------------------------------------------------------
#
# A folder with the .exe in it, zipped, and then an installer built from that
# same folder — see windows_installer below.
#
# The zip came first and this used to say an installer was deliberately not
# built, on the reasoning that an unsigned installer asks for more trust than
# an unsigned executable while offering the same binary. That is true and it is
# not a reason to ship no installer: "unzip this and find the exe" is not how a
# program arrives on Windows, and somebody who wants the plain executable still
# has the zip. The lack of a signature is said plainly in the installer and in
# the documentation instead of being worked around by leaving the installer out.
build_windows() {
    clear_old '*-windows-*'

    local dir="$ROOT/build/windows/PN-Scripts-Assistant"

    log "Windows package"

    rm -rf "$dir"
    mkdir -p "$dir"

    # -H windowsgui: linked as a GUI program, so double-clicking it does not
    # put a black console window behind the browser on every single launch.
    # The cost is that stderr goes nowhere, which is why the one message that
    # must not be lost — setup could not be opened — is a message box instead.
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
        go build -C "$ROOT/clients" -trimpath -ldflags="-s -w -H windowsgui $(version_stamp)" \
        -o "$dir/pn-scripts-assistant.exe" ./cmd/pn-scripts-assistant || die "windows build failed"

    cp "$ROOT/build/windows/README.txt" "$dir/" 2>/dev/null || true

    # Removed first: zip adds to an archive that already exists rather than
    # replacing it, so building the same version twice would otherwise produce
    # an archive holding both builds.
    rm -f "$OUT/pn-scripts-assistant-${VERSION}-windows-amd64.zip"

    archive_zip "$ROOT/build/windows" "PN-Scripts-Assistant" \
        "$OUT/pn-scripts-assistant-${VERSION}-windows-amd64.zip" || die "zip failed"

    log "  $OUT/pn-scripts-assistant-${VERSION}-windows-amd64.zip"

    windows_installer "$dir"
}

# ---------------------------------------------------------------------------
# The Windows installer
# ---------------------------------------------------------------------------
#
# The zip above is still built and still useful — somebody who wants one file
# and no installation gets exactly that. But "unzip this and find the exe" is
# not how a program arrives on Windows, and the thing most people want is a
# setup that puts it in the Start menu and can uninstall itself.
#
# Inno Setup compiles it, which means this only runs where iscc exists: a
# Windows machine, or the Windows runner in CI. Everywhere else it says so
# and carries on, because a Linux machine not producing a Windows installer is
# not a failed build.
#
# Unsigned, like everything else here, and the script says so where somebody
# will read it.
windows_installer() {
    local from="$1"
    local script="$ROOT/packaging/windows/pn-scripts-assistant.iss"

    local iscc=""

    for candidate in iscc ISCC.exe "/c/Program Files (x86)/Inno Setup 6/ISCC.exe"; do
        if command -v "$candidate" >/dev/null 2>&1; then
            iscc="$candidate"

            break
        fi
    done

    if [ -z "$iscc" ]; then
        warn "  Inno Setup (iscc) is not here, so no Windows installer was built"
        warn "  the zip above is the whole program; CI builds the installer"

        return
    fi

    # Real Windows paths, and Git Bash told to leave the arguments alone.
    #
    # It rewrites anything that looks like a Unix path on its way to a native
    # program, which is right for `go build -C` and wrong here: "/DVersion=..."
    # looks like a path to it and arrives at the compiler mangled. cygpath
    # gives the paths in the form ISCC wants; MSYS2_ARG_CONV_EXCL stops the
    # switches being touched at all.
    local from_win="$from" out_win="$OUT" script_win="$script"

    if command -v cygpath >/dev/null; then
        from_win="$(cygpath -w "$from")"
        out_win="$(cygpath -w "$OUT")"
        script_win="$(cygpath -w "$script")"
    fi

    MSYS2_ARG_CONV_EXCL='*' "$iscc" \
        "/DVersion=$VERSION" "/DSourceDir=$from_win" "/O$out_win" "$script_win" >/dev/null ||
        die "the Windows installer would not compile"

    log "  $OUT/pn-scripts-assistant-${VERSION}-windows-setup.exe"
}

case "$want" in
    deb)     build_deb ;;
    macos)   prepare_notes; build_macos ;;
    windows) prepare_notes; build_windows ;;
    all)     build_deb; prepare_notes; build_macos; build_windows ;;
    *)       die "unknown target: $want (deb, macos, windows, all)" ;;
esac

log "Done. Packages in $OUT"
ls -lh "$OUT" 2>/dev/null | tail -n +2 | awk '{printf "    %-52s %s\n", $9, $5}'
