// Package exe finds the helper programs the brain runs.
//
// Not exec.LookPath alone, because PATH is not the same thing depending on how
// this program was started. A shell sources a profile and ends up with
// ~/.local/bin on it; an application launched from the desktop menu is started
// by the session and gets a short, standard PATH with none of that.
//
// The symptom is precise and baffling: the brain hears perfectly when started
// from a terminal and reports "listening unavailable" when started from the
// menu, on the same machine, with the same files on disk. It appeared here the
// moment the menu entry was added, because that was the first time this program
// had ever been started by anything but a shell.
package exe

import (
	"os"
	"os/exec"
	"path/filepath"
)

/*
 * alsoLook are the places people put programs they built themselves.
 *
 * whisper.cpp and piper are both compiled by hand more often than installed
 * from a package, and they land in one of these. Searched after PATH, so a
 * properly installed copy still wins.
 */
func alsoLook() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	return []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
		filepath.Join(home, ".local", "src", "whisper.cpp", "build", "bin"),
		filepath.Join(home, ".local", "src", "whisper.cpp"),
		filepath.Join(home, "whisper.cpp", "build", "bin"),
		filepath.Join(home, ".local", "share", "piper"),
		"/usr/local/bin",
		"/opt/homebrew/bin",
	}
}

/*
 * Look finds the first of these programs that exists and can be run.
 *
 * Returns the full path, so whatever runs it does not depend on PATH either —
 * a child process started from here inherits the same short PATH and would
 * fail the same way.
 */
func Look(names ...string) (string, bool) {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, true
		}
	}

	for _, dir := range alsoLook() {
		for _, name := range names {
			path := filepath.Join(dir, name)

			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}

			// Executable by somebody, which is as much as can be checked
			// without trying to run it.
			if info.Mode().Perm()&0o111 == 0 {
				continue
			}

			return path, true
		}
	}

	return "", false
}

// Has reports whether any of these programs can be found.
func Has(names ...string) bool {
	_, found := Look(names...)

	return found
}
