package agent

import (
	"context"
	"encoding/json"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/tools"
)

// readingBoth is a safe tool that says its card in both languages.
type readingBoth struct{ counted }

func (r readingBoth) Summarize(a json.RawMessage) string { return r.SummarizeIn(a, "en") }

func (readingBoth) SummarizeIn(_ json.RawMessage, lang string) string {
	return tools.Say(lang, "Read notes.txt", "Прочитане на notes.txt")
}

// newestStep is the newest row of Doing now of a kind.
func newestStep(t *testing.T, kind string) progress.Entry {
	t.Helper()

	all := progress.Recent()

	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Kind == kind {
			return all[i]
		}
	}

	t.Fatalf("Doing now has no %q step: %+v", kind, all)

	return progress.Entry{}
}

/*
 * A question waits in Doing now and in the record in both languages.
 *
 * The approval card was Bulgarian on a Bulgarian page while the line beside
 * it, "Чака вашето решение: Always asked (installing software): Install
 * Godot 4.7.2", and the record of what was done still carried the English
 * card. The step and the deed are handed the same Bulgarian card the
 * question was stored with, so all three say one thing.
 */
func TestAWaitingQuestionSaysItsCardInBulgarianEverywhere(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, installingBoth{installing{counted{name: "install_requirement", ran: &ran}}})
	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }

	var deeds []Deed
	loop.Did = func(d Deed) { deeds = append(deeds, d) }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("install_requirement"),
		[]llm.Message{{Role: llm.RoleUser, Content: "Инсталирай Godot, моля"}}, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 0 || len(res.Pending) != 1 {
		t.Fatalf("ran %d, pending %+v", ran, res.Pending)
	}

	const (
		card   = "Always asked (installing software): Install Godot 4.7.2"
		cardBG = "Винаги се пита (инсталиране на софтуер): Инсталиране на Godot 4.7.2"
	)

	waiting := newestStep(t, "waiting")
	if waiting.Note != "Waiting for you: "+card || waiting.SummaryBG != cardBG {
		t.Errorf("Doing now: %q / %q", waiting.Note, waiting.SummaryBG)
	}

	// The same card the question was stored with, word for word.
	if kept, err := db.PendingInvocations(); err != nil || len(kept) != 1 || kept[0].SummaryBG != waiting.SummaryBG {
		t.Errorf("stored as %+v (%v)", kept, err)
	}

	if len(deeds) != 1 || deeds[0].Outcome != DeedWaiting ||
		deeds[0].Step.Summary != card || deeds[0].Step.SummaryBG != cardBG {
		t.Errorf("the record: %+v", deeds)
	}

	// Refused, the row keeps its Bulgarian card under the new heading.
	if !progress.Decided(res.Pending[0].ID, progress.Refused) {
		t.Fatal("the waiting row was not found")
	}

	if decided := newestStep(t, progress.KindDecided); decided.Note != "Refused: "+card || decided.SummaryBG != cardBG {
		t.Errorf("decided: %q / %q", decided.Note, decided.SummaryBG)
	}
}

// A tool that simply runs says what it does in both languages too: in
// Doing now, in the steps under the answer and in the record.
func TestARunningToolSaysItsCardInBulgarianEverywhere(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, readingBoth{counted{name: "read_file", ran: &ran}})

	var deeds []Deed
	loop.Did = func(d Deed) { deeds = append(deeds, d) }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("read_file"),
		[]llm.Message{{Role: llm.RoleUser, Content: "Прочети бележките"}}, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 || len(res.Steps) != 1 {
		t.Fatalf("ran %d, steps %+v", ran, res.Steps)
	}

	if s := res.Steps[0]; s.Summary != "Read notes.txt" || s.SummaryBG != "Прочитане на notes.txt" {
		t.Errorf("the step under the answer: %+v", s)
	}

	if tool := newestStep(t, "tool"); tool.Tool != "read_file" || tool.Note != "Read notes.txt" ||
		tool.SummaryBG != "Прочитане на notes.txt" {
		t.Errorf("Doing now: %+v", tool)
	}

	if len(deeds) != 1 || deeds[0].Outcome != DeedDone || deeds[0].Step.SummaryBG != "Прочитане на notes.txt" {
		t.Errorf("the record: %+v", deeds)
	}

	// Written as summary_bg, the name the approval card already has.
	out, _ := json.Marshal(res.Steps[0])

	var read map[string]any
	if json.Unmarshal(out, &read); read["summary_bg"] != "Прочитане на notes.txt" {
		t.Errorf("the step is written as %s", out)
	}
}

// A tool with no Bulgarian card (one from an integration, say) carries
// none, so the page shows the line as it came.
func TestAToolWithoutABulgarianCardCarriesNone(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, counted{name: "decide", ran: &ran})

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("decide"),
		[]llm.Message{{Role: llm.RoleUser, Content: "Decide"}}, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 || len(res.Steps) != 1 || res.Steps[0].SummaryBG != "" {
		t.Fatalf("ran %d, steps %+v", ran, res.Steps)
	}

	if tool := newestStep(t, "tool"); tool.SummaryBG != "" {
		t.Errorf("Doing now: %+v", tool)
	}

	out, _ := json.Marshal(res.Steps[0])
	if containsKey(out, "summary_bg") {
		t.Errorf("an empty card is written: %s", out)
	}
}

func containsKey(doc []byte, key string) bool {
	var read map[string]any

	json.Unmarshal(doc, &read)

	_, there := read[key]

	return there
}
