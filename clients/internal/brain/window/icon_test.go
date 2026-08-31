//go:build linux && cgo

package window

import (
	"path/filepath"
	"testing"
)

/*
 * A build from source finds its icon.
 *
 * The path used to be read straight from APPDIR, which only exists inside an
 * AppImage — so every build from source ran with the blank default icon,
 * including the setup window, which is the very first thing anybody sees on a
 * new machine and the one most worth looking like it belongs to something.
 */
func TestTheIconIsFoundWhereverItIs(t *testing.T) {
	const (
		appdir = "/opt/appimage"
		home   = "/home/petar"
	)

	// A build from source: dist/pn-brain, with assets/pn-brain.png beside it.
	source := "/work/pn-brain/dist/pn-brain"
	wantSource := "/work/pn-brain/assets/pn-brain.png"

	only := func(want string) func(string) bool {
		return func(path string) bool { return filepath.Clean(path) == want }
	}

	if got := findIcon("", source, home, only(wantSource)); got != wantSource {
		t.Errorf("a build from source found %q, want %q", got, wantSource)
	}

	// An AppImage: its own directory wins, because that is the packaged mark.
	wantPackaged := filepath.Join(appdir, "pn-brain.png")

	if got := findIcon(appdir, source, home, only(wantPackaged)); got != wantPackaged {
		t.Errorf("an AppImage found %q, want %q", got, wantPackaged)
	}

	// Installed for the desktop, with nothing beside the binary.
	wantInstalled := filepath.Join(home, ".local", "share", "icons", "pn-brain.png")

	if got := findIcon("", "/usr/bin/pn-brain", home, only(wantInstalled)); got != wantInstalled {
		t.Errorf("an installed copy found %q, want %q", got, wantInstalled)
	}

	// And nothing anywhere is not a failure: a window with no icon still opens.
	none := func(string) bool { return false }

	if got := findIcon(appdir, source, home, none); got != "" {
		t.Errorf("invented an icon at %q when none exists", got)
	}
}
