package speech

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// aRecordingStoppedMidway builds what pw-record leaves behind when it is
// killed: all the audio, and both lengths still at their placeholders.
func aRecordingStoppedMidway(t *testing.T, path string, samples int) {
	t.Helper()

	raw := make([]byte, 44+samples*2)

	copy(raw[0:], "RIFF")
	binary.LittleEndian.PutUint32(raw[4:], 8) // the placeholder, never fixed
	copy(raw[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(raw[16:], 16)
	binary.LittleEndian.PutUint16(raw[20:], 1)
	binary.LittleEndian.PutUint16(raw[22:], 1)
	binary.LittleEndian.PutUint32(raw[24:], 16000)
	copy(raw[36:], "data")
	binary.LittleEndian.PutUint32(raw[40:], 0) // and this one too

	for i := 0; i < samples; i++ {
		binary.LittleEndian.PutUint16(raw[44+i*2:], uint16(int16(i%2000-1000)))
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

/*
 * The header of a recording that was stopped rather than finished.
 *
 * A WAV says how long it is twice and neither can be written until the
 * recording ends, so a recorder writes placeholders and goes back to fix them
 * when it closes. This program never lets it close: a turn ends when somebody
 * stops talking and the recorder is killed on the spot.
 *
 * Nothing here noticed, because everything in this package reads the samples
 * directly. whisper does not: it reads the header, concludes the file is empty
 * and answers "Invalid request" — which arrived as "loud enough, but no words
 * came back" on thirteen of fourteen recordings.
 */
func TestRepairingARecordingThatWasCutOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cut.wav")

	aRecordingStoppedMidway(t, path, 8000)

	fixed, err := RepairWAV(path)
	if err != nil {
		t.Fatal(err)
	}

	if !fixed {
		t.Fatal("a header with both lengths at zero was thought to be fine")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := binary.LittleEndian.Uint32(raw[4:]), uint32(len(raw)-8); got != want {
		t.Errorf("the RIFF length is %d, want %d", got, want)
	}

	if got, want := binary.LittleEndian.Uint32(raw[40:]), uint32(len(raw)-44); got != want {
		t.Errorf("the data length is %d, want %d", got, want)
	}

	// And the audio itself is untouched.
	for i := 0; i < 8000; i++ {
		if int16(binary.LittleEndian.Uint16(raw[44+i*2:])) != int16(i%2000-1000) {
			t.Fatalf("sample %d changed while repairing the header", i)
		}
	}

	// Repairing a correct file changes nothing and says so.
	if again, err := RepairWAV(path); err != nil || again {
		t.Errorf("a correct header was rewritten again: %v %v", again, err)
	}
}

// Something that is not a recording at all is refused, rather than having
// numbers written into the middle of it.
func TestRepairingSomethingThatIsNotAWav(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notaudio.wav")

	if err := os.WriteFile(path, []byte("this is a text file, quite long, but not audio at all!!"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := RepairWAV(path); err == nil {
		t.Error("a text file was accepted as a recording and rewritten")
	}
}
