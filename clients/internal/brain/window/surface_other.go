//go:build !linux || !cgo

package window

// Embedding another program's window is an X11 arrangement.
//
// On other platforms the interface falls back to drawing the same things
// itself, which is what it did before any of this existed and is why that code
// is still there rather than having been deleted.
func Embed(child uint64, x, y, width, height int) bool { return false }

// Place does nothing where embedding is not possible.
func Place(child uint64, x, y, width, height int) {}

// Show does nothing where embedding is not possible.
func Show(child uint64, visible bool) {}
