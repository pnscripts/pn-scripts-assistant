package speech

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

// tone builds raw 16-bit mono audio of a constant amplitude.
func tone(samples int, amplitude float64) []byte {
	raw := make([]byte, samples*2)

	for i := 0; i < samples; i++ {
		// A square wave, so every sample is at the amplitude and the RMS of
		// any window is the amplitude exactly. A sine would make the expected
		// value depend on where the window happened to fall.
		v := amplitude
		if i%2 == 1 {
			v = -amplitude
		}

		binary.LittleEndian.PutUint16(raw[i*2:], uint16(int16(v)))
	}

	return raw
}

// The envelope must line up with the sound in time.
//
// This is the whole reason the type exists rather than measuring the pipe
// directly, so it is the thing worth testing: audio written in three loud and
// quiet stretches has to read back loud and quiet at the same offsets.
func TestVoiceMeterEnvelopeFollowsPlaybackTime(t *testing.T) {
	const rate = 22050

	m := newVoiceMeter(rate)

	// Half a second of each, in turn.
	half := rate / 2

	if _, err := m.Write(tone(half, 4000)); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := m.Write(tone(half, 0)); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := m.Write(tone(half, 2000)); err != nil {
		t.Fatalf("write: %v", err)
	}

	cases := []struct {
		at   time.Duration
		want float64
	}{
		{250 * time.Millisecond, 4000},
		{750 * time.Millisecond, 0},
		{1250 * time.Millisecond, 2000},
	}

	for _, c := range cases {
		got := m.at(c.at)

		if math.Abs(got-c.want) > 1 {
			t.Errorf("at %v: RMS %.0f, want %.0f", c.at, got, c.want)
		}
	}

	// Past the end, while more sound is still coming, the newest measurement
	// stands: the synthesiser is behind, not finished.
	if got := m.at(3 * time.Second); math.Abs(got-2000) > 1 {
		t.Errorf("past the envelope while unsealed: RMS %.0f, want the newest 2000", got)
	}

	// Once the synthesiser has finished, past the end really is silence.
	// Holding the last value here is what would leave the display lit after
	// the voice had stopped.
	m.seal()

	if got := m.at(3 * time.Second); got != 0 {
		t.Errorf("past the envelope after sealing: RMS %.0f, want 0", got)
	}
}

// A display must not go flat while the brain is still talking.
//
// This is the case that made the distinction necessary. Synthesis on this
// machine runs only a little faster than speech, so under load the player runs
// out of audio and waits. Playback pauses; the wall clock does not. Treating
// the clock as the playback position then walks off the end of what has been
// generated, and the trace falls to nothing in the middle of a sentence — the
// exact opposite of what it is for.
func TestVoiceMeterHoldsTheLineWhileSynthesisLags(t *testing.T) {
	const rate = 22050

	m := newVoiceMeter(rate)

	// A tenth of a second of sound has been produced so far.
	if _, err := m.Write(tone(rate/10, 3200)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The clock is a long way past that, because generation is behind.
	if got := m.at(2 * time.Second); math.Abs(got-3200) > 1 {
		t.Errorf("while generation lags: RMS %.0f, want the newest 3200", got)
	}
}

// Samples split across writes must not shift the envelope.
//
// The pipe hands over whatever size chunks it likes, and if a half sample were
// dropped at each boundary the envelope would slide steadily out of step with
// the audio over a long answer — quietly, and worse the longer the brain talks.
func TestVoiceMeterKeepsTimeAcrossOddWrites(t *testing.T) {
	const rate = 1000

	m := newVoiceMeter(rate)

	// One second of steady tone, delivered in chunks that mostly end in the
	// middle of a sample.
	raw := tone(rate, 3000)

	for i := 0; i < len(raw); i += 7 {
		end := i + 7
		if end > len(raw) {
			end = len(raw)
		}

		if _, err := m.Write(raw[i:end]); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// 20ms blocks over one second, less any final partial block.
	if len(m.blocks) < 49 {
		t.Fatalf("envelope holds %d blocks, want the whole second", len(m.blocks))
	}

	for _, at := range []time.Duration{0, 500 * time.Millisecond, 960 * time.Millisecond} {
		if got := m.at(at); math.Abs(got-3000) > 1 {
			t.Errorf("at %v: RMS %.0f, want 3000", at, got)
		}
	}
}

// A reading has to expire, or the display freezes holding the last word.
func TestLiveLevelExpires(t *testing.T) {
	publishLevel("mic", LevelReference/2)

	if got := LiveLevel(); got.Source != "mic" || math.Abs(got.Level-0.5) > 0.001 {
		t.Fatalf("fresh reading: %+v, want mic at 0.5", got)
	}

	meter.mu.Lock()
	meter.mic.written = time.Now().Add(-2 * LevelFreshness)
	meter.mu.Unlock()

	if got := LiveLevel(); got.Source != "" || got.Level != 0 {
		t.Errorf("stale reading: %+v, want silence", got)
	}

	// Loud sound must not read as more than full scale, or the interface has
	// to defend itself against its own data source.
	publishLevel("voice", LevelReference*10)

	if got := LiveLevel(); got.Level != 1 {
		t.Errorf("very loud: level %v, want it capped at 1", got.Level)
	}

	clearLevel("voice")

	if got := LiveLevel(); got.Source != "" {
		t.Errorf("after clearing: %+v, want silence", got)
	}
}

// The brain's own voice must be visible to the microphone.
//
// This is what stops the brain transcribing itself on speakers, so the two
// sources have to be tracked separately. They shared a slot once, and because
// both publish continuously the answer to "is the brain talking?" depended on
// which had written most recently — it was wrong about half the time it was
// asked, which is worse than useless for a guard.
func TestSpeakingIsNotLostWhenTheMicrophoneIsAlsoOpen(t *testing.T) {
	clearLevel("voice")
	clearLevel("mic")

	if Speaking() {
		t.Fatal("nothing is playing, but the brain reports itself as speaking")
	}

	publishLevel("voice", LevelReference/2)

	// The microphone goes on reporting the room the whole time the brain is
	// talking, which is exactly the situation that broke the shared slot.
	for i := 0; i < 5; i++ {
		publishLevel("mic", LevelReference/4)

		if !Speaking() {
			t.Fatalf("microphone reading %d hid the fact that the brain is speaking", i)
		}
	}

	// And the display shows the voice rather than flickering to the room.
	if got := LiveLevel(); got.Source != "voice" {
		t.Errorf("the display shows %q, want the voice while it is talking", got.Source)
	}

	clearLevel("voice")

	if Speaking() {
		t.Error("the voice has stopped but still reports as speaking")
	}

	if got := LiveLevel(); got.Source != "mic" {
		t.Errorf("after the voice stopped the display shows %q, want the microphone", got.Source)
	}
}
