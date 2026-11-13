//go:build !linux

package programs

import (
	"context"
	"errors"
)

// addWatch has nothing to do here; it exists so tests build everywhere.
var addWatch = func(int, string, uint32) (int, error) { return -1, errors.ErrUnsupported }

// newEvents: no file events on this system yet (Windows gets
// ReadDirectoryChangesW with its own sources), so every target is polled.
func newEvents() (events, error) { return nil, errors.ErrUnsupported }

func readEvents(context.Context, *Watching, events) {}
