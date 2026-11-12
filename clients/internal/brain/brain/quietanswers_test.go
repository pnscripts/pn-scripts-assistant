package brain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * A question held for the quiet hours alone answers itself when they open.
 *
 * Real store, real registry, the real coding agent tool and Decide; the
 * agent is the fake Claude Code, so nothing leaves the machine. Outside the
 * window nothing happens; inside it the held call runs once, its result is
 * said in the conversation it was asked in, and the record says nobody
 * pressed yes.
 */

// heldQuestion stores a held call to the coding agent the way the loop
// does, marked to answer itself, in a conversation of its own.
func heldQuestion(t *testing.T, b *Brain, access string) (store.Invocation, int64) {
	t.Helper()

	conv, err := b.DB.NewConversation("Tidy the notes")
	if err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]string{"agent": "claude-code", "task": "Write the notes.",
		"folder": t.TempDir(), "access": access})

	id, err := b.DB.RecordInvocation(conv, "ask_coding_agent", string(args),
		"Always asked (held for your quiet hours): Ask Claude Code", string(tools.Mutating))
	if err != nil {
		t.Fatal(err)
	}

	if err := b.DB.AnswersItselfInQuietHours(id); err != nil {
		t.Fatal(err)
	}

	inv, _ := b.DB.Invocation(id)

	return *inv, conv
}

func freedomHere(b *Brain, f permits.Freedom) {
	b.mu.Lock()
	b.Cfg.Freedom = string(f)
	b.mu.Unlock()
}

func everythingAllowedHere(b *Brain) { freedomHere(b, permits.Everything) }

func TestAHeldQuestionAnswersItselfWhenTheQuietHoursOpen(t *testing.T) {
	var jobs []orchestrator.Job

	b := agentBrain(t, fakeAgent{jobs: &jobs})
	everythingAllowedHere(b)

	inv, conv := heldQuestion(t, b, tools.ReachFolder)

	// No window: nothing answers itself, ever.
	if n := b.AnswerHeldQuestions(context.Background()); n != 0 || len(jobs) != 0 {
		t.Fatalf("answered with no quiet hours: %d, %d jobs", n, len(jobs))
	}

	// Shut: it waits.
	b.SetQuietHours(outside())

	if n := b.AnswerHeldQuestions(context.Background()); n != 0 || len(jobs) != 0 {
		t.Fatalf("answered outside the quiet hours: %d, %d jobs", n, len(jobs))
	}

	// Open: it runs, once.
	b.SetQuietHours(inside())

	if n := b.AnswerHeldQuestions(context.Background()); n != 1 || len(jobs) != 1 {
		t.Fatalf("not answered when the quiet hours opened: %d, %d jobs", n, len(jobs))
	}

	if n := b.AnswerHeldQuestions(context.Background()); n != 0 || len(jobs) != 1 {
		t.Fatalf("answered twice: %d, %d jobs", n, len(jobs))
	}

	done, _ := b.DB.Invocation(inv.ID)
	if done.Status != store.InvocationDone || !done.AnsweredItself || !strings.Contains(done.Result, "Wrote notes.md.") {
		t.Fatalf("%+v", done)
	}

	// Said where it was asked, so its user finds the answer there.
	msgs, err := b.DB.History(conv)
	if err != nil {
		t.Fatal(err)
	}

	said := ""
	for _, m := range msgs {
		said += m.Content + "\n"
	}

	if !strings.Contains(said, "Your quiet hours opened") || !strings.Contains(said, "Wrote notes.md.") {
		t.Fatalf("the conversation was not told:\n%s", said)
	}

	// And the record says nobody pressed yes.
	deeds, err := b.DB.Deeds(store.DeedFilter{Conversation: conv})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, d := range deeds {
		if d.Tool == "ask_coding_agent" {
			found = strings.Contains(d.Request, "by itself") && !strings.Contains(d.Request, "you approved")
		}
	}

	if !found {
		t.Fatalf("the record says its user approved it: %+v", deeds)
	}
}

/*
 * Whatever changed while it waited is read again at the opening: a
 * permission now set to ask, or never, leaves the question to its user and
 * stops promising to start by itself; a question its user already answered
 * is never answered again.
 */
func TestAHeldQuestionIsJudgedAgainWhenTheQuietHoursOpen(t *testing.T) {
	var jobs []orchestrator.Job

	b := agentBrain(t, fakeAgent{jobs: &jobs})
	b.SetQuietHours(inside())

	// Asking every time now: its user changed it while the question waited.
	freedomHere(b, permits.AskEveryTime)

	asks, _ := heldQuestion(t, b, tools.ReachRead)

	if n := b.AnswerHeldQuestions(context.Background()); n != 0 || len(jobs) != 0 {
		t.Fatalf("answered although its user's permission asks now: %d, %d jobs", n, len(jobs))
	}

	left, _ := b.DB.Invocation(asks.ID)
	if left.Status != store.InvocationPending || left.AnswersItself {
		t.Fatalf("not left to its user: %+v", left)
	}

	// Never, now.
	everythingAllowedHere(b)

	if err := b.Permits.Remember("ask_coding_agent", permits.Refuse, "test"); err != nil {
		t.Fatal(err)
	}

	never, _ := heldQuestion(t, b, tools.ReachRead)

	if n := b.AnswerHeldQuestions(context.Background()); n != 0 || len(jobs) != 0 {
		t.Fatalf("answered against a never: %d, %d jobs", n, len(jobs))
	}

	if left, _ := b.DB.Invocation(never.ID); left.Status != store.InvocationPending || left.AnswersItself {
		t.Fatalf("not left to its user: %+v", left)
	}

	// Answered by its user first: refused, so never run.
	if err := b.Permits.Forget("ask_coding_agent"); err != nil {
		t.Fatal(err)
	}

	first, _ := heldQuestion(t, b, tools.ReachRead)

	if _, err := b.Decide(context.Background(), first.ID, false); err != nil {
		t.Fatal(err)
	}

	if n := b.AnswerHeldQuestions(context.Background()); n != 0 || len(jobs) != 0 {
		t.Fatalf("a refused question ran: %d, %d jobs", n, len(jobs))
	}

	if got, _ := b.DB.Invocation(first.ID); got.Status != store.InvocationDenied || got.AnsweredItself {
		t.Fatalf("%+v", got)
	}
}

// Said in its user's language, with no dashes, failure included.
func TestWhatAHeldQuestionDidIsSaidInItsUsersLanguage(t *testing.T) {
	ok := heldAnswerSaid("en", "Ask Claude Code (only reads /p and answers)", "Wrote notes.md.", nil)
	if ok != "Your quiet hours opened, so I started what was waiting (Ask Claude Code (only reads /p and answers)). "+
		"It said:\n\nWrote notes.md." {
		t.Errorf("%q", ok)
	}

	bg := heldAnswerSaid("bg", "Ask Claude Code", "Готово.", nil)
	if !strings.HasPrefix(bg, "Тихите ви часове настъпиха") || !strings.HasSuffix(bg, "Готово.") || strings.Contains(bg, "Ask") {
		t.Errorf("%q", bg)
	}

	failed := heldAnswerSaid("bg-BG", "Ask Claude Code", "", errors.New("Claude Code cannot be asked now: not signed in"))
	if !strings.Contains(failed, "но не стана: Claude Code cannot be asked now") {
		t.Errorf("%q", failed)
	}

	for _, s := range []string{ok, bg, failed, heldAnswerSaid("", "x", "", errors.New("no"))} {
		if strings.ContainsAny(s, "–—") {
			t.Errorf("a dash: %q", s)
		}
	}
}
