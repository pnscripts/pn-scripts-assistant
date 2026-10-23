package agent

import (
	"context"
	"testing"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
)

/*
 * Everything allowed (ADR 0007): the always-asked approvals run without a
 * question, a hire's tool list and the project's folders no longer refuse,
 * and the call is still recorded as a step.
 */
func TestEverythingAllowedAsksNothing(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, installing{counted{name: "install_requirement", ran: &ran}})

	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	loop.NothingAsks = func() bool { return true }
	loop.Unbounded = func() bool { return true }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("install_requirement"), nil,
		Brief{WithTools: true, Only: []string{"read_file", "install_requirement"}})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 || len(res.Pending) != 0 {
		t.Fatalf("it asked (%d) or did not run (%d)", len(res.Pending), ran)
	}

	if len(res.Steps) != 1 || res.Steps[0].Failed {
		t.Errorf("the call was not recorded as a step: %+v", res.Steps)
	}
}

func TestEverythingAllowedReachesOutsideTheProject(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, writing{counted{name: "write_it", ran: &ran}})
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	loop.Unbounded = func() bool { return true }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, writingTo("/home/p/notes.txt"), nil,
		Brief{WithTools: true, Within: project("/home/p/tetris"), Never: []string{"write_it"}})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 || len(res.Pending) != 0 {
		t.Errorf("a write outside the project was refused or asked about: ran %d, %+v", ran, res)
	}
}

// A tool its owner refused by name stays refused, everything allowed or not.
func TestEverythingAllowedKeepsARefusal(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, installing{counted{name: "install_requirement", ran: &ran}})
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Refuse }
	loop.Unbounded = func() bool { return true }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("install_requirement"), nil,
		Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 0 || len(res.Pending) != 0 {
		t.Errorf("a refused tool ran (%d) or was asked about (%d)", ran, len(res.Pending))
	}
}

// Everything allowed is nothing asked, not borrowing another expert's tools
// (owner 2026-10-23, "Само своите"): a tool outside the expert's own is
// refused at run time, without a question, and marked so the work can go to
// an expert who has it.
func TestEverythingAllowedStillRunsOnlyTheExpertsOwnTools(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, installing{counted{name: "install_requirement", ran: &ran}})

	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	loop.NothingAsks = func() bool { return true }
	loop.Unbounded = func() bool { return true }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("install_requirement"), nil,
		Brief{WithTools: true, Only: []string{"read_file"}})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 0 {
		t.Fatal("a tool outside the expert's own ran")
	}

	if len(res.Pending) != 0 {
		t.Fatalf("the owner was asked: %+v", res.Pending)
	}

	if got := res.OutsideTools(); len(got) != 1 || got[0] != "install_requirement" {
		t.Fatalf("the refusal was not marked for re-routing: %v", got)
	}
}
