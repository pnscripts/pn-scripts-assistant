package store

import (
	"testing"
	"time"
)

// A proposal is decided once. A second answer — a double click, a replayed
// request — changes nothing, so nobody is hired twice.
func TestAProposalIsDecidedOnce(t *testing.T) {
	db := open(t)

	id, err := db.Propose(Proposal{Kind: ProposeHire, ConversationID: 7, Request: "hire a writer", Body: `{"a":1}`})
	if err != nil {
		t.Fatal(err)
	}

	open, err := db.OpenProposal(7, ProposeHire, time.Hour)
	if err != nil || open == nil || open.ID != id {
		t.Fatalf("the open proposal was not found: %+v %v", open, err)
	}

	if err := db.UpdateProposal(id, `{"a":2}`); err != nil {
		t.Fatal(err)
	}

	moved, err := db.DecideProposal(id, ProposalAccepted, "hired alex")
	if err != nil || !moved {
		t.Fatalf("the first answer did not land: %v %v", moved, err)
	}

	moved, _ = db.DecideProposal(id, ProposalDeclined, "no")
	if moved {
		t.Fatal("a decided proposal was decided again")
	}

	if err := db.UpdateProposal(id, `{"a":3}`); err == nil {
		t.Error("a decided proposal was rewritten")
	}

	got, _ := db.ProposalByID(id)

	if got.State != ProposalAccepted || got.Body != `{"a":2}` || got.Outcome != "hired alex" {
		t.Errorf("read back as %+v", got)
	}

	if open, _ := db.OpenProposal(7, ProposeHire, time.Hour); open != nil {
		t.Error("a decided proposal is still open")
	}
}

func TestEvidenceAndAProjectAreKeptWithTheTask(t *testing.T) {
	db := open(t)

	id, err := db.NewTask(Task{Name: "tetris", Goal: "make tetris", Project: "/home/x/tetris",
		Packages: "software.game.godot"})
	if err != nil {
		t.Fatal(err)
	}

	task, err := db.Task(id)
	if err != nil || task.Project != "/home/x/tetris" || task.Packages != "software.game.godot" {
		t.Fatalf("the project was not kept: %+v %v", task, err)
	}

	for _, e := range []Evidence{
		{TaskID: id, Kind: EvidenceCommand, Subject: "godot --headless --quit", Detail: "exit 0", OK: true},
		{TaskID: id, Kind: EvidenceCheck, Subject: "headless check", Detail: "2 errors", OK: false},
	} {
		if err := db.Record(e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.EvidenceFor(id)
	if err != nil || len(got) != 2 || got[0].Kind != EvidenceCommand || got[1].OK {
		t.Errorf("evidence read back as %+v %v", got, err)
	}

	if err := db.NoteIntegration(IntegrationEvent{Server: "files", Event: "call", Tool: "read", TaskID: id}); err != nil {
		t.Fatal(err)
	}

	events, err := db.IntegrationEvents("files", 10)
	if err != nil || len(events) != 1 || events[0].Tool != "read" {
		t.Errorf("integration events read back as %+v %v", events, err)
	}
}
