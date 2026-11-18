package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/tools"
)

// changing is a tool that changes something.
type changing struct{ counted }

func (changing) Risk() tools.Risk                 { return tools.Mutating }
func (changing) Summarize(json.RawMessage) string { return "Write notes.txt" }

/*
 * What another program asked for is put to the person at the desk, with
 * the program's name on the question, whatever is allowed there: the
 * person allowed their own requests, not everybody's.
 */
func TestWhatAnotherProgramAsksForIsAlwaysPutToThePerson(t *testing.T) {
	always := func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }

	for name, allow := range map[string]func(*Loop){
		"always allowed": func(l *Loop) { l.MayI = always },
		"nothing asks": func(l *Loop) {
			l.MayI = always
			l.NothingAsks = func() bool { return true }
		},
		"everything allowed": func(l *Loop) {
			l.MayI = always
			l.NothingAsks = func() bool { return true }
			l.Unbounded = func() bool { return true }
		},
	} {
		t.Run(name, func(t *testing.T) {
			ran := 0
			loop, db := newLoop(t, changing{counted{name: "write_file", ran: &ran}})
			allow(loop)

			var deeds []Deed
			loop.Did = func(d Deed) { deeds = append(deeds, d) }

			conv, _ := db.NewConversation("t")
			ctx := tools.AskedBy(context.Background(), "claude-code 2.1.0")

			res, err := loop.RunBrief(ctx, conv, askingFor("write_file"), nil, Brief{WithTools: true})
			if err != nil {
				t.Fatal(err)
			}

			if ran != 0 || len(res.Pending) != 1 {
				t.Fatalf("ran %d times, %d questions", ran, len(res.Pending))
			}

			q, _ := db.Invocation(res.Pending[0].ID)
			if q.AskedBy != "claude-code 2.1.0" {
				t.Errorf("the question does not say who asked: %q", q.AskedBy)
			}

			if !strings.HasPrefix(q.Summary, "Always asked (another program asked for it): ") ||
				!strings.HasPrefix(q.SummaryBG, "Винаги се пита (поискано от друга програма): ") {
				t.Errorf("the card: %q / %q", q.Summary, q.SummaryBG)
			}

			if len(deeds) != 1 || deeds[0].AskedBy != "claude-code 2.1.0" || deeds[0].Outcome != DeedWaiting {
				t.Errorf("the record: %+v", deeds)
			}
		})
	}
}

// The same request from the desk runs as it always did, and says nobody
// else asked.
func TestTheSameRequestFromTheDeskIsNotAskedTwice(t *testing.T) {
	ran := 0
	loop, db := newLoop(t, changing{counted{name: "write_file", ran: &ran}})
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }

	var deeds []Deed
	loop.Did = func(d Deed) { deeds = append(deeds, d) }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("write_file"), nil, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 || len(res.Pending) != 0 {
		t.Fatalf("ran %d times, %d questions", ran, len(res.Pending))
	}

	if len(deeds) != 1 || deeds[0].AskedBy != "" {
		t.Errorf("the record: %+v", deeds)
	}
}

// Something that only reads runs for another program too, and the record
// says who it was for.
func TestWhatOnlyReadsRunsForAnotherProgramAndSaysWho(t *testing.T) {
	ran := 0
	loop, db := newLoop(t, counted{name: "list_programs", ran: &ran})

	var deeds []Deed
	loop.Did = func(d Deed) { deeds = append(deeds, d) }

	conv, _ := db.NewConversation("t")

	// Named in the brief, as the door will name it.
	res, err := loop.RunBrief(context.Background(), conv, askingFor("list_programs"), nil,
		Brief{WithTools: true, AskedBy: "Cursor 3.0"})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 || len(res.Pending) != 0 {
		t.Fatalf("ran %d times, %d questions", ran, len(res.Pending))
	}

	if len(deeds) != 1 || deeds[0].AskedBy != "Cursor 3.0" || deeds[0].Outcome != DeedDone {
		t.Errorf("the record: %+v", deeds)
	}
}

// A tool reached through another tool still knows who asked: the name
// travels with the call.
type asksWho struct {
	counted
	heard *string
}

func (a asksWho) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	*a.heard = tools.AskedByOf(ctx)

	return a.counted.Execute(ctx, args)
}

func TestAToolIsToldWhoAsked(t *testing.T) {
	ran, heard := 0, ""
	loop, db := newLoop(t, asksWho{counted{name: "list_programs", ran: &ran}, &heard})

	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("list_programs"), nil,
		Brief{WithTools: true, AskedBy: "Cursor 3.0"}); err != nil {
		t.Fatal(err)
	}

	if ran != 1 || heard != "Cursor 3.0" {
		t.Errorf("ran %d, heard %q", ran, heard)
	}
}
