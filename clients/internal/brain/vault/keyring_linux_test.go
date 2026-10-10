//go:build linux && cgo

package vault

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

/*
 * The real Secret Service, through libsecret. Only when asked for, and only
 * inside a session bus of its own with a keyring daemon of its own (see the
 * ADR for the command), because the session's keyring is its owner's.
 */
func TestTheSecretServiceKeepsAKey(t *testing.T) {
	if os.Getenv("PN_LIVE_KEYRING") == "" {
		t.Skip("set PN_LIVE_KEYRING=1, inside dbus-run-session with a scratch gnome-keyring-daemon")
	}

	ring, ok := platformKeyring()
	if !ok {
		t.Fatal("libsecret or the session bus is not here")
	}

	k := key(t)

	if _, err := ring.Get(k.ID()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a key never kept: %v", err)
	}

	if err := ring.Set(k.ID(), k.Raw()); err != nil {
		t.Fatal(err)
	}

	got, err := ring.Get(k.ID())
	if err != nil || !bytes.Equal(got, k.Raw()) {
		t.Fatalf("read back: %v", err)
	}

	if err := ring.Delete(k.ID()); err != nil {
		t.Fatal(err)
	}

	if _, err := ring.Get(k.ID()); !errors.Is(err, ErrNotFound) {
		t.Errorf("still there after delete: %v", err)
	}
}
