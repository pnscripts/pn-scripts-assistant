package latest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Channel is how this copy was installed, which decides how it is updated.
type Channel string

const (
	// Source is a copy built by hand from a checkout: selfupdate's business.
	Source Channel = "source"

	// Apt is the .deb with this program's apt repository configured: Software
	// Updater and `apt upgrade` bring new versions.
	Apt Channel = "apt"

	// Deb is the .deb whose repository file was removed — its owner said no
	// to updates through apt, so a new version is a download they choose.
	Deb Channel = "deb"

	// Snap is installed from the Snap Store, which refreshes it by itself.
	Snap Channel = "snap"

	// Installer is the Windows setup: a new version is the new setup, run
	// silently over the old one.
	Installer Channel = "installer"

	// Portable is a loose copy — the Windows zip, or the bare Linux binary —
	// that nothing installed and nothing may quietly replace.
	Portable Channel = "portable"
)

// AptSources is the file the .deb installs to keep its repository.
const AptSources = "/etc/apt/sources.list.d/pn-scripts-assistant.sources"

// How works out the channel of the running copy. version is the released
// version it was built as, empty for a build by hand.
func How(version string) Channel {
	exe, _ := os.Executable()

	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	return how(version, runtime.GOOS, exe, os.Getenv("SNAP_NAME"), exists, dpkgHasIt)
}

/*
 * dpkgHasIt asks dpkg whether the package is installed — through
 * dpkg-query, the interface dpkg offers for that, rather than reading its
 * database files, which are dpkg's own business.
 */
func dpkgHasIt() bool {
	out, err := exec.Command("dpkg-query", "-W", "-f=${Status}", Repo).Output()

	return err == nil && strings.HasSuffix(strings.TrimSpace(string(out)), " installed")
}

func how(version, goos, exe, snapName string, present func(string) bool, packaged func() bool) Channel {
	if strings.TrimSpace(version) == "" {
		return Source
	}

	if snapName == Repo {
		return Snap
	}

	switch goos {
	case "windows":
		// Inno Setup leaves its uninstaller beside what it installed; the zip
		// has none.
		if present(filepath.Join(filepath.Dir(exe), "unins000.exe")) {
			return Installer
		}

		return Portable

	case "linux":
		if exe == "/usr/bin/"+Repo && packaged() {
			if present(AptSources) {
				return Apt
			}

			return Deb
		}
	}

	return Portable
}

func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// AssetFor is the release file a channel installs from, or empty when the
// channel does not download anything itself.
func AssetFor(c Channel, goos, version string) string {
	switch {
	case c == Installer:
		return "pn-scripts-assistant-" + version + "-windows-setup.exe"

	case c == Portable && goos == "windows":
		return "pn-scripts-assistant-" + version + "-windows-amd64.zip"

	case c == Deb:
		return "pn-scripts-assistant_" + version + "_amd64.deb"

	case c == Portable && goos == "linux":
		return "pn-scripts-assistant-linux-amd64"
	}

	return ""
}

// Outcome is what an update did, or what the person does next.
type Outcome struct {
	// Started is true when an installer is running and the program should
	// close so it can be replaced.
	Started bool `json:"started,omitempty"`

	// Opened names a program that was opened for the person (Software
	// Updater).
	Opened string `json:"opened,omitempty"`

	// Command is a line to run when nothing could be opened.
	Command string `json:"command,omitempty"`

	// Open is a page to show: the release, for a copy nothing may replace.
	Open string `json:"open,omitempty"`

	// Say is the sentence for the person, in English; Key names it for the
	// page's own translation.
	Say string `json:"say"`
	Key string `json:"key"`
}

// InstallerArgs are what the Windows setup is started with when the person
// pressed "Update": a progress window and no questions, the running copy
// closed, and the new one opened again at the end (RELAUNCH, read by the
// installer's own script). winget and silent installs leave RELAUNCH out.
var InstallerArgs = []string{"/SILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/CLOSEAPPLICATIONS", "/RELAUNCH=1"}

// QuietArgs install with no window at all and leave the program closed: the
// command line's `update`, which is what an unattended machine runs.
var QuietArgs = []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/CLOSEAPPLICATIONS"}

// InstallQuietly fetches the Windows installer of rel, checks it and starts it
// with QuietArgs, without waiting: the program that started it has to be gone
// before the installer can replace it.
func InstallQuietly(ctx context.Context, rel Release, dir string) error {
	return runInstaller(ctx, rel, dir, QuietArgs)
}

func runInstaller(ctx context.Context, rel Release, dir string, args []string) error {
	path, err := Fetch(ctx, nil, rel, AssetFor(Installer, "windows", rel.Version), dir)
	if err != nil {
		return err
	}

	if err := start(path, args...); err != nil {
		return fmt.Errorf("the installer could not be started: %w", err)
	}

	return nil
}

// start runs a program without waiting for it. A variable for the tests.
var start = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Process.Release()
}

// lookPath finds a program. A variable for the tests.
var lookPath = exec.LookPath

/*
 * Apply updates this copy the way it was installed.
 *
 * Only the Windows installer downloads anything: it is fetched, checked
 * against SHA256SUMS and started, and the caller then closes the program.
 * Everything a package manager installed is left to the package manager.
 */
func Apply(ctx context.Context, c Channel, rel Release, dir string) (Outcome, error) {
	switch c {
	case Installer:
		if err := runInstaller(ctx, rel, dir, InstallerArgs); err != nil {
			return Outcome{}, err
		}

		return Outcome{
			Started: true,
			Key:     "upd.installing",
			Say:     fmt.Sprintf("Installing %s. The assistant closes now and opens again when it is done.", rel.Version),
		}, nil

	case Apt:
		if updater, err := lookPath("update-manager"); err == nil {
			if err := start(updater); err == nil {
				return Outcome{
					Opened: "Software Updater",
					Key:    "upd.updater",
					Say:    "Software Updater is open: choose Install Now there.",
				}, nil
			}
		}

		return Outcome{
			Command: "sudo apt update && sudo apt upgrade",
			Key:     "upd.aptCommand",
			Say:     "Run this in a terminal to update:",
		}, nil

	case Snap:
		return Outcome{
			Command: "snap refresh " + Repo,
			Key:     "upd.snap",
			Say:     "The Snap Store updates it by itself. To update now, run:",
		}, nil

	case Deb, Portable:
		return Outcome{
			Open: rel.Page,
			Key:  "upd.download",
			Say:  "Nothing installed this copy, so nothing replaces it by itself: download the new version from the release page.",
		}, nil
	}

	return Outcome{}, errors.New("this copy was built by hand; update it from its checkout")
}
