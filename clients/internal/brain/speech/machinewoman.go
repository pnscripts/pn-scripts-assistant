package speech

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
 * The voice this answers in: a woman's, and plainly a machine.
 *
 * Asked for, and it took two goes to get right. The first change picked which
 * neural model the robot is built from — correct, and inaudible on a machine
 * with no neural voice installed, which is most of them and is this one. The
 * voice that actually comes out of the speakers here is espeak-ng, and espeak
 * was being run with no voice argument at all, so it used its own default,
 * which is a man.
 *
 * espeak calls these variants and ships a set of them. anikaRobot is declared
 * female, with flat formants and an echo — a woman's voice that is audibly
 * synthesised, which is the whole of what was asked for and needs no treatment
 * on the audio afterwards. f3 is the plain female variant, kept behind it for
 * an espeak build that has no anikaRobot.
 *
 * Probed once rather than assumed. A variant that is not installed makes
 * espeak fail outright, and failing to speak because of a voice preference
 * would be a worse outcome than the wrong voice.
 */

/*
 * TheRobot is the voice the robot speaks in, best first.
 *
 * Changed on its owner's instruction, and the instruction was about what the
 * three choices mean: "the woman must be for woman, the man must be for man,
 * but the third voice must be for robot". The third was a woman's voice
 * spoken flat — chosen on the reasoning that a machine is made by delivery
 * rather than by timbre, which is true of what a voice *does* and says
 * nothing about whose voice it is. Somebody choosing "a robot" from three
 * options where the other two are a woman and a man is not asking for a third
 * person. They are asking for a machine.
 *
 * klatt is that. It is not a recording of anybody and not modelled on one: it
 * is the Klatt formant synthesiser, the sound of a machine speaking, and it
 * belongs to no sex — which is the whole reason it is the right answer here
 * and the reason it was not considered before.
 *
 * High, because that is what was asked for, and because a formant voice
 * carries better high than low: the consonants sit above the vowels rather
 * than under them.
 *
 * Andrea and the f-variants are still here, behind it, for an espeak build
 * with no klatt at all — a worse robot rather than a different kind of thing.
 */
var TheRobot = []Delivery{
	/*
	 * klatt4 at 45, chosen by ear against five others.
	 *
	 * The first answer here was klatt at 70 — high, because high was asked
	 * for — and it was rejected on hearing it: too thin and too chirpy to
	 * read as a machine. Then klatt4 at 45, and that was still too high.
	 * Fifteen is where it stopped: hard and electronic, low enough to have
	 * some weight behind it, and still espeak's own pitch control rather
	 * than anything done to the audio.
	 *
	 * Three goes, each decided by listening to it. "High" was a guess at the
	 * sound rather than a description of it, which is why none of this could
	 * be settled by reading.
	 *
	 * Nothing is done to the audio. A ring modulator was tried alongside
	 * these — it is the obvious way to make a voice electronic, and it is
	 * also how every earlier attempt at a robot here ended up unintelligible.
	 * It was not needed: the hardness is in the synthesiser, and a voice that
	 * is hard because of its formants is still a voice you can follow.
	 */
	{Variant: "klatt4", Words: 175, Pitch: 15},

	// The others in the family, for a build whose espeak has no klatt4.
	// Different formant tables rather than different treatments.
	{Variant: "klatt2", Words: 175, Pitch: 15},
	{Variant: "klatt", Words: 175, Pitch: 15},
	{Variant: "klatt3", Words: 175, Pitch: 15},
	{Variant: "klatt5", Words: 175, Pitch: 15},

	// And the old answer, for a build with no klatt: a woman's voice spoken
	// flat, which is a machine by delivery if not by timbre.
	{Variant: "Andrea", Words: 175, Pitch: 30},
	{Variant: "f4", Words: 175, Pitch: 45},
}

/*
 * TheWomanRobot is what the robot used to be, and is now what "a woman"
 * means on a machine with no neural voice: a woman's voice, spoken flat.
 *
 * Two wrong answers preceded this one and both are worth keeping written down,
 * because each was wrong in a way that sounded reasonable.
 *
 * anikaRobot first: unmistakably a machine and barely a voice. It declares
 * "echo 10 10000" — a ten-millisecond echo at an amplitude of ten thousand —
 * which smears every word into the one after it.
 *
 * Then f4, slowed to 165 words a minute with a gap between words, on the
 * reasoning that slower is clearer. That is true of the neural voice and false
 * here. espeak stretches vowels when it is slowed, so "good morning" came out
 * as "gooood mmoorrniiing" — and f4's own stressAdd puts +120 on the last
 * stress, which is exactly the syllable that drags.
 *
 * Andrea is the answer to both. It has no echo, no breath, no flutter and
 * roughness explicitly zero — nothing that smears a consonant. And it carries
 * the machine in the two lines that matter: "intonation 3", a flat contour
 * rather than a rising and falling one, and a stressLength table of small
 * numbers, which is espeak's way of saying do not stretch the stressed
 * syllables. Flat and unstretched is what a machine sounds like, and it is
 * also what is easiest to follow.
 *
 * Which is the same conclusion machinevoice.go reaches for the neural voice,
 * arrived at the hard way a second time: the machine is in the delivery, and
 * the delivery is a property of the voice, not something done to the sound
 * afterwards.
 */
var TheWomanRobot = []Delivery{
	// Andrea speaks high — 200 to 265 — so the pitch is brought a long way
	// down to make it soft. Nothing else is touched.
	{Variant: "Andrea", Words: 175, Pitch: 30},

	// f4 is lower to begin with, so it needs far less bringing down. Kept
	// behind Andrea for a machine whose espeak has no Andrea.
	{Variant: "f4", Words: 175, Pitch: 45},

	{Variant: "f2", Words: 175, Pitch: 45},
}

/*
 * TheMan is what "a man" means without a neural voice.
 *
 * espeak's own default is a man, which is why the very first version of this
 * — espeak run with no voice argument at all — sounded like one when a woman
 * had been asked for. m1 rather than that default, so the choice is a choice
 * rather than an absence.
 */
var TheMan = []Delivery{
	{Variant: "m1", Words: 175, Pitch: 40},
	{Variant: "m3", Words: 175, Pitch: 40},
}

/*
 * Delivery is one voice and how it is spoken.
 *
 * Per voice rather than one setting for all of them, because the variants
 * start from different pitches — 142 for f4 and 200 for Andrea — and a single
 * adjustment that softens one makes the other a mumble.
 *
 * No gap between words at all. A gap was tried, on the reasoning that an even
 * pace reads as a machine; what it actually does in espeak is make the speech
 * sound spaced out and dragged, which is the opposite of easy to follow.
 */
type Delivery struct {
	Variant string

	// Words a minute. espeak's own default is 175 and it is tuned for it;
	// below that it stretches vowels rather than slowing down.
	Words int

	// Pitch on espeak's scale of 0 to 99, where 50 is the middle.
	Pitch int
}

var (
	variantOnce      sync.Once
	chosenVoiceStyle Delivery

	womanOnce  sync.Once
	womanStyle Delivery

	manOnce  sync.Once
	manStyle Delivery
)

/*
 * Variant is the voice variant this machine can actually use, or empty.
 *
 * Worked out once and kept, because it cannot change while the program runs:
 * it is a question about which data files are installed beside espeak.
 */
func Variant() string { return Style().Variant }

// Style is the robot's voice on this machine, which is also the default.
func Style() Delivery {
	variantOnce.Do(func() {
		chosenVoiceStyle = firstThatWorks(TheRobot)
	})

	return chosenVoiceStyle
}

/*
 * StyleFor is the voice of one kind — robot, woman or man — on this machine.
 *
 * Three lists rather than one, because the three choices mean three different
 * things: "the woman must be for woman, the man must be for man, but the
 * third voice must be for robot". Before this there was one espeak voice for
 * all of them, so a machine with no neural voices offered three buttons and
 * one sound.
 *
 * Probed rather than assumed, and the answer kept: a variant that is not
 * installed makes espeak fail outright, and which data files are beside it
 * cannot change while the program runs.
 */
func StyleFor(kind string) Delivery {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "woman":
		womanOnce.Do(func() { womanStyle = firstThatWorks(TheWomanRobot) })

		return womanStyle

	case "man":
		manOnce.Do(func() { manStyle = firstThatWorks(TheMan) })

		return manStyle

	default:
		return Style()
	}
}

/*
 * chosenStyle is how the voice in use is spoken.
 *
 * A voice picked by name carries its own delivery and is used exactly as
 * picked; one of the three by-kind choices resolves to whichever variant this
 * machine has. Without this, every one of the thirty voices somebody can now
 * choose from would be read in the default of its kind — the list would
 * change, the sound would not, and nothing would say why.
 */
func chosenStyle() Delivery {
	chosen := CurrentVoice()

	if chosen.Style != nil {
		return *chosen.Style
	}

	return StyleFor(chosen.Sex)
}

/*
 * preferredStyles is the voices to ask for, best first, for an engine that
 * cannot be asked whether it took the first one.
 *
 * The one chosen, then the rest of its kind. This used to be the women's list
 * whatever had been chosen, so speech-dispatcher answered in a woman's voice
 * to somebody who had asked for a man — silently, because spd-say exits zero
 * either way.
 */
func preferredStyles() []Delivery {
	chosen := CurrentVoice()

	var out []Delivery

	if chosen.Style != nil {
		out = append(out, *chosen.Style)
	}

	switch chosen.Sex {
	case "woman":
		return append(out, TheWomanRobot...)
	case "man":
		return append(out, TheMan...)
	default:
		return append(out, TheRobot...)
	}
}

// firstThatWorks is the first of these voices espeak will actually accept.
func firstThatWorks(styles []Delivery) Delivery {
	for _, style := range styles {
		if variantWorks(style.Variant) {
			return style
		}
	}

	return Delivery{}
}

// UseVariant fixes the variant, for a test that needs to know which one it is
// exercising rather than which one this machine happens to have.
func UseVariant(name string) {
	variantOnce.Do(func() {})

	chosenVoiceStyle = Delivery{Variant: name, Words: 175, Pitch: 30}

	for _, known := range [][]Delivery{TheRobot, TheWomanRobot, TheMan} {
		for _, style := range known {
			if style.Variant == name {
				chosenVoiceStyle = style
			}
		}
	}

	if name == "" {
		chosenVoiceStyle = Delivery{}
	}
}

/*
 * variantWorks asks espeak rather than looking for a file.
 *
 * The data lives in a different place on every distribution, and a check that
 * guesses the path would answer "no" on a machine where the voice is perfectly
 * fine. -q writes nothing and speaks nothing; the only thing being read is
 * whether espeak accepted the name.
 */
func variantWorks(name string) bool {
	engine := fallbackEngine()

	if engine == nil || !strings.HasPrefix(engine.Command, "espeak") {
		return false
	}

	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()

	return exec.CommandContext(ctx, engine.Command,
		"-v", "en+"+name, "-q", "--", "x").Run() == nil
}

/*
 * speakingAs is the voice arguments for a system engine, given what is about
 * to be said.
 *
 * The language comes from the script, because an English voice handed Cyrillic
 * reads the letters out one at a time. The variant rides on top of whichever
 * language is chosen, which is how espeak composes them: bg+anikaRobot is the
 * same woman speaking Bulgarian.
 */
func speakingAs(engine *Engine, text string) []string {
	if engine == nil {
		return nil
	}

	switch {
	case strings.HasPrefix(engine.Command, "espeak"):
		// Whichever voice is chosen, rather than the robot every time: three
		// buttons that make one sound is three buttons that are lying, and a
		// list of thirty variants that all make one sound is worse.
		style := chosenStyle()

		if style.Variant == "" {
			return nil
		}

		return []string{
			"-v", espeakLanguage(text) + "+" + style.Variant,
			"-s", strconv.Itoa(style.Words),
			"-p", strconv.Itoa(style.Pitch),
		}

	case engine.Command == "spd-say":
		/*
		 * speech-dispatcher drives espeak and offers the same variants, but
		 * under display names — "English (Great Britain)+anikaRobot".
		 *
		 * So the name is taken from the list it reports rather than written
		 * down here. Those names differ between versions and between
		 * languages, and a hard-coded one would fail silently on the next
		 * release: spd-say exits zero whether or not it could use the voice,
		 * which is the same trap that once had this program reporting speech
		 * while producing nothing at all.
		 */
		if named := spdVoice(espeakLanguage(text), preferredStyles()); named != "" {
			return []string{"-y", named}
		}

		// Nothing matched. A voice type is coarser — female rather than a
		// particular machine — and it is closer than the default. Of the kind
		// chosen, though: "female1" whatever had been asked for was how a
		// man's voice came out as a woman's here.
		return []string{"-t", spdType(CurrentVoice().Sex)}
	}

	return nil
}

/*
 * spdType is the coarse voice speech-dispatcher understands, for a kind.
 *
 * It has no robot, so a robot asks for the same synthetic voice a woman does —
 * which is what speech-dispatcher's espeak sounds like anyway, and is the
 * closest thing to a machine in a list of three sexes.
 */
func spdType(kind string) string {
	if kind == "man" {
		return "male1"
	}

	return "female1"
}

// espeakLanguage is which language espeak should read this in. Cyrillic means
// Bulgarian here; everything else is read as English, which is what the rest
// of this program assumes when it has nothing better to go on.
func espeakLanguage(text string) string {
	if scriptOf(text) == "cyrillic" {
		return "bg"
	}

	if spoken := strings.ToLower(strings.TrimSpace(Language())); spoken != "" {
		if len(spoken) >= 2 {
			return spoken[:2]
		}
	}

	return "en"
}

var (
	spdOnce   sync.Once
	spdVoices []spdOption
)

type spdOption struct{ name, language, variant string }

/*
 * spdVoice is what speech-dispatcher calls the voice we want, for a language.
 *
 * Asked once. The list is a subprocess and several hundred lines, and it
 * cannot change while the program runs any more than the installed data can.
 */
func spdVoice(language string, styles []Delivery) string {
	spdOnce.Do(readSpdVoices)

	for _, style := range styles {
		want := style.Variant

		for _, code := range asked(language) {
			for _, v := range spdVoices {
				if strings.EqualFold(v.variant, want) && strings.EqualFold(v.language, code) {
					return v.name
				}
			}
		}

		// Any dialect of it, rather than none at all.
		for _, v := range spdVoices {
			if strings.EqualFold(v.variant, want) &&
				strings.HasPrefix(strings.ToLower(v.language), language+"-") {
				return v.name
			}
		}
	}

	return ""
}

/*
 * asked is the language codes to look for, best first.
 *
 * There is no plain "en" in speech-dispatcher's list — only dialects — and
 * they are listed alphabetically, so taking the first match gave Caribbean
 * English to somebody who had not asked for it. British before American
 * because the rest of this program is written in British English, and both
 * before whatever happens to sort first.
 */
func asked(language string) []string {
	switch language {
	case "en":
		return []string{"en", "en-GB", "en-US"}
	default:
		return []string{language}
	}
}

/*
 * readSpdVoices parses what spd-say -L prints.
 *
 * Three columns, and the first one contains spaces — "English (Great
 * Britain)+anikaRobot". So it is read from the right: the last field is the
 * variant, the one before it the language, and everything else the name.
 * Reading from the left would work until the first voice with a space in it,
 * which is most of them.
 */
func readSpdVoices() {
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()

	out, err := exec.CommandContext(ctx, "spd-say", "-L").Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)

		if len(fields) < 3 {
			continue
		}

		variant := fields[len(fields)-1]
		language := fields[len(fields)-2]
		name := strings.Join(fields[:len(fields)-2], " ")

		if variant == "VARIANT" || variant == "none" {
			continue
		}

		spdVoices = append(spdVoices, spdOption{
			name: name, language: language, variant: variant,
		})
	}
}
