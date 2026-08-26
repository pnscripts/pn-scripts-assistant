package speech

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Recogniser is a speech-to-text program and the model it needs.
type Recogniser struct {
	Command string
	Model   string
}

// recogniserNames, in preference order. whisper.cpp first: it is built for CPU
// inference and on a machine without a GPU it is several times faster than the
// Python implementation, which drags in torch for the same result.
var recogniserNames = []string{"whisper-cli", "whisper-cpp", "whisper", "main"}

// modelSearch are the places a whisper.cpp model is normally left.
//
// The model is looked for as well as the binary, because whisper without one
// starts, prints an error about a missing file, and exits — which presents as
// "listening is broken" rather than "listening needs a model".
func modelSearch(home string) []string {
	return []string{
		filepath.Join(home, ".local", "src", "whisper.cpp", "models"),
		filepath.Join(home, ".cache", "whisper"),
		filepath.Join(home, "whisper.cpp", "models"),
		"/usr/share/whisper.cpp/models",
		"/usr/local/share/whisper.cpp/models",
	}
}

// preferredModels, best-value first for a CPU. base.en is the sweet spot:
// noticeably better than tiny, and roughly three times realtime on four cores.
var preferredModels = []string{
	"ggml-base.en.bin", "ggml-base.bin",
	"ggml-small.en.bin", "ggml-small.bin",
	"ggml-tiny.en.bin", "ggml-tiny.bin",
	"ggml-medium.en.bin", "ggml-medium.bin",
}

// FindRecogniser locates a usable speech recogniser, or reports what is missing.
func FindRecogniser() (*Recogniser, string) {
	var command string

	for _, name := range recogniserNames {
		if path, err := exec.LookPath(name); err == nil {
			command = path

			break
		}
	}

	if command == "" {
		return nil, "No speech recogniser found. Build whisper.cpp and put whisper-cli on your PATH."
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	for _, dir := range modelSearch(home) {
		for _, name := range preferredModels {
			path := filepath.Join(dir, name)

			// Test models shipped with whisper.cpp are a megabyte of nothing
			// and would transcribe silence; a real one is over 50MB.
			info, err := os.Stat(path)
			if err != nil || info.Size() < 50<<20 {
				continue
			}

			return &Recogniser{Command: command, Model: path}, ""
		}
	}

	return nil, command + " is installed but no model was found. Fetch one with: " +
		"bash ./models/download-ggml-model.sh base.en"
}

// MaxRecordSeconds bounds one utterance.
//
// Long enough for a real question, short enough that a forgotten open
// microphone stops on its own rather than recording the room indefinitely.
const MaxRecordSeconds = 20

// Record captures audio from the default microphone into a WAV file.
//
// 16kHz mono signed 16-bit, because that is what whisper expects; anything else
// is resampled internally at best and misheard at worst.
func Record(ctx context.Context, seconds int, path string) error {
	if _, err := exec.LookPath("arecord"); err != nil {
		return fmt.Errorf("arecord is not installed, so the microphone cannot be read")
	}

	if seconds <= 0 || seconds > MaxRecordSeconds {
		seconds = MaxRecordSeconds
	}

	cmd := exec.CommandContext(ctx, "arecord",
		"-q", "-f", "S16_LE", "-r", "16000", "-c", "1",
		"-d", fmt.Sprint(seconds), path)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("recording failed: %s", strings.TrimSpace(string(out)))
	}

	return nil
}

// Transcribe turns a WAV file into text.
func Transcribe(ctx context.Context, wav string) (string, error) {
	r, why := FindRecogniser()
	if r == nil {
		return "", fmt.Errorf("%s", why)
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	// -nt drops timestamps; the brain wants the sentence, not a subtitle file.
	cmd := exec.CommandContext(ctx, r.Command, "-m", r.Model, "-f", wav, "-nt", "-np")

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}

	return CleanTranscript(string(out)), nil
}

// CleanTranscript tidies what whisper produces.
//
// Whisper emits bracketed annotations for non-speech — [BLANK_AUDIO],
// (silence), *coughs* — and passing those to the brain as a question produces
// an answer to nothing. Silence should read as silence.
func CleanTranscript(raw string) string {
	var kept []string

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Drop a line that is entirely an annotation.
		if isAnnotation(line) {
			continue
		}

		kept = append(kept, line)
	}

	text := strings.Join(kept, " ")

	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

func isAnnotation(line string) bool {
	pairs := [][2]string{{"[", "]"}, {"(", ")"}, {"*", "*"}}

	for _, p := range pairs {
		if strings.HasPrefix(line, p[0]) && strings.HasSuffix(line, p[1]) {
			return true
		}
	}

	return false
}

// Listen records from the microphone and returns what was said.
func Listen(ctx context.Context, seconds int) (string, error) {
	f, err := os.CreateTemp("", "pn-brain-listen-*.wav")
	if err != nil {
		return "", err
	}

	path := f.Name()
	f.Close()

	defer os.Remove(path)

	if err := Record(ctx, seconds, path); err != nil {
		return "", err
	}

	return Transcribe(ctx, path)
}
