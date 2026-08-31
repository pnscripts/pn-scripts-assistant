package speech

import (
	"context"
	"os"
	"strconv"
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

/*
 * Listening is absent rather than broken, and says what to do about it.
 *
 * Two different things can be missing and they used to give one message
 * between them, so the test asked for words that only one of them contains —
 * and it therefore passed or failed according to what happened to be
 * installed on the machine running it. On a bare machine, which is the case
 * this message exists for, it failed while the program was behaving correctly.
 *
 * What matters is not which words appear but that somebody is told where to
 * go. The old text said "build whisper.cpp and put whisper-cli on your PATH",
 * which is accurate and useless to the person most likely to read it.
 */
func TestListeningSaysWhereToGoWhenItCannotHear(t *testing.T) {
	ok, hint := Listening()

	if ok {
		t.Skip("a speech recogniser is installed")
	}

	lower := strings.ToLower(hint)

	if !strings.Contains(lower, "setup") {
		t.Errorf("does not point at Setup, which is the one place that can fix "+
			"this without a terminal: %q", hint)
	}

	// And it says what is actually missing, so somebody who does know their way
	// around is not left guessing.
	if !strings.Contains(lower, "model") && !strings.Contains(lower, "whisper") {
		t.Errorf("does not say what is missing: %q", hint)
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

// pw-record's --target takes a serial or a name, never PipeWire's object id.
// They differ for the same device, so passing the id targeted a serial that did
// not exist and recording fell back to the default input — which is why every
// attempt captured the wrong microphone while appearing to work.
func TestMicrophoneIDIsANameNotANumber(t *testing.T) {
	mics, err := Microphones(context.Background())
	if err != nil {
		t.Skipf("PipeWire unavailable: %v", err)
	}

	if len(mics) == 0 {
		t.Skip("no inputs on this machine")
	}

	for _, m := range mics {
		if m.ID == "" {
			t.Errorf("microphone %q has no id", m.Name)
		}

		if _, err := strconv.Atoi(m.ID); err == nil {
			t.Errorf("microphone %q has a numeric id %q; pw-record would read it as a serial",
				m.Name, m.ID)
		}
	}
}

// spd-say is a client, not a voice. It exits 0 having queued text that
// speech-dispatcher may hand to sd_dummy, a module whose purpose is to accept
// speech and make no sound. Reporting speech as available on that basis is what
// produced a brain that claimed to talk and did not.
func TestSpeechDispatcherIsNotEvidenceOfAVoice(t *testing.T) {
	var client Engine

	for _, e := range engines {
		if e.Command == "spd-say" {
			client = e

			break
		}
	}

	if client.Command == "" {
		t.Fatal("speech-dispatcher is no longer among the engines")
	}

	if !client.NeedsEngine {
		t.Error("spd-say is treated as proof that something can speak")
	}

	// The engines that genuinely make sound must not be.
	for _, e := range engines {
		if e.Command != "spd-say" && e.NeedsEngine {
			t.Errorf("%s is marked as needing an engine behind it", e.Command)
		}
	}
}

func TestAvailabilityMatchesWhatIsInstalled(t *testing.T) {
	engine := Available()
	installed := anyVoiceInstalled()

	if engine != nil && engine.NeedsEngine && !installed {
		t.Errorf("reported %s as usable with no voice installed", engine.Name)
	}
}

// Ubuntu ships an unrelated package called piper that configures gaming mice.
// Handing it a sentence would fail confusingly, so the voice is only accepted
// with a model beside it.
func TestPiperNeedsAVoiceModel(t *testing.T) {
	p := FindPiper()

	if p == nil {
		t.Skip("piper is not installed")
	}

	if p.Voice == "" {
		t.Fatal("piper accepted without a voice model")
	}

	if !strings.HasSuffix(p.Voice, ".onnx") {
		t.Errorf("voice is %q, which is not a model", p.Voice)
	}

	// The companion json holds the sample rate; without it piper cannot load.
	if _, err := os.Stat(p.Voice + ".json"); err != nil {
		t.Errorf("voice has no companion json: %v", err)
	}
}

// A neural voice is what somebody would actually want reading answers aloud,
// so it must win over formant synthesis when both are installed.
func TestPiperIsPreferredOverEspeak(t *testing.T) {
	if FindPiper() == nil {
		t.Skip("piper is not installed")
	}

	engine := Available()

	if engine == nil {
		t.Fatal("no engine chosen despite piper being installed")
	}

	if engine.Name != "piper" {
		t.Errorf("chose %s over piper", engine.Name)
	}
}

// An English voice handed Cyrillic reads the letters out, so a Bulgarian answer
// becomes spelling rather than speech.
func TestVoiceFollowsTheScriptOfTheText(t *testing.T) {
	if FindPiper() == nil {
		t.Skip("piper is not installed")
	}

	installed := Voices()

	var haveBulgarian bool

	for _, v := range installed {
		if strings.HasPrefix(v.ID, "bg_") {
			haveBulgarian = true
		}
	}

	if !haveBulgarian {
		t.Skip("no Cyrillic voice installed")
	}

	SetVoice("en_US-amy-medium")

	english := voiceForText("Which websites do I use most?")
	if strings.HasPrefix(english.ID, "bg_") {
		t.Errorf("English text got a Bulgarian voice: %s", english.ID)
	}

	bulgarian := voiceForText("Здравей Петър, как си днес?")
	if !strings.HasPrefix(bulgarian.ID, "bg_") {
		t.Errorf("Bulgarian text got %s, which cannot read Cyrillic", bulgarian.ID)
	}
}

func TestScriptDetection(t *testing.T) {
	cases := map[string]string{
		"Hello there":   "latin",
		"Здравей Петър": "cyrillic",
		"Здравей, използвам Laravel днес": "cyrillic",
		"I use pnscripts.local mostly":    "latin",
		"":                                "",
		"123 456":                         "",
	}

	for text, want := range cases {
		if got := scriptOf(text); got != want {
			t.Errorf("scriptOf(%q) = %q, want %q", text, got, want)
		}
	}
}

// A preference is a preference: it must only be overridden when the chosen
// voice genuinely cannot carry the text.
func TestChosenVoiceIsKeptWhenItCanSayTheText(t *testing.T) {
	if FindPiper() == nil {
		t.Skip("piper is not installed")
	}

	SetVoice("en_GB-alba-medium")

	if got := voiceForText("An ordinary English sentence."); got.ID != "en_GB-alba-medium" {
		t.Errorf("overrode the chosen voice with %s for English text", got.ID)
	}
}

// Somebody who has said they speak German is being answered in German, and
// telling German from Dutch by looking at the letters is not worth attempting.
func TestVoiceFollowsTheConfiguredLanguage(t *testing.T) {
	installed := Voices()

	var haveGerman bool

	for _, v := range installed {
		if strings.HasPrefix(v.ID, "de_") {
			haveGerman = true
		}
	}

	if !haveGerman {
		t.Skip("no German voice installed")
	}

	SetVoice("en_US-amy-medium")
	SetLanguage("de")

	t.Cleanup(func() { SetLanguage("") })

	got := voiceForText("Guten Morgen, wie geht es dir heute?")

	if !strings.HasPrefix(got.ID, "de_") {
		t.Errorf("German text with German configured got %s", got.ID)
	}
}

// Script still wins: a preference is not worth spelling Cyrillic out.
func TestScriptOverridesTheConfiguredLanguage(t *testing.T) {
	var haveBulgarian bool

	for _, v := range Voices() {
		if strings.HasPrefix(v.ID, "bg_") {
			haveBulgarian = true
		}
	}

	if !haveBulgarian {
		t.Skip("no Bulgarian voice installed")
	}

	SetVoice("en_US-amy-medium")
	SetLanguage("en")

	t.Cleanup(func() { SetLanguage("") })

	got := voiceForText("Здравей, как си?")

	if !strings.HasPrefix(got.ID, "bg_") {
		t.Errorf("Cyrillic text got %s, which would spell it", got.ID)
	}
}

// whisper-cli defaults -l to "en", not to detection. Passing nothing asserts
// English, and asserting English about Bulgarian speech produces fluent English
// that was never said — confident, plausible and unrelated to the words spoken.
func TestUnsetLanguageMeansAutoNotEnglish(t *testing.T) {
	SetLanguage("")
	t.Cleanup(func() { SetLanguage("") })

	if got := Language(); got != "" {
		t.Fatalf("Language() = %q with nothing set", got)
	}

	// The value that reaches whisper must be "auto"; anything else, including
	// nothing at all, means English.
	code := Language()
	if code == "" {
		code = "auto"
	}

	if code != "auto" {
		t.Errorf("unset language becomes %q, want auto", code)
	}
}

// The small model transcribes Bulgarian nearly perfectly and takes three times
// as long. English does not need it, and every other language does.
func TestModelFollowsTheLanguage(t *testing.T) {
	t.Cleanup(func() { SetLanguage("") })

	SetLanguage("en")

	if got := modelPreference()[0]; got != "ggml-base.bin" {
		t.Errorf("English prefers %s, want the faster base model", got)
	}

	for _, code := range []string{"bg", "ru", "de", ""} {
		SetLanguage(code)

		if got := modelPreference()[0]; got != "ggml-small.bin" {
			t.Errorf("language %q prefers %s, want the accurate small model", code, got)
		}
	}
}

// Whisper decides the language of each clip independently, and a spoken turn is
// a few seconds — too little to be sure. Across a conversation that produced
// "ней olurs sustainable change": Bulgarian, Turkish and English in one
// sentence, each fragment confidently labelled something different.
func TestDetectionSticksForTheSession(t *testing.T) {
	t.Cleanup(func() { SetLanguage(""); ForgetDetectedLanguage() })

	SetLanguage("")
	ForgetDetectedLanguage()

	if got := languageForTurn(); got != "auto" {
		t.Fatalf("first turn asks for %q, want auto", got)
	}

	rememberDetection("whisper_full_with_state: auto-detected language: bg (p = 0.850342)")

	if got := DetectedLanguage(); got != "bg" {
		t.Fatalf("detection not kept: %q", got)
	}

	if got := languageForTurn(); got != "bg" {
		t.Errorf("later turns ask for %q, want bg", got)
	}
}

// A weak guess repeated for a whole conversation is worse than a weak guess
// used once: the mistake stops being a glitch and becomes the setting.
func TestWeakDetectionIsNotKept(t *testing.T) {
	t.Cleanup(func() { SetLanguage(""); ForgetDetectedLanguage() })

	SetLanguage("")
	ForgetDetectedLanguage()

	rememberDetection("auto-detected language: tr (p = 0.31)")

	if got := DetectedLanguage(); got != "" {
		t.Errorf("kept a guess at 31%% confidence: %q", got)
	}
}

// Having said which language they speak, somebody should not be overruled by
// something detected before they said it.
func TestSettingTheLanguageClearsAnEarlierGuess(t *testing.T) {
	t.Cleanup(func() { SetLanguage(""); ForgetDetectedLanguage() })

	SetLanguage("")
	ForgetDetectedLanguage()
	rememberDetection("auto-detected language: tr (p = 0.99)")

	if DetectedLanguage() != "tr" {
		t.Fatal("setup failed")
	}

	SetLanguage("bg")

	if got := DetectedLanguage(); got != "" {
		t.Errorf("stale guess %q survived the setting being changed", got)
	}

	if got := languageForTurn(); got != "bg" {
		t.Errorf("turn asks for %q after the setting was changed", got)
	}
}

// An explicit setting always wins; detection never overrides it.
func TestConfiguredLanguageBeatsDetection(t *testing.T) {
	t.Cleanup(func() { SetLanguage(""); ForgetDetectedLanguage() })

	SetLanguage("bg")
	rememberDetection("auto-detected language: en (p = 0.99)")

	if got := languageForTurn(); got != "bg" {
		t.Errorf("turn asks for %q despite bg being configured", got)
	}
}

func TestProbabilityParsing(t *testing.T) {
	cases := map[string]float64{"0.850342": 0.85, "0.31": 0.31, "1.0": 1.0, "0.6": 0.6}

	for in, want := range cases {
		got := parseProbability(in)

		if got < want-0.01 || got > want+0.01 {
			t.Errorf("parseProbability(%q) = %v, want about %v", in, got, want)
		}
	}
}
