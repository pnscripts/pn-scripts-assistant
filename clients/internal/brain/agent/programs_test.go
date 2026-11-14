package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/tools"
)

// usingAProgram is a tool that uses another program (like open_app or
// type_text) with a verb.
type usingAProgram struct {
	counted
	verb programs.Verb
}

func (usingAProgram) Risk() tools.Risk                 { return tools.Mutating }
func (u usingAProgram) Name() string                   { return u.counted.name }
func (usingAProgram) Description() string              { return "use krita" }
func (usingAProgram) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (usingAProgram) Summarize(json.RawMessage) string { return "Open krita" }
func (u usingAProgram) Execute(ctx context.Context, a json.RawMessage) (string, error) {
	return u.counted.Execute(ctx, a)
}

func (u usingAProgram) ProgramUse(context.Context, json.RawMessage) programs.Use {
	return programs.Use{Verb: u.verb, Name: "krita"}
}

func krita(present bool) *programs.Record {
	r := &programs.Record{Program: programs.Program{Kind: programs.App, ID: "org.kde.krita.desktop", Name: "Krita"}, Present: present}
	if !present {
		at := time.Date(2026, 11, 12, 14, 3, 0, 0, time.Local)
		r.RemovedAt = &at
	}

	return r
}

func programTurn(t *testing.T, verb programs.Verb, ruling programs.Ruling, setup func(*Loop)) (int, Result, []string) {
	t.Helper()

	ran := 0
	loop, db := newLoop(t, usingAProgram{counted{name: "open_app", ran: &ran}, verb})

	var used []string

	loop.Programs = func(_ context.Context, u programs.Use) programs.Ruling {
		used = append(used, string(u.Verb)+":"+u.Name)
		return ruling
	}

	if setup != nil {
		setup(loop)
	}

	conv, _ := db.NewConversation("t")
	model := askingFor("open_app")

	res, err := loop.RunBrief(context.Background(), conv, model, nil, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	// What the model was told about the call is in the conversation.
	msgs, _ := db.History(conv)
	var told []string
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			told = append(told, m.Content)
		}
	}

	// Weighed with what the tool said it uses, unless the tool itself
	// was refused first.
	if len(used) > 0 && used[0] != string(verb)+":krita" {
		t.Errorf("weighed %v", used)
	}

	return ran, res, told
}

func TestARemovedProgramIsRefusedWithItsDateWhateverIsAllowed(t *testing.T) {
	gone := krita(false)
	ruling := programs.Ruling{Answer: programs.Never, Why: programs.WhyGone, Program: gone, Said: programs.GoneSentence(*gone)}

	ran, res, told := programTurn(t, programs.VerbStart, ruling, func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
		l.NothingAsks = func() bool { return true }
		l.Unbounded = func() bool { return true }
	})

	if ran != 0 || len(res.Pending) != 0 {
		t.Fatalf("a removed program was used (%d) or asked about (%d)", ran, len(res.Pending))
	}

	if len(told) == 0 || !strings.Contains(strings.Join(told, "\n"), "Krita was removed from this computer on 12 Nov 2026 at 14:03") {
		t.Errorf("the model was not told since when: %q", told)
	}
}

func TestNeverOnAProgramsCardIsRefusedUnderEverything(t *testing.T) {
	ruling := programs.Ruling{Answer: programs.Never, Why: programs.WhyRefused, Program: krita(true), Said: "Refused: never start it."}

	ran, res, _ := programTurn(t, programs.VerbStart, ruling, func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
		l.NothingAsks = func() bool { return true }
	})

	if ran != 0 || len(res.Pending) != 0 {
		t.Errorf("never ran (%d) or asked (%d)", ran, len(res.Pending))
	}
}

func TestAToolsOwnRefusalComesBeforeAProgramsAlways(t *testing.T) {
	ruling := programs.Ruling{Answer: programs.Always, Why: programs.WhyAllowed, Program: krita(true)}

	ran, res, _ := programTurn(t, programs.VerbStart, ruling, func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Refuse }
	})

	if ran != 0 || len(res.Pending) != 0 {
		t.Errorf("a refused tool ran because a program allowed it (%d, %d)", ran, len(res.Pending))
	}
}

func TestAlwaysOnAProgramsCardRunsWithoutAsking(t *testing.T) {
	ruling := programs.Ruling{Answer: programs.Always, Why: programs.WhyAllowed, Program: krita(true)}

	ran, res, _ := programTurn(t, programs.VerbStart, ruling, func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Ask }
	})

	if ran != 1 || len(res.Pending) != 0 {
		t.Errorf("always did not run it unasked (%d, %d)", ran, len(res.Pending))
	}
}

func TestUndecidedFollowsTheGeneralSettingAndNamesTheProgram(t *testing.T) {
	ruling := programs.Ruling{Answer: programs.AskEach, Program: krita(true)}

	ran, res, _ := programTurn(t, programs.VerbStart, ruling, func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Ask }
	})

	if ran != 0 || len(res.Pending) != 1 || !strings.HasPrefix(res.Pending[0].Summary, "Krita: Open krita") {
		t.Fatalf("undecided did not ask, or the question does not name the program: %+v", res.Pending)
	}

	ran, res, _ = programTurn(t, programs.VerbStart, ruling, func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	})

	if ran != 1 || len(res.Pending) != 0 {
		t.Errorf("undecided did not follow an allowing setting (%d, %d)", ran, len(res.Pending))
	}
}

func TestAnAlwaysGivenToAnOlderVersionAsksAndSaysWhy(t *testing.T) {
	ruling := programs.Ruling{Answer: programs.AskEach, Why: programs.WhyUpdated, Program: krita(true),
		Said: "Krita was updated since it was allowed to start it, so this is asked again."}

	_, res, _ := programTurn(t, programs.VerbStart, ruling, func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Ask }
	})

	if len(res.Pending) != 1 || !strings.Contains(res.Pending[0].Summary, "was updated since") {
		t.Errorf("the question does not say why it asks again: %+v", res.Pending)
	}
}

func TestTypingIntoAProgramIsAskedUnderEverythingUnlessItsCardSaysAlways(t *testing.T) {
	everything := func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
		l.NothingAsks = func() bool { return true }
	}

	ran, res, _ := programTurn(t, programs.VerbInput, programs.Ruling{Answer: programs.AskEach, Program: krita(true)}, everything)
	if ran != 0 || len(res.Pending) != 1 || !strings.Contains(res.Pending[0].Summary, "typing and clicking in another program") {
		t.Fatalf("typing ran unasked under everything (%d): %+v", ran, res.Pending)
	}

	// An unknown window is asked too.
	ran, res, _ = programTurn(t, programs.VerbInput, programs.Ruling{Answer: programs.AskEach}, everything)
	if ran != 0 || len(res.Pending) != 1 {
		t.Fatalf("typing into an unknown program ran unasked (%d, %d)", ran, len(res.Pending))
	}

	ran, res, _ = programTurn(t, programs.VerbInput, programs.Ruling{Answer: programs.Always, Why: programs.WhyAllowed, Program: krita(true)}, everything)
	if ran != 1 || len(res.Pending) != 0 {
		t.Errorf("the program's always did not let typing run (%d, %d)", ran, len(res.Pending))
	}
}

/*
 * "Stop using other programs" (design every-program 3 point 7): one switch
 * refuses every tool that uses another program, before anything can allow
 * it: the general setting, everything allowed, the program's own Always.
 */
func TestStoppedRefusesEveryProgramToolWhateverIsAllowed(t *testing.T) {
	since := time.Date(2026, 11, 14, 9, 30, 0, 0, time.Local)

	for _, verb := range []programs.Verb{programs.VerbStart, programs.VerbInput, programs.VerbChange} {
		weighed := false
		ruling := programs.Ruling{Answer: programs.Always, Why: programs.WhyAllowed, Program: krita(true)}

		ran, res, told := programTurn(t, verb, ruling, func(l *Loop) {
			l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
			l.NothingAsks = func() bool { return true }
			l.Unbounded = func() bool { return true }
			l.ProgramsStopped = func() (time.Time, bool) { return since, true }

			inner := l.Programs
			l.Programs = func(ctx context.Context, u programs.Use) programs.Ruling {
				weighed = true
				return inner(ctx, u)
			}
		})

		if ran != 0 || len(res.Pending) != 0 {
			t.Fatalf("%s: a stopped program tool ran (%d) or asked (%d)", verb, ran, len(res.Pending))
		}

		if weighed {
			t.Errorf("%s: the card was weighed although everything is stopped", verb)
		}

		if !strings.Contains(strings.Join(told, "\n"), programs.StoppedSentence(since)) {
			t.Errorf("%s: the model was not told why: %q", verb, told)
		}
	}
}

// A tool that acts in other programs' windows without naming one (scroll)
// is stopped too.
type scrollingSomewhere struct{ counted }

func (scrollingSomewhere) Risk() tools.Risk                 { return tools.Safe }
func (s scrollingSomewhere) Name() string                   { return s.counted.name }
func (scrollingSomewhere) Description() string              { return "scroll" }
func (scrollingSomewhere) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (scrollingSomewhere) Summarize(json.RawMessage) string { return "Scroll" }
func (scrollingSomewhere) ActsInOtherPrograms()             {}
func (s scrollingSomewhere) Execute(ctx context.Context, a json.RawMessage) (string, error) {
	return s.counted.Execute(ctx, a)
}

func TestStoppedRefusesScrollingInOtherProgramsAndNothingElse(t *testing.T) {
	scrolled, read := 0, 0
	loop, db := newLoop(t, scrollingSomewhere{counted{name: "scroll", ran: &scrolled}}, counted{name: "read_notes", ran: &read})
	loop.ProgramsStopped = func() (time.Time, bool) { return time.Now(), true }
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }

	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("scroll"), nil, Brief{WithTools: true}); err != nil {
		t.Fatal(err)
	}

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("read_notes"), nil, Brief{WithTools: true}); err != nil {
		t.Fatal(err)
	}

	if scrolled != 0 {
		t.Errorf("scrolling in another program ran while stopped")
	}

	if read != 1 {
		t.Errorf("a tool that uses no other program was stopped too (%d)", read)
	}
}

func TestNotStoppedChangesNothing(t *testing.T) {
	ruling := programs.Ruling{Answer: programs.Always, Why: programs.WhyAllowed, Program: krita(true)}

	ran, _, _ := programTurn(t, programs.VerbStart, ruling, func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Ask }
		l.ProgramsStopped = func() (time.Time, bool) { return time.Time{}, false }
	})

	if ran != 1 {
		t.Errorf("the switch off stopped an allowed program (%d)", ran)
	}
}

/*
 * The question about a program carries which program and version it is
 * about, and whether that program's own Always would stop it being asked,
 * so the approval card can offer "Always for Krita" for exactly that.
 */
func TestAQuestionAboutAProgramCarriesTheProgram(t *testing.T) {
	k := krita(true)
	k.RowID, k.Version = 7, "5.2.1"

	cases := []struct {
		name   string
		verb   programs.Verb
		setup  func(*Loop)
		always bool
	}{
		{"start asked by the setting", programs.VerbStart, func(l *Loop) {
			l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Ask }
		}, true},
		{"typing, asked whatever the setting", programs.VerbInput, func(l *Loop) {
			l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
			l.NothingAsks = func() bool { return true }
		}, true},
		{"asked for its own reason too", programs.VerbChange, func(l *Loop) {
			l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Ask }
		}, false},
	}

	for _, c := range cases {
		ran := 0

		var tool tools.Tool = usingAProgram{counted{name: "open_app", ran: &ran}, c.verb}
		if !c.always {
			tool = consentingProgram{usingAProgram{counted{name: "open_app", ran: &ran}, c.verb}}
		}

		loop, db := newLoop(t, tool)
		loop.Programs = func(context.Context, programs.Use) programs.Ruling {
			return programs.Ruling{Answer: programs.AskEach, Program: k}
		}
		c.setup(loop)

		conv, _ := db.NewConversation("t")

		res, err := loop.RunBrief(context.Background(), conv, askingFor("open_app"), nil, Brief{WithTools: true})
		if err != nil {
			t.Fatal(err)
		}

		if len(res.Pending) != 1 {
			if c.always {
				t.Fatalf("%s: not asked: %+v", c.name, res.Pending)
			}

			continue
		}

		inv, err := db.Invocation(res.Pending[0].ID)
		if err != nil || inv == nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if inv.ProgramID != 7 || inv.ProgramVerb != string(c.verb) || inv.ProgramVersion != "5.2.1" {
			t.Errorf("%s: the question does not carry the program: %+v", c.name, inv)
		}

		if inv.ProgramAlways != c.always {
			t.Errorf("%s: whether the program's Always would help = %v, want %v", c.name, inv.ProgramAlways, c.always)
		}
	}
}

// consentingProgram is always asked for a reason of its own (like
// installing), which a program's Always cannot take away.
type consentingProgram struct{ usingAProgram }

func (consentingProgram) Consent(json.RawMessage) string { return "installing software" }

func TestAQuestionAboutNoKnownProgramCarriesNone(t *testing.T) {
	ran := 0
	loop, db := newLoop(t, usingAProgram{counted{name: "open_app", ran: &ran}, programs.VerbStart})
	loop.Programs = func(context.Context, programs.Use) programs.Ruling { return programs.Ruling{Answer: programs.AskEach} }
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Ask }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("open_app"), nil, Brief{WithTools: true})
	if err != nil || len(res.Pending) != 1 {
		t.Fatalf("not asked: %v %+v", err, res.Pending)
	}

	inv, _ := db.Invocation(res.Pending[0].ID)
	if inv.ProgramID != 0 || inv.ProgramAlways {
		t.Errorf("a question about no known program names one: %+v", inv)
	}
}
