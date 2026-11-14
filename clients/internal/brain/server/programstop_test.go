package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

// startingDemo opens "another program" by counting, nothing more.
type startingDemo struct{ ran *int }

func (startingDemo) Name() string                     { return "zz_start_demo" }
func (startingDemo) Description() string              { return "start a demo" }
func (startingDemo) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (startingDemo) Risk() tools.Risk                 { return tools.Mutating }
func (startingDemo) Summarize(json.RawMessage) string { return "Start demo" }
func (s startingDemo) Execute(context.Context, json.RawMessage) (string, error) {
	*s.ran++
	return "started", nil
}

func (startingDemo) ProgramUse(context.Context, json.RawMessage) programs.Use {
	return programs.Use{Verb: programs.VerbStart, Name: "demo"}
}

func demoAsked(t *testing.T, db *store.DB, always bool) (programs.Record, int64) {
	t.Helper()

	known, _ := db.Programs(programs.Deb)
	save, _ := programs.Reconcile(programs.Deb, known, []programs.Program{{Kind: programs.Deb, ID: "demo", Name: "Demo", Version: "1.0"}}, time.Now())
	db.SavePrograms(save)

	rs, _ := db.Programs(programs.Deb)

	id, err := db.RecordInvocation(0, "zz_start_demo", `{}`, "Demo: Start demo", string(tools.Mutating))
	if err != nil {
		t.Fatal(err)
	}

	if err := db.InvocationProgram(id, rs[0].RowID, programs.VerbStart, "1.0", always); err != nil {
		t.Fatal(err)
	}

	return rs[0], id
}

type approvalCard struct {
	ID      int64  `json:"id"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	Stopped bool   `json:"programs_stopped"`
	Program *struct {
		Kind    string `json:"kind"`
		ID      string `json:"id"`
		Name    string `json:"name"`
		Verb    string `json:"verb"`
		Version string `json:"version"`
		Always  bool   `json:"always"`
	} `json:"program"`
}

// The approval card knows its program, and "Always for Demo" gives that
// program's card Always for that use and runs the call.
func TestTheApprovalCardsAlwaysForOneProgram(t *testing.T) {
	ts, db, b := newServer(t)

	ran := 0
	if err := b.Agent.Registry.Register(startingDemo{ran: &ran}); err != nil {
		t.Fatal(err)
	}

	demo, id := demoAsked(t, db, true)

	var cards []approvalCard
	getJSON(t, ts.URL+"/api/approvals", &cards)

	if len(cards) != 1 || cards[0].Program == nil || cards[0].Program.Name != "Demo" || cards[0].Program.Kind != "deb" ||
		cards[0].Program.ID != "demo" || cards[0].Program.Verb != "start" || !cards[0].Program.Always || cards[0].Stopped {
		t.Fatalf("cards %+v", cards)
	}

	code, out := postPermit(t, ts.URL+"/api/approvals/"+itoa(id)+"/always-program", nil)
	if code != http.StatusOK || out["error"] != nil || ran != 1 {
		t.Fatalf("always for the program: %d %v (%d)", code, out, ran)
	}

	ps, _ := db.ProgramPermits()
	if p := ps[demo.RowID]; len(p) != 1 || p[0].Answer != programs.Always || p[0].Verb != programs.VerbStart {
		t.Errorf("the program's card: %+v", p)
	}
}

func TestAlwaysForAProgramIsRefusedWhereItMeansNothing(t *testing.T) {
	ts, db, b := newServer(t)

	ran := 0
	b.Agent.Registry.Register(startingDemo{ran: &ran})

	_, id := demoAsked(t, db, false)

	if code, _ := postPermit(t, ts.URL+"/api/approvals/"+itoa(id)+"/always-program", nil); code != http.StatusConflict || ran != 0 {
		t.Errorf("always where it changes nothing: %d (%d)", code, ran)
	}

	if code, _ := postPermit(t, ts.URL+"/api/approvals/999/always-program", nil); code != http.StatusNotFound {
		t.Errorf("no such question: %d", code)
	}
}

// One click stops every use of other programs; another starts it again.
func TestStopUsingOtherProgramsFromThePage(t *testing.T) {
	ts, db, b := newServer(t)

	ran := 0
	b.Agent.Registry.Register(startingDemo{ran: &ran})

	_, id := demoAsked(t, db, true)

	var got struct {
		Stopped *string `json:"stopped_since"`
	}

	getJSON(t, ts.URL+"/api/programs", &got)
	if got.Stopped != nil {
		t.Fatalf("stopped before anything was pressed: %v", *got.Stopped)
	}

	if code, _ := postPermit(t, ts.URL+"/api/programs/stop", nil); code != http.StatusOK {
		t.Fatalf("stop: %d", code)
	}

	getJSON(t, ts.URL+"/api/programs", &got)
	if got.Stopped == nil {
		t.Fatal("the page is not told it is stopped")
	}

	var cards []approvalCard
	getJSON(t, ts.URL+"/api/approvals", &cards)
	if len(cards) != 1 || !cards[0].Stopped {
		t.Errorf("the waiting card does not say it is stopped: %+v", cards)
	}

	_, out := postPermit(t, ts.URL+"/api/approvals/"+itoa(id)+"/approve", nil)
	if out["error"] == nil || ran != 0 {
		t.Errorf("an approval ran while stopped: %v (%d)", out, ran)
	}

	if code, _ := postPermit(t, ts.URL+"/api/programs/resume", nil); code != http.StatusOK {
		t.Fatalf("resume: %d", code)
	}

	got.Stopped = nil
	getJSON(t, ts.URL+"/api/programs", &got)
	if got.Stopped != nil {
		t.Error("still stopped after using programs again")
	}
}
