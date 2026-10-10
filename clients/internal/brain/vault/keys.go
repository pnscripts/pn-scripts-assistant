package vault

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

/*
 * Where a brain's key is, and how it is found again.
 *
 * The data key is kept in this machine's keyring and nowhere else in the
 * clear. Beside the brain, in vault.json, is the same key wrapped with a
 * recovery key: thirty-two random bytes its owner is shown once and keeps.
 * The brain lives on a drive that travels (see paths), and on a machine whose
 * keyring has never seen it, the recovery key unwraps the data key and puts
 * it into that machine's keyring — after which it opens there as it does here.
 *
 * vault.json holds nothing that opens anything without the recovery key: the
 * wrapped key, the key's id, and — until its owner says he has kept it — the
 * recovery key itself, sealed with the data key, so it can be shown to him
 * once even across a restart and to nobody who does not already have the
 * data key.
 */

// FileName is the record beside a brain.
const FileName = "vault.json"

// ErrNeedsRecovery is a sealed brain whose key this machine does not have.
var ErrNeedsRecovery = errors.New("this brain is encrypted and this machine does not have its key yet; " +
	"it opens with the recovery key")

// ErrBadRecoveryKey is a recovery key that does not open this brain.
var ErrBadRecoveryKey = errors.New("that recovery key does not open this brain")

type record struct {
	Version  int    `json:"version"`
	KeyID    string `json:"key_id"`
	Recovery string `json:"recovery"`
	Unshown  string `json:"unshown,omitempty"`
	Created  string `json:"created"`
}

// Unlocked is what unlocking found out.
type Unlocked struct {
	Key *Key

	// Created is true when this brain was given its key just now.
	Created bool

	// Keyring is where the key is kept on this machine, in words.
	Keyring string

	// Warning is something its owner should know about where it is kept.
	Warning string
}

func recordPath(root string) string { return filepath.Join(root, FileName) }

func readRecord(root string) (*record, error) {
	raw, err := os.ReadFile(recordPath(root))
	if err != nil {
		return nil, err
	}

	var r record

	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%s is damaged: %w", FileName, err)
	}

	return &r, nil
}

func writeRecord(root string, r *record) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}

	return WriteAtomic(recordPath(root), append(raw, '\n'), 0o600)
}

// Exists reports whether a brain has been given a key.
func Exists(root string) bool {
	_, err := os.Stat(recordPath(root))

	return err == nil
}

/*
 * Unlock finds a brain's key on this machine, or gives a brain without one
 * its key. mayCreate is false when the brain's data is already sealed and its
 * record is missing: a new key then would make what is there unreadable.
 */
func Unlock(root string, ring Keyring, mayCreate bool) (Unlocked, error) {
	r, err := readRecord(root)

	if errors.Is(err, os.ErrNotExist) {
		if !mayCreate {
			return Unlocked{}, fmt.Errorf("this brain's data is encrypted but %s is missing from %s, "+
				"so its key cannot be found; put the file back from a copy of the brain", FileName, root)
		}

		return create(root, ring)
	}

	if err != nil {
		return Unlocked{}, err
	}

	raw, err := ring.Get(r.KeyID)
	if errors.Is(err, ErrNotFound) {
		return Unlocked{}, ErrNeedsRecovery
	}

	if err != nil {
		return Unlocked{}, fmt.Errorf("asking %s for this brain's key: %w", ring.Name(), err)
	}

	k, err := NewKey(raw)
	if err != nil {
		return Unlocked{}, err
	}

	if k.ID() != r.KeyID {
		return Unlocked{}, fmt.Errorf("%s holds a key for this brain that is not its key", ring.Name())
	}

	return Unlocked{Key: k, Keyring: ring.Name(), Warning: ring.Warning()}, nil
}

func create(root string, ring Keyring) (Unlocked, error) {
	k, err := Generate()
	if err != nil {
		return Unlocked{}, err
	}

	recovery := make([]byte, KeySize)

	if _, err := io.ReadFull(rand.Reader, recovery); err != nil {
		return Unlocked{}, err
	}

	wrapper, err := recoveryKey(recovery)
	if err != nil {
		return Unlocked{}, err
	}

	r := &record{
		Version:  1,
		KeyID:    k.ID(),
		Recovery: base64.StdEncoding.EncodeToString(wrapper.Seal(k.Raw())),
		Unshown:  k.SealString(FormatRecovery(recovery)),
		Created:  time.Now().UTC().Format(time.RFC3339),
	}

	// Into the keyring first: a record naming a key no keyring holds is a
	// brain nobody can open without the recovery key it was never shown.
	if err := ring.Set(k.ID(), k.Raw()); err != nil {
		return Unlocked{}, fmt.Errorf("keeping the new key in %s: %w", ring.Name(), err)
	}

	if err := writeRecord(root, r); err != nil {
		return Unlocked{}, err
	}

	return Unlocked{Key: k, Created: true, Keyring: ring.Name(), Warning: ring.Warning()}, nil
}

/*
 * Recover opens a brain with its recovery key and gives this machine's
 * keyring the data key, so it opens here from now on without being asked.
 */
func Recover(root string, ring Keyring, text string) (Unlocked, error) {
	r, err := readRecord(root)
	if err != nil {
		return Unlocked{}, err
	}

	recovery, err := ParseRecovery(text)
	if err != nil {
		return Unlocked{}, err
	}

	wrapper, err := recoveryKey(recovery)
	if err != nil {
		return Unlocked{}, err
	}

	wrapped, err := base64.StdEncoding.DecodeString(r.Recovery)
	if err != nil {
		return Unlocked{}, fmt.Errorf("%s is damaged: %w", FileName, err)
	}

	raw, err := wrapper.Open(wrapped)
	if err != nil {
		return Unlocked{}, ErrBadRecoveryKey
	}

	k, err := NewKey(raw)
	if err != nil || k.ID() != r.KeyID {
		return Unlocked{}, ErrBadRecoveryKey
	}

	if err := ring.Set(k.ID(), k.Raw()); err != nil {
		return Unlocked{}, fmt.Errorf("keeping the key in %s: %w", ring.Name(), err)
	}

	return Unlocked{Key: k, Keyring: ring.Name(), Warning: ring.Warning()}, nil
}

// RecoveryToShow is the recovery key, while its owner has not yet said he
// has kept it.
func RecoveryToShow(root string, k *Key) (string, bool) {
	r, err := readRecord(root)
	if err != nil || r.Unshown == "" {
		return "", false
	}

	text, err := k.OpenString(r.Unshown)

	return text, err == nil
}

// RecoveryKept forgets the recovery key: its owner has it, and the program
// never shows it again.
func RecoveryKept(root string) error {
	r, err := readRecord(root)
	if err != nil {
		return err
	}

	r.Unshown = ""

	return writeRecord(root, r)
}

// recoveryKey is the key a recovery key wraps the data key with. The recovery
// key is 256 random bits, so it needs no stretching, only separating.
func recoveryKey(recovery []byte) (*Key, error) {
	sum := sha256.Sum256(append([]byte("pn-scripts-assistant recovery v1\x00"), recovery...))

	return NewKey(sum[:])
}

var recoveryText = base32.StdEncoding.WithPadding(base32.NoPadding)

// FormatRecovery writes a recovery key as somebody can copy it: groups of
// four, letters and digits only.
func FormatRecovery(recovery []byte) string {
	plain := recoveryText.EncodeToString(recovery)

	var groups []string

	for len(plain) > 4 {
		groups = append(groups, plain[:4])
		plain = plain[4:]
	}

	return strings.Join(append(groups, plain), "-")
}

// ParseRecovery reads a recovery key however it was copied: spaces, dashes
// and case do not matter.
func ParseRecovery(text string) ([]byte, error) {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == '-' || r == ' ' || r == '\n' || r == '\t' || r == '\r':
			return -1
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		}

		return r
	}, text)

	raw, err := recoveryText.DecodeString(clean)
	if err != nil || len(raw) != KeySize {
		return nil, errors.New("that is not a recovery key: it is 52 letters and digits, in groups of four")
	}

	return raw, nil
}

var remembered atomic.Pointer[Unlocked]

// Remember keeps what unlocking found out, without the key, for the interface.
func Remember(u Unlocked) {
	u.Key = nil
	remembered.Store(&u)
}

// Remembered is what Remember kept, or nothing.
func Remembered() Unlocked {
	if u := remembered.Load(); u != nil {
		return *u
	}

	return Unlocked{}
}
