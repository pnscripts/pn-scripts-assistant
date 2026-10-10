package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/vault"
)

const marker = "ZEBRA-PERSONAL-MARKER-7731"

func sealKey(t *testing.T) *vault.Key {
	t.Helper()

	k, err := vault.Generate()
	if err != nil {
		t.Fatal(err)
	}

	return k
}

// plainIn reports which files beside the database still hold the marker.
func plainIn(t *testing.T, path string) []string {
	t.Helper()

	var found []string

	for _, f := range []string{path, path + "-wal", path + "-shm"} {
		raw, err := os.ReadFile(f)
		if err == nil && bytes.Contains(raw, []byte(marker)) {
			found = append(found, filepath.Base(f))
		}
	}

	return found
}

// fill writes personal things the way the program does.
func fill(t *testing.T, d *DB, messages int) int64 {
	t.Helper()

	conv, err := d.NewConversation("about " + marker)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < messages; i++ {
		if _, err := d.AddMessage(conv, "user", "", "", fmt.Sprintf("message %d: %s", i, marker)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := d.AddFact("person", "the owner told me "+marker, []float32{0.25, 0.5, 0.75}); err != nil {
		t.Fatal(err)
	}

	if _, err := d.RecordInvocation(conv, "read_file", `{"path":"/home/x/`+marker+`"}`, "Read "+marker, "safe"); err != nil {
		t.Fatal(err)
	}

	return conv
}

func checkReadsBack(t *testing.T, d *DB, conv int64, messages int) {
	t.Helper()

	history, err := d.History(conv)
	if err != nil {
		t.Fatal(err)
	}

	user := 0

	for _, m := range history {
		if m.Role == "user" {
			if !strings.Contains(m.Content, marker) {
				t.Fatalf("a message came back as %q", m.Content)
			}

			user++
		}
	}

	if user != messages {
		t.Fatalf("%d messages came back, want %d", user, messages)
	}

	found, err := d.Search([]float32{0.25, 0.5, 0.75}, 5, 0.1)
	if err != nil || len(found) != 1 || !strings.Contains(found[0].Content, marker) {
		t.Fatalf("recall by meaning: %+v %v", found, err)
	}
}

/*
 * Written sealed, read back in the clear, and the file holds nothing that
 * says anything — the database, its journal, nothing.
 */
func TestASealedBrainSaysNothingOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")
	k := sealKey(t)

	d, err := OpenSealed(path, k)
	if err != nil {
		t.Fatal(err)
	}

	conv := fill(t, d, 20)
	checkReadsBack(t, d, conv, 20)

	if _, err := d.SealInPlace(context.Background()); err != nil {
		t.Fatal(err)
	}

	d.Close()

	if found := plainIn(t, path); len(found) > 0 {
		t.Fatalf("plain text in %v", found)
	}

	// Restarted: the same key reads it all back.
	again, err := OpenSealed(path, k)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()

	checkReadsBack(t, again, conv, 20)
}

// Another key reads nothing, and says so rather than handing on gibberish.
func TestAnotherKeyCannotReadTheBrain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, _ := OpenSealed(path, sealKey(t))
	conv := fill(t, d, 3)
	d.SealInPlace(context.Background())
	d.Close()

	other, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	if _, err := other.History(conv); !errors.Is(err, vault.ErrWrongKey) {
		t.Errorf("another key: %v", err)
	}

	if _, err := other.SealInPlace(context.Background()); !errors.Is(err, vault.ErrWrongKey) {
		t.Errorf("sealing again with another key: %v", err)
	}

	none, _ := Open(path)
	defer none.Close()

	if _, err := none.History(conv); !errors.Is(err, vault.ErrNoKey) {
		t.Errorf("no key: %v", err)
	}
}

// A changed byte in a stored value fails the read.
func TestAChangedValueInTheDatabaseIsCaught(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")
	k := sealKey(t)

	d, _ := OpenSealed(path, k)
	defer d.Close()

	conv := fill(t, d, 1)

	var stored string

	d.sql().QueryRow(`SELECT hex(content) FROM messages WHERE role = 'user'`).Scan(&stored)

	raw, _ := hex.DecodeString(stored)
	text := []byte(string(raw))
	text[len(text)-3] ^= 0x01 // inside the base64 of the tag

	if text[len(text)-3] == '=' || !strings.HasPrefix(string(text), vault.Prefix) {
		t.Skip("could not alter it in place")
	}

	d.sql().Exec(`UPDATE messages SET content = ? WHERE role = 'user'`, string(text))

	if _, err := d.History(conv); err == nil {
		t.Error("a changed message was read as if nothing had happened")
	}
}

/*
 * The power is cut while a plain brain is being sealed: the process is killed
 * after two batches. Started again, it carries on, and at the end every
 * message is there, sealed, readable — and the copy taken first opens to the
 * brain as it was.
 */
func TestSealingSurvivesBeingCutOffPartWay(t *testing.T) {
	if os.Getenv("PN_SEAL_CRASH_DB") != "" {
		crashMidSeal()

		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "brain.sqlite")

	plain, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	const messages = 1000

	conv := fill(t, plain, messages)
	plain.Close()

	k := sealKey(t)

	cmd := exec.Command(os.Args[0], "-test.run=^TestSealingSurvivesBeingCutOffPartWay$")
	cmd.Env = append(os.Environ(), "PN_SEAL_CRASH_DB="+path, "PN_SEAL_CRASH_KEY="+hex.EncodeToString(k.Raw()))

	out, err := cmd.CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("the sealing was not cut off as planned: %v\n%s", err, out)
	}

	// Part sealed, part not, and nothing lost.
	half, err := OpenSealed(path, k)
	if err != nil {
		t.Fatal(err)
	}

	var sealed, total int

	half.sql().QueryRow(`SELECT COUNT(*), SUM(content LIKE 'pnsealed1:%') FROM messages`).Scan(&total, &sealed)

	if sealed == 0 || sealed >= total {
		t.Fatalf("not cut part way: %d of %d sealed", sealed, total)
	}

	if half.state("backup") != "verified" {
		t.Fatal("rows were sealed before the copy was proved")
	}

	checkReadsBack(t, half, conv, messages)

	if _, err := half.SealInPlace(context.Background()); err != nil {
		t.Fatalf("carrying on: %v", err)
	}

	checkReadsBack(t, half, conv, messages)
	half.Close()

	if found := plainIn(t, path); len(found) > 0 {
		t.Fatalf("plain text left in %v after finishing", found)
	}

	// And the copy taken first is the brain as it was, sealed.
	backup, err := os.ReadFile(BackupBeforeSealing(path))
	if err != nil || bytes.Contains(backup, []byte(marker)) {
		t.Fatalf("the copy taken first: %v, plain %v", err, bytes.Contains(backup, []byte(marker)))
	}

	restored := filepath.Join(dir, "restored.sqlite")

	f, _ := os.Create(restored)
	err = vault.OpenStream(k, f, bytes.NewReader(backup))
	f.Close()

	if err != nil {
		t.Fatal(err)
	}

	old, err := Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()

	checkReadsBack(t, old, conv, messages)
}

// crashMidSeal is the child: it seals, and dies after the second batch the
// way a power cut would — no cleanup, no close.
func crashMidSeal() {
	raw, _ := hex.DecodeString(os.Getenv("PN_SEAL_CRASH_KEY"))
	k, _ := vault.NewKey(raw)

	d, err := OpenSealed(os.Getenv("PN_SEAL_CRASH_DB"), k)
	if err != nil {
		os.Exit(2)
	}

	afterSealBatch = func(n int) {
		if n == 2 {
			os.Exit(7)
		}
	}

	d.SealInPlace(context.Background())
	os.Exit(3)
}

func TestSealedWithNamesTheKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	if SealedWith(path) != "" {
		t.Error("no database, but a key")
	}

	k := sealKey(t)
	d, _ := OpenSealed(path, k)
	d.SealInPlace(context.Background())
	d.Close()

	if got := SealedWith(path); got != k.ID() {
		t.Errorf("sealed with %q, want %q", got, k.ID())
	}
}
