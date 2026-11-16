//go:build !unix

package askclaude

// noFollow: this system has no such flag for opening; the file opened is
// compared with the one looked at instead (readWhole).
const noFollow = 0
