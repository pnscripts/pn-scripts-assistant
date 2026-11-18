package brain

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/store"
)

// permitAsked is one permission question put through the brain in the
// background, and what came of it.
type permitAsked struct {
	allowed bool
	ended   PermitEnded
	err     error
}

func askPermit(b *Brain, ctx context.Context, by, tool, input string, wait time.Duration) chan permitAsked {
	out := make(chan permitAsked, 1)

	go func() {
		allowed, ended, err := b.AskPermissionForOutside(ctx, by, tool, json.RawMessage(input), nil, wait)
		out <- permitAsked{allowed, ended, err}
	}()

	return out
}

// waitingPermits is the permission questions at the desk, once n wait.
func waitingPermits(t *testing.T, b *Brain, n int) []store.Invocation {
	t.Helper()

	var mine []store.Invocation

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		pending, err := b.PendingApprovals()
		if err != nil {
			t.Fatal(err)
		}

		mine = mine[:0]

		for _, inv := range pending {
			if inv.Tool == otherprograms.PermissionTool {
				mine = append(mine, inv)
			}
		}

		if len(mine) == n {
			return mine
		}
	}

	t.Fatalf("%d permission questions wait at the desk, not %d", len(mine), n)

	return nil
}

func ended(t *testing.T, asked chan permitAsked) permitAsked {
	t.Helper()

	select {
	case a := <-asked:
		return a
	case <-time.After(5 * time.Second):
		t.Fatal("the program was never given an answer")
	}

	return permitAsked{}
}

func permitsAllowed(t *testing.T) *Brain {
	t.Helper()

	b := testBrain(t)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	return b
}

/*
 * A permission question of another program is put to the assistant's
 * person as a waiting question that says who asked and what for; their yes
 * goes back to the program, and the question, who asked, who decided and
 * what the program was told are on the record.
 */
func TestAPermissionQuestionIsAnsweredByItsPersonAtTheDesk(t *testing.T) {
	b := permitsAllowed(t)

	asked := askPermit(b, context.Background(), "claude-code 2.1.303", "Bash", `{"command":"rm -rf node_modules"}`, time.Minute)

	card := waitingPermits(t, b, 1)[0]

	if card.AskedBy != "claude-code 2.1.303" || card.Summary != "Wants to run this command: rm -rf node_modules" ||
		card.SummaryBG != "Иска да изпълни тази команда: rm -rf node_modules" || card.AnswersItself {
		t.Fatalf("the card: %+v", card)
	}

	select {
	case a := <-asked:
		t.Fatalf("the program was answered before anybody decided: %+v", a)
	case <-time.After(50 * time.Millisecond):
	}

	inv, err := b.Decide(context.Background(), card.ID, true)
	if err != nil || inv.Status != store.InvocationDone || inv.Result != "Allowed on its card: the program was told it may." {
		t.Fatalf("the yes: %+v %v", inv, err)
	}

	if a := ended(t, asked); !a.allowed || a.ended != PermitAllowed || a.err != nil {
		t.Fatalf("the program was told: %+v", a)
	}

	deeds := outsideDeeds(t, b)
	if len(deeds) != 1 {
		t.Fatalf("the record: %+v", deeds)
	}

	d := deeds[0]
	if d.Tool != otherprograms.PermissionTool || d.AskedBy != "claude-code 2.1.303" || d.Outcome != "done" ||
		d.Summary != card.Summary || d.SummaryBG != card.SummaryBG || d.Request != "" ||
		d.Result != "Allowed on its card: the program was told it may." ||
		d.Asked != `Permission: {"tool_name":"Bash","input":{"command":"rm -rf node_modules"}}` {
		t.Errorf("the record of the yes: %+v", d)
	}

	if left := waitingPermits(t, b, 0); len(left) != 0 {
		t.Error("the question still waits")
	}

	// Decided once: a second answer changes nothing.
	if _, err := b.Decide(context.Background(), card.ID, false); err == nil {
		t.Error("a question already answered was answered again")
	}
}

// Their no goes back as a no, and is on the record as theirs.
func TestAPermissionQuestionRefusedAtTheDesk(t *testing.T) {
	b := permitsAllowed(t)

	asked := askPermit(b, context.Background(), "claude-code", "Write", `{"file_path":"/home/you/a.txt","content":"x"}`, time.Minute)

	card := waitingPermits(t, b, 1)[0]

	inv, err := b.Decide(context.Background(), card.ID, false)
	if err != nil || inv.Status != store.InvocationDenied {
		t.Fatalf("the no: %+v %v", inv, err)
	}

	if a := ended(t, asked); a.allowed || a.ended != PermitRefused || a.err != nil {
		t.Fatalf("the program was told: %+v", a)
	}

	deeds := outsideDeeds(t, b)
	if len(deeds) != 1 || deeds[0].Outcome != "refused" ||
		deeds[0].Result != "Not allowed on its card: the program was told it may not." {
		t.Errorf("the record of the no: %+v", deeds)
	}
}

// With the switch off nothing is asked at the desk and nothing waits.
func TestNoPermissionQuestionWhileTheSwitchIsOff(t *testing.T) {
	b := testBrain(t)

	// The other switches do not open this one.
	for _, s := range []otherprograms.Subject{otherprograms.Programs, otherprograms.Ask, otherprograms.Memory} {
		if err := b.AllowOutside(s, true); err != nil {
			t.Fatal(err)
		}
	}

	allowed, _, err := b.AskPermissionForOutside(context.Background(), "claude-code", "Bash", json.RawMessage(`{"command":"ls"}`), nil, time.Second)
	if allowed || !errors.Is(err, ErrOutsideNotAllowed) {
		t.Fatalf("with the switch off: %v %v", allowed, err)
	}

	waitingPermits(t, b, 0)
}

/*
 * Nothing but its person's own yes is a yes: not "everything allowed", not
 * the quiet hours, not a decision said out loud or typed to the assistant.
 * A question nobody answers is a no, on the record as unanswered.
 */
func TestNothingAnswersAPermissionQuestionButItsPerson(t *testing.T) {
	b := permitsAllowed(t)

	// Everything allowed at the desk: for the assistant's own tools only.
	b.Cfg.Freedom, b.Cfg.Confine = "everything", false

	if !b.Unbounded() {
		t.Fatal("the test did not allow everything at the desk")
	}

	asked := askPermit(b, context.Background(), "claude-code", "Bash", `{"command":"make deploy"}`, 400*time.Millisecond)

	card := waitingPermits(t, b, 1)[0]

	// "Approve everything waiting" does not list it, and cannot decide it
	// by number either.
	actions, err := b.PendingActions()
	if err != nil || len(actions) != 0 {
		t.Fatalf("what a spoken decision would decide: %+v %v", actions, err)
	}

	if _, err := b.DecideAction(context.Background(), card.ID, true); err == nil || !strings.Contains(err.Error(), "on its card") {
		t.Fatalf("decided by something other than the card: %v", err)
	}

	// The quiet hours answer only what was held for them alone.
	if moved, err := b.DB.AnswerItself(card.ID); err != nil || moved {
		t.Fatalf("answered by itself: %v %v", moved, err)
	}

	a := ended(t, asked)
	if a.allowed || a.ended != PermitUnanswered || a.err != nil {
		t.Fatalf("nobody answered, and the program was told: %+v", a)
	}

	waitingPermits(t, b, 0)

	deeds := outsideDeeds(t, b)
	if len(deeds) != 1 || deeds[0].Outcome != "refused" ||
		!strings.HasPrefix(deeds[0].Result, "Not allowed: nobody answered in time") {
		t.Errorf("the record of the unanswered question: %+v", deeds)
	}

	// The closed question keeps what became of it, as the record does.
	if closed, err := b.DB.Invocation(card.ID); err != nil || closed.Status != store.InvocationDenied ||
		!strings.HasPrefix(closed.Result, "Not allowed: nobody answered in time") {
		t.Errorf("the closed question: %+v %v", closed, err)
	}

	// Too late: the card is gone and a yes now reaches nobody.
	if _, err := b.Decide(context.Background(), card.ID, true); err == nil {
		t.Error("a question closed as unanswered was answered after it")
	}
}

// A program that stops waiting takes its question off the desk.
func TestAProgramThatStopsWaitingTakesItsQuestionAway(t *testing.T) {
	b := permitsAllowed(t)

	ctx, stop := context.WithCancel(context.Background())

	asked := askPermit(b, ctx, "claude-code", "Bash", `{"command":"ls"}`, time.Minute)

	waitingPermits(t, b, 1)
	stop()

	if a := ended(t, asked); a.allowed || a.ended != PermitGone {
		t.Fatalf("after it stopped waiting: %+v", a)
	}

	waitingPermits(t, b, 0)

	deeds := outsideDeeds(t, b)
	if len(deeds) != 1 || !strings.Contains(deeds[0].Result, "the program stopped waiting") || deeds[0].Outcome != "refused" {
		t.Errorf("the record: %+v", deeds)
	}
}

// Only a few wait at once: one more is refused at once and leaves no card.
func TestOnlyAFewPermissionQuestionsWaitAtOnce(t *testing.T) {
	b := permitsAllowed(t)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	var all []chan permitAsked
	for i := 0; i < otherprograms.MostPermissionsWaiting; i++ {
		all = append(all, askPermit(b, ctx, "claude-code", "Bash", `{"command":"ls"}`, time.Minute))
	}

	waitingPermits(t, b, otherprograms.MostPermissionsWaiting)

	if _, _, err := b.AskPermissionForOutside(ctx, "claude-code", "Bash", json.RawMessage(`{"command":"ls"}`), nil, time.Minute); !errors.Is(err, ErrPermitsFull) {
		t.Fatalf("one more than may wait: %v", err)
	}

	waitingPermits(t, b, otherprograms.MostPermissionsWaiting)

	// One answered makes room for one more.
	if _, err := b.Decide(context.Background(), waitingPermits(t, b, otherprograms.MostPermissionsWaiting)[0].ID, false); err != nil {
		t.Fatal(err)
	}

	all = append(all, askPermit(b, ctx, "claude-code", "Bash", `{"command":"ls"}`, time.Minute))
	waitingPermits(t, b, otherprograms.MostPermissionsWaiting)

	stop()

	for _, each := range all {
		ended(t, each)
	}
}

// Turning the switch off closes the questions that wait, as a no, at once.
func TestTurningPermissionQuestionsOffClosesTheOnesThatWait(t *testing.T) {
	b := permitsAllowed(t)

	one := askPermit(b, context.Background(), "claude-code", "Bash", `{"command":"ls"}`, time.Minute)
	two := askPermit(b, context.Background(), "cursor", "Read", `{"file_path":"/etc/hosts"}`, time.Minute)

	cards := waitingPermits(t, b, 2)

	// Another switch turned off takes nothing of this one back.
	if err := b.AllowOutside(otherprograms.Ask, false); err != nil {
		t.Fatal(err)
	}

	waitingPermits(t, b, 2)

	if err := b.AllowOutside(otherprograms.Permit, false); err != nil {
		t.Fatal(err)
	}

	for _, each := range []chan permitAsked{one, two} {
		if a := ended(t, each); a.allowed || a.ended != PermitTakenBack || a.err != nil {
			t.Fatalf("after the switch was turned off: %+v", a)
		}
	}

	waitingPermits(t, b, 0)

	var takenBack int

	for _, d := range outsideDeeds(t, b) {
		if d.Tool == otherprograms.PermissionTool && strings.Contains(d.Result, "taken back before anybody answered") && d.Outcome == "refused" {
			takenBack++
		}
	}

	if takenBack != 2 {
		t.Errorf("questions on the record as taken back: %d", takenBack)
	}

	// A yes pressed on a card still open in a window reaches nobody.
	if _, err := b.Decide(context.Background(), cards[0].ID, true); err == nil {
		t.Error("a question taken back was answered after it")
	}

	// And with the switch off, no new one is taken.
	if _, _, err := b.AskPermissionForOutside(context.Background(), "claude-code", "Bash", json.RawMessage(`{}`), nil, time.Second); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("after the switch was turned off: %v", err)
	}
}

// Closing the assistant answers the questions that wait with a no, and
// takes no more.
func TestClosingTheAssistantClosesThePermissionQuestions(t *testing.T) {
	b := permitsAllowed(t)

	asked := askPermit(b, context.Background(), "claude-code", "Bash", `{"command":"ls"}`, time.Minute)

	waitingPermits(t, b, 1)

	// The whole way out, as the program goes it.
	b.Stop()

	if a := ended(t, asked); a.allowed || a.ended != PermitTakenBack {
		t.Fatalf("after closing: %+v", a)
	}

	if _, _, err := b.AskPermissionForOutside(context.Background(), "claude-code", "Bash", json.RawMessage(`{}`), nil, time.Second); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("while closing: %v", err)
	}
}

/*
 * A question left waiting by a run that ended without closing it (a power
 * cut) is closed when the assistant opens again: nobody waits for it, and
 * a card that could only answer nobody must not sit at the desk. Other
 * waiting questions are left alone.
 */
func TestAPermissionQuestionLeftByAnEarlierRunIsClosed(t *testing.T) {
	b := permitsAllowed(t)

	left, err := b.DB.RecordInvocationIn(0, otherprograms.PermissionTool, `{"tool_name":"Bash"}`, "Wants to use Bash.", "Иска да използва Bash.", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	if err := b.DB.InvocationAskedBy(left, "claude-code"); err != nil {
		t.Fatal(err)
	}

	own, err := b.DB.RecordInvocation(0, "write_file", `{}`, "Write notes.txt", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	// Opening the same brain beside the running assistant (as "ingest"
	// and "promote" do) closes nothing: only the assistant's own start.
	beside := New(b.DB, b.Cfg, b.Root, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	_ = beside

	if still, err := b.PendingApprovals(); err != nil || len(still) != 2 {
		t.Fatalf("opening the brain beside the running one closed a question: %+v %v", still, err)
	}

	b.CloseLeftPermits()

	pending, err := b.PendingApprovals()
	if err != nil || len(pending) != 1 || pending[0].ID != own {
		t.Fatalf("what still waits: %+v %v", pending, err)
	}

	deeds := outsideDeeds(t, b)
	if len(deeds) != 1 || !strings.Contains(deeds[0].Result, "the program stopped waiting") || deeds[0].AskedBy != "claude-code" {
		t.Errorf("the record: %+v", deeds)
	}
}

// A yes pressed for a question whose program is gone says so, and is not
// on the record as a yes.
func TestAYesWithNobodyWaitingIsNotAYes(t *testing.T) {
	b := permitsAllowed(t)

	id, err := b.DB.RecordInvocationIn(0, otherprograms.PermissionTool, `{"tool_name":"Bash"}`, "Wants to use Bash.", "Иска да използва Bash.", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	if err := b.DB.InvocationAskedBy(id, "claude-code"); err != nil {
		t.Fatal(err)
	}

	inv, err := b.Decide(context.Background(), id, true)
	if err == nil || inv.Status != store.InvocationDenied || !strings.Contains(err.Error(), "no longer waiting") {
		t.Fatalf("a yes to nobody: %+v %v", inv, err)
	}

	deeds := outsideDeeds(t, b)
	if len(deeds) != 1 || deeds[0].Outcome != "refused" {
		t.Errorf("the record: %+v", deeds)
	}
}

// What a program sends cannot make the record unreadable, whatever it
// begins with, and a question about nothing is no question.
func TestAPermissionQuestionThatLooksSealedDoesNotBreakTheRecord(t *testing.T) {
	b := permitsAllowed(t)

	asked := askPermit(b, context.Background(), "pnsealed1:AAAA", "pnsealed1:AAAA", `{"command":"pnsealed1:AAAA"}`, time.Minute)

	card := waitingPermits(t, b, 1)[0]

	if _, err := b.Decide(context.Background(), card.ID, false); err != nil {
		t.Fatal(err)
	}

	ended(t, asked)

	if deeds := outsideDeeds(t, b); len(deeds) != 1 {
		t.Errorf("the record after a question that looks sealed: %+v", deeds)
	}

	if _, _, err := b.AskPermissionForOutside(context.Background(), "claude-code", " \n\u200b", nil, nil, time.Second); !errors.Is(err, ErrNoPermission) {
		t.Errorf("a question about nothing: %v", err)
	}
}

/*
 * Found by the independent review. A yes pressed on the card in the very
 * moment the wait ends (or the program stops waiting) has moved the
 * question already, and decidePermit has taken it off the list of the
 * waiting; the waiter then found no channel there, skipped its wait and
 * told the program "no", while the record said "allowed". The waiter now
 * waits on its own channel for the answer that is on its way.
 *
 * The steps of that moment are taken here one by one, in that order.
 */
func TestAYesPressedAsTheWaitEndsIsWhatTheProgramIsTold(t *testing.T) {
	b := permitsAllowed(t)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	asked := askPermit(b, ctx, "claude-code", "Bash", `{"command":"ls"}`, time.Minute)

	card := waitingPermits(t, b, 1)[0]

	// The card's yes moves the question, and decidePermit begins: the
	// question is off the list, its channel in hand.
	if moved, err := b.DB.DecideInvocation(card.ID, store.InvocationApproved); err != nil || !moved {
		t.Fatalf("the yes did not move the question: %v %v", moved, err)
	}

	b.permits.mu.Lock()
	answer := b.permits.waiting[card.ID]
	delete(b.permits.waiting, card.ID)
	b.permits.mu.Unlock()

	// In that moment the program stops waiting.
	stop()

	select {
	case a := <-asked:
		t.Fatalf("the program was told %+v before the yes was handed over", a)
	case <-time.After(150 * time.Millisecond):
	}

	// decidePermit finishes: the yes is handed over.
	answer <- permitAnswer{allowed: true, how: PermitAllowed}

	if a := ended(t, asked); !a.allowed || a.ended != PermitAllowed || a.err != nil {
		t.Fatalf("the yes was on its way, and the program was told: %+v", a)
	}
}

// And when a decision that moved the question is never handed over, the
// waiter does not hang: it is a no after a short while.
func TestADecisionNeverHandedOverIsANo(t *testing.T) {
	before := permitHandOver
	permitHandOver = 200 * time.Millisecond

	t.Cleanup(func() { permitHandOver = before })

	b := permitsAllowed(t)

	asked := askPermit(b, context.Background(), "claude-code", "Bash", `{"command":"ls"}`, 100*time.Millisecond)

	card := waitingPermits(t, b, 1)[0]

	if moved, err := b.DB.DecideInvocation(card.ID, store.InvocationApproved); err != nil || !moved {
		t.Fatal(err)
	}

	if a := ended(t, asked); a.allowed || a.err != nil {
		t.Fatalf("with no answer handed over: %+v", a)
	}
}

/*
 * The record says where a permission question was answered, never who
 * answered it: the assistant's own window and a program on this computer
 * posting to the same address look the same to it (the known limit of
 * this address), and a record that said "you" would claim what it cannot
 * know.
 */
func TestTheRecordOfAPermissionQuestionDoesNotSayWhoAnswered(t *testing.T) {
	b := permitsAllowed(t)

	for _, approve := range []bool{true, false} {
		asked := askPermit(b, context.Background(), "claude-code", "Bash", `{"command":"ls"}`, time.Minute)

		if _, err := b.Decide(context.Background(), waitingPermits(t, b, 1)[0].ID, approve); err != nil {
			t.Fatal(err)
		}

		ended(t, asked)
	}

	for _, d := range outsideDeeds(t, b) {
		said := strings.ToLower(d.Request + " " + d.Result)

		if strings.Contains(said, "you ") || strings.Contains(said, "your ") || !strings.Contains(said, "on its card") {
			t.Errorf("the record says who answered: %q / %q", d.Request, d.Result)
		}
	}
}
