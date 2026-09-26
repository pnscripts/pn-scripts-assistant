package speech

import (
	"strings"
	"sync"
)

/*
 * Every system voice, offered under its own name, so one can be picked by ear.
 *
 * The three buttons — a robot, a woman, a man — pick the best one of each kind
 * this machine has. That is the right question for somebody who wants to be
 * read to and the wrong one for somebody who wants a particular sound, and
 * settling the robot proved it: klatt at 70 was rejected, klatt4 at 45 was too
 * high, klatt4 at 15 was right. Three goes, each decided by listening. None of
 * it could have been read off a list of names.
 *
 * So the list is in the program now, and the choice is made the way it has to
 * be made — the panel speaks a line the moment a voice is chosen, so every
 * entry here can be heard before it is kept. Asked for in those words: "I want
 * to be able to choose them manually in the program but by default let it make
 * to be 2 for now". The default is untouched, and is klatt4 at 15.
 *
 * Only what espeak will actually accept is listed. A variant that is not
 * installed makes espeak fail outright, so offering one would be offering
 * silence.
 */

/*
 * Choice is one system voice somebody can pick by name.
 *
 * Kind is who it is meant to be, so the list reads as three groups rather than
 * thirty codes, and Note is what it sounds like when there is something worth
 * saying — read off the variant's own definition rather than guessed at.
 */
type Choice struct {
	Kind string
	Note string

	Delivery
}

/*
 * Choosable is every system voice offered under its own name, in the order the
 * list shows them.
 *
 * The pitch of each is the family's, not a number applied across the board.
 * espeak's own scale has 50 in the middle, and each of these families starts
 * from a different place: the klatt synthesiser declares no pitch of its own,
 * the f-variants sit at 140 to 160 Hz, the m-variants at 65 to 88, and Andrea
 * and anika up at 200 to 300. One adjustment for all of them would soften one
 * family and turn another into a mumble, which is the mistake Delivery exists
 * to prevent.
 *
 * So: the klatt family at 15, the number settled by ear; the f-variants at 45
 * and the m-variants at 40, the numbers settled for f4 and m1 in those same
 * families; Andrea and anika at 30, a long way down because they start high;
 * and the processed machines at espeak's middle, which leaves them sounding as
 * whoever wrote them intended.
 */
var Choosable = []Choice{
	/*
	 * The klatt family: the Klatt formant synthesiser, numbered.
	 *
	 * Different formant tables rather than different treatments, and not one
	 * of them is modelled on a person — which is why the robot is one of
	 * these and why they belong to no sex.
	 */
	{Kind: "robot", Delivery: Delivery{Variant: "klatt", Words: 175, Pitch: 15}},
	{Kind: "robot", Delivery: Delivery{Variant: "klatt2", Words: 175, Pitch: 15}},
	{Kind: "robot", Delivery: Delivery{Variant: "klatt3", Words: 175, Pitch: 15}},
	{Kind: "robot", Delivery: Delivery{Variant: "klatt4", Words: 175, Pitch: 15}},
	{Kind: "robot", Delivery: Delivery{Variant: "klatt5", Words: 175, Pitch: 15}},
	{Kind: "robot", Delivery: Delivery{Variant: "klatt6", Words: 175, Pitch: 15}},

	/*
	 * Machines of the other sort: a voice with its formants flattened and a
	 * ring put on it.
	 *
	 * The echo is the whole difference between them and it is read off the
	 * variant files rather than guessed: robosoft rings at an amplitude of
	 * 1,000, robosoft2 at 600, and robosoft5 and UniRobot at 10,000 — which
	 * is the setting that smears every word into the one after it, and is the
	 * reason anikaRobot was taken out of the robot's own list. Said in the
	 * name, because it is a real difference and somebody may well want it.
	 */
	{Kind: "robot", Note: "ringing", Delivery: Delivery{Variant: "robosoft", Words: 175, Pitch: 50}},
	{Kind: "robot", Note: "ringing, lower", Delivery: Delivery{Variant: "robosoft2", Words: 175, Pitch: 50}},
	{Kind: "robot", Note: "flat", Delivery: Delivery{Variant: "Tweaky", Words: 175, Pitch: 50}},
	{Kind: "robot", Note: "a long echo", Delivery: Delivery{Variant: "robosoft5", Words: 175, Pitch: 50}},
	{Kind: "robot", Note: "a long echo", Delivery: Delivery{Variant: "UniRobot", Words: 175, Pitch: 50}},
	{Kind: "robot", Note: "a woman, a long echo", Delivery: Delivery{Variant: "anikaRobot", Words: 175, Pitch: 30}},

	// The women. Andrea is flat and unstretched, which is why it is the one
	// the woman's button picks; the rest are ordinary voices with their own
	// roughness and breath.
	{Kind: "woman", Note: "flat", Delivery: Delivery{Variant: "Andrea", Words: 175, Pitch: 30}},
	{Kind: "woman", Delivery: Delivery{Variant: "anika", Words: 175, Pitch: 30}},
	{Kind: "woman", Delivery: Delivery{Variant: "f1", Words: 175, Pitch: 45}},
	{Kind: "woman", Delivery: Delivery{Variant: "f2", Words: 175, Pitch: 45}},
	{Kind: "woman", Delivery: Delivery{Variant: "f3", Words: 175, Pitch: 45}},
	{Kind: "woman", Delivery: Delivery{Variant: "f4", Words: 175, Pitch: 45}},
	{Kind: "woman", Delivery: Delivery{Variant: "f5", Words: 175, Pitch: 45}},

	// And the men.
	{Kind: "man", Delivery: Delivery{Variant: "m1", Words: 175, Pitch: 40}},
	{Kind: "man", Delivery: Delivery{Variant: "m2", Words: 175, Pitch: 40}},
	{Kind: "man", Delivery: Delivery{Variant: "m3", Words: 175, Pitch: 40}},
	{Kind: "man", Delivery: Delivery{Variant: "m4", Words: 175, Pitch: 40}},
	{Kind: "man", Delivery: Delivery{Variant: "m5", Words: 175, Pitch: 40}},
	{Kind: "man", Delivery: Delivery{Variant: "m6", Words: 175, Pitch: 40}},
	{Kind: "man", Delivery: Delivery{Variant: "m7", Words: 175, Pitch: 40}},
	{Kind: "man", Delivery: Delivery{Variant: "m8", Words: 175, Pitch: 40}},
}

// probesAtOnce is how many variants are asked about together.
//
// Each probe is a subprocess, and thirty of them one after another is half a
// second in front of the first list of voices anybody sees. Eight at a time is
// no load worth worrying about on any machine this runs on and turns that into
// about a tenth of it.
const probesAtOnce = 8

var (
	eachOnce   sync.Once
	eachChoice []Voice
)

/*
 * EachVoice is every system voice this machine will actually speak in, each
 * under its own name.
 *
 * Worked out once and kept, like the rest of this: it is a question about
 * which data files are installed beside espeak, and those cannot change while
 * the program is running.
 */
func EachVoice() []Voice {
	eachOnce.Do(findEachVoice)

	return eachChoice
}

func findEachVoice() {
	engine := fallbackEngine()

	// Only espeak has variants. speech-dispatcher names them differently and
	// cannot be asked whether it accepted one — see spdVoice.
	if engine == nil || !strings.HasPrefix(engine.Command, "espeak") {
		return
	}

	/*
	 * Whatever the three buttons already use is not listed twice.
	 *
	 * "A robot (espeak-ng klatt4)" and "A robot · klatt4" are the same sound
	 * under two names, and a list where the same voice appears twice invites
	 * somebody to hunt for a difference between them that is not there.
	 */
	taken := map[string]bool{}

	for _, kind := range []string{"robot", "woman", "man"} {
		if v := StyleFor(kind).Variant; v != "" {
			taken[v] = true
		}
	}

	works := make([]bool, len(Choosable))

	var (
		wg   sync.WaitGroup
		room = make(chan struct{}, probesAtOnce)
	)

	for i := range Choosable {
		if taken[Choosable[i].Variant] {
			continue
		}

		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			room <- struct{}{}
			defer func() { <-room }()

			works[i] = variantWorks(Choosable[i].Variant)
		}(i)
	}

	wg.Wait()

	for i, choice := range Choosable {
		if !works[i] {
			continue
		}

		// Copied, so the entry carries its own delivery rather than a pointer
		// into a package variable anything could later change.
		style := choice.Delivery

		eachChoice = append(eachChoice, Voice{
			ID:     RobotVoice + ":" + choice.Variant,
			Sex:    choice.Kind,
			Name:   choice.name(),
			Engine: engine.Name,
			Style:  &style,
		})
	}
}

/*
 * name is what the list calls this voice.
 *
 * Who it is meant to be first, because that is what somebody is choosing
 * between, then the variant's own name, because that is the only thing that
 * tells two klatts apart and because it is what to say when asking for this
 * one again.
 */
func (c Choice) name() string {
	name := aVoiceLike(c.Kind) + " · " + c.Variant

	if c.Note != "" {
		name += " (" + c.Note + ")"
	}

	return name
}

// aVoiceLike names a kind the way the three buttons name it.
func aVoiceLike(kind string) string {
	switch kind {
	case "woman":
		return "A woman"
	case "man":
		return "A man"
	default:
		return "A robot"
	}
}
