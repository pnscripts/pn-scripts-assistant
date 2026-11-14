package brain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

// openingDemo is a tool that opens another program, counted, so nothing
// is started on the computer running the test.
type openingDemo struct{ ran *int }

func (openingDemo) Name() string                     { return "zz_open_demo" }
func (openingDemo) Description() string              { return "open a demo program" }
func (openingDemo) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (openingDemo) Risk() tools.Risk                 { return tools.Mutating }
func (openingDemo) Summarize(json.RawMessage) string { return "Open demo" }
func (o openingDemo) Execute(context.Context, json.RawMessage) (string, error) {
	*o.ran++
	return "opened", nil
}

func (openingDemo) ProgramUse(context.Context, json.RawMessage) programs.Use {
	return programs.Use{Verb: programs.VerbStart, Name: "demo"}
}

// demoProgram writes one program into the brain's inventory.
func demoProgram(t *testing.T, b *Brain, version string) programs.Record {
	t.Helper()

	known, _ := b.DB.Programs(programs.Deb)
	save, _ := programs.Reconcile(programs.Deb, known, []programs.Program{{Kind: programs.Deb, ID: "demo", Name: "Demo", Version: version}}, time.Now())

	if err := b.DB.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	rs, _ := b.DB.Programs(programs.Deb)
	for _, r := range rs {
		if r.ID == "demo" {
			return r
		}
	}

	t.Fatal("demo was not written")

	return programs.Record{}
}

// askedAbout records a waiting question about a program, as the loop does.
func askedAbout(t *testing.T, b *Brain, r programs.Record, always bool) int64 {
	t.Helper()

	id, err := b.DB.RecordInvocation(0, "zz_open_demo", `{}`, "Demo: Open demo", string(tools.Mutating))
	if err != nil {
		t.Fatal(err)
	}

	if err := b.DB.InvocationProgram(id, r.RowID, programs.VerbStart, r.Version, always); err != nil {
		t.Fatal(err)
	}

	return id
}

func demoBrain(t *testing.T) (*Brain, *int) {
	t.Helper()

	b := quietBrain(t)
	b.Cfg.Freedom = string(permits.WhatIveAllowed)

	ran := 0
	if err := b.Agent.Registry.Register(openingDemo{ran: &ran}); err != nil {
		t.Fatal(err)
	}

	return b, &ran
}

/*
 * "Always for Demo" on the approval card: the program's own card gets
 * Always for that use and the version that was asked about, the call is
 * approved and runs, and the next call runs without asking.
 */
func TestAlwaysForTheProgramOnTheApprovalCard(t *testing.T) {
	b, ran := demoBrain(t)
	demo := demoProgram(t, b, "1.0")
	id := askedAbout(t, b, demo, true)

	inv, err := b.AlwaysForProgram(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	if inv.Status != store.InvocationDone || *ran != 1 {
		t.Fatalf("the call was not carried out: %+v (%d)", inv, *ran)
	}

	ps, _ := b.ProgramPermits()
	got := ps[demo.RowID]

	if len(got) != 1 || got[0].Verb != programs.VerbStart || got[0].Answer != programs.Always || got[0].Version != "1.0" {
		t.Fatalf("the program's card was not given Always for 1.0: %+v", got)
	}

	if r := b.ProgramRuling(context.Background(), programs.Use{Verb: programs.VerbStart, Name: "demo"}); r.Answer != programs.Always {
		t.Errorf("the next call is not allowed: %+v", r)
	}

	// Only other uses of the program stay as they were.
	if r := b.ProgramRuling(context.Background(), programs.Use{Verb: programs.VerbChange, Name: "demo"}); r.Answer != programs.AskEach {
		t.Errorf("Always for starting it allowed running it as a command: %+v", r)
	}

	// And nothing tool-wide was granted.
	if a := b.Agent.MayI("", "zz_open_demo", true, risk.Low); a == permits.Allow {
		t.Error("the program's Always allowed the tool for every program")
	}

	// A decided question cannot be answered again.
	if _, err := b.AlwaysForProgram(context.Background(), id); err == nil {
		t.Error("a decided question took Always again")
	}
}

func TestAlwaysForTheProgramRefusesWhatItCannotStandFor(t *testing.T) {
	b, ran := demoBrain(t)
	demo := demoProgram(t, b, "1.0")

	// Asked for a reason of its own too: the program's Always would not
	// answer it, so it is not given.
	other := askedAbout(t, b, demo, false)
	if _, err := b.AlwaysForProgram(context.Background(), other); err == nil {
		t.Error("Always was given where it would change nothing")
	}

	// A question about no program.
	plain, _ := b.DB.RecordInvocation(0, "zz_open_demo", `{}`, "Open demo", string(tools.Mutating))
	if _, err := b.AlwaysForProgram(context.Background(), plain); err == nil {
		t.Error("Always for a program was given to a question about none")
	}

	// Updated since it was asked: the person decides about the new
	// version on its card; the question itself still waits.
	asked := askedAbout(t, b, demo, true)
	demoProgram(t, b, "2.0")

	_, err := b.AlwaysForProgram(context.Background(), asked)
	if err == nil || !strings.Contains(err.Error(), "updated") {
		t.Errorf("Always went to a version nobody was asked about: %v", err)
	}

	if inv, _ := b.DB.Invocation(asked); inv.Status != store.InvocationPending {
		t.Errorf("the question did not keep waiting: %s", inv.Status)
	}

	if ps, _ := b.ProgramPermits(); len(ps[demo.RowID]) != 0 || *ran != 0 {
		t.Errorf("something was granted or ran: %+v %d", ps, *ran)
	}
}

/*
 * "Stop using other programs": one switch, remembered, refuses a waiting
 * question about a program when it is approved, and every ruling, until
 * it is undone.
 */
func TestStoppingOtherProgramsRefusesApprovalsUntilUndone(t *testing.T) {
	b, ran := demoBrain(t)
	demo := demoProgram(t, b, "1.0")

	if _, stopped := b.ProgramsStopped(); stopped {
		t.Fatal("a new brain starts stopped")
	}

	id := askedAbout(t, b, demo, true)

	if err := b.StopUsingPrograms(); err != nil {
		t.Fatal(err)
	}

	since, stopped := b.ProgramsStopped()
	if !stopped || time.Since(since) > time.Minute {
		t.Fatalf("not stopped: %v %v", since, stopped)
	}

	inv, err := b.Decide(context.Background(), id, true)
	if err == nil || !strings.Contains(err.Error(), "stopped all use of other programs") || *ran != 0 {
		t.Fatalf("an approval ran while stopped: %v (%d)", err, *ran)
	}

	if inv.Status != store.InvocationFailed {
		t.Errorf("the refused question is %s", inv.Status)
	}

	second := askedAbout(t, b, demo, true)
	if _, err := b.AlwaysForProgram(context.Background(), second); err == nil || *ran != 0 {
		t.Errorf("Always for the program ran while stopped: %v", err)
	}

	if ps, _ := b.ProgramPermits(); len(ps[demo.RowID]) != 0 {
		t.Errorf("Always was written while stopped: %+v", ps)
	}

	// The pending cards say so.
	cards, err := b.ApprovalCards()
	if err != nil {
		t.Fatal(err)
	}

	if len(cards) != 1 || !cards[0].Stopped || cards[0].Program == nil || cards[0].Program.Name != "Demo" {
		t.Fatalf("cards: %+v", cards)
	}

	if err := b.UseProgramsAgain(); err != nil {
		t.Fatal(err)
	}

	if _, stopped := b.ProgramsStopped(); stopped {
		t.Fatal("still stopped")
	}

	if _, err := b.Decide(context.Background(), second, true); err != nil || *ran != 1 {
		t.Errorf("approved after using programs again: %v (%d)", err, *ran)
	}
}

// A refusal for no program at all is never a refusal for a tool that uses none.
func TestStoppedLeavesOtherApprovalsAlone(t *testing.T) {
	b, _ := demoBrain(t)

	id, _ := b.DB.RecordInvocation(0, "remember", `{"what":"x"}`, "Remember x", string(tools.Mutating))

	if err := b.StopUsingPrograms(); err != nil {
		t.Fatal(err)
	}

	if _, err := b.Decide(context.Background(), id, true); err != nil && strings.Contains(err.Error(), "other programs") {
		t.Errorf("a tool that uses no program was stopped: %v", err)
	}
}

func TestApprovalCardsNameTheProgramAndWhatAlwaysWouldDo(t *testing.T) {
	b, _ := demoBrain(t)
	demo := demoProgram(t, b, "1.0")

	askedAbout(t, b, demo, true)
	b.DB.RecordInvocation(0, "remember", `{}`, "Remember", string(tools.Mutating))

	cards, err := b.ApprovalCards()
	if err != nil || len(cards) != 2 {
		t.Fatalf("%v %+v", err, cards)
	}

	p := cards[0].Program
	if p == nil || p.Name != "Demo" || p.Kind != programs.Deb || p.ID != "demo" || p.Verb != programs.VerbStart || p.Version != "1.0" || !p.Always {
		t.Errorf("the first card's program: %+v", p)
	}

	if cards[1].Program != nil || cards[1].Stopped {
		t.Errorf("a card about no program: %+v", cards[1])
	}
}

// The choices list_programs reads: decisions, the general setting, the switch.
func TestProgramChoicesFollowTheSettings(t *testing.T) {
	b, _ := demoBrain(t)
	demo := demoProgram(t, b, "1.0")

	if _, err := b.SetProgramPermit(programs.Deb, "demo", programs.VerbChange, programs.Never); err != nil {
		t.Fatal(err)
	}

	c, err := b.ProgramChoices()
	if err != nil {
		t.Fatal(err)
	}

	if len(c.Permits[demo.RowID]) != 1 || !c.Honoured || c.Unasked || c.Stopped {
		t.Errorf("what I've allowed: %+v", c)
	}

	b.Cfg.Freedom = string(permits.Everything)
	if c, _ = b.ProgramChoices(); !c.Honoured || !c.Unasked {
		t.Errorf("everything: %+v", c)
	}

	b.Cfg.Freedom = string(permits.AskEveryTime)
	if c, _ = b.ProgramChoices(); c.Honoured || c.Unasked {
		t.Errorf("ask every time: %+v", c)
	}

	b.StopUsingPrograms()
	if c, _ = b.ProgramChoices(); !c.Stopped || c.Since.IsZero() {
		t.Errorf("stopped: %+v", c)
	}
}
