//go:build !linux || !cgo

package browse

import "fmt"

// RenderHere is not available in this build. Said plainly rather than silently
// returning nothing, because "the page was empty" and "this program cannot
// open pages" are different problems with different answers.
func RenderHere(string, int) error {
	return fmt.Errorf(
		"this build cannot open pages: it was built without WebKit, so only the " +
			"plain fetch is available")
}

// webKitHere reports whether this build has WebKit in it.
func webKitHere() bool { return false }

// SnapshotHere is not available in this build either.
func SnapshotHere(string, int, string, int, int) error {
	return fmt.Errorf("this build cannot run pages: it was built without WebKit")
}
