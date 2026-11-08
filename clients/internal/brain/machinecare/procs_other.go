//go:build !linux

package machinecare

import "errors"

// ErrNotHere is a look this system does not offer yet.
var ErrNotHere = errors.New("this is only looked at on Linux so far")

// ErrNotALeftover is a process that is not, or no longer, on the list.
var ErrNotALeftover = errors.New("that process is not one of the leftovers; nothing else is stopped from here")

// Processes is not read on this system yet.
func Processes() ([]Proc, error) { return nil, ErrNotHere }

// Stop is not offered on this system yet.
func Stop(int, string) error { return ErrNotHere }
