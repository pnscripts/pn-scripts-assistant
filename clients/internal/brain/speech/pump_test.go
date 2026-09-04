package speech

import (
	"bytes"
	"os/exec"
	"testing"
)

// Every byte must reach the player.
//
// This is a regression test for a real fault: the end of every spoken sentence
// was being cut off, by more than a second, because the synthesiser was waited
// for while audio was still in flight. See the comment on pumpAudio.
//
// The shape of the test is the whole of it, and getting it wrong is easy: the
// first version of this test passed against the broken code.
//
// The fault needs the generator to have *exited* while audio is still in
// flight, because it is waiting on the exited generator that closes the pipe.
// A generator producing more than the pipes can hold cannot exit — it blocks on
// its own write — so a large volume hides the fault rather than exposing it.
// The amount here is chosen to fit inside the buffers so that the generator
// finishes immediately, as a synthesiser generating faster than speech does,
// and the reader waits long enough that the sound is certain to still be in the
// pipe at that moment.
func TestPumpAudioDeliversEveryByte(t *testing.T) {
	const size = 100_000

	synth := fakeVoice(t, "head -c 100000 /dev/zero")

	var heard bytes.Buffer

	play := exec.Command("sh", "-c", "sleep 1.2; cat")
	play.Stdout = &heard

	var measured bytes.Buffer

	if err := pumpAudio(synth, play, &measured, audioHooks{}); err != nil {
		t.Fatalf("pumpAudio: %v", err)
	}

	if heard.Len() != size {
		t.Errorf("the player received %d bytes, want %d — the sound was cut short",
			heard.Len(), size)
	}

	// The meter must see the same sound, or the display would stop moving
	// before the voice did.
	if measured.Len() != size {
		t.Errorf("the meter saw %d bytes, want %d", measured.Len(), size)
	}
}

// The callbacks bracket the sound.
//
// The meter's clock starts from the first and stops at the second, so if they
// fired at the wrong moments the trace would be offset from the voice.
func TestPumpAudioSignalsStartAndEnd(t *testing.T) {
	var order []string

	synth := fakeVoice(t, "head -c 2000 /dev/zero")

	play := exec.Command("cat")
	play.Stdout = &bytes.Buffer{}

	err := pumpAudio(synth, play, &bytes.Buffer{}, audioHooks{
		playing:   func() { order = append(order, "playing") },
		generated: func() { order = append(order, "generated") },
		done:      func() { order = append(order, "done") },
	})
	if err != nil {
		t.Fatalf("pumpAudio: %v", err)
	}

	want := []string{"playing", "generated", "done"}

	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Errorf("callbacks fired as %v, want %v", order, want)
	}
}

// A player that cannot start must be reported, not hung on.
func TestPumpAudioReportsAMissingPlayer(t *testing.T) {
	synth := fakeVoice(t, "head -c 100 /dev/zero")
	play := exec.Command("/nonexistent/player")

	if err := pumpAudio(synth, play, &bytes.Buffer{}, audioHooks{}); err == nil {
		t.Fatal("a missing player was reported as success")
	}
}

/*
 * fakeVoice stands in for a synthesiser that is already running.
 *
 * The real one is started before there is anything to say and handed a line
 * when there is — see warmvoice.go — so what the pump receives is a process
 * already producing, not one to launch. That is the state these tests have to
 * reproduce, since the fault they exist for is entirely about when the
 * generator exits relative to the audio still in flight.
 */
func fakeVoice(t *testing.T, script string) *warmVoice {
	t.Helper()

	cmd := exec.Command("sh", "-c", script)

	audio, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	words, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	return &warmVoice{cmd: cmd, audio: audio, words: words, voice: "test"}
}
