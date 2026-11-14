package brain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Two of the person's decisions about other programs that are made in one
 * click (design "every program", section 3 points 1 and 7):
 *
 *   - "Stop using other programs": every tool that opens, types or clicks
 *     in, scrolls, or runs another program is refused until "Use other
 *     programs again". Checked by the loop before anything can allow the
 *     call, and again when a waiting question is approved.
 *   - "Always for <program>" on a waiting question: that program's own
 *     card gets Always for that use and the version that was asked about,
 *     and the call goes ahead, as if the person had pressed Always on the
 *     Programs page and then Approve. No tool-wide grant is written.
 */

// ProgramsStopped is since when every use of other programs is stopped,
// and whether it is. A switch that cannot be read counts as on: a safety
// switch that cannot do its check refuses, it does not pass.
func (b *Brain) ProgramsStopped() (time.Time, bool) {
	since, stopped, err := b.DB.ProgramsStopped()
	if err != nil {
		b.Log.Warn("could not read whether other programs are stopped; treating them as stopped", "err", err)

		return time.Now(), true
	}

	return since, stopped
}

// StopUsingPrograms is the one click that stops every use of other programs.
func (b *Brain) StopUsingPrograms() error {
	if err := b.DB.StopPrograms(time.Now()); err != nil {
		return err
	}

	b.Log.Info("the use of other programs was stopped")

	return nil
}

// UseProgramsAgain undoes StopUsingPrograms.
func (b *Brain) UseProgramsAgain() error {
	if err := b.DB.UseProgramsAgain(); err != nil {
		return err
	}

	b.Log.Info("other programs may be used again")

	return nil
}

// stoppedFor is the refusal for a tool while other programs are stopped,
// or empty.
func (b *Brain) stoppedFor(tool tools.Tool) string {
	if !tools.UsesOtherPrograms(tool) {
		return ""
	}

	if since, stopped := b.ProgramsStopped(); stopped {
		return programs.StoppedSentence(since)
	}

	return ""
}

// ApprovalProgram is the program a waiting question is about, as its card
// names it.
type ApprovalProgram struct {
	Kind    programs.Kind `json:"kind"`
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Verb    programs.Verb `json:"verb"`
	Version string        `json:"version,omitempty"`

	// Always: that program's own "always" would answer this question, so
	// the card offers "Always for <name>".
	Always bool `json:"always"`
}

// ApprovalCard is a waiting question as the page shows it.
type ApprovalCard struct {
	store.Invocation

	// Program is the program it is about, if it is about one that is here.
	Program *ApprovalProgram `json:"program,omitempty"`

	// Stopped: it uses another program and every use of other programs is
	// stopped, so approving it would be refused.
	Stopped bool `json:"programs_stopped,omitempty"`
}

// ApprovalCards is every waiting question, with the program each is about.
func (b *Brain) ApprovalCards() ([]ApprovalCard, error) {
	pending, err := b.PendingApprovals()
	if err != nil {
		return nil, err
	}

	cards := make([]ApprovalCard, 0, len(pending))

	var byRow map[int64]programs.Record

	_, stopped := b.ProgramsStopped()

	for _, inv := range pending {
		card := ApprovalCard{Invocation: inv}

		if inv.ProgramID > 0 {
			if byRow == nil {
				byRow, err = b.programsByRow()
				if err != nil {
					return nil, err
				}
			}

			if r, ok := byRow[inv.ProgramID]; ok && r.Present {
				card.Program = &ApprovalProgram{Kind: r.Kind, ID: r.ID, Name: programName(r),
					Verb: programs.Verb(inv.ProgramVerb), Version: inv.ProgramVersion, Always: inv.ProgramAlways}
			}
		}

		if stopped {
			if tool, ok := b.Agent.Registry.Get(inv.Tool); ok && tools.UsesOtherPrograms(tool) {
				card.Stopped = true
			}
		}

		cards = append(cards, card)
	}

	return cards, nil
}

func (b *Brain) programsByRow() (map[int64]programs.Record, error) {
	rs, err := b.DB.Programs("")
	if err != nil {
		return nil, err
	}

	out := make(map[int64]programs.Record, len(rs))
	for _, r := range rs {
		out[r.RowID] = r
	}

	return out, nil
}

func programName(r programs.Record) string {
	if r.Name != "" {
		return r.Name
	}

	return r.ID
}

// Why "Always for <program>" was not given, for the page to say.
var (
	ErrNotAboutAProgram = errors.New("this question is not about one program, so there is no program to allow")
	ErrAlwaysWouldNotDo = errors.New("this question is asked for another reason too, so allowing the program would not stop it")
	ErrNotWaiting       = errors.New("this question is no longer waiting")
)

// ErrNoSuchQuestion is a question id that was never asked.
var ErrNoSuchQuestion = errors.New("there is no such question")

/*
 * AlwaysForProgram is "Always for <program>" on a waiting question: the
 * program's card gets Always for the question's use, given to the version
 * the question was asked about, and then the question is approved (Decide,
 * with everything that checks there). Refused, and nothing written, when
 * the question is about no program, when something else asks it too, when
 * other programs are stopped, or when the program was updated or removed
 * since it was asked (the person decides about that on its card).
 */
func (b *Brain) AlwaysForProgram(ctx context.Context, id int64) (store.Invocation, error) {
	inv, err := b.DB.Invocation(id)
	if err != nil {
		return store.Invocation{}, err
	}

	if inv == nil {
		return store.Invocation{}, ErrNoSuchQuestion
	}

	if inv.Status != store.InvocationPending {
		return *inv, ErrNotWaiting
	}

	if inv.ProgramID <= 0 {
		return *inv, ErrNotAboutAProgram
	}

	if !inv.ProgramAlways {
		return *inv, ErrAlwaysWouldNotDo
	}

	if tool, ok := b.Agent.Registry.Get(inv.Tool); ok {
		if why := b.stoppedFor(tool); why != "" {
			return *inv, errors.New(why)
		}
	}

	byRow, err := b.programsByRow()
	if err != nil {
		return *inv, err
	}

	r, ok := byRow[inv.ProgramID]

	switch {
	case !ok || !r.Present:
		return *inv, fmt.Errorf("%s is not on this computer any more, so it was not allowed", programName(r))
	case r.Version != inv.ProgramVersion:
		return *inv, fmt.Errorf("%s was updated from %s to %s since this was asked; decide about the new version on its card on the Programs page",
			programName(r), inv.ProgramVersion, r.Version)
	}

	if err := b.DB.SetProgramPermit(r.RowID, programs.Verb(inv.ProgramVerb), programs.Always, inv.ProgramVersion, time.Now()); err != nil {
		return *inv, err
	}

	b.Log.Info("a program was allowed always from its question", "verb", inv.ProgramVerb)

	return b.Decide(ctx, id, true)
}

// ProgramChoices is what list_programs says about each program's use.
func (b *Brain) ProgramChoices() (programs.Choices, error) {
	ps, err := b.DB.ProgramPermits()
	if err != nil {
		return programs.Choices{}, err
	}

	f := b.Freedom()
	since, stopped := b.ProgramsStopped()

	return programs.Choices{
		Permits:  ps,
		Honoured: f != permits.AskEveryTime,
		Unasked:  f == permits.Everything,
		Stopped:  stopped,
		Since:    since,
	}, nil
}
