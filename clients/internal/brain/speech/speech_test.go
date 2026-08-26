package speech

import (
	"strings"
	"testing"
)

// A path read out character by character is unbearable and conveys nothing.
// It is on screen, where it can be read properly.
func TestReadableStripsWhatShouldNotBeSpoken(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		absent  []string
		present []string
	}{
		{
			name:    "long path",
			in:      "It lives at /media/petar/c8fc2986/DEV/Projects/xplorer/xplorer-golang-api now.",
			absent:  []string{"c8fc2986", "xplorer-golang-api"},
			present: []string{"It lives at", "that path", "now."},
		},
		{
			name:    "url",
			in:      "See https://github.com/PNScripts/pn-brain for details.",
			absent:  []string{"github.com", "https"},
			present: []string{"a link", "for details."},
		},
		{
			name:    "code fence",
			in:      "Run this:\n```bash\ngo build ./cmd/brain\n```\nThen try again.",
			absent:  []string{"go build", "```"},
			present: []string{"Run this:", "code", "Then try again."},
		},
		{
			name:    "markdown",
			in:      "That is **important** and _worth_ noting.",
			absent:  []string{"**", "_"},
			present: []string{"important", "worth", "noting."},
		},
		{
			name:    "inline code keeps its content",
			in:      "The flag is `--apply` here.",
			absent:  []string{"`"},
			present: []string{"--apply"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Readable(c.in)

			for _, a := range c.absent {
				if strings.Contains(got, a) {
					t.Errorf("still says %q: %q", a, got)
				}
			}

			for _, p := range c.present {
				if !strings.Contains(got, p) {
					t.Errorf("lost %q: %q", p, got)
				}
			}
		})
	}
}

// Nobody listens to four minutes of synthetic speech, and there is no way to
// skim it.
func TestSpokenTextIsBounded(t *testing.T) {
	got := Readable(strings.Repeat("This is a sentence worth hearing. ", 200))

	if len([]rune(got)) > MaxSpokenChars {
		t.Errorf("would speak %d characters, want at most %d", len([]rune(got)), MaxSpokenChars)
	}

	// Cutting mid-word sounds like a fault rather than an ending.
	if !strings.HasSuffix(strings.TrimSpace(got), ".") {
		t.Errorf("did not end on a sentence: %q", got[len(got)-40:])
	}
}

func TestOrdinaryProseIsUnchanged(t *testing.T) {
	s := "There are five Go projects, and two of them are under xplorer."

	if got := Readable(s); got != s {
		t.Errorf("ordinary prose was altered:\n  got  %q\n  want %q", got, s)
	}
}

// Being unable to speak is a normal state on a server or a minimal desktop, and
// the message has to say what to install.
func TestMissingEngineExplainsItself(t *testing.T) {
	if Available() != nil {
		t.Skip("this machine has a speech engine; the failure path cannot be exercised")
	}

	err := Speak(nil, "hello")
	if err == nil {
		t.Fatal("expected an error with no engine")
	}

	if !strings.Contains(err.Error(), "install") {
		t.Errorf("does not say how to fix it: %v", err)
	}
}

// Listening is absent rather than broken, and says what it needs.
func TestListeningReportsWhatIsMissing(t *testing.T) {
	ok, hint := Listening()

	if ok {
		t.Skip("a speech recogniser is installed")
	}

	for _, want := range []string{"model", "whisper"} {
		if !strings.Contains(strings.ToLower(hint), want) {
			t.Errorf("hint does not mention %q: %q", want, hint)
		}
	}
}

// Whisper emits bracketed annotations for non-speech. Passing those on as a
// question produces a confident answer to nothing.
func TestAnnotationsAreNotTreatedAsSpeech(t *testing.T) {
	silent := []string{
		"[BLANK_AUDIO]",
		"(silence)",
		"*coughs*",
		"[ Silence ]\n[BLANK_AUDIO]",
		"",
		"\n\n  \n",
	}

	for _, raw := range silent {
		if got := CleanTranscript(raw); got != "" {
			t.Errorf("CleanTranscript(%q) = %q, want empty", raw, got)
		}
	}
}

func TestRealSpeechSurvivesCleaning(t *testing.T) {
	raw := "\n [BLANK_AUDIO]\n Where does the xplorer project live?\n"

	got := CleanTranscript(raw)

	if got != "Where does the xplorer project live?" {
		t.Errorf("got %q", got)
	}
}

// Recording longer than this means a forgotten open microphone.
func TestRecordingIsBounded(t *testing.T) {
	if MaxRecordSeconds > 60 {
		t.Errorf("MaxRecordSeconds is %d; an open microphone should stop sooner", MaxRecordSeconds)
	}
}

// The 1MB placeholders shipped with whisper.cpp would transcribe everything as
// silence, and the failure would look like a broken microphone.
func TestTestModelsAreNotMistakenForRealOnes(t *testing.T) {
	r, why := FindRecogniser()

	if r == nil {
		t.Skipf("no recogniser installed: %s", why)
	}

	if strings.Contains(r.Model, "for-tests-") {
		t.Errorf("selected a test placeholder as the model: %s", r.Model)
	}
}

// "Heard nothing" has causes that need opposite fixes. Conflating them sends
// somebody to their audio settings when they simply need to speak.
func TestAdviceDistinguishesSilenceFromRoomNoiseFromSpeech(t *testing.T) {
	cases := []struct {
		name  string
		level Level
		text  string
		want  string
	}{
		{"understood", Level{Peak: 12000, Ratio: 0.37}, "hello there", ""},
		{"nothing at all", Level{Peak: 200, Silent: true, Ratio: 0.006}, "", "silent"},
		{"room tone only", Level{Peak: 3555, Ratio: 0.108}, "", "room noise"},
		{"loud but unclear", Level{Peak: 20000, Ratio: 0.61}, "", "Speech-level"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Explain(c.level, c.text)

			if c.want == "" {
				if got != "" {
					t.Errorf("advised on a successful transcript: %q", got)
				}

				return
			}

			if !strings.Contains(got, c.want) {
				t.Errorf("advice %q does not mention %q", got, c.want)
			}
		})
	}
}

// A sentence that reads the same at every volume tells a person nothing about
// whether speaking louder is helping.
func TestAdviceQuotesTheActualLevel(t *testing.T) {
	quiet := Explain(Level{Peak: 3555, Ratio: 0.108}, "")
	louder := Explain(Level{Peak: 6000, Ratio: 0.183}, "")

	if quiet == louder {
		t.Error("advice is identical at different levels")
	}

	if !strings.Contains(quiet, "10%") || !strings.Contains(louder, "18%") {
		t.Errorf("levels not reported:\n  %q\n  %q", quiet, louder)
	}
}

// The real recording that started this: peak 3555 from an input that works.
func TestTheMeasuredRoomToneIsNotCalledSilent(t *testing.T) {
	level := Level{Peak: 3555, RMS: 996, Ratio: 3555.0 / 32767}

	if level.Peak < SilenceThreshold {
		t.Fatal("room tone would be reported as a dead input")
	}

	if level.Peak >= SpeechPeak {
		t.Fatal("room tone would be reported as speech")
	}
}
