//go:build !linux && !windows

package programs

import (
	"context"
	"errors"
)

// addWatch has nothing to do here; it exists so tests build everywhere.
var addWatch = func(int, string, uint32) (int, error) { return -1, errors.ErrUnsupported }

// newEvents: no file events on this system (macOS, the BSDs) yet, so
// every target is polled.
func newEvents(*Watching) (events, error) { return nil, errors.ErrUnsupported }

func readEvents(context.Context, *Watching, events) {}
