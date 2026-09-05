package speech

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"pn-brain/internal/brain/exe"
	"pn-brain/internal/brain/pace"
	"strings"
	"sync"
)

// Piper is a neural voice that runs on the CPU.
//
// Worth the extra machinery over espeak-ng, which is formant synthesis and
// sounds it. Measured here: 4.6 seconds of speech generated in 0.84, so five
// times faster than real time — fast enough that the voice is never what a
// conversation is waiting for.
//
// It reads text on standard input and writes raw samples out, which are piped
// straight to the sound server rather than through a temporary file. That means
// audio starts as soon as the first samples exist instead of after the whole
// sentence has been synthesised.
type Piper struct {
	Binary string
	Voice  string
	Rate   int
}

// piperSearch are the places a piper installation is normally put.
func piperSearch(home string) []string {
	return []string{
		filepath.Join(home, ".local", "src", "piper"),
		filepath.Join(home, ".local", "share", "piper"),
		filepath.Join(home, "piper"),
		"/opt/piper",
		"/usr/local/lib/piper",
	}
}

var (
	piperOnce  sync.Once
	piperFound *Piper
)

// FindPiper locates a piper binary and a voice for it.
//
// Both are required. piper without a voice model exits with an error, which
// would present as a broken installation rather than a missing download.
func FindPiper() *Piper {
	piperOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			home = ""
		}

		var binary string

		// A piper on PATH is preferred, but the tarball is usually unpacked
		// somewhere without adding it.
		if path, found := exe.Look("piper"); found && !isMouseTool(path) {
			binary = path
		}

		dirs := piperSearch(home)

		if binary == "" {
			for _, dir := range dirs {
				candidate := filepath.Join(dir, "piper")

				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					binary = candidate

					break
				}
			}
		}

		if binary == "" {
			return
		}

		// The voice is a .onnx beside the binary, or under a voices directory.
		roots := append([]string{filepath.Dir(binary)}, dirs...)

		for _, root := range roots {
			for _, sub := range []string{"voices", "."} {
				matches, _ := filepath.Glob(filepath.Join(root, sub, "*.onnx"))

				for _, voice := range matches {
					// The companion .json holds the sample rate; without it
					// piper cannot load the voice.
					if _, err := os.Stat(voice + ".json"); err != nil {
						continue
					}

					piperFound = &Piper{Binary: binary, Voice: voice, Rate: 22050}

					return
				}
			}
		}
	})

	return piperFound
}

// isMouseTool guards against Ubuntu's unrelated package of the same name, which
// configures gaming mice and would fail confusingly if handed a sentence.
func isMouseTool(path string) bool {
	return strings.Contains(path, "/bin/piper") &&
		fileContains("/usr/share/applications/piper.desktop", "libratbag")
}

func fileContains(path, needle string) bool {
	raw, err := os.ReadFile(path)

	return err == nil && strings.Contains(string(raw), needle)
}

// players that can take raw samples on standard input.
var players = []struct {
	Command string
	Args    func(rate int, level float64) []string
}{
	{"pw-play", func(rate int, level float64) []string {
		args := []string{"--rate", fmt.Sprint(rate), "--channels", "1", "--format", "s16"}

		/*
		 * Its own level, which is the only one this program touches.
		 *
		 * Set on this stream and nowhere else: turning the speakers down to
		 * make the assistant quieter takes the music with it, and every other
		 * program on the machine has a volume of its own. See loudness.go.
		 */
		args = append(args, volumeArgs(level)...)

		/*
		 * Played into the canceller's sink, when there is one.
		 *
		 * This is what lets the assistant be interrupted mid-sentence. The
		 * canceller removes from the microphone whatever was played into its
		 * sink, so sound sent straight to the hardware output is sound it
		 * never sees and cannot subtract — and the microphone then hears the
		 * assistant's own voice as clearly as the person's. Saying its name
		 * while it was talking started a turn made of its own words.
		 */
		if sink := PreferredSpeaker(context.Background()); sink != "" {
			args = append(args, "--target", sink)
		}

		return append(args, "-")
	}},
	// aplay and paplay take no level, so on a machine with only those the
	// setting has nothing to act on and the voice plays at the system's level.
	{"aplay", func(rate int, _ float64) []string {
		return []string{"-q", "-r", fmt.Sprint(rate), "-f", "S16_LE", "-c", "1", "-t", "raw", "-"}
	}},
	{"paplay", func(rate int, _ float64) []string {
		return []string{"--raw", "--rate=" + fmt.Sprint(rate), "--channels=1", "--format=s16le"}
	}},
}

// Speak synthesises and plays, returning when the sound has finished.
func (p *Piper) Speak(ctx context.Context, text string) error {
	// Recorded by the callers in speech.go, so that every engine does it and
	// not only this one. See SpeakAndWait.

	player := players[0]
	found := false

	for _, candidate := range players {
		if exe.Has(candidate.Command) {
			player = candidate
			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf("no way to play audio: none of pw-play, aplay or paplay is installed")
	}

	voice := p.Voice

	/*
	 * The voice follows the language of the text. An English model handed
	 * Cyrillic reads the letters out, so a Bulgarian answer becomes spelling.
	 */
	asMachine := false

	if chosen := voiceForText(text); chosen.Engine == "piper" && chosen.Path != "" {
		voice = chosen.Path
		asMachine = chosen.ID == RobotID
	}

	/*
	 * The synthesiser, which has usually been running since the last thing was
	 * said and is waiting for a line.
	 *
	 * Falling back to starting one here is not a rare path — it is what
	 * happens on the very first sentence after the language changes — so it
	 * has to work exactly as it used to, one second slower.
	 */
	synth := takeVoice(voice)

	if synth == nil {
		started, err := startVoice(p.Binary, voice)
		if err != nil {
			return err
		}

		synth = started
	}

	// And another for whatever is said next, made while this one is speaking.
	defer WarmVoice(p.Binary, voice)

	// A spare belongs to no turn, so this turn takes on stopping it. Without
	// this, interrupting kills the player and leaves the voice generating.
	defer synth.dieWith(ctx)()

	if err := synth.say(text); err != nil {
		return err
	}

	/*
	 * And the rest of the room's sound comes down for as long as this takes.
	 *
	 * Here rather than at the top of SpeakAndWait, because that path hands
	 * this to a goroutine and returns — so the release would run before a
	 * sound had been made and the music would never dip at all.
	 */
	defer duckWhileTalking(ctx)()

	// The sound is measured on its way past, so the interface can respond to
	// the voice that is actually being produced. See voiceMeter for why this
	// is not simply read off the pipe as it flows.
	level := newVoiceMeter(p.Rate)

	play := exec.CommandContext(ctx, player.Command, player.Args(p.Rate, LevelFor(ctx))...)
	play.Stderr = io.Discard

	/*
	 * And the machine timbre, put on the way past, when the robot is the one
	 * speaking.
	 *
	 * Here rather than in the model, because the words and the timbre come
	 * from different places on purpose: the clarity is the neural voice's and
	 * the character is this. See robot.go.
	 */
	var treat func(io.Reader) io.Reader

	if asMachine {
		treat = func(from io.Reader) io.Reader { return robotise(from, p.Rate) }
	}

	return pumpAudio(synth, play, level, audioHooks{
		playing:   level.start,
		generated: level.seal,
		done:      level.finish,
		treat:     treat,
	})
}

// pumpAudio carries the synthesiser's output to the player, showing everything
// that passes to meter, and does not return until the sound has been heard.
//
// The plumbing is separated out and written by hand because doing the obvious
// thing here silently cuts the end off every sentence, and it is worth being
// precise about why.
//
// The obvious version sets play.Stdin to an io.TeeReader wrapping the
// synthesiser's StdoutPipe. Two documented behaviours then collide. Because the
// reader is not an *os.File, os/exec cannot hand it to the child directly, so
// it runs a goroutine copying from it into a pipe of its own. And StdoutPipe's
// contract says Wait closes the pipe as soon as the command exits, so it is
// wrong to call Wait before every read has finished. Piper generates far faster
// than the sound plays, so it exits while the copying goroutine is still
// blocked writing into a player that is only consuming at the speed of speech.
// Waiting on the synthesiser at that moment closes the pipe under the goroutine
// and throws away everything still in it — up to a pipe buffer of audio, which
// is well over a second of talking. The voice simply stops mid-sentence.
//
// Handing the player a real file descriptor takes os/exec's goroutine out of it
// and puts the copy here, where it can be waited for before anything is closed.
// audioHooks are the three moments the caller may care about.
//
// Named rather than positional because "generated" and "done" are easy to
// confuse and mean very different things: the first is when the synthesiser has
// produced everything, the second is when the sound has finished being heard,
// and on a slow machine they can be seconds apart.
type audioHooks struct {
	// playing fires when the player has started.
	playing func()
	// generated fires when every byte has left the synthesiser.
	generated func()
	// done fires when the sound has finished.
	done func()

	// treat wraps the audio on its way to the player, for the robot. Nil
	// leaves the sound exactly as the synthesiser produced it.
	treat func(io.Reader) io.Reader
}

func pumpAudio(synth *warmVoice, play *exec.Cmd, meter io.Writer, hooks audioHooks) error {
	audio := synth.audio

	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}

	// An *os.File, so os/exec passes it to the child as-is.
	play.Stdin = pr

	if err := play.Start(); err != nil {
		pr.Close()
		pw.Close()
		synth.discard()

		return fmt.Errorf("could not start %s: %w", play.Path, err)
	}

	// The player has its own copy now.
	pr.Close()

	copied := make(chan struct{})

	go func() {
		defer close(copied)

		sound := io.Reader(audio)

		if hooks.treat != nil {
			sound = hooks.treat(sound)
		}

		/*
		 * The first byte of sound, which is where the waiting really ends.
		 *
		 * Not when the player was started: that happens immediately, while the
		 * synthesiser is still working out the first phonemes, and timing from
		 * there would report every answer as instant and hide the one cost
		 * that no better machine will fix.
		 */
		sound = firstByte(sound, pace.FirstSound)

		/*
		 * The meter sees what is played, not what was generated.
		 *
		 * It drives the light in the interface and the detection of the
		 * brain's own voice coming back off the speakers, and both of those
		 * are about the sound in the room — which after the treatment is not
		 * quite the sound the synthesiser made.
		 */
		io.Copy(io.MultiWriter(pw, meter), sound)

		// Closing the write end is what tells the player the sound has ended.
		pw.Close()
	}()

	if hooks.playing != nil {
		hooks.playing()
	}

	// Every byte is out of the synthesiser before it is waited for. This
	// ordering is the whole point of the function.
	<-copied

	if hooks.generated != nil {
		hooks.generated()
	}

	synth.cmd.Wait()

	// Waiting on the player, not the synthesiser: piper finishes generating
	// well before the sound has been heard, and conversation mode must not
	// reopen the microphone until the room is quiet again.
	err = play.Wait()

	if hooks.done != nil {
		hooks.done()
	}

	return err
}

/*
 * firstByte calls back the moment anything is actually read.
 *
 * A reader rather than a check inside the copy loop, because the copy is an
 * io.Copy into a MultiWriter and unpicking that to count bytes would mean
 * hand-rolling the part of this file that took two attempts to get right.
 */
func firstByte(from io.Reader, at func()) io.Reader {
	var once sync.Once

	return readerFunc(func(p []byte) (int, error) {
		n, err := from.Read(p)

		if n > 0 {
			once.Do(at)
		}

		return n, err
	})
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }
