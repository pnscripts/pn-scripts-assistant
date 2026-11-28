package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/tools"
)

// writingWhileLearning is a tool whose run outlasts the turn's step: while
// it works, background learning starts, and it says something of its own
// as it goes, as ask_claude does.
type writingWhileLearning struct{ counted }

func (w writingWhileLearning) Summarize(a json.RawMessage) string { return w.SummarizeIn(a, "en") }

func (writingWhileLearning) SummarizeIn(_ json.RawMessage, lang string) string {
	return tools.Say(lang, "Write notes.txt", "Записване на notes.txt")
}

func (w writingWhileLearning) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	*w.ran++

	progress.SetBackground("learning", "Learning from the last conversation")
	progress.DetailFor(ctx, "Claude Code is answering")

	return "Wrote 22 bytes to notes.txt", nil
}

/*
 * What a tool says, and what came back, stay on the tool's own step.
 *
 * Seen 15.11: "Claude Code is answering" under "Learning from the last
 * conversation", and on a Bulgarian page the result line "Wrote 22 bytes
 * to ... in 0ms" in English. The loop hands the tool its step, writes the
 * result there, and gives the line in Bulgarian beside it: a tool's result
 * is written for the model, so the page says how much came back and how
 * long it took.
 */
func TestAToolsLinesStayOnItsStepAndTheResultIsSaidInBulgarian(t *testing.T) {
	progress.Done()
	defer progress.Done()

	ran := 0

	loop, db := newLoop(t, writingWhileLearning{counted{name: "write_file", ran: &ran}})

	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("write_file"),
		[]llm.Message{{Role: llm.RoleUser, Content: "Запиши бележките"}}, Brief{WithTools: true}); err != nil {
		t.Fatal(err)
	}

	if ran != 1 {
		t.Fatalf("ran %d", ran)
	}

	var tool, learning progress.Entry

	for _, e := range progress.Recent() {
		switch e.Note {
		case "Write notes.txt":
			tool = e
		case "Learning from the last conversation":
			learning = e
		}
	}

	if len(learning.Detail) != 0 {
		t.Errorf("the background step took the tool's lines: %q", learning.Detail)
	}

	said := strings.Join(tool.Detail, " | ")

	if !strings.Contains(said, "Claude Code is answering") || !strings.Contains(said, "Wrote 22 bytes to notes.txt in ") {
		t.Fatalf("the tool's own step says %q", tool.Detail)
	}

	if len(tool.DetailBG) != len(tool.Detail) {
		t.Fatalf("detail %q and its Bulgarian %q are not side by side", tool.Detail, tool.DetailBG)
	}

	for i, line := range tool.Detail {
		if !strings.HasPrefix(line, "Wrote 22 bytes") {
			continue
		}

		bg := tool.DetailBG[i]

		if !strings.HasPrefix(bg, "резултат от 27 знака за ") || !strings.HasSuffix(bg, " мс") {
			t.Errorf("the result line in Bulgarian is %q", bg)
		}
	}
}

// The result in Bulgarian: what came back is the tool's text for the model,
// so it is said by its size, with the time as Bulgarian writes it; a result
// with no Latin letters at all (a number) is shown as it is.
func TestTheResultLineInBulgarian(t *testing.T) {
	cases := []struct{ output, took, want string }{
		{"Wrote 22 bytes to notes.txt", "3ms", "резултат от 27 знака за 3 мс"},
		{"y", "1.5s", "резултат от 1 знак за 1,5 с"},
		{"42", "80ms", "42 за 80 мс"},
		{"Готово", "1m 05s", "Готово за 1 мин 05 с"},
		{"", "3ms", ""},
		{strings.Repeat("line\n", 200), "3ms", ""},
	}

	for _, c := range cases {
		if got := resultInBulgarian(c.output, c.took); got != c.want {
			t.Errorf("%q in %s: %q, want %q", c.output, c.took, got, c.want)
		}
	}
}

// One word is one word: "1 words in 80ms" read wrong in English, and the
// page said it as "1 думи" in Bulgarian.
func TestOneWordIsSaidAsOne(t *testing.T) {
	if got := rateOfReply("Done.", 80*time.Millisecond); got != "1 word in 80ms" {
		t.Errorf("one word: %q", got)
	}

	if got := rateOfReply("Done it.", 80*time.Millisecond); got != "2 words in 80ms" {
		t.Errorf("two words: %q", got)
	}
}
