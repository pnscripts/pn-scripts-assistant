package brain

import (
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/profile"
)

/*
 * What its owner wrote about themselves does not leave the machine unasked.
 *
 * The most personal thing in the program, and unlike a message somebody types
 * it would be attached to every single turn — including all the ones with
 * nothing to do with them. Opening privacy says this conversation may be
 * answered elsewhere; it does not say that a standing description of somebody's
 * business goes with all of them. Two decisions, so two settings.
 */
func TestTheProfileStaysOnThisMachineUnlessSaidOtherwise(t *testing.T) {
	b := testBrain(t)

	const secret = "I run a small studio and I never chase late payments myself."

	if err := profile.Write(b.Root, secret); err != nil {
		t.Fatal(err)
	}

	// The model on this machine is told, in every mode. Nothing leaves.
	if got := b.personaFor(llm.Local, false); !strings.Contains(got, secret) {
		t.Error("the local model was not told who it works for")
	}

	// A paid service is not, by default.
	for _, hosted := range []string{"anthropic", "openai", "groq"} {
		if got := b.personaFor(hosted, false); strings.Contains(got, secret) {
			t.Errorf("%s was sent the profile without being allowed to have it", hosted)
		}
	}

	// And is, once its owner says so — from inside the program.
	b.Cfg.ProfileToHosted = true

	if got := b.personaFor("anthropic", false); !strings.Contains(got, secret) {
		t.Error("after allowing it, the paid service still was not told")
	}
}

/*
 * The persona is rebuilt every turn rather than read back from the row.
 *
 * It used to be written once, when the conversation was created, and never
 * looked at again — so a thread open since the morning reported how many
 * things were remembered that morning, and anything written since about its
 * owner would never have been read at all.
 */
func TestWhatIsWrittenTodayIsReadToday(t *testing.T) {
	b := testBrain(t)

	before := b.personaFor(llm.Local, false)

	if err := profile.Write(b.Root, "I moved to Plovdiv in March."); err != nil {
		t.Fatal(err)
	}

	after := b.personaFor(llm.Local, false)

	if before == after {
		t.Fatal("the persona did not change after the profile was written")
	}

	if !strings.Contains(after, "Plovdiv") {
		t.Errorf("what was written was not picked up: %q", after)
	}
}

// A spoken turn gets the short persona and the opening of the profile. Every
// token of prompt is a second of silence before anybody hears a word.
func TestASpokenTurnIsGivenLess(t *testing.T) {
	b := testBrain(t)

	if err := profile.Write(b.Root, "I run a studio.\n\n"+strings.Repeat("Detail. ", 200)); err != nil {
		t.Fatal(err)
	}

	typed := b.personaFor(llm.Local, false)
	spoken := b.personaFor(llm.Local, true)

	if len(spoken) >= len(typed) {
		t.Errorf("a spoken turn was given %d characters against %d typed", len(spoken), len(typed))
	}

	if !strings.Contains(spoken, "I run a studio.") {
		t.Error("the spoken turn was not told the most important part")
	}
}
