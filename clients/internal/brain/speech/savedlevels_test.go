package speech

import (
	"context"
	"encoding/binary"
	"testing"
)

/*
 * Putting right what the old way of turning the room down left saved.
 *
 * The lines below are this machine's own WirePlumber state from the day it was
 * found: Brave, both Chromes and the brain's own espeak voice saved at a fifth
 * of a fifth, next to programs saved where their owner left them.
 */
const stateFound = `[restore-stream]
Output/Audio:application.name:AnyDesk:channelVolumes=1.0;1.0;
Output/Audio:application.name:Brave:channelMap=MONO;
Output/Audio:application.name:Brave:channelVolumes=0.0058320006355643;
Output/Audio:application.name:Chromium:channelVolumes=0.0080000003799796;0.0080000003799796;
Output/Audio:application.name:Google\sChrome:channelVolumes=0.0080000003799796;0.0080000003799796;
Output/Audio:application.name:eSpeak:channelVolumes=0.0080000003799796;
Output/Audio:application.name:eSpeak:volume=1.0
Output/Audio:media.role:Game:channelVolumes=1.0;1.0;1.0;1.0;1.0;1.0;
Output/Audio:media.name:Echo-Cancel\sPlayback:channelVolumes=0.0080000003799796;0.0080000003799796;
Input/Audio:application.name:Brave\sinput:channelVolumes=0.0080000003799796;
Output/Audio:application.name:Quiet\sOn\sPurpose:channelVolumes=0.0421875;
Output/Audio:application.name:Slider:channelVolumes=0.00514;0.00514;
Output/Audio:application.name:Uneven:channelVolumes=0.008;0.5;
`

func TestTheOldWaysLevelsAreFoundAndNothingElse(t *testing.T) {
	got := damagedSavedLevels(stateFound)

	want := map[string]savedLevel{
		"Brave":         {Kind: "application.name", Value: "Brave", Channels: 1, Was: 0.9},
		"Chromium":      {Kind: "application.name", Value: "Chromium", Channels: 2, Was: 1},
		"Google Chrome": {Kind: "application.name", Value: "Google Chrome", Channels: 2, Was: 1},
		"eSpeak":        {Kind: "application.name", Value: "eSpeak", Channels: 1, Was: 1},
	}

	if len(got) != len(want) {
		t.Fatalf("found %d levels to put back, want %d: %+v", len(got), len(want), got)
	}

	for _, d := range got {
		w, ok := want[d.Value]

		if !ok {
			t.Errorf("%q was taken for the old way's damage", d.Value)

			continue
		}

		if d.Kind != w.Kind || d.Channels != w.Channels || d.Was != w.Was {
			t.Errorf("%s read as %+v, want %+v", d.Value, d, w)
		}
	}
}

/*
 * The fingerprint, one level at a time.
 *
 * 0.0421875 is 0.35 cubed — somebody's own choice, above a fifth — and
 * 0.00514 is not a round thousandth on wpctl's scale, which a slider almost
 * never is. Neither is touched.
 */
func TestOnlyTheOldWaysFingerprintCounts(t *testing.T) {
	for _, c := range []struct {
		saved float64
		was   float64
		ok    bool
	}{
		{0.0080000003799796, 1, true},
		{0.0058320006355643, 0.9, true},
		{0.001, 0.5, true},
		{1, 0, false},
		{0.0421875, 0, false},
		{0.00514, 0, false},
		{0, 0, false},
	} {
		was, ok := leftByOldDucking(c.saved)

		if ok != c.ok || (ok && was != c.was) {
			t.Errorf("%v read as %v/%v, want %v/%v", c.saved, was, ok, c.was, c.ok)
		}
	}
}

func TestStateKeysAreUnescapedTheWayWirePlumberWritesThem(t *testing.T) {
	for in, want := range map[string]string{
		`Google\sChrome`:            "Google Chrome",
		`PipeWire\sALSA\s\oaplay\c`: "PipeWire ALSA [aplay]",
		`a\eb`:                      "a=b",
		`back\\slash`:               `back\slash`,
		`plain`:                     "plain",
	} {
		if got := unescapeStateKey(in); got != want {
			t.Errorf("%q unescaped to %q, want %q", in, got, want)
		}
	}
}

// The silent stream is opened under exactly the saved key, quoted so a name
// with a quote in it cannot break out of it.
func TestTheStreamCarriesTheSavedKey(t *testing.T) {
	got := streamProps(savedLevel{Kind: "application.name", Value: `Google "Chrome"`})

	if got != `{ application.name = "Google \"Chrome\"" }` {
		t.Errorf("opened with %s", got)
	}
}

func TestTheSilenceIsAWavFileOfSilence(t *testing.T) {
	wav := silentWAV(2, 22050, 1e9)

	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatal("not a WAV file")
	}

	if channels := binary.LittleEndian.Uint16(wav[22:24]); channels != 2 {
		t.Errorf("%d channels, want 2", channels)
	}

	if data := binary.LittleEndian.Uint32(wav[40:44]); int(data) != len(wav)-44 || data != 22050*2*2 {
		t.Errorf("data length %d for a file of %d bytes", data, len(wav))
	}

	for _, b := range wav[44:] {
		if b != 0 {
			t.Fatal("the silence is not silent")
		}
	}
}

/*
 * At startup: what a crashed run left down goes back, and what WirePlumber
 * saved is put right — reported, so the log says why a volume changed.
 */
func TestStartingUpPutsBackBothKinds(t *testing.T) {
	left := playing("Brave", 51, "900", 0.8, 0.8)
	left[0].Gain = []float64{0.16, 0.16}

	room := inRoom(t, left, playing("mpv", 52, "901", 1, 1))

	// It also clears the old way's note from the home directory, which is not
	// this test's to clear.
	t.Setenv("HOME", t.TempDir())

	wasRead, wasPut := readSavedLevels, putSavedLevelBack
	var put []savedLevel

	readSavedLevels = func() string { return stateFound }
	putSavedLevelBack = func(_ context.Context, d savedLevel) error {
		put = append(put, d)

		return nil
	}

	t.Cleanup(func() { readSavedLevels, putSavedLevelBack = wasRead, wasPut })

	said := PutBackAnythingLeftDown()

	if got := room.gain["900"]; len(got) != 2 || !near(got[0], 0.8) {
		t.Errorf("the stream left at a fifth came back at %v, want 0.8", got)
	}

	if _, touched := room.gain["901"]; touched {
		t.Error("a stream at its own level was touched")
	}

	if len(put) != 4 {
		t.Errorf("put back %d saved levels, want 4", len(put))
	}

	if len(said) != 5 {
		t.Errorf("reported %d things, want 5: %v", len(said), said)
	}
}
