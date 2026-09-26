package speech

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

/*
 * Every voice in the list can actually be chosen, and choosing one changes
 * what espeak is asked for.
 *
 * The failure this guards against is the one this whole file exists to fix: a
 * list of thirty names where every entry produces the same sound, because the
 * delivery was looked up from the kind rather than from the voice. It would
 * look right in the panel and be inaudible.
 */
func TestAChosenVariantIsTheOneSpoken(t *testing.T) {
	offered := EachVoice()

	if len(offered) == 0 {
		t.Skip("no espeak variants on this machine")
	}

	was := CurrentVoice().ID
	t.Cleanup(func() { SetVoice(was) })

	espeak := &Engine{Name: "espeak-ng", Command: "espeak-ng"}

	for _, v := range offered {
		if v.Style == nil {
			t.Errorf("%s carries no delivery", v.ID)

			continue
		}

		SetVoice(v.ID)

		got := speakingAs(espeak, "forty four packages passing")

		want := []string{"-v", "en+" + v.Style.Variant, "-s", "175", "-p", strconv.Itoa(v.Style.Pitch)}

		if !slices.Equal(got, want) {
			t.Errorf("%s ran with %v, wanted %v", v.ID, got, want)
		}
	}
}

/*
 * Each voice is listed once, under an id that says what it is.
 *
 * Two entries with the same id would make the panel's selection ambiguous —
 * the browser picks the first — and two with the same name would have somebody
 * hunting for a difference between them that is not there.
 */
func TestEveryVoiceIsListedOnce(t *testing.T) {
	seen := map[string]string{}
	names := map[string]bool{}

	for _, v := range Voices() {
		if was, twice := seen[v.ID]; twice {
			t.Errorf("%s is listed twice: %q and %q", v.ID, was, v.Name)
		}

		seen[v.ID] = v.Name

		if names[v.Name] {
			t.Errorf("two voices are both called %q", v.Name)
		}

		names[v.Name] = true
	}

	for _, v := range EachVoice() {
		if !strings.HasPrefix(v.ID, RobotVoice+":") {
			t.Errorf("%s is not named as a system voice", v.ID)
		}

		if v.Sex != "robot" && v.Sex != "woman" && v.Sex != "man" {
			t.Errorf("%s is a %q", v.ID, v.Sex)
		}
	}
}

/*
 * The default is still the robot, and the robot is still klatt4 at 15.
 *
 * Settled by ear over three goes — klatt at 70 rejected, klatt4 at 45 too
 * high, klatt4 at 15 kept — and the whole point of the list is that it is a
 * choice rather than a change. Somebody who opens the panel and picks nothing
 * hears what they picked last time this was decided.
 */
func TestTheDefaultIsStillKlatt4At15(t *testing.T) {
	if !variantWorks("klatt4") {
		t.Skip("no klatt4 on this machine")
	}

	if got := TheRobot[0]; got.Variant != "klatt4" || got.Pitch != 15 {
		t.Errorf("the robot is %+v", got)
	}

	was := CurrentVoice().ID
	t.Cleanup(func() { SetVoice(was) })

	SetVoice("")

	chosen := CurrentVoice()

	if chosen.Sex != "robot" {
		t.Fatalf("with nothing chosen it speaks as %q (%s)", chosen.Sex, chosen.ID)
	}

	if style := chosenStyle(); style.Variant != "klatt4" || style.Pitch != 15 {
		t.Errorf("it speaks with %+v", style)
	}
}

// What the list looks like, for the record: the three by kind, then every
// variant under its own name.
func TestTheListReads(t *testing.T) {
	for _, v := range Voices() {
		t.Logf("%-22s %-10s %s", v.ID, v.Sex, v.Name)
	}
}
