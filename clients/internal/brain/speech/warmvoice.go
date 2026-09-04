package speech

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

/*
 * A synthesiser started before there is anything to say.
 *
 * Measured on this machine: piper takes 0.94 seconds to start and load its
 * voice, and then produces speech at four and a half times real time. Started
 * fresh for each sentence — which is what this did — that load is paid on the
 * first sentence of every answer, and it lands in the worst possible place:
 * after the model has finished thinking, while somebody is sitting in silence
 * waiting for a reply that has in fact already been written.
 *
 * It is also the cost that does not improve. A graphics card takes the model
 * from ninety seconds to half a second and does nothing whatever to this, so on
 * the machine this program is aiming at, a second of process startup stops
 * being a rounding error and becomes most of the delay.
 *
 * So one is kept started and waiting on an open pipe. Answering then costs
 * writing a line to it. The spare is replaced as soon as it is used, during the
 * answer it is speaking, which is time nobody is waiting through.
 *
 * The cost of keeping it: 96MB of memory and no processor at all — it sits
 * blocked on a read. That is a fair trade for a second, and it is one process
 * rather than a pool, because the second sentence of an answer is generated
 * while the first is still being heard and was never waiting on anything.
 */
type warmVoice struct {
	cmd   *exec.Cmd
	words io.WriteCloser
	audio io.ReadCloser
	voice string

	// started is when it was made ready, so a spare that has been sitting
	// unused for a long time can be replaced rather than trusted.
	started time.Time
}

// StaleSpare is how long a waiting synthesiser is trusted.
//
// Long enough to cover a normal gap in a conversation, short enough that one
// left over from a voice or a device that has since changed is not the thing
// that speaks. Its stdin has been open and empty the whole time, which nothing
// in piper minds, but the audio device it will write to may have moved.
const StaleSpare = 30 * time.Minute

var (
	spareMu sync.Mutex
	spare   *warmVoice
	warming bool
)

/*
 * startVoice starts a synthesiser and leaves it waiting for words.
 *
 * Not bound to a context. A spare outlives the turn that created it by
 * design — that is the whole point — so the turn that eventually uses it takes
 * on the job of killing it if it is interrupted. See Piper.Speak.
 */
func startVoice(binary, voice string) (*warmVoice, error) {
	cmd := exec.Command(binary, "--model", voice, "--output-raw")

	dieWithParent(cmd)

	cmd.Stderr = io.Discard

	words, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	audio, err := cmd.StdoutPipe()
	if err != nil {
		words.Close()

		return nil, err
	}

	if err := cmd.Start(); err != nil {
		words.Close()
		audio.Close()

		return nil, fmt.Errorf("could not start the synthesiser: %w", err)
	}

	return &warmVoice{
		cmd: cmd, words: words, audio: audio, voice: voice, started: time.Now(),
	}, nil
}

/*
 * WarmVoice makes a synthesiser ready for the next thing to be said.
 *
 * Safe to call whenever: it does nothing if one is already waiting for this
 * voice, and it never blocks the caller. Called at startup, and again each
 * time a spare is used up.
 */
func WarmVoice(binary, voice string) {
	if binary == "" || voice == "" {
		return
	}

	spareMu.Lock()

	if warming || (spare != nil && spare.voice == voice) {
		spareMu.Unlock()

		return
	}

	warming = true

	// A spare for a different voice is no use — the language changed, or the
	// robot was chosen — and holding it would keep the right one from being
	// made.
	stale := spare
	spare = nil

	spareMu.Unlock()

	if stale != nil {
		stale.discard()
	}

	go func() {
		ready, err := startVoice(binary, voice)

		spareMu.Lock()
		defer spareMu.Unlock()

		warming = false

		if err != nil {
			return
		}

		// Something may have taken and replaced the spare while this one was
		// loading. The newer one wins; this is closed rather than leaked.
		if spare != nil {
			go ready.discard()

			return
		}

		spare = ready
	}()
}

// takeVoice hands over the waiting synthesiser if it is the right one.
func takeVoice(voice string) *warmVoice {
	spareMu.Lock()
	defer spareMu.Unlock()

	if spare == nil || spare.voice != voice {
		return nil
	}

	if time.Since(spare.started) > StaleSpare {
		stale := spare
		spare = nil

		go stale.discard()

		return nil
	}

	ready := spare
	spare = nil

	return ready
}

// say hands the synthesiser its line and tells it that is all.
//
// One line: piper treats each line of its input as a separate thing to say, so
// a sentence with a newline in it would be spoken as two with a pause between.
func (w *warmVoice) say(text string) error {
	line := strings.Join(strings.Fields(text), " ")

	if _, err := io.WriteString(w.words, line+"\n"); err != nil {
		w.words.Close()

		return err
	}

	// Closing stdin is what tells piper there is nothing more coming, and it
	// is what makes it exit when this sentence is done.
	return w.words.Close()
}

// discard stops a synthesiser that will not be used.
func (w *warmVoice) discard() {
	w.words.Close()

	if w.cmd.Process != nil {
		w.cmd.Process.Kill()
	}

	w.cmd.Wait()
}

// dieWith kills the synthesiser when the turn is abandoned.
//
// A spare is not tied to any context, so this restores what
// exec.CommandContext used to do: interrupting an answer has to stop the voice
// generating it, or the sentence goes on being produced into a player that has
// already been killed.
func (w *warmVoice) dieWith(ctx context.Context) (stop func() bool) {
	return context.AfterFunc(ctx, func() {
		if w.cmd.Process != nil {
			w.cmd.Process.Kill()
		}
	})
}

// StopWarmVoice lets go of any waiting synthesiser. Used when speech is turned
// off, so nothing is held open for a voice that will not be used.
func StopWarmVoice() {
	spareMu.Lock()
	held := spare
	spare = nil
	spareMu.Unlock()

	if held != nil {
		held.discard()
	}
}

/*
 * WarmTheVoice makes ready whichever voice would speak next.
 *
 * Called at startup and whenever the voice is changed. The choice is made the
 * same way it is made for a real sentence — by asking for an empty one — so a
 * machine set to Bulgarian warms the Bulgarian voice rather than warming the
 * wrong one and paying the second anyway.
 */
func WarmTheVoice() {
	p := FindPiper()
	if p == nil {
		return
	}

	voice := p.Voice

	if chosen := voiceForText(""); chosen.Engine == "piper" && chosen.Path != "" {
		voice = chosen.Path
	}

	WarmVoice(p.Binary, voice)
}
