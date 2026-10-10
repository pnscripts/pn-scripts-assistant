package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func key(t *testing.T) *Key {
	t.Helper()

	k, err := Generate()
	if err != nil {
		t.Fatal(err)
	}

	return k
}

func TestSealedComesBackAndSaysNothing(t *testing.T) {
	k := key(t)

	const secret = "The owner's overdraft is with the bank on the corner"

	sealed := k.SealString(secret)

	if strings.Contains(sealed, "overdraft") || !IsSealedString(sealed) {
		t.Fatalf("sealed text gives itself away: %q", sealed)
	}

	if again := k.SealString(secret); again == sealed {
		t.Error("the same text sealed twice came out the same; the nonce is not fresh")
	}

	back, err := k.OpenString(sealed)
	if err != nil || back != secret {
		t.Fatalf("round trip: %q %v", back, err)
	}

	blob := []byte{1, 2, 3, 0, 255}

	if out, err := k.Open(k.Seal(blob)); err != nil || !bytes.Equal(out, blob) {
		t.Errorf("blob round trip: %v %v", out, err)
	}
}

func TestAnotherKeyCannotOpenIt(t *testing.T) {
	sealed := key(t).SealString("private")

	if _, err := key(t).OpenString(sealed); !errors.Is(err, ErrWrongKey) {
		t.Errorf("another key: %v", err)
	}
}

func TestAChangedValueIsCaught(t *testing.T) {
	k := key(t)
	sealed := k.Seal([]byte("the memory as it was written"))

	for _, at := range []int{len(magic) + idSize + 3, len(sealed) - 1, len(sealed) / 2} {
		changed := append([]byte{}, sealed...)
		changed[at] ^= 0x01

		if _, err := k.Open(changed); err == nil {
			t.Errorf("a byte changed at %d went unnoticed", at)
		}
	}

	if _, err := k.Open(sealed[:len(sealed)-5]); err == nil {
		t.Error("a cut value went unnoticed")
	}
}

/*
 * The brain on a drive taken to a new machine: that machine's keyring has
 * never seen it, so it asks for the recovery key; the recovery key opens it
 * and puts the key into the new keyring, after which it opens without asking.
 */
func TestANewMachineOpensItWithTheRecoveryKey(t *testing.T) {
	root := t.TempDir()
	home := &MemoryKeyring{}

	first, err := Unlock(root, home, true)
	if err != nil || !first.Created {
		t.Fatalf("first unlock: %+v %v", first, err)
	}

	sealed := first.Key.SealString("what it learned at home")

	recovery, pending := RecoveryToShow(root, first.Key)
	if !pending || len(strings.ReplaceAll(recovery, "-", "")) != 52 {
		t.Fatalf("the recovery key is not there to be shown: %q", recovery)
	}

	// The drive holds nothing that opens it without the recovery key.
	raw, _ := os.ReadFile(filepath.Join(root, FileName))
	if strings.Contains(string(raw), recovery) || bytes.Contains(raw, first.Key.Raw()) {
		t.Fatal("vault.json holds the key or the recovery key in the clear")
	}

	if err := RecoveryKept(root); err != nil {
		t.Fatal(err)
	}

	if _, pending := RecoveryToShow(root, first.Key); pending {
		t.Error("the recovery key is still shown after its owner said he has kept it")
	}

	again, err := Unlock(root, home, true)
	if err != nil || again.Created || again.Key.ID() != first.Key.ID() {
		t.Fatalf("second unlock at home: %+v %v", again, err)
	}

	elsewhere := &MemoryKeyring{}

	if _, err := Unlock(root, elsewhere, true); !errors.Is(err, ErrNeedsRecovery) {
		t.Fatalf("a new machine opened it without the recovery key: %v", err)
	}

	if _, err := Recover(root, elsewhere, strings.Repeat("A", 52)); !errors.Is(err, ErrBadRecoveryKey) {
		t.Errorf("a wrong recovery key: %v", err)
	}

	got, err := Recover(root, elsewhere, strings.ToLower(strings.ReplaceAll(recovery, "-", " ")))
	if err != nil {
		t.Fatalf("the recovery key, typed loosely: %v", err)
	}

	if text, err := got.Key.OpenString(sealed); err != nil || text != "what it learned at home" {
		t.Errorf("what was sealed at home: %q %v", text, err)
	}

	if after, err := Unlock(root, elsewhere, true); err != nil || after.Key.ID() != first.Key.ID() {
		t.Errorf("the new machine still asks after recovering: %v", err)
	}
}

// A brain whose data is sealed and whose record is gone is not given a new
// key: that would make everything already there unreadable.
func TestAMissingRecordIsNotReplacedOverSealedData(t *testing.T) {
	if _, err := Unlock(t.TempDir(), &MemoryKeyring{}, false); err == nil {
		t.Error("a new key was made for a brain whose data is already sealed")
	}
}

func TestTheKeyFileIsOnlyItsOwners(t *testing.T) {
	ring := FileKeyring{Dir: filepath.Join(t.TempDir(), "keys")}
	k := key(t)

	if _, err := ring.Get(k.ID()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an empty key folder: %v", err)
	}

	if err := ring.Set(k.ID(), k.Raw()); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(ring.Dir, k.ID()+".key"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the key file is %v (%v)", info.Mode().Perm(), err)
	}

	if raw, err := ring.Get(k.ID()); err != nil || !bytes.Equal(raw, k.Raw()) {
		t.Errorf("read back: %v", err)
	}

	if ring.Warning() == "" {
		t.Error("the key file does not say what it does not protect")
	}
}

func TestASealedCopyReadsBackExactlyOrNotAtAll(t *testing.T) {
	k := key(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "brain.sqlite")

	data := bytes.Repeat([]byte("conversation about the electricity bill\n"), 70000) // ~2.7MB, three chunks

	os.WriteFile(src, data, 0o600)

	dst := filepath.Join(dir, "copy.sealed")

	if err := SealedCopy(k, src, dst); err != nil {
		t.Fatal(err)
	}

	sealed, _ := os.ReadFile(dst)
	if bytes.Contains(sealed, []byte("electricity")) {
		t.Fatal("the sealed copy holds the plain text")
	}

	var back bytes.Buffer

	if err := OpenStream(k, &back, bytes.NewReader(sealed)); err != nil || !bytes.Equal(back.Bytes(), data) {
		t.Fatalf("read back: %v", err)
	}

	// A chunk cut off the end is caught, not read as a shorter file.
	if err := OpenStream(k, &bytes.Buffer{}, bytes.NewReader(sealed[:len(sealed)/2])); err == nil {
		t.Error("a cut copy was read as whole")
	}
}

func TestALogSaysNothingOnDisk(t *testing.T) {
	k := key(t)
	Use(k)
	t.Cleanup(func() { Use(nil) })

	var disk bytes.Buffer

	w := &LineSealer{W: &disk}
	w.Write([]byte("msg=\"the owner said his PIN is"))
	w.Write([]byte(" in the drawer\"\nsecond line\n"))

	if strings.Contains(disk.String(), "drawer") || strings.Count(disk.String(), "\n") != 2 {
		t.Fatalf("the log holds plain text: %q", disk.String())
	}

	if got := OpenLines(k, disk.String()); !strings.Contains(got, "in the drawer") || !strings.Contains(got, "second line") {
		t.Errorf("read back: %q", got)
	}
}
