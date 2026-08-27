package speech

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
		if path, err := exec.LookPath("piper"); err == nil && !isMouseTool(path) {
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
	Args    func(rate int) []string
}{
	{"pw-play", func(rate int) []string {
		return []string{"--rate", fmt.Sprint(rate), "--channels", "1", "--format", "s16", "-"}
	}},
	{"aplay", func(rate int) []string {
		return []string{"-q", "-r", fmt.Sprint(rate), "-f", "S16_LE", "-c", "1", "-t", "raw", "-"}
	}},
	{"paplay", func(rate int) []string {
		return []string{"--raw", "--rate=" + fmt.Sprint(rate), "--channels=1", "--format=s16le"}
	}},
}

// Speak synthesises and plays, returning when the sound has finished.
func (p *Piper) Speak(ctx context.Context, text string) error {
	player := players[0]
	found := false

	for _, candidate := range players {
		if _, err := exec.LookPath(candidate.Command); err == nil {
			player = candidate
			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf("no way to play audio: none of pw-play, aplay or paplay is installed")
	}

	voice := p.Voice

	// The owner's choice wins over whichever model happened to be found first.
	if chosen := CurrentVoice(); chosen.Engine == "piper" && chosen.Path != "" {
		voice = chosen.Path
	}

	synth := exec.CommandContext(ctx, p.Binary,
		"--model", voice, "--output-raw")
	synth.Stdin = strings.NewReader(text)
	synth.Stderr = io.Discard

	audio, err := synth.StdoutPipe()
	if err != nil {
		return err
	}

	play := exec.CommandContext(ctx, player.Command, player.Args(p.Rate)...)
	play.Stdin = audio
	play.Stderr = io.Discard

	if err := synth.Start(); err != nil {
		return fmt.Errorf("could not start piper: %w", err)
	}

	if err := play.Start(); err != nil {
		synth.Process.Kill()
		synth.Wait()

		return fmt.Errorf("could not start %s: %w", player.Command, err)
	}

	synth.Wait()

	// Waiting on the player, not the synthesiser: piper finishes generating
	// well before the sound has been heard, and conversation mode must not
	// reopen the microphone until the room is quiet again.
	return play.Wait()
}
