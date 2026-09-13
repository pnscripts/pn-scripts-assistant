//go:build !linux || !cgo

package window

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Close does nothing in a build with no window to shut.
func Close() {}

// Present does nothing in a build with no window to bring forward.
func Present() {}

// Available reports whether a native window can be opened by this build.
func Available() bool { return false }

// Open explains what is missing rather than failing obscurely.
//
// The brain is useful without a window — it serves its interface over loopback
// and any browser can show it — so a build without one is a reduced program,
// not a broken one. The message says exactly what to install, because "cgo is
// disabled" means nothing to somebody who just wants their assistant.
func Open(url, title string, width, height int) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf(
			"a native window is not implemented for %s yet; open %s in a browser",
			runtime.GOOS, url)
	}

	/*
	 * Two different causes, and telling somebody the wrong one costs an hour.
	 *
	 * This used to say "install them: sudo apt install …" whichever it was.
	 * On a machine where the libraries are already installed — the ordinary
	 * case, because the window worked yesterday — that sends the reader to
	 * install what they have, and the actual cause goes unmentioned: the
	 * binary was built without cgo, which on a machine where `go env
	 * CGO_ENABLED` is 0 is what a plain `go build` quietly produces.
	 */
	if toolkitPresent() {
		return fmt.Errorf(
			"this build has no native window: the libraries are installed, but the "+
				"program was compiled without cgo.\n\n"+
				"  rebuild:  CGO_ENABLED=1 go build ./cmd/pn-scripts-assistant\n"+
				"  (a plain `go build` is enough only where `go env CGO_ENABLED` is 1)\n\n"+
				"In the meantime the brain is running: open %s", url)
	}

	return fmt.Errorf(
		"this build has no native window because the WebKit libraries are not "+
			"installed here.\n\n"+
			"  install them:  sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev\n"+
			"  then rebuild:  CGO_ENABLED=1 go build ./cmd/pn-scripts-assistant\n\n"+
			"In the meantime the brain is running: open %s", url)
}

/*
 * toolkitPresent reports whether this machine could show a window if the
 * program had been built to.
 *
 * The shared library rather than the development headers, and by looking for
 * the file rather than asking pkg-config: this runs while explaining a failure,
 * and an explanation that depends on another tool being installed is one more
 * thing that can leave somebody with no answer at all.
 */
func toolkitPresent() bool {
	for _, dir := range []string{
		"/lib/x86_64-linux-gnu", "/usr/lib/x86_64-linux-gnu",
		"/lib/aarch64-linux-gnu", "/usr/lib/aarch64-linux-gnu",
		"/usr/lib", "/usr/lib64", "/lib",
	} {
		for _, name := range []string{
			"libwebkit2gtk-4.1.so.0", "libwebkit2gtk-4.0.so.37",
		} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return true
			}
		}
	}

	return false
}

// OpenWithNavigation matches the cgo build's signature so callers need no
// build tags of their own.
func OpenWithNavigation(url, title string, width, height int, navigate <-chan string) error {
	return Open(url, title, width, height)
}
