//go:build linux && cgo

package window

// #include <stdlib.h>
// int pnbrain_embed_surface(unsigned long child, int x, int y, int width, int height);
// void pnbrain_place_surface(unsigned long child, int x, int y, int width, int height);
// void pnbrain_show_surface(unsigned long child, int visible);
import "C"

// Embed makes another program's window a child of this one.
//
// Reports whether it worked. It does not when the window has not been realised
// yet — the visual can start before the window exists — in which case the
// caller should try again rather than treat it as a failure.
func Embed(child uint64, x, y, width, height int) bool {
	return C.pnbrain_embed_surface(C.ulong(child),
		C.int(x), C.int(y), C.int(width), C.int(height)) != 0
}

// Place moves an embedded window to where the page says its panel is.
func Place(child uint64, x, y, width, height int) {
	C.pnbrain_place_surface(C.ulong(child),
		C.int(x), C.int(y), C.int(width), C.int(height))
}

// Show maps or unmaps an embedded window.
//
// Used when the page moves to a view that does not contain the panel. Leaving
// it mapped would leave a rectangle of unrelated graphics over whatever is
// there instead, since the window knows nothing about the page's layout.
func Show(child uint64, visible bool) {
	shown := 0

	if visible {
		shown = 1
	}

	C.pnbrain_show_surface(C.ulong(child), C.int(shown))
}
