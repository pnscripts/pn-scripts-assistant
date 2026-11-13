//go:build windows

package programs

// kindsHere: the Start menu first, then the installed programs, then the
// app packages (one PowerShell call, the slowest).
var kindsHere = []Kind{App, Win, Msix}
