package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
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

// open is a tool that says other programs may use it without a question.
type open struct{ counted }

func (open) OpenToOtherPrograms() bool { return true }

/*
 * A turn another program asked for runs nothing unasked that does not say
 * it is open to other programs. "Only reads" is not enough: reading a
 * file, the mail or the memory is reading what is the person's, and some
 * tools that count as safe at the desk write (a to-do, a reminder, a
 * hire). So the rule is by name, not by kind: closed unless opened.
 */
func TestATurnForAnotherProgramRunsOnlyWhatIsOpenToOtherPrograms(t *testing.T) {
	readRan, openRan := 0, 0
	loop, db := newLoop(t, counted{name: "read_file", ran: &readRan}, open{counted{name: "list_programs", ran: &openRan}})
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	loop.NothingAsks = func() bool { return true }
	loop.Unbounded = func() bool { return true }

	var deeds []Deed
	loop.Did = func(d Deed) { deeds = append(deeds, d) }

	conv, _ := db.NewConversation("t")

	model := &scripted{replies: []llm.Response{{
		Content: "Looking.",
		ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "list_programs", Arguments: json.RawMessage(`{}`)},
			{ID: "c2", Name: "read_file", Arguments: json.RawMessage(`{}`)},
		},
	}}}

	// Named in the brief, as the door will name it.
	res, err := loop.RunBrief(context.Background(), conv, model, nil, Brief{WithTools: true, AskedBy: "Cursor 3.0"})
	if err != nil {
		t.Fatal(err)
	}

	if openRan != 1 {
		t.Errorf("the tool open to other programs ran %d times", openRan)
	}

	if readRan != 0 || len(res.Pending) != 1 || res.Pending[0].Tool != "read_file" {
		t.Fatalf("a tool that is not open to other programs ran %d times, %d questions", readRan, len(res.Pending))
	}

	q, _ := db.Invocation(res.Pending[0].ID)
	if q.AskedBy != "Cursor 3.0" || !strings.HasPrefix(q.Summary, "Always asked (another program asked for it): ") {
		t.Errorf("the question: %+v", q)
	}

	if len(deeds) != 2 || deeds[0].AskedBy != "Cursor 3.0" || deeds[0].Outcome != DeedDone ||
		deeds[1].AskedBy != "Cursor 3.0" || deeds[1].Outcome != DeedWaiting {
		t.Errorf("the record: %+v", deeds)
	}

	// The same safe tool from the desk runs without a question.
	readRan = 0
	conv, _ = db.NewConversation("t2")

	if res, err = loop.RunBrief(context.Background(), conv, askingFor("read_file"), nil, Brief{WithTools: true}); err != nil ||
		readRan != 1 || len(res.Pending) != 0 {
		t.Errorf("from the desk: ran %d times, %d questions (%v)", readRan, len(res.Pending), err)
	}
}

// A tool reached through another tool still knows who asked: the name
// travels with the call.
type asksWho struct {
	open
	heard *string
}

func (a asksWho) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	*a.heard = tools.AskedByOf(ctx)

	return a.open.Execute(ctx, args)
}

func TestAToolIsToldWhoAsked(t *testing.T) {
	ran, heard := 0, ""
	loop, db := newLoop(t, asksWho{open{counted{name: "list_programs", ran: &ran}}, &heard})

	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("list_programs"), nil,
		Brief{WithTools: true, AskedBy: "Cursor 3.0"}); err != nil {
		t.Fatal(err)
	}

	if ran != 1 || heard != "Cursor 3.0" {
		t.Errorf("ran %d, heard %q", ran, heard)
	}
}

/*
 * Another program cannot say yes for the person. The tool that carries out
 * what is waiting is the person's own (said out loud at the desk it needs
 * no approval, since the sentence is the approval); a turn another program
 * asked for is refused it under every setting, "everything allowed" too,
 * and is not offered it. Otherwise a program could ask for something, be
 * put to the person, and approve its own question.
 */
func TestAnotherProgramCannotApproveWhatIsWaiting(t *testing.T) {
	for name, allow := range map[string]func(*Loop){
		"as it comes": func(*Loop) {},
		"everything allowed": func(l *Loop) {
			l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
			l.NothingAsks = func() bool { return true }
			l.Unbounded = func() bool { return true }
		},
	} {
		t.Run(name, func(t *testing.T) {
			ran := 0
			loop, db := newLoop(t, counted{name: "decide_waiting", ran: &ran})
			allow(loop)

			conv, _ := db.NewConversation("t")
			model := askingFor("decide_waiting")

			res, err := loop.RunBrief(tools.AskedBy(context.Background(), "claude-code 2.1.0"), conv, model, nil, Brief{WithTools: true})
			if err != nil {
				t.Fatal(err)
			}

			if ran != 0 || len(res.Pending) != 0 {
				t.Fatalf("it decided what was waiting (%d times, %d questions)", ran, len(res.Pending))
			}

			// Not offered, and told why when it names it anyway.
			for _, req := range model.seen {
				for _, spec := range req.Tools {
					if spec.Name == "decide_waiting" {
						t.Error("a turn another program asked for was offered decide_waiting")
					}
				}
			}

			msgs, _ := db.History(conv)

			var told string
			for _, m := range model.seen[len(model.seen)-1].Messages {
				if m.Role == llm.RoleTool {
					told += m.Content
				}
			}

			if !strings.Contains(told, "Only the person at this computer decides what is waiting") {
				t.Errorf("the refusal does not say whose decision it is: %q (%d messages kept)", told, len(msgs))
			}
		})
	}

	// From the desk the same tool runs, as it always did.
	ran := 0
	loop, db := newLoop(t, counted{name: "decide_waiting", ran: &ran})
	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("decide_waiting"), nil, Brief{WithTools: true}); err != nil || ran != 1 {
		t.Errorf("from the desk: ran %d times (%v)", ran, err)
	}

	if len(NotForAnotherProgram) != 1 || NotForAnotherProgram[0] != "decide_waiting" {
		t.Errorf("what another program may never use: %v", NotForAnotherProgram)
	}
}
