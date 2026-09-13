package speech

import (
	"slices"
	"strings"
	"testing"
)

/*
 * The voice this answers in is a woman's, and it is asked for by name.
 *
 * The first attempt at this changed which neural model the robot is built
 * from, which was correct and inaudible: there is no neural model on most
 * machines, so what comes out is espeak — and espeak was being run with no
 * voice argument at all, using its own default, which is a man.
 */
func TestTheSystemVoiceIsAskedForByName(t *testing.T) {
	espeak := &Engine{Name: "espeak-ng", Command: "espeak-ng"}

	UseVariant("Andrea")

	got := speakingAs(espeak, "forty four packages passing")

	if !slices.Equal(got, []string{"-v", "en+Andrea", "-s", "175", "-p", "30"}) {
		t.Errorf("english ran with %v", got)
	}

	// And the same woman in Bulgarian, because an English voice handed
	// Cyrillic reads the letters out one at a time.
	if got := speakingAs(espeak, "четиридесет и четири пакета"); got[1] != "bg+Andrea" {
		t.Errorf("bulgarian ran with %v", got)
	}
}

/*
 * The language is a base code, never a dialect.
 *
 * Measured, not assumed: espeak accepts "en-GB+anikaRobot", exits zero, and
 * uses its default voice — byte-for-byte identical audio. "en+anikaRobot"
 * is the form that actually changes the sound, and the difference between
 * them is invisible from the exit status.
 */
func TestTheLanguageIsNeverADialect(t *testing.T) {
	espeak := &Engine{Name: "espeak-ng", Command: "espeak-ng"}

	UseVariant("Andrea")

	for _, text := range []string{"hello", "четиридесет", ""} {
		got := speakingAs(espeak, text)

		if len(got) < 2 {
			t.Fatalf("%q ran with %v", text, got)
		}

		language, _, _ := strings.Cut(got[1], "+")

		if strings.Contains(language, "-") {
			t.Errorf("%q asked for the dialect %q", text, language)
		}
	}
}

/*
 * A variant is never sent without a language.
 *
 * espeak segfaults on a bare variant name — exit 139, a core dump, and no
 * sound. Worth a test rather than a comment, because it is the sort of thing
 * a later simplification would reintroduce.
 */
func TestAVariantIsNeverSentAlone(t *testing.T) {
	UseVariant("Andrea")

	got := speakingAs(&Engine{Name: "espeak-ng", Command: "espeak-ng"}, "hello")

	if len(got) >= 2 && !strings.Contains(got[1], "+") {
		t.Errorf("a bare variant was sent: %v", got)
	}
}

// A machine with no variant installed says nothing about voices rather than
// naming one that is not there, since espeak fails outright on a name it does
// not know and failing to speak is worse than the wrong voice.
func TestAMachineWithoutTheVoiceAsksForNothing(t *testing.T) {
	UseVariant("")

	if got := speakingAs(&Engine{Name: "espeak-ng", Command: "espeak-ng"}, "hello"); got != nil {
		t.Errorf("it asked for %v", got)
	}
}

// And a voice type for speech-dispatcher, which has no variants of its own
// and cannot be checked — it exits zero whether or not the voice was used.
func TestSpeechDispatcherStillGetsAWoman(t *testing.T) {
	got := speakingAs(&Engine{Name: "speech-dispatcher", Command: "spd-say"}, "hello")

	if len(got) != 2 {
		t.Fatalf("it ran with %v", got)
	}

	if got[0] != "-y" && !(got[0] == "-t" && got[1] == "female1") {
		t.Errorf("it ran with %v", got)
	}
}

/*
 * It is never slowed down, because slowing espeak stretches vowels.
 *
 * The mistake this replaced: 165 words a minute with a gap between words, on
 * the reasoning that slower is clearer. True of the neural voice, false here —
 * "good morning" came out as "gooood mmoorrniiing". espeak is tuned for its
 * own default and reads worse below it.
 */
func TestItIsNeverSlowedDown(t *testing.T) {
	for _, style := range TheWomanRobot {
		if style.Words < 175 {
			t.Errorf("%s speaks at %d words a minute", style.Variant, style.Words)
		}
	}

	UseVariant("Andrea")

	for _, arg := range speakingAs(&Engine{Name: "espeak-ng", Command: "espeak-ng"}, "hello") {
		if arg == "-g" {
			t.Error("a gap between words was asked for")
		}
	}
}

/*
 * The voices it will use carry no echo, breath, roughness or flutter.
 *
 * Every one of those smears a consonant, and each was tried: anikaRobot's echo
 * at an amplitude of ten thousand, then f4's, which is mild and still an echo.
 * What makes it a machine is the flat intonation and the unstretched stresses,
 * which cost nothing in clarity.
 */
func TestTheVoicesDoNotSmearConsonants(t *testing.T) {
	if TheWomanRobot[0].Variant != "Andrea" {
		t.Errorf("the first choice is %q", TheWomanRobot[0].Variant)
	}

	for _, style := range TheWomanRobot {
		if style.Variant == "anikaRobot" {
			t.Error("anikaRobot is back; it smears every word into the next")
		}
	}
}

// Each voice is brought down from its own pitch, not from a shared one: f4
// starts at 142 and Andrea at 200, and one adjustment that softens the second
// makes the first a mumble.
func TestEachVoiceIsTunedFromItsOwnPitch(t *testing.T) {
	byName := map[string]Delivery{}

	for _, style := range TheWomanRobot {
		byName[style.Variant] = style
	}

	if byName["Andrea"].Pitch >= byName["f4"].Pitch {
		t.Errorf("Andrea starts higher and is not brought down further: %+v", byName)
	}
}
