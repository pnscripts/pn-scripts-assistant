package pair

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

/*
 * A device proves itself with a token it was given in person.
 *
 * The whole security story: to pair something you must be standing in front of
 * the computer the brain is on, reading a code off its screen.
 */
func TestADeviceIsPairedByACodeReadOffTheScreen(t *testing.T) {
	root := t.TempDir()

	book, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	code, until, err := book.Offer(Limited)
	if err != nil {
		t.Fatal(err)
	}

	if len(code) != 6 {
		t.Errorf("the code is %q, which is not six characters to read off a screen", code)
	}

	if time.Until(until) > HowLongACodeLasts+time.Second {
		t.Error("the code lasts longer than it was meant to")
	}

	device, token, err := book.Accept(code, "Petar's phone", "192.168.1.9")
	if err != nil {
		t.Fatal(err)
	}

	if token == "" {
		t.Fatal("no token came back, so the device has nothing to prove itself with")
	}

	if device.Can != Limited {
		t.Errorf("it was paired as %q", device.Can)
	}

	// And the token identifies it afterwards.
	who, known := book.Who(token)
	if !known || who.ID != device.ID {
		t.Errorf("the token did not identify the device: %+v %v", who, known)
	}

	// Anything else does not.
	if _, known := book.Who("not-the-token"); known {
		t.Error("something that is not the token was let in")
	}

	if _, known := book.Who(""); known {
		t.Error("an empty token was let in")
	}
}

/*
 * What is stored is the hash, not the key.
 *
 * A copy of this file is a list of devices rather than a way in — which
 * matters because the file sits on a removable drive that travels.
 */
func TestTheFileIsNotAListOfKeys(t *testing.T) {
	root := t.TempDir()

	book, _ := Load(root)
	code, _, _ := book.Offer(Full)

	_, token, err := book.Accept(code, "laptop", "192.168.1.9")
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(raw), token) {
		t.Fatal("the token itself is written to disk")
	}

	// And it still works after a restart, because the hash is enough to check.
	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, known := again.Who(token); !known {
		t.Error("the device was forgotten when the program restarted")
	}
}

/*
 * A wrong guess spends the code.
 *
 * Six characters is not many if something may try all afternoon, and a code
 * that survives a wrong guess is a code that can be guessed at.
 */
func TestAWrongGuessSpendsTheCode(t *testing.T) {
	book, _ := Load(t.TempDir())

	code, _, _ := book.Offer(Limited)

	if _, _, err := book.Accept("WRONG1", "phone", ""); err == nil {
		t.Fatal("a wrong code paired a device")
	}

	if _, _, err := book.Accept(code, "phone", ""); err == nil {
		t.Error("the right code still worked after a wrong guess, so it can be guessed at")
	}
}

// A code that nobody used runs out on its own, rather than sitting on a screen
// while somebody makes coffee.
func TestACodeRunsOut(t *testing.T) {
	book, _ := Load(t.TempDir())

	code, _, _ := book.Offer(Limited)

	book.mu.Lock()
	book.codeUntil = time.Now().Add(-time.Second)
	book.mu.Unlock()

	if _, _, err := book.Accept(code, "phone", ""); err == nil {
		t.Error("an expired code still worked")
	}

	if waiting, _, _ := book.Waiting(); waiting != "" {
		t.Error("an expired code is still being shown on the screen")
	}
}

// With nothing offered, nothing can pair — so a device cannot let itself in by
// guessing at a code that was never asked for.
func TestNothingPairsWhenNothingIsOffered(t *testing.T) {
	book, _ := Load(t.TempDir())

	if _, _, err := book.Accept("ABC234", "phone", ""); err == nil {
		t.Error("a device paired itself with no code on offer")
	}
}

/*
 * Unpairing takes effect at once.
 *
 * A phone left in a taxi is the case this exists for, and "at the next
 * restart" is not an answer to it.
 */
func TestUnpairingIsImmediate(t *testing.T) {
	root := t.TempDir()

	book, _ := Load(root)
	code, _, _ := book.Offer(Full)
	device, token, _ := book.Accept(code, "the phone in the taxi", "192.168.1.9")

	if err := book.Revoke(device.ID); err != nil {
		t.Fatal(err)
	}

	if _, known := book.Who(token); known {
		t.Error("a revoked device is still let in")
	}

	// And it stays gone.
	again, _ := Load(root)

	if _, known := again.Who(token); known {
		t.Error("it came back when the program restarted")
	}
}

// The list says what somebody needs to recognise their own devices, and
// nothing they should not see.
func TestTheListShowsNothingSecret(t *testing.T) {
	book, _ := Load(t.TempDir())
	code, _, _ := book.Offer(Full)

	book.Accept(code, "laptop", "192.168.1.9")

	for _, d := range book.List() {
		if d.Hash != "" {
			t.Error("the list shows the hashed token")
		}

		if d.Name == "" || d.ID == "" {
			t.Errorf("a device with nothing to recognise it by: %+v", d)
		}
	}
}

// A code somebody can read across a room: no O against 0, no I against 1.
func TestTheCodeIsReadableAcrossARoom(t *testing.T) {
	for i := 0; i < 200; i++ {
		code, err := readableCode(6)
		if err != nil {
			t.Fatal(err)
		}

		if strings.ContainsAny(code, "OI01") {
			t.Fatalf("the code %q has characters somebody has to guess at", code)
		}
	}
}

/*
 * The address on the screen is the address that answers.
 *
 * They differed by one — the panel said 43231 while the listener was on
 * 43232 — which is the whole feature failing in a way that looks like the
 * network being broken rather than like a program being wrong.
 */
func TestTheAddressOnTheScreenIsTheOneThatAnswers(t *testing.T) {
	if got := NetworkPort("8790"); got != "8791" {
		t.Errorf("the network listener would be on %q", got)
	}

	// Nonsense is left alone rather than turned into a different nonsense.
	for _, odd := range []string{"", "not-a-port", "65535", "0"} {
		if got := NetworkPort(odd); got != odd {
			t.Errorf("%q became %q", odd, got)
		}
	}

	// And the addresses offered use whatever port they are given.
	for _, address := range Addresses("8791") {
		if !strings.HasSuffix(address, ":8791") {
			t.Errorf("offered %q", address)
		}

		if !strings.HasPrefix(address, "https://") {
			t.Errorf("offered %q, which is not encrypted", address)
		}
	}
}
