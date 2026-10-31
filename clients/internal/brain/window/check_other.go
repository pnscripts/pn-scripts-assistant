//go:build !(windows && amd64)

package window

import (
	"errors"
	"time"
)

// Check is the Windows window check; elsewhere there is nothing to check
// this way (the Linux window is checked by the package test in CI).
func Check(url string, within time.Duration) error {
	return errors.New("the window check is for the Windows build")
}

// Engine names what draws the window.
// Icon is said by window-check on Windows only; elsewhere the icon theme
// draws it.
func Icon() string { return "" }

func Engine() string {
	if Available() {
		return "WebKitGTK"
	}

	return "none: the browser shows the interface"
}
