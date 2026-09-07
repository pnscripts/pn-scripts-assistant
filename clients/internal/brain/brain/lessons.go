package brain

import (
	"context"
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
