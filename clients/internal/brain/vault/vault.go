/*
 * Package vault keeps what the assistant knows about its owner unreadable to
 * anything but this program (ADR 0008).
 *
 * One random 256-bit data key per brain, AES-256-GCM from the standard
 * library, a fresh nonce for every value. The key never touches the brain's
 * own drive in the clear: it lives in this machine's keyring (see keys.go),
 * and the drive carries only a copy wrapped with a recovery key its owner
 * keeps, so the brain can be opened on another machine and nowhere else.
 *
 * What it protects against, said plainly: the drive or a backup being taken
 * or copied, another user of the machine, another machine. What it does not:
 * a program running as its owner, which can ask the same keyring this one
 * asks. That is the honest limit of any program-only encryption on a desktop.
 */
package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// KeySize is the data key's length: AES-256.
const KeySize = 32

// magic opens every sealed blob, so a sealed value is told from a plain one
// by looking at it — which is what makes sealing a brain in place resumable.
var magic = []byte("\x89PNSEAL\x01")

// Prefix opens every sealed string: the blob, base64, behind it.
const Prefix = "pnsealed1:"

const idSize = 8

var (
	// ErrWrongKey is a value sealed with another brain's key.
	ErrWrongKey = errors.New("this was sealed with a different key")

	// ErrTampered is a value that was changed, or damaged, after sealing.
	ErrTampered = errors.New("this sealed value has been changed or damaged and cannot be opened")

	// ErrNoKey is a sealed value with no key to open it.
	ErrNoKey = errors.New("this is sealed and the key to open it is not here")
)

// Key is one brain's data key, ready to seal and open.
type Key struct {
	raw  []byte
	id   []byte
	aead cipher.AEAD
}

// NewKey makes a key from 32 bytes.
func NewKey(raw []byte) (*Key, error) {
	if len(raw) != KeySize {
		return nil, fmt.Errorf("a data key is %d bytes, not %d", KeySize, len(raw))
	}

	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(append([]byte("pn-scripts-assistant key id\x00"), raw...))

	return &Key{raw: append([]byte{}, raw...), id: sum[:idSize], aead: aead}, nil
}

// Generate makes a new random key.
func Generate() (*Key, error) {
	raw := make([]byte, KeySize)

	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return nil, err
	}

	return NewKey(raw)
}

// ID names the key without revealing it.
func (k *Key) ID() string { return hex.EncodeToString(k.id) }

// Raw is the key itself, for the keyring and nothing else.
func (k *Key) Raw() []byte { return append([]byte{}, k.raw...) }

// Seal encrypts and authenticates.
func (k *Key) Seal(plain []byte) []byte {
	nonce := make([]byte, k.aead.NonceSize())

	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		panic("vault: no randomness: " + err.Error())
	}

	out := make([]byte, 0, len(magic)+idSize+len(nonce)+len(plain)+k.aead.Overhead())
	out = append(out, magic...)
	out = append(out, k.id...)
	out = append(out, nonce...)

	return k.aead.Seal(out, nonce, plain, k.id)
}

// Open checks and decrypts a sealed blob.
func (k *Key) Open(sealed []byte) ([]byte, error) {
	if !IsSealed(sealed) {
		return nil, errors.New("not a sealed value")
	}

	rest := sealed[len(magic):]

	if len(rest) < idSize+k.aead.NonceSize()+k.aead.Overhead() {
		return nil, ErrTampered
	}

	if !bytes.Equal(rest[:idSize], k.id) {
		return nil, ErrWrongKey
	}

	nonce := rest[idSize : idSize+k.aead.NonceSize()]

	plain, err := k.aead.Open(nil, nonce, rest[idSize+len(nonce):], k.id)
	if err != nil {
		return nil, ErrTampered
	}

	return plain, nil
}

// SealString seals text into text.
func (k *Key) SealString(plain string) string {
	return Prefix + base64.RawStdEncoding.EncodeToString(k.Seal([]byte(plain)))
}

// OpenString opens what SealString made.
func (k *Key) OpenString(sealed string) (string, error) {
	if !strings.HasPrefix(sealed, Prefix) {
		return "", errors.New("not a sealed value")
	}

	raw, err := base64.RawStdEncoding.DecodeString(sealed[len(Prefix):])
	if err != nil {
		return "", ErrTampered
	}

	plain, err := k.Open(raw)

	return string(plain), err
}

// IsSealed reports whether a blob was sealed here.
func IsSealed(b []byte) bool { return bytes.HasPrefix(b, magic) }

// IsSealedString reports whether text was sealed here.
func IsSealedString(s string) bool { return strings.HasPrefix(s, Prefix) }

/*
 * The key this program is running with.
 *
 * One brain per process, so one key: set once the brain is unlocked, read by
 * everything that writes something personal to a file. Nil means nothing is
 * sealed — a test, or a tool that never opens a brain.
 */
var active atomic.Pointer[Key]

// Use makes a key the one in force.
func Use(k *Key) { active.Store(k) }

// Active is the key in force, or nil.
func Active() *Key { return active.Load() }

// SealIfActive seals text with the key in force, or leaves it as it is.
func SealIfActive(plain string) string {
	if k := Active(); k != nil && plain != "" && !IsSealedString(plain) {
		return k.SealString(plain)
	}

	return plain
}

// OpenIfSealed opens text sealed with the key in force; plain text is
// returned as it is.
func OpenIfSealed(text string) (string, error) {
	if !IsSealedString(text) {
		return text, nil
	}

	k := Active()
	if k == nil {
		return "", ErrNoKey
	}

	return k.OpenString(text)
}

/*
 * WriteFile writes a file sealed with the key in force, or plain when there
 * is none. Written whole, synced and moved into place, so a power cut leaves
 * either the old file or the new one and never half of either.
 */
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if k := Active(); k != nil {
		data = k.Seal(data)
	}

	return WriteAtomic(path, data, perm)
}

// WriteAtomic writes bytes as they are: temp file, sync, rename, sync folder.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	temp := path + ".new"

	f, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		f.Close()

		return err
	}

	if err := f.Sync(); err != nil {
		f.Close()

		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(temp, path); err != nil {
		return err
	}

	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		dir.Sync()
		dir.Close()
	}

	return nil
}

// ReadFile reads a file, opening it when it is sealed.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil || !IsSealed(data) {
		return data, err
	}

	k := Active()
	if k == nil {
		return nil, ErrNoKey
	}

	return k.Open(data)
}

// SealFileInPlace seals a plain file where it lies; a sealed one is left.
func SealFileInPlace(path string, k *Key) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	if IsSealed(data) {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	return WriteAtomic(path, k.Seal(data), info.Mode().Perm())
}
