/*
 * Package pair is which devices may reach this brain, and how they proved it.
 *
 * Every route in this program has run without any authentication at all, and
 * that was correct for exactly as long as it could only be reached from this
 * machine: the loopback interface is a proof of identity, because nothing else
 * can connect to it. The moment it binds wider that proof is gone, and
 * everything it knows — the memory, the mail, the files, the ability to run a
 * command — is behind a door with no lock.
 *
 * So: a device proves itself with a token it was given in person, by somebody
 * reading a short code off the screen of the computer the brain is on. There
 * is no password to guess, no account, no server anywhere else, and nothing to
 * phone home to. The token is stored hashed, so a copy of this file is not a
 * copy of the keys.
 */
package pair

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileName is where the paired devices live, in the brain's own folder, so
// they travel with the brain and go when it goes.
const FileName = "paired.json"

/*
 * How much a device may do.
 *
 * Two levels, and the line between them is one question: may this device make
 * the decisions? A phone on a train is a fine place to ask the assistant
 * something and a poor place to approve it deleting files, and somebody who
 * loses that phone should not have lost the gate with it.
 */
const (
	// Full can do everything the window on the computer can.
	Full = "full"

	// Limited can talk to it, see what it is doing, and read what it knows.
	// It cannot approve an action, change a permission, change privacy, or
	// change any setting — those stay at the desk.
	Limited = "limited"
)

// Device is one thing that may reach this brain.
type Device struct {
	// ID is short and public, so a device can be named in a log without the
	// log becoming a list of keys.
	ID string `json:"id"`

	Name string `json:"name"`
	Can  string `json:"can"`

	// Hash is the token, hashed. The token itself is shown once, when the
	// device is paired, and never stored — so a copy of this file is not a
	// copy of the keys.
	Hash string `json:"hash"`

	Paired   time.Time `json:"paired"`
	LastSeen time.Time `json:"last_seen,omitempty"`

	// From is the address it last came from, which is the only way somebody
	// can tell "my phone" from something that is not their phone.
	From string `json:"from,omitempty"`
}

// Book is the paired devices, and the code that is currently being offered.
type Book struct {
	root string

	mu      sync.RWMutex
	devices map[string]Device

	// The pairing code being offered right now. One at a time, short-lived,
	// and used once — see Offer.
	code      string
	codeUntil time.Time
	codeCan   string
}

// Load reads the paired devices for a brain. A missing file is no devices,
// which is every brain until somebody pairs one.
func Load(root string) (*Book, error) {
	b := &Book{root: root, devices: map[string]Device{}}

	raw, err := os.ReadFile(filepath.Join(root, FileName))

	if os.IsNotExist(err) {
		return b, nil
	}

	if err != nil {
		return b, fmt.Errorf("reading %s: %w", FileName, err)
	}

	var list []Device

	if err := json.Unmarshal(raw, &list); err != nil {
		return b, fmt.Errorf("reading %s: %w", FileName, err)
	}

	for _, d := range list {
		b.devices[d.ID] = d
	}

	return b, nil
}

/*
 * HowLongACodeLasts is deliberately short.
 *
 * A code is read off one screen and typed into another in the same room, which
 * takes under a minute. Anything longer is a code sitting on a screen while
 * somebody makes coffee.
 */
const HowLongACodeLasts = 3 * time.Minute

/*
 * Offer produces the code to be typed into a new device.
 *
 * Shown on the computer the brain is on, which is the whole of the security
 * story: to pair a device you must be standing in front of it. Six letters and
 * digits, from an alphabet with no characters that look like each other,
 * because it is being read aloud or copied by eye.
 */
func (b *Book) Offer(can string) (string, time.Time, error) {
	code, err := readableCode(6)
	if err != nil {
		return "", time.Time{}, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.code = code
	b.codeUntil = time.Now().Add(HowLongACodeLasts)
	b.codeCan = level(can)

	return code, b.codeUntil, nil
}

// Waiting reports the code currently on offer, so the screen can keep showing
// it and stop when it expires.
func (b *Book) Waiting() (string, time.Time, string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.code == "" || time.Now().After(b.codeUntil) {
		return "", time.Time{}, ""
	}

	return b.code, b.codeUntil, b.codeCan
}

// StopOffering withdraws the code, for somebody who changed their mind.
func (b *Book) StopOffering() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.code, b.codeUntil, b.codeCan = "", time.Time{}, ""
}

/*
 * Accept takes a code and, if it is the one on offer, pairs the device.
 *
 * Returns the token once. It is never stored and never shown again: what is
 * kept here is its hash, so somebody who copies this file has a list of
 * devices rather than a way in.
 *
 * The code is spent whether or not it was right. A code that survives a wrong
 * guess is a code that can be guessed at, and six characters is not many if
 * something may try all afternoon.
 */
func (b *Book) Accept(code, name, from string) (Device, string, error) {
	b.mu.Lock()

	offered, until, can := b.code, b.codeUntil, b.codeCan

	b.code, b.codeUntil, b.codeCan = "", time.Time{}, ""

	b.mu.Unlock()

	if offered == "" || time.Now().After(until) {
		return Device{}, "", fmt.Errorf(
			"no code is being offered — ask for one on the computer the assistant runs on")
	}

	given := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(code, " ", "")))

	// Compared in constant time. The difference is unmeasurable over a network
	// and costs nothing, and a comparison that returns early is a comparison
	// that answers questions one character at a time.
	if subtle.ConstantTimeCompare([]byte(given), []byte(offered)) != 1 {
		return Device{}, "", fmt.Errorf("that code is not right; ask for a new one")
	}

	token, err := secret()
	if err != nil {
		return Device{}, "", err
	}

	id, err := readableCode(8)
	if err != nil {
		return Device{}, "", err
	}

	device := Device{
		ID:     id,
		Name:   plainName(name),
		Can:    can,
		Hash:   hashOf(token),
		Paired: time.Now().UTC(),
		From:   from,
	}

	b.mu.Lock()
	b.devices[device.ID] = device
	b.mu.Unlock()

	if err := b.save(); err != nil {
		return Device{}, "", err
	}

	return device, token, nil
}

/*
 * Who identifies the device holding a token, or false.
 *
 * A linear walk over the devices, hashing once and comparing in constant time.
 * A person has a handful of devices; an index would be a structure to keep in
 * step for no measurable gain.
 */
func (b *Book) Who(token string) (Device, bool) {
	if token == "" {
		return Device{}, false
	}

	want := hashOf(token)

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, d := range b.devices {
		if subtle.ConstantTimeCompare([]byte(d.Hash), []byte(want)) == 1 {
			return d, true
		}
	}

	return Device{}, false
}

// Seen records that a device has just been heard from, so the list can say
// which of them is actually in use.
func (b *Book) Seen(id, from string) {
	b.mu.Lock()

	d, known := b.devices[id]
	if !known {
		b.mu.Unlock()

		return
	}

	// Written at most once a minute. Every poll from every device would
	// otherwise rewrite this file several times a second.
	if time.Since(d.LastSeen) < time.Minute && d.From == from {
		b.mu.Unlock()

		return
	}

	d.LastSeen = time.Now().UTC()
	d.From = from
	b.devices[id] = d

	b.mu.Unlock()

	b.save()
}

// List is every paired device, oldest first, without anything secret in it.
func (b *Book) List() []Device {
	b.mu.RLock()

	out := make([]Device, 0, len(b.devices))

	for _, d := range b.devices {
		d.Hash = ""
		out = append(out, d)
	}

	b.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Paired.Before(out[j].Paired) })

	return out
}

// Revoke unpairs a device. A phone left in a taxi is the case this exists for,
// so it takes effect at once rather than at the next restart.
func (b *Book) Revoke(id string) error {
	b.mu.Lock()

	if _, known := b.devices[id]; !known {
		b.mu.Unlock()

		return nil
	}

	delete(b.devices, id)
	b.mu.Unlock()

	return b.save()
}

// Any reports whether anything is paired at all, so the interface can say what
// opening the door would currently mean.
func (b *Book) Any() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.devices) > 0
}

func (b *Book) save() error {
	b.mu.RLock()

	list := make([]Device, 0, len(b.devices))

	for _, d := range b.devices {
		list = append(list, d)
	}

	b.mu.RUnlock()

	sort.Slice(list, func(i, j int) bool { return list[i].Paired.Before(list[j].Paired) })

	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	// Written whole and moved into place, like everything else in this folder.
	temp := filepath.Join(b.root, FileName+".new")

	if err := os.WriteFile(temp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", FileName, err)
	}

	return os.Rename(temp, filepath.Join(b.root, FileName))
}

func level(can string) string {
	if can == Full {
		return Full
	}

	return Limited
}

func plainName(name string) string {
	name = strings.TrimSpace(name)

	if name == "" {
		return "a device"
	}

	if len([]rune(name)) > 40 {
		name = string([]rune(name)[:40])
	}

	return name
}

func hashOf(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

// secret is the token itself: 32 bytes of randomness, which is not guessable
// by anything.
func secret() (string, error) {
	raw := make([]byte, 32)

	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("could not make a key: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

/*
 * readableCode is characters somebody can read off a screen and type.
 *
 * No O and no 0, no I and no 1: the code is copied by eye, often across the
 * room, and a character somebody has to guess at is a code that fails and
 * makes them think the feature is broken.
 *
 * Exactly thirty-two characters, which matters: 256 divides by 32, so taking a
 * random byte modulo the alphabet is even. An alphabet of any other size would
 * make the first few characters more likely than the rest, quietly, and a code
 * that is not uniform is a code with fewer possibilities than it appears to
 * have.
 */
const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func readableCode(n int) (string, error) {
	raw := make([]byte, n)

	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("could not make a code: %w", err)
	}

	out := make([]byte, n)

	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}

	return string(out), nil
}
