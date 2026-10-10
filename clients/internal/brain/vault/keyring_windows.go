//go:build windows

package vault

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
 * DPAPI: the key is encrypted by Windows for this user on this machine and
 * the result kept in a file. Another user, another machine, or the file on
 * its own cannot open it.
 */
type dpapi struct{ dir string }

func platformKeyring() (Keyring, bool) { return dpapi{dir: keyFolder()}, true }

func (dpapi) Name() string    { return "Windows (DPAPI, for your account on this computer)" }
func (dpapi) Warning() string { return "" }

func (d dpapi) path(id string) string { return filepath.Join(d.dir, id+".dpapi") }

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}

	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func (d dpapi) Get(id string) ([]byte, error) {
	sealed, err := os.ReadFile(d.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	var out windows.DataBlob

	if err := windows.CryptUnprotectData(blob(sealed), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}

	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))

	return append([]byte{}, unsafe.Slice(out.Data, out.Size)...), nil
}

func (d dpapi) Set(id string, secret []byte) error {
	var out windows.DataBlob

	if err := windows.CryptProtectData(blob(secret), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return err
	}

	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))

	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return err
	}

	return WriteAtomic(d.path(id), append([]byte{}, unsafe.Slice(out.Data, out.Size)...), 0o600)
}

func (d dpapi) Delete(id string) error {
	if err := os.Remove(d.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
