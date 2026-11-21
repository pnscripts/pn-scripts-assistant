package brain

import (
	"context"
	"errors"
	"fmt"

	"pn-scripts-assistant/internal/brain/learning"
)

// LessonDecision is what happened to a proposed lesson.
type LessonDecision struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	FactID    int64  `json:"fact_id,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
	Message   string `json:"message"`
}

// DecideLesson accepts or rejects something the brain proposed to remember.
//
// Accepting runs the same Curator as an automatic promotion, so a lesson a
// person approves is still checked for being something already known. Without
// that, the one path a human touches would be the one path that can create
// duplicates.
func (b *Brain) DecideLesson(ctx context.Context, id int64, accept bool) (LessonDecision, error) {
	return b.decideLesson(ctx, id, accept, false)
}

/*
 * DecideLessonInTheWindow is DecideLesson for an answer given on the
 * lesson's own card in the assistant's window (the server takes it with
 * the window's key). The only way something another program told the
 * assistant to remember is accepted: every other way of saying yes (out
 * loud, typed to the assistant, "remember them all", the queue worked
 * through in the background) leaves it where it is, because none of them
 * shows its person that one thing with who told it.
 */
func (b *Brain) DecideLessonInTheWindow(ctx context.Context, id int64, accept bool) (LessonDecision, error) {
	return b.decideLesson(ctx, id, accept, true)
}

// ErrToldInTheWindow is a yes, given anywhere but on its card, to
// something another program told the assistant to remember.
var ErrToldInTheWindow = errors.New("what another program told me to remember is accepted on its card in the assistant's window")

func (b *Brain) decideLesson(ctx context.Context, id int64, accept, inTheWindow bool) (LessonDecision, error) {
	lesson, err := b.DB.Lesson(id)
	if err != nil {
		return LessonDecision{}, err
	}

	if lesson == nil {
		return LessonDecision{}, fmt.Errorf("there is no lesson with id %d", id)
	}

	if lesson.Status != learning.StatusProposed {
		return LessonDecision{}, fmt.Errorf(
			"that lesson was already %s and cannot be decided again", lesson.Status)
	}

	if accept && !inTheWindow && ToldByAnotherProgram(*lesson) {
		return LessonDecision{}, ErrToldInTheWindow
	}

	if !accept {
		if err := b.DB.SetLessonStatus(id, learning.StatusRejected); err != nil {
			return LessonDecision{}, err
		}

		return LessonDecision{ID: id, Status: learning.StatusRejected,
			Message: "Rejected. The brain will not remember it."}, nil
	}

	factID, err := b.Learner.Curator.Promote(ctx, *lesson)
	if err != nil {
		return LessonDecision{}, fmt.Errorf("could not store that: %w", err)
	}

	if factID == 0 {
		return LessonDecision{ID: id, Status: learning.StatusRejected, Duplicate: true,
			Message: "The brain already knew that, so it was not stored twice."}, nil
	}

	return LessonDecision{ID: id, Status: learning.StatusPromoted, FactID: factID,
		Message: "Remembered."}, nil
}
