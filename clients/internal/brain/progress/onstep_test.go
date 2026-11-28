package progress

import (
	"context"
	"testing"
)

// detailsOf is the detail lines of the entries whose note is note.
func detailsOf(note string) []string {
	var out []string

	for _, e := range Recent() {
		if e.Note == note {
			out = append(out, e.Detail...)
		}
	}

	return out
}

/*
 * Work handed a step says its specifics on that step, whatever runs now.
 *
 * Seen 15.11 in the real window: a question to Claude Code, approved on its
 * card, said "Claude Code is answering" under "Learning from the last
 * conversation", the background step that had started since. The tool ran
 * after the turn had moved on, and Detail writes on whatever is last. A
 * context that carries the step's mark keeps the line where it belongs.
 */
func TestDetailForAStepLandsOnItWhateverRunsNow(t *testing.T) {
	fresh(t)

	Done()
	defer Done()

	SetToolIn("ask_claude", "Ask Claude", "Питане на Claude")

	ctx := OnStep(context.Background(), Mark())

	SetBackground("learning", "Learning from the last conversation")

	DetailFor(ctx, "Claude Code is answering")

	if got := detailsOf("Ask Claude"); len(got) != 1 || got[0] != "Claude Code is answering" {
		t.Errorf("the question's own step says %q", got)
	}

	if got := detailsOf("Learning from the last conversation"); len(got) != 0 {
		t.Errorf("the step running at the moment took the line: %q", got)
	}
}

// A context with no step keeps the old behaviour, and a quiet one (mark 0,
// as a loop that runs Quietly hands) writes nowhere.
func TestDetailForWithoutAStepOrQuietly(t *testing.T) {
	fresh(t)

	Done()
	defer Done()

	Set("thinking", "Thinking")

	DetailFor(context.Background(), "plain")
	DetailFor(OnStep(context.Background(), 0), "quiet")

	if got := detailsOf("Thinking"); len(got) != 1 || got[0] != "plain" {
		t.Errorf("the running step says %q, want only the line with no step named", got)
	}
}

/*
 * A question decided on its card is found again by its number, so what the
 * approved tool says lands on "Approved: ...", the question's own row.
 */
func TestTheStepOfAQuestionIsFoundByItsNumber(t *testing.T) {
	fresh(t)

	Done()
	defer Done()

	WaitingIn("Waiting for you: Ask Claude", "Питане на Claude", 41)
	Set("thinking", "Thinking")
	Decided(41, Approved)
	SetBackground("learning", "Learning from the last conversation")

	ctx := OnStep(context.Background(), StepOf(41))

	DetailFor(ctx, "Claude Code is answering")

	if got := detailsOf("Approved: Ask Claude"); len(got) != 1 {
		t.Errorf("the decided question says %q", got)
	}

	if StepOf(42) != 0 {
		t.Error("a question nobody asked has a step")
	}
}

/*
 * A line can carry its own Bulgarian, kept beside the English in the same
 * place, so the page can say it without matching the English.
 */
func TestADetailCarriesItsBulgarian(t *testing.T) {
	fresh(t)

	Done()
	defer Done()

	SetToolIn("write_file", "Write notes.txt", "Записване на notes.txt")

	mark := Mark()

	DetailOn(mark, "path: notes.txt")
	DetailOnIn(mark, "Wrote 22 bytes to notes.txt in 3ms", "резултат от 34 знака за 3 мс")

	for _, e := range Recent() {
		if e.Note != "Write notes.txt" {
			continue
		}

		if len(e.Detail) != 2 || len(e.DetailBG) != 2 {
			t.Fatalf("detail %q, in Bulgarian %q: want two lines side by side", e.Detail, e.DetailBG)
		}

		if e.DetailBG[0] != "" || e.DetailBG[1] != "резултат от 34 знака за 3 мс" {
			t.Errorf("in Bulgarian %q", e.DetailBG)
		}

		return
	}

	t.Fatal("the step is gone")
}
