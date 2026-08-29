package speech

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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

// preferredModels, best first for a CPU.
//
// Two orderings, both chosen from measurements on Bulgarian speech.
//
// Multilingual before .en: an .en model given Bulgarian does not fail, it
// forces the sounds into English words, so "здравей Петър" came back as
// "Strava pater". Somebody who only speaks English loses nothing by having the
// multilingual model; somebody who does not loses everything by the other.
//
// small before base: on the same sentence base produced "Здравей, Асм Петер і
// Днес Работй по проектам" and small produced "Здравей, аз съм Петер и днес
// работи по проекта" — two word errors against a mangled sentence. base is
// adequate for English and not for much else, and transcription is not what a
// reply is waiting for: the model stays resident, so the cost is paid on a few
// seconds of audio rather than on the minute of thinking that follows.
var preferredModels = []string{
	"ggml-small.bin", "ggml-base.bin", "ggml-medium.bin",
	"ggml-small.en.bin", "ggml-base.en.bin", "ggml-medium.en.bin",
	"ggml-tiny.bin", "ggml-tiny.en.bin",
}

// englishModels are preferred when English is the configured language.
//
// The smaller model is three times faster and, on English, no worse in any way
// that showed. It is only on other languages that it falls apart — which is
// exactly when the slower one earns its seconds.
var englishModels = []string{
	"ggml-base.bin", "ggml-base.en.bin", "ggml-small.bin", "ggml-small.en.bin",
	"ggml-tiny.bin", "ggml-tiny.en.bin",
}

// modelPreference is the order to look in, given what is being spoken.
func modelPreference() []string {
	if strings.EqualFold(Language(), "en") {
		return englishModels
	}

	return preferredModels
}

// language is the spoken language, as an ISO code. Empty means whisper decides.
var (
	language   string
	languageMu sync.RWMutex
)

// SetLanguage fixes which language is being spoken.
//
// Worth setting rather than leaving to detection. Told nothing, whisper may
// translate instead of transcribing — Bulgarian speech came back as "Hello,
// Peter. I am your great assistant." rather than in Cyrillic — which is a
// confusing failure because the words are right and the language is not.
func SetLanguage(code string) {
	languageMu.Lock()
	language = code
	languageMu.Unlock()

	// Somebody who has just said which language they speak should not be
	// overruled by something guessed before they said it.
	ForgetDetectedLanguage()
}

// Language reports the configured spoken language.
func Language() string {
	languageMu.RLock()
	defer languageMu.RUnlock()

	return language
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
		for _, name := range modelPreference() {
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

// Record captures audio into a WAV file.
//
// 16kHz mono signed 16-bit, because that is what whisper expects; anything else
// is resampled internally at best and misheard at worst.
//
// PipeWire is preferred and a device may be named, because ALSA's "default"
// capture device is not reliably a microphone somebody is speaking into. On
// this machine it is the built-in analog jack, which records near-silence while
// a USB microphone sits unused — and that presents as a broken recogniser.
func Record(ctx context.Context, seconds int, device, path string) error {
	if seconds <= 0 || seconds > MaxRecordSeconds {
		seconds = MaxRecordSeconds
	}

	if _, err := exec.LookPath("pw-record"); err == nil {
		return recordPipeWire(ctx, seconds, device, path)
	}

	if _, err := exec.LookPath("arecord"); err != nil {
		return fmt.Errorf("no way to record: neither pw-record nor arecord is installed")
	}

	args := []string{"-q", "-f", "S16_LE", "-r", "16000", "-c", "1", "-d", fmt.Sprint(seconds)}

	if device != "" {
		args = append(args, "-D", device)
	}

	cmd := exec.CommandContext(ctx, "arecord", append(args, path)...)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("recording failed: %s", strings.TrimSpace(string(out)))
	}

	return nil
}

// recordPipeWire records for a fixed time.
//
// pw-record has no duration flag, so it is stopped by the clock here. It is
// asked to stop politely first: killing it outright can leave the WAV header
// unwritten, and a header-less file transcribes as nothing at all.
func recordPipeWire(ctx context.Context, seconds int, device, path string) error {
	args := []string{"--rate", "16000", "--channels", "1", "--format", "s16"}

	if device != "" {
		args = append(args, "--target", device)
	}

	cmd := exec.CommandContext(ctx, "pw-record", append(args, path)...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start recording: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		cmd.Process.Signal(syscall.SIGINT)
		<-done

		return ctx.Err()
	case <-time.After(time.Duration(seconds) * time.Second):
		cmd.Process.Signal(syscall.SIGINT)

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
			<-done
		}
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
	// The language is always passed, and "auto" is a real value rather than
	// the absence of one.
	//
	// whisper-cli defaults -l to "en", not to detection. Passing nothing
	// therefore asserts English, and asserting English about Bulgarian speech
	// does not fail — it produces fluent English that was never said. "Здравей,
	// аз съм Петър" came back as "Hello, I'm Petr", which is the worst kind of
	// wrong: confident, plausible, and unrelated to the words spoken.
	args := []string{"-m", r.Model, "-f", wav, "-nt", "-np", "-l", languageForTurn()}

	cmd := exec.CommandContext(ctx, r.Command, args...)

	// Whisper announces its detection on stderr. Reading it here means a
	// normal transcription teaches the session its language, without the extra
	// pass that asking separately would cost.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}

	rememberDetection(stderr.String())

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

// Heard is the result of listening.
type Heard struct {
	Text  string `json:"text"`
	Level Level  `json:"level"`

	// Advice is empty when something was understood. Otherwise it says which
	// kind of nothing happened.
	Advice string `json:"advice,omitempty"`

	/*
	 * What the detector actually measured.
	 *
	 * Carried out of here because "I said its name and nothing happened" has
	 * several completely different causes that look identical from the
	 * outside: the room never crossed the threshold, or it did and the
	 * transcript came back empty, or it came back as a different word. Without
	 * these numbers all three are the same silence.
	 */
	HeardSpeech bool `json:"heard_speech"`
	PeakRMS     int  `json:"peak_rms"`
	NoiseFloor  int  `json:"noise_floor"`
	Threshold   int  `json:"threshold"`
	SpokeForMS  int  `json:"spoke_for_ms"`
}

// Listen records from a microphone and returns what was said.
func Listen(ctx context.Context, seconds int, device string) (Heard, error) {
	f, err := os.CreateTemp("", "pn-brain-listen-*.wav")
	if err != nil {
		return Heard{}, err
	}

	path := f.Name()
	f.Close()

	defer os.Remove(path)

	if err := Record(ctx, seconds, device, path); err != nil {
		return Heard{}, err
	}

	level, err := MeasureWAV(path)
	if err != nil {
		return Heard{}, err
	}

	// Transcribing silence wastes seconds and invites whisper to invent
	// something from the noise floor, which it does — "(waves crashing)" from
	// an empty room on this machine.
	if level.Silent {
		return Heard{Level: level, Advice: Explain(level, "")}, nil
	}

	text, err := Transcribe(ctx, path)
	if err != nil {
		return Heard{Level: level}, err
	}

	return Heard{Text: text, Level: level, Advice: Explain(level, text)}, nil
}
