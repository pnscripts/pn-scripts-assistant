//go:build !windows

package window

// Alert has nothing to do where stderr is a real place. Linux and macOS both
// leave a console attached, and printing is both quieter and more useful than
// a dialog somebody has to dismiss.
func Alert(title, message string) {}
