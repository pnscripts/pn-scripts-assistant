//go:build windows

package window

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
 * Saying something on a system where there is nowhere to print it.
 *
 * The Windows build is linked as a GUI program so that double-clicking it does
 * not put a black console window behind the browser on every launch. That is
 * the right look and it removes the only place this program had to speak: with
 * no console attached, everything written to stderr goes nowhere at all.
 *
 * So the one case that must never be silent — setup could not be opened — gets
 * a message box. Through syscall rather than cgo, because user32 is always
 * present on Windows and adding a C toolchain to reach it would be a large
 * price for one dialog.
 */
func Alert(title, message string) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	box := user32.NewProc("MessageBoxW")

	head, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	body, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}

	// MB_OK | MB_ICONINFORMATION, and no owner window because there is none.
	box.Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(head)), 0x40)
}
