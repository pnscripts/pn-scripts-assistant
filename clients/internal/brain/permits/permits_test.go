package permits

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * Privacy and permission are different questions.
 *
 * A brain set to keep everything on this machine is not thereby allowed to
 * delete things, and one allowed to use a hosted model is not thereby
 * forbidden from writing a note. Nothing in this package knows what privacy
 * is, and that is the point of it existing.
 */
func TestReadingNeverNeedsPermission(t *testing.T) {
	b, _ := Load(t.TempDir())

	for _, freedom := range []Freedom{AskEveryTime, WhatIveAllowed, Everything} {
		if got := b.Decide("", "list_directory", false, freedom); got != Allow {
			t.Errorf("looking at something needed permission under %q: %v", freedom, got)
		}
	}
}

// The default is what it always was: anything that changes something stops.
func TestChangingSomethingAsksByDefault(t *testing.T) {
	b, _ := Load(t.TempDir())

	if got := b.Decide("", "write_file", true, AskEveryTime); got != Ask {
		t.Errorf("a mutating tool ran without asking: %v", got)
	}
}

/*
 * "Yes, and stop asking me about this one."
 *
 * The whole reason for this package. Before it, somebody who uses a capability
 * daily either approved it by hand every time or did without it.
 */
func TestAStandingGrantIsHonouredOnceGrantsAreHonoured(t *testing.T) {
	root := t.TempDir()
	b, _ := Load(root)

	if err := b.Remember("write_file", Allow, "Write a note to ~/notes.md"); err != nil {
		t.Fatal(err)
	}

	/*
	 * Still asks while the brain is set to ask about everything.
	 *
	 * Otherwise "allow always" would quietly change the behaviour of a brain
	 * whose owner has not said they want their grants honoured — which is
	 * agreeing to one thing and getting another.
	 */
	if got := b.Decide("", "write_file", true, AskEveryTime); got != Ask {
		t.Errorf("a grant took effect under ask-every-time: %v", got)
	}

	if got := b.Decide("", "write_file", true, WhatIveAllowed); got != Allow {
		t.Errorf("a granted capability still asked: %v", got)
	}

	// And it survives a restart, because that is what "always" means.
	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if got := again.Decide("", "write_file", true, WhatIveAllowed); got != Allow {
		t.Error("the grant did not survive a restart")
	}

	if list := again.List(); len(list) != 1 || list[0].Why == "" {
		t.Errorf("the record does not say what was agreed to: %+v", list)
	}
}

// "Just while I am doing this" must not outlive the program, or it is not what
// was agreed to.
func TestAGrantForThisRunIsNotWrittenDown(t *testing.T) {
	root := t.TempDir()
	b, _ := Load(root)

	b.ForThisRun("run_command")

	if got := b.Decide("", "run_command", true, AskEveryTime); got != Allow {
		t.Errorf("a grant for this run was not honoured: %v", got)
	}

	if _, err := os.Stat(filepath.Join(root, FileName)); err == nil {
		t.Error("a grant for this run was written to disk")
	}

	again, _ := Load(root)

	if got := again.Decide("", "run_command", true, WhatIveAllowed); got != Ask {
		t.Error("a grant for this run survived a restart")
	}
}

/*
 * Never means never.
 *
 * A refusal is about the capability, not about the moment, so no freedom
 * setting overrides it — including the one called "everything". Somebody who
 * has said the brain must not send mail has said something that should hold
 * even on the day they get bored of being asked about everything else.
 */
func TestARefusalOutranksEveryFreedom(t *testing.T) {
	b, _ := Load(t.TempDir())

	b.Remember("send_email", Refuse, "never send mail as me")
	b.ForThisRun("send_email")

	for _, freedom := range []Freedom{AskEveryTime, WhatIveAllowed, Everything} {
		if got := b.Decide("", "send_email", true, freedom); got != Refuse {
			t.Errorf("a refusal was overridden by %q: %v", freedom, got)
		}
	}
}

// Told to do everything, it does everything — except what it was told never to.
func TestDoingEverythingStillAsksNobody(t *testing.T) {
	b, _ := Load(t.TempDir())

	if got := b.Decide("", "write_file", true, Everything); got != Allow {
		t.Errorf("still asking under 'everything': %v", got)
	}
}

// Taking a permission back puts the capability where it started.
func TestForgettingAGrantGoesBackToAsking(t *testing.T) {
	root := t.TempDir()
	b, _ := Load(root)

	b.Remember("write_file", Allow, "notes")
	b.ForThisRun("write_file")

	if err := b.Forget("write_file"); err != nil {
		t.Fatal(err)
	}

	if got := b.Decide("", "write_file", true, WhatIveAllowed); got != Ask {
		t.Errorf("a forgotten grant was still honoured: %v", got)
	}

	if len(b.List()) != 0 {
		t.Error("the grant is still listed")
	}
}

/*
 * A damaged file grants nothing, and says so.
 *
 * Refusing to start would refuse somebody their assistant; starting with
 * half-parsed permissions would be worse than starting with none.
 */
func TestADamagedFileGrantsNothing(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, FileName), []byte("{ this is not json"), 0o600)

	b, err := Load(root)
	if err == nil {
		t.Error("a damaged permissions file was accepted silently")
	}

	if got := b.Decide("", "write_file", true, WhatIveAllowed); got != Ask {
		t.Errorf("a damaged file granted something: %v", got)
	}
}

/*
 * What one agent has been allowed is not what all of them have been allowed.
 *
 * The property the organisation rests on. Without it a roster would be a way
 * of widening authority: hire an agent, have its work approved once, and every
 * other agent inherits the grant. Several agents have to mean several amounts
 * of authority, or they may as well be one.
 */
func TestOneAgentsPermissionIsNotEverybodys(t *testing.T) {
	b := bookIn(t)

	if err := b.RememberFor("developer", "run_command", Allow, "building the project"); err != nil {
		t.Fatal(err)
	}

	if got := b.Decide("developer", "run_command", true, WhatIveAllowed); got != Allow {
		t.Errorf("the developer was told %q", got)
	}

	for _, who := range []string{"", "writer", "bookkeeper"} {
		if got := b.Decide(who, "run_command", true, WhatIveAllowed); got != Ask {
			t.Errorf("%q was told %q, want to be asked", who, got)
		}
	}
}

/*
 * A refusal about one agent stops that one even where everybody else may.
 *
 * "Use the most restrictive applicable policy", and it is checked before
 * anything else so that no later rule can talk its way past it — including
 * Freedom being set to everything, which is otherwise a yes to all of it.
 */
func TestTheMostRestrictiveDecisionWins(t *testing.T) {
	b := bookIn(t)

	if err := b.Remember("send_email", Allow, "replying to the post"); err != nil {
		t.Fatal(err)
	}

	if err := b.RememberFor("researcher", "send_email", Refuse, "it only reads"); err != nil {
		t.Fatal(err)
	}

	if got := b.Decide("writer", "send_email", true, WhatIveAllowed); got != Allow {
		t.Errorf("the writer was told %q", got)
	}

	for _, freedom := range []Freedom{AskEveryTime, WhatIveAllowed, Everything} {
		if got := b.Decide("researcher", "send_email", true, freedom); got != Refuse {
			t.Errorf("under %q the researcher was told %q", freedom, got)
		}
	}
}

/*
 * A refusal about everybody stops every agent, however it was hired.
 *
 * The other direction, and the one that makes a global refusal worth writing:
 * it has to hold for agents that did not exist when it was written.
 */
func TestARefusalAboutEverybodyStopsEveryAgent(t *testing.T) {
	b := bookIn(t)

	if err := b.Remember("run_command", Refuse, "not on this machine"); err != nil {
		t.Fatal(err)
	}

	for _, who := range []string{"", "developer", "somebody_hired_later"} {
		if got := b.Decide(who, "run_command", true, Everything); got != Refuse {
			t.Errorf("%q was told %q", who, got)
		}
	}
}

/*
 * A book written before any of this still means what it meant.
 *
 * Every permissions file in existence has grants with no agent on them. If
 * those stopped applying to everybody, an upgrade would silently revoke every
 * permission somebody had given — and the first sign would be the assistant
 * asking about things it had been told not to ask about.
 */
func TestABookWrittenBeforeAgentsStillApplies(t *testing.T) {
	root := t.TempDir()

	written := `[{"tool":"write_file","answer":"allow","given":"2026-01-01T00:00:00Z","why":"notes"}]`

	if err := os.WriteFile(filepath.Join(root, FileName), []byte(written), 0o600); err != nil {
		t.Fatal(err)
	}

	b, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, who := range []string{"", "writer", "developer"} {
		if got := b.Decide(who, "write_file", true, WhatIveAllowed); got != Allow {
			t.Errorf("%q was told %q", who, got)
		}
	}
}

// And for this run only, one agent at a time, gone when the program stops.
func TestAPermissionForThisRunCanBeForOneAgent(t *testing.T) {
	b := bookIn(t)

	b.ForThisRunBy("developer", "run_command")

	if got := b.Decide("developer", "run_command", true, AskEveryTime); got != Allow {
		t.Errorf("the developer was told %q", got)
	}

	if got := b.Decide("writer", "run_command", true, AskEveryTime); got != Ask {
		t.Errorf("the writer was told %q", got)
	}
}

func bookIn(t *testing.T) *Book {
	t.Helper()

	b, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return b
}
