//go:build darwin

package vault

import (
	"encoding/hex"
	"errors"
	"os/exec"
	"strings"
)

// The macOS Keychain, through the security command every Mac has.
type keychain struct{}

const keychainService = "PN Scripts Assistant"

func platformKeyring() (Keyring, bool) {
	_, err := exec.LookPath("security")

	return keychain{}, err == nil
}

func (keychain) Name() string    { return "the macOS Keychain" }
func (keychain) Warning() string { return "" }

func (keychain) Get(id string) ([]byte, error) {
	out, err := exec.Command("security", "find-generic-password", "-a", id, "-s", keychainService, "-w").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 44 {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return hex.DecodeString(strings.TrimSpace(string(out)))
}

func (keychain) Set(id string, secret []byte) error {
	return exec.Command("security", "add-generic-password", "-U", "-a", id, "-s", keychainService,
		"-l", "PN Scripts Assistant brain key", "-w", hex.EncodeToString(secret)).Run()
}

func (keychain) Delete(id string) error {
	return exec.Command("security", "delete-generic-password", "-a", id, "-s", keychainService).Run()
}
