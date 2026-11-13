//go:build windows

package programs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// windowsHere fills in this user's Start menu and the registry.
func windowsHere(s *Scanner) {
	s.Windows = true
	s.Registry = readRegistry

	if d := os.Getenv("APPDATA"); d != "" {
		s.StartMenus = append(s.StartMenus, filepath.Join(d, "Microsoft", "Windows", "Start Menu", "Programs"))
	}

	if d := os.Getenv("ProgramData"); d != "" {
		s.StartMenus = append(s.StartMenus, filepath.Join(d, "Microsoft", "Windows", "Start Menu", "Programs"))
	}

	s.LocalAppData = os.Getenv("LOCALAPPDATA")
}

// noWindow: PowerShell is a console program; started from the window
// (a GUI program) it would flash a console of its own.
func noWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// openKey opens HKLM\… or HKCU\… for reading, the 64-bit view (the
// 32-bit one is named by its WOW6432Node path).
func openKey(path string, access uint32) (registry.Key, error) {
	root, rest, _ := strings.Cut(path, `\`)

	var base registry.Key

	switch root {
	case "HKLM":
		base = registry.LOCAL_MACHINE
	case "HKCU":
		base = registry.CURRENT_USER
	default:
		return 0, errors.New("no such registry root: " + root)
	}

	k, err := registry.OpenKey(base, rest, access|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return 0, os.ErrNotExist
	}

	return k, err
}

// mostSubkeys bounds one key's listing.
const mostSubkeys = 20000

// readRegistry lists a key's subkeys with their string and number
// values. A subkey that cannot be opened (another user's, being written)
// is left out; the key itself failing is an error.
func readRegistry(path string) ([]RegEntry, error) {
	k, err := openKey(path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}

	defer k.Close()

	names, err := k.ReadSubKeyNames(mostSubkeys)
	if err != nil && !errors.Is(err, errNoMoreItems) {
		return nil, err
	}

	out := make([]RegEntry, 0, len(names))

	for _, name := range names {
		sub, err := registry.OpenKey(k, name, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			continue
		}

		e := RegEntry{Key: name, Values: map[string]any{}}

		values, _ := sub.ReadValueNames(0)
		for _, v := range values {
			if s, _, err := sub.GetStringValue(v); err == nil {
				e.Values[v] = s
			} else if n, _, err := sub.GetIntegerValue(v); err == nil {
				e.Values[v] = n
			}
		}

		sub.Close()

		out = append(out, e)
	}

	return out, nil
}

var errNoMoreItems = windows.ERROR_NO_MORE_ITEMS

// keyStamp is when a key last changed, for the poll; false when it is not
// there.
func keyStamp(path string) (string, bool) {
	k, err := openKey(path, registry.QUERY_VALUE)
	if err != nil {
		return "-", false
	}

	defer k.Close()

	info, err := k.Stat()
	if err != nil {
		return "-", false
	}

	return info.ModTime().UTC().Format(time.RFC3339Nano) + "/" + strconv.FormatUint(uint64(info.SubKeyCount), 10), true
}
