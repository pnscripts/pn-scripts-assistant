package brain

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/protocol"
)

/*
 * Permission questions of another program (the "permissions" switch).
 *
 * Claude Code, started with --permission-prompt-tool, asks its tool before
 * it runs a command or writes a file it has no standing permission for.
 * When that tool is the assistant's, the question comes here, and what is
 * done with it is one thing only: it is put to the assistant's person as a
 * waiting question, with the name the program gave, and their answer is
 * handed back. Three things follow from that, and each is code, not a
 * promise:
 *
 *   - The assistant never answers one. There is no model on this path, no
 *     rule that says yes, no "always", and "everything allowed" does not
 *     reach it. A question nobody answered is a no.
 *   - Only the card's route answers: a decision said out loud or typed to
 *     the assistant ("approve everything waiting") leaves these where they
 *     are. (A program running under the same account can post to that
 *     route as the window does; that is the known limit of everything at
 *     this address, and the record does not claim more than it knows.)
 *   - The answer is on the record before the program is given it. A yes
 *     that cannot be written down is handed back as a no.
 */

// PermitsWaiting is the permission questions of other programs that wait
// for an answer now.
type permitsWaiting struct {
	mu      sync.Mutex
	waiting map[int64]chan permitAnswer
	closed  bool
}

// permitAnswer is what became of one question.
type permitAnswer struct {
	allowed bool
	how     PermitEnded
}

// PermitEnded is how a permission question ended.
type PermitEnded string

const (
	// PermitAllowed and PermitRefused are its person's own answers.
	PermitAllowed PermitEnded = "allowed"
	PermitRefused PermitEnded = "refused"

	// PermitUnanswered is a question nobody answered in time.
	PermitUnanswered PermitEnded = "unanswered"

	// PermitGone is a question whose program stopped waiting for it.
	PermitGone PermitEnded = "gone"

	// PermitTakenBack is a question that waited when the switch was
	// turned off, or when the assistant closed.
	PermitTakenBack PermitEnded = "taken back"

	// PermitUnrecorded is an answer that could not be written on the
	// record, and so was not given.
	PermitUnrecorded PermitEnded = "unrecorded"
)

var (
	// ErrPermitsFull is one question more than may wait at once.
	ErrPermitsFull = errors.New("too many permission questions are waiting already")

	// ErrNoPermission is a question with nothing readable to ask about.
	ErrNoPermission = errors.New("the permission question names nothing to use")
)

/*
 * How each ending is written on the record: what the program was told.
 * "On its card", not "by you": the assistant's own window and a program
 * on this computer that posts to the same address cannot be told apart
 * (see AllowOutside), so the record says where the answer was given and
 * nothing it cannot know.
 */
var permitSaid = map[PermitEnded]string{
	PermitAllowed:    "Allowed on its card: the program was told it may.",
	PermitRefused:    "Not allowed on its card: the program was told it may not.",
	PermitUnanswered: "Not allowed: nobody answered in time, and a question without an answer is a no.",
	PermitGone:       "Not answered: the program stopped waiting before anybody answered.",
	PermitTakenBack:  "Not allowed: the question was taken back before anybody answered.",
}

/*
 * AskPermissionForOutside puts one permission question of another program
 * to the assistant's person and waits for their answer, at most wait. It
 * reports whether they allowed it and how the question ended; anything but
 * a yes on its card is a no. cut is how long each text of input really is
 * where it was sent here shorter (otherprograms.PermissionLine).
 */
func (b *Brain) AskPermissionForOutside(ctx context.Context, by, tool string, input json.RawMessage, cut map[string]int, wait time.Duration) (bool, PermitEnded, error) {
	if tool = otherprograms.PermissionAsks(tool); tool == "" {
		return false, "", ErrNoPermission
	}

	if by == "" {
		return false, "", errors.New("a permission question with nobody named")
	}

	arguments, err := json.Marshal(struct {
		Tool  string          `json:"tool_name"`
		Input json.RawMessage `json:"input,omitempty"`

		// How long each text really is, where it arrived cut.
		Cut map[string]int `json:"cut,omitempty"`
	}{tool, input, cut})
	if err != nil {
		return false, "", err
	}

	en, bg := otherprograms.PermissionLine(tool, input, cut)

	/*
	 * The switch is read, and the question takes its place, under the lock
	 * that turning the switch off takes the waiting ones back under: a
	 * question cannot slip in behind a switch that has just been closed.
	 */
	b.permits.mu.Lock()

	if b.permits.closed || !b.OutsideMay(otherprograms.Permit) {
		b.permits.mu.Unlock()

		return false, "", ErrOutsideNotAllowed
	}

	if len(b.permits.waiting) >= otherprograms.MostPermissionsWaiting {
		b.permits.mu.Unlock()

		return false, "", ErrPermitsFull
	}

	// The question and who asked, written as one: it never waits as a
	// question of nobody's.
	id, err := b.DB.RecordInvocationAskedBy(0, otherprograms.PermissionTool, string(arguments), en, bg, string(tools.Mutating), by)
	if err != nil {
		b.permits.mu.Unlock()

		return false, "", err
	}

	answer := make(chan permitAnswer, 1)

	if b.permits.waiting == nil {
		b.permits.waiting = map[int64]chan permitAnswer{}
	}

	b.permits.waiting[id] = answer
	b.permits.mu.Unlock()

	b.Happens.Say(protocol.New(protocol.ApprovalRequired, en).
		With(map[string]any{"invocation": id, "tool": otherprograms.PermissionTool, "asked_by": by}))

	timer := time.NewTimer(wait)
	defer timer.Stop()

	how := PermitUnanswered

	select {
	case a := <-answer:
		return a.allowed, a.how, nil
	case <-timer.C:
	case <-ctx.Done():
		how = PermitGone
	}

	// Nobody answered. The question is closed as a no, unless its person
	// answered in this very moment: then theirs stands.
	if a, theirs := b.closePermit(id, how, answer); theirs {
		return a.allowed, a.how, nil
	}

	return false, how, nil
}

/*
 * closePermit ends a question nobody answered: refused on the record, with
 * why. When its person's answer got there first it changes nothing and
 * returns that answer instead.
 */
func (b *Brain) closePermit(id int64, how PermitEnded, answer chan permitAnswer) (permitAnswer, bool) {
	moved, err := b.DB.DecideInvocation(id, store.InvocationDenied)
	if err != nil {
		b.Log.Warn("could not close a permission question nobody answered", "id", id, "error", err)
	}

	/*
	 * Not moved: it was decided on its card a moment ago, and that answer
	 * is on its way through the waiter's own channel (decidePermit has
	 * taken the question off the list already, so the channel is not
	 * looked up there: it is the one this waiter made).
	 */
	if !moved && err == nil {
		select {
		case a := <-answer:
			return a, true
		case <-time.After(permitHandOver):
		}
	}

	b.forgetPermit(id)

	if moved {
		inv, err := b.DB.Invocation(id)
		if err == nil && inv != nil {
			b.recordPermit(*inv, how)
		}

		if err := b.DB.CompleteInvocation(id, store.InvocationDenied, permitSaid[how]); err != nil {
			b.Log.Warn("could not finish a permission question nobody answered", "id", id, "error", err)
		}

		progress.Decided(id, progress.Refused)

		b.Happens.Say(protocol.New(protocol.ApprovalReceived, "").
			With(map[string]any{"invocation": id, "tool": otherprograms.PermissionTool, "approved": false}))
	}

	return permitAnswer{}, false
}

// permitHandOver is how long a waiter that found its question decided
// waits for that decision to be handed to it. A variable for a test.
var permitHandOver = 5 * time.Second

func (b *Brain) forgetPermit(id int64) {
	b.permits.mu.Lock()
	delete(b.permits.waiting, id)
	b.permits.mu.Unlock()
}

/*
 * decidePermit is its person's answer to a permission question, from the
 * card (decide, after the row moved from waiting). Nothing is carried out
 * here: the answer is written on the record and handed to the program
 * that waits for it. A question whose program is no longer waiting (the
 * assistant was closed and opened again, say) can only be closed.
 */
func (b *Brain) decidePermit(inv store.Invocation, approve bool) (store.Invocation, error) {
	b.permits.mu.Lock()
	answer := b.permits.waiting[inv.ID]
	delete(b.permits.waiting, inv.ID)
	b.permits.mu.Unlock()

	how := PermitRefused
	if approve {
		how = PermitAllowed
	}

	if answer == nil {
		how = PermitGone
	}

	// No record, no yes.
	if err := b.recordPermit(inv, how); err != nil && how == PermitAllowed {
		how = PermitUnrecorded
	}

	inv.Status, inv.Result = store.InvocationDenied, permitResult(how)
	if how == PermitAllowed {
		inv.Status = store.InvocationDone
	}

	if err := b.DB.CompleteInvocation(inv.ID, inv.Status, inv.Result); err != nil {
		b.Log.Warn("could not finish a permission question", "id", inv.ID, "error", err)
	}

	if answer != nil {
		answer <- permitAnswer{allowed: how == PermitAllowed, how: how}
	}

	switch how {
	case PermitGone:
		return inv, errors.New("the program that asked is no longer waiting for an answer, so there was nobody to tell")
	case PermitUnrecorded:
		return inv, errors.New("the answer could not be written on the record, so the program was told no")
	}

	return inv, nil
}

func permitResult(how PermitEnded) string {
	if how == PermitUnrecorded {
		return "Not allowed: the answer could not be written on the record, so the program was told no."
	}

	return permitSaid[how]
}

// recordPermit writes how a permission question ended on the record of
// what was done, with who asked.
func (b *Brain) recordPermit(inv store.Invocation, how PermitEnded) error {
	outcome := agent.DeedRefused
	if how == PermitAllowed {
		outcome = agent.DeedDone
	}

	_, err := b.DB.AddDeed(store.Deed{
		Tool:      inv.Tool,
		Summary:   inv.Summary,
		SummaryBG: inv.SummaryBG,
		// The label keeps another program's words from standing at the
		// start of what is stored.
		Asked:   "Permission: " + inv.Arguments,
		Result:  permitSaid[how],
		Outcome: outcome,
		AskedBy: inv.AskedBy,
	})
	if err != nil {
		b.Log.Warn("could not record a permission question", "id", inv.ID, "error", err)
	}

	return err
}

/*
 * takeBackPermits closes every permission question that waits, as a no:
 * when the switch is turned off, and when the assistant closes. The
 * programs that wait are told at once.
 */
func (b *Brain) takeBackPermits(closing bool) {
	b.permits.mu.Lock()

	if closing {
		b.permits.closed = true
	}

	ids := make([]int64, 0, len(b.permits.waiting))
	for id := range b.permits.waiting {
		ids = append(ids, id)
	}

	b.permits.mu.Unlock()

	for _, id := range ids {
		// Answered at the desk in this moment: that answer stands, and
		// decidePermit hands it over.
		if moved, err := b.DB.DecideInvocation(id, store.InvocationDenied); err != nil || !moved {
			continue
		}

		b.permits.mu.Lock()
		answer := b.permits.waiting[id]
		delete(b.permits.waiting, id)
		b.permits.mu.Unlock()

		if inv, err := b.DB.Invocation(id); err == nil && inv != nil {
			b.recordPermit(*inv, PermitTakenBack)
			b.DB.CompleteInvocation(id, store.InvocationDenied, permitSaid[PermitTakenBack])
		}

		progress.Decided(id, progress.Refused)

		if answer != nil {
			answer <- permitAnswer{how: PermitTakenBack}
		}
	}
}

/*
 * CloseLeftPermits closes the permission questions left waiting by a run
 * of the assistant that ended without closing them (a power cut): the
 * program that asked is not waiting any more, and a card that could only
 * answer nobody must not sit at the desk.
 *
 * For the running assistant alone, once, as it starts (Start): the
 * commands that open the same brain beside it (ingest, promote) must not
 * close what waits for an answer in the running one.
 */
func (b *Brain) CloseLeftPermits() {
	if b.DB == nil {
		return
	}

	pending, err := b.DB.PendingInvocations()
	if err != nil {
		b.Log.Warn("could not look for permission questions left waiting", "error", err)

		return
	}

	for _, inv := range pending {
		if inv.Tool != otherprograms.PermissionTool {
			continue
		}

		b.permits.mu.Lock()
		_, waited := b.permits.waiting[inv.ID]
		b.permits.mu.Unlock()

		if waited {
			continue
		}

		if moved, err := b.DB.DecideInvocation(inv.ID, store.InvocationDenied); err == nil && moved {
			b.recordPermit(inv, PermitGone)
			b.DB.CompleteInvocation(inv.ID, store.InvocationDenied, permitSaid[PermitGone])
		}
	}
}

// IsPermitQuestion reports whether a waiting question is another
// program's permission question, which only its card answers.
func IsPermitQuestion(inv store.Invocation) bool { return inv.Tool == otherprograms.PermissionTool }

// errPermitOnTheCard is said to anything but the card that tries to
// answer a permission question of another program.
var errPermitOnTheCard = errors.New("a permission question of another program is answered on its card in the assistant's window")
