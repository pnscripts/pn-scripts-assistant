//go:build !linux || !cgo

package window

import (
	"fmt"
	"runtime"
)

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

	return fmt.Errorf(
		"this build has no native window because it was compiled without cgo or "+
			"without the WebKit development headers.\n\n"+
			"  install them:  sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev\n"+
			"  then rebuild:  CGO_ENABLED=1 go build ./cmd/brain\n\n"+
			"In the meantime the brain is running: open %s", url)
}

// OpenWithNavigation matches the cgo build's signature so callers need no
// build tags of their own.
func OpenWithNavigation(url, title string, width, height int, navigate <-chan string) error {
	return Open(url, title, width, height)
}
