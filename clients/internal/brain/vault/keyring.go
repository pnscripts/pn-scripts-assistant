package vault

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrNotFound is a keyring that does not hold the key asked for.
var ErrNotFound = errors.New("not in the keyring")

// Keyring is somewhere this machine keeps a brain's key.
type Keyring interface {
	// Name says where, in words.
	Name() string

	Get(id string) ([]byte, error)
	Set(id string, secret []byte) error
	Delete(id string) error

	// Warning is what its owner should know about keeping it here, or empty.
	Warning() string
}

// KeyringEnv forces the key file instead of the system's keyring: "file".
const KeyringEnv = "PN_SCRIPTS_ASSISTANT_KEYRING"

/*
 * System is this machine's keyring: the Secret Service on Linux (through the
 * libsecret already on any desktop with GTK), DPAPI on Windows, the Keychain
 * on macOS — and, where none of those answers, a key file only its owner can
 * read, with a warning that says what that does and does not protect.
 */
func System() Keyring {
	file := FileKeyring{Dir: keyFolder()}

	if strings.EqualFold(os.Getenv(KeyringEnv), "file") {
		return file
	}

	if system, ok := platformKeyring(); ok {
		return &chain{first: system, then: file}
	}

	return file
}

func keyFolder() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}

	return filepath.Join(dir, "pn-scripts-assistant", "keys")
}

/*
 * FileKeyring keeps keys in files readable by their owner alone, in this
 * machine's settings folder — never on the brain's drive, or taking the drive
 * would take the key with it.
 */
type FileKeyring struct{ Dir string }

func (f FileKeyring) Name() string { return "a key file in " + f.Dir }

func (f FileKeyring) Warning() string {
	return "The key is kept in a file only your account can read, because this machine has no " +
		"keyring this program can use. It still protects the brain if its drive or a copy is taken, " +
		"but anybody who can read your home folder can read the key."
}

func (f FileKeyring) path(id string) (string, error) {
	if _, err := hex.DecodeString(id); err != nil || id == "" {
		return "", fmt.Errorf("%q is not a key id", id)
	}

	return filepath.Join(f.Dir, id+".key"), nil
}

func (f FileKeyring) Get(id string) ([]byte, error) {
	at, err := f.path(id)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(at)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return hex.DecodeString(strings.TrimSpace(string(raw)))
}

func (f FileKeyring) Set(id string, secret []byte) error {
	at, err := f.path(id)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}

	return WriteAtomic(at, []byte(hex.EncodeToString(secret)+"\n"), 0o600)
}

func (f FileKeyring) Delete(id string) error {
	at, err := f.path(id)
	if err != nil {
		return err
	}

	if err := os.Remove(at); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// chain asks the system's keyring first and falls back to the key file,
// remembering which of the two holds the key.
type chain struct {
	first, then Keyring

	mu   sync.Mutex
	used Keyring
}

func (c *chain) which() Keyring {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.used != nil {
		return c.used
	}

	return c.first
}

func (c *chain) Name() string    { return c.which().Name() }
func (c *chain) Warning() string { return c.which().Warning() }

func (c *chain) Get(id string) ([]byte, error) {
	if raw, err := c.first.Get(id); err == nil {
		c.mu.Lock()
		c.used = c.first
		c.mu.Unlock()

		return raw, nil
	}

	raw, err := c.then.Get(id)
	if err == nil {
		c.mu.Lock()
		c.used = c.then
		c.mu.Unlock()
	}

	return raw, err
}

func (c *chain) Set(id string, secret []byte) error {
	if err := c.first.Set(id, secret); err == nil {
		c.mu.Lock()
		c.used = c.first
		c.mu.Unlock()

		return nil
	}

	if err := c.then.Set(id, secret); err != nil {
		return err
	}

	c.mu.Lock()
	c.used = c.then
	c.mu.Unlock()

	return nil
}

func (c *chain) Delete(id string) error {
	c.first.Delete(id)

	return c.then.Delete(id)
}

// MemoryKeyring is a keyring that forgets when the program stops: for tests,
// and for "a new machine" in them.
type MemoryKeyring struct {
	mu   sync.Mutex
	keys map[string][]byte
}

func (m *MemoryKeyring) Name() string    { return "memory" }
func (m *MemoryKeyring) Warning() string { return "" }

func (m *MemoryKeyring) Get(id string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if raw, ok := m.keys[id]; ok {
		return append([]byte{}, raw...), nil
	}

	return nil, ErrNotFound
}

func (m *MemoryKeyring) Set(id string, secret []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.keys == nil {
		m.keys = map[string][]byte{}
	}

	m.keys[id] = append([]byte{}, secret...)

	return nil
}

func (m *MemoryKeyring) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.keys, id)

	return nil
}
