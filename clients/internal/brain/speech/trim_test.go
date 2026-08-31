package speech

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

/*
 * Cutting in leaves only what the person said.
 *
 * The recording of an interrupted turn holds both voices: the assistant still
 * talking, then somebody across the top of it. Transcribing all of it asks the
 * recogniser to hear two people at once, and what comes back is a blend — a
 * sentence neither of them said, delivered with nothing to mark it as a guess.
 */
func TestCuttingInDropsTheAssistantsOwnVoice(t *testing.T) {
	const header = 44

	dir := t.TempDir()
	path := filepath.Join(dir, "turn.wav")

	// A second of the assistant, then a second of the person.
	mine := make([]byte, 32000)
	yours := make([]byte, 32000)

	for i := range yours {
		yours[i] = 0x40
	}

	raw := append(make([]byte, header), append(mine, yours...)...)
	copy(raw, "RIFF")
	copy(raw[8:], "WAVEfmt ")
	copy(raw[36:], "data")

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := TrimTo(path, header+int64(len(mine))); err != nil {
		t.Fatalf("trimming failed: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != header+len(yours) {
		t.Errorf("file is %d bytes, want %d — the wrong amount was dropped",
			len(out), header+len(yours))
	}

	// The header has to describe the new file. Left saying the old length, the
	// recogniser reads past the end and refuses it — the same fault RepairWAV
	// exists for.
	if got := binary.LittleEndian.Uint32(out[40:]); int(got) != len(yours) {
		t.Errorf("the header says %d bytes of audio, but there are %d", got, len(yours))
	}

	if got := binary.LittleEndian.Uint32(out[4:]); int(got) != len(out)-8 {
		t.Errorf("the RIFF size says %d, want %d", got, len(out)-8)
	}

	// And what survived is the person, not the assistant.
	if out[header] != 0x40 {
		t.Error("the audio kept was the assistant's voice, not the person's")
	}
}

// An ordinary turn, with no interruption, is left exactly as it was.
func TestATurnNobodyInterruptedIsUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "turn.wav")

	raw := make([]byte, 44+1000)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := TrimTo(path, 0); err != nil {
		t.Fatalf("an untouched turn reported an error: %v", err)
	}

	out, _ := os.ReadFile(path)
	if len(out) != len(raw) {
		t.Errorf("a turn nobody interrupted was changed: %d bytes, want %d",
			len(out), len(raw))
	}
}
