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
		Brief{WithTools: true, Only: []string{"read_file"}})
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
