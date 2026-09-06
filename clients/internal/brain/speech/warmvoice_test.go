package speech

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

/*
 * The synthesiser kept waiting for the next thing to be said.
 *
 * Measured on this machine: starting piper and loading a voice is 0.94
 * seconds, and speaking is 4.6 times faster than real time once it has. Which
 * means the startup is not a tax on speaking — it is the whole of the delay
 * before the first word, and it is paid at the worst moment, after the model
 * has already finished and while somebody is sitting in silence.
 *
 * A real voice is not started here. These tests use `cat`, which behaves the
 * way piper does in the only respects the pump cares about: it waits on stdin,
 * it writes what it is given to stdout, and it exits when stdin closes.
 */

/*
 * A stand-in for piper: it ignores the model arguments and echoes its input.
 *
 * Written to a file rather than run as `cat` directly, because startVoice
 * hands the binary --model and --output-raw and cat rejects them — which is
 * itself worth knowing, since it is exactly what a wrong piper on the PATH
 * does.
 */
func fakeBinary(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "not-piper")

	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec cat\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	return path
}

func spareFor(t *testing.T, voice string) *warmVoice {
	t.Helper()

	WarmVoice(fakeBinary(t), voice, false)

	// Started in the background, on purpose — nothing should ever wait for a
	// spare to be ready.
	for i := 0; i < 200; i++ {
		spareMu.Lock()
		ready := spare != nil && spare.voice == voice
		spareMu.Unlock()

		if ready {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	got := takeVoice(voice, false)
	if got == nil {
		t.Fatal("no synthesiser was made ready")
	}

	return got
}

func TestAVoiceIsMadeReadyBeforeThereIsAnythingToSay(t *testing.T) {
	defer StopWarmVoice()

	ready := spareFor(t, "a-voice")
	defer ready.discard()

	// The point of the whole thing: by the time there are words, the process
	// exists and the model is loaded.
	if ready.cmd.Process == nil {
		t.Fatal("the synthesiser was not actually started")
	}
}

// Taking the spare must leave nothing behind, or the next sentence would be
// handed a process that is already busy speaking the last one.
func TestASpareIsOnlyHandedOutOnce(t *testing.T) {
	defer StopWarmVoice()

	first := spareFor(t, "a-voice")
	defer first.discard()

	if again := takeVoice("a-voice", false); again != nil {
		again.discard()

		t.Fatal("the same synthesiser was handed out twice")
	}
}

/*
 * A spare for the wrong voice is worse than none.
 *
 * The voice changes with the language of the answer — a Bulgarian sentence
 * cannot be said by an English model, it gets spelled out — so handing over
 * whatever happens to be warm would trade one second of silence for an answer
 * nobody can understand.
 */
func TestASpareForAnotherVoiceIsNotUsed(t *testing.T) {
	defer StopWarmVoice()

	ready := spareFor(t, "english")
	defer ready.discard()

	spareMu.Lock()
	spare = ready
	spareMu.Unlock()

	if got := takeVoice("bulgarian", false); got != nil {
		t.Fatal("a spare for the wrong voice was handed over")
	}
}

// Warming a different voice replaces the one waiting, rather than leaving a
// process holding 96MB for a language nobody is speaking.
func TestWarmingAnotherVoiceLetsGoOfTheOldOne(t *testing.T) {
	defer StopWarmVoice()

	first := spareFor(t, "english")

	spareMu.Lock()
	spare = first
	spareMu.Unlock()

	second := spareFor(t, "bulgarian")
	defer second.discard()

	if second.voice != "bulgarian" {
		t.Fatalf("warmed the wrong voice: %s", second.voice)
	}

	// The first one is stopped, not left running.
	if err := first.cmd.Wait(); err == nil {
		// Killed processes report an error; a clean exit here would mean it
		// was never stopped at all. Either way it must not still be running.
		if first.cmd.ProcessState == nil {
			t.Fatal("the old synthesiser was left running")
		}
	}
}

// A spare that has sat all afternoon is replaced rather than trusted: the
// audio device it was going to write to may not be there any more.
func TestAVeryOldSpareIsNotUsed(t *testing.T) {
	defer StopWarmVoice()

	ready := spareFor(t, "a-voice")

	ready.started = time.Now().Add(-StaleSpare - time.Minute)

	spareMu.Lock()
	spare = ready
	spareMu.Unlock()

	if got := takeVoice("a-voice", false); got != nil {
		got.discard()

		t.Fatal("a stale synthesiser was handed over")
	}
}

// One line, whatever it is given. Piper says each line separately, so a
// sentence with a newline in it would be spoken as two with a gap between.
func TestASentenceReachesTheVoiceAsOneLine(t *testing.T) {
	defer StopWarmVoice()

	ready := spareFor(t, "a-voice")

	if err := ready.say("Yes.\nAnd here is why.\n\n  Really."); err != nil {
		t.Fatal(err)
	}

	said, err := io.ReadAll(ready.audio)
	if err != nil {
		t.Fatal(err)
	}

	ready.cmd.Wait()

	if string(said) != "Yes. And here is why. Really.\n" {
		t.Fatalf("the voice was handed %q", string(said))
	}
}

/*
 * Interrupting must stop the voice generating, not only playing.
 *
 * A spare belongs to no turn — that is what makes it fast — so the turn that
 * uses it takes on killing it. Without that, cutting in kills the player and
 * leaves a synthesiser producing a sentence nobody will hear, which on a
 * four-core machine is a quarter of a core spent on nothing.
 */
func TestInterruptingStopsTheVoiceGenerating(t *testing.T) {
	defer StopWarmVoice()

	ready := spareFor(t, "a-voice")

	ctx, cancel := context.WithCancel(context.Background())

	stop := ready.dieWith(ctx)
	defer stop()

	cancel()

	done := make(chan error, 1)

	go func() { done <- ready.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the synthesiser went on running after the turn was abandoned")
	}
}

/*
 * The robot and the ordinary voice are the same model file.
 *
 * They differ only in how piper is told to speak it, which means a waiting
 * synthesiser has to be identified by both. Keyed on the path alone, a spare
 * started for one would be handed the other's sentences — and the robot would
 * occasionally answer in the plain voice, which is the kind of fault that
 * looks like a haunting.
 */
func TestARobotSpareIsNotUsedForThePlainVoice(t *testing.T) {
	defer StopWarmVoice()

	const model = "/voices/lessac.onnx"

	ready := spareFor(t, model)

	spareMu.Lock()
	spare = ready
	spare.voice = key(model, true)
	spareMu.Unlock()

	if got := takeVoice(model, false); got != nil {
		got.discard()

		t.Fatal("a synthesiser set up for the robot was handed a plain sentence")
	}

	if got := takeVoice(model, true); got == nil {
		t.Fatal("the robot's own spare was not handed back to it")
	}
}

/*
 * What makes it a machine is how it speaks, not what is done to the sound.
 *
 * Every earlier robot treated the audio — espeak, a ring modulator, a comb
 * filter, a pitch shift — and every one of them cost some of the words. The
 * expression is turned down instead, which leaves the phonemes exactly as the
 * neural voice made them.
 */
func TestTheMachineIsInTheDeliveryAndCostsNoClarity(t *testing.T) {
	args := machineArgs()

	var flat, even, paced bool

	for i, a := range args {
		if i+1 >= len(args) {
			break
		}

		switch a {
		case "--noise_scale":
			flat = args[i+1] < "0.300"
		case "--noise_w":
			even = args[i+1] < "0.300"
		case "--length_scale":
			paced = args[i+1] > "1.000"
		}
	}

	if !flat || !even {
		t.Fatalf("the delivery is not flat enough to read as a machine: %v", args)
	}

	if !paced {
		t.Fatalf("a machine that is understood is not in a hurry: %v", args)
	}

	// And nothing here touches the audio: no filter, no modulator, no
	// resampler. That is the whole point of the change.
	if len(args) != 6 {
		t.Fatalf("something beyond the delivery was added: %v", args)
	}
}
