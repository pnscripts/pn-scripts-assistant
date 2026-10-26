package server

import (
	"errors"
	"net/http"
	"time"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/study"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The Learning page: topics and how they are going, a lesson at a level,
 * practice questions answered one at a time, and the questions due again.
 *
 * A question goes to the page without its answer; the answer comes back
 * only once the learner has given theirs. Everything here writes only the
 * assistant's own record of what is being learnt.
 */

// question is a card as the page may see it before it is answered.
type question struct {
	ID       int64      `json:"id"`
	TopicID  int64      `json:"topic_id"`
	Kind     string     `json:"kind"`
	Question string     `json:"question"`
	Options  []string   `json:"options,omitempty"`
	Box      int        `json:"box"`
	Due      time.Time  `json:"due"`
	Right    int        `json:"right"`
	Wrong    int        `json:"wrong"`
	LastAt   *time.Time `json:"last_at,omitempty"`
}

func questionsOf(cards []store.StudyCard) []question {
	out := make([]question, 0, len(cards))

	for _, c := range cards {
		out = append(out, question{
			ID: c.ID, TopicID: c.TopicID, Kind: c.Kind, Question: c.Question, Options: c.Options,
			Box: c.Box, Due: c.Due, Right: c.Right, Wrong: c.Wrong, LastAt: c.LastAt,
		})
	}

	return out
}

func (s *Server) handleStudy(w http.ResponseWriter, _ *http.Request) {
	topics, err := s.brain.DB.StudyTopics(time.Now())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	due := 0
	for _, t := range topics {
		due += t.Due
	}

	ok(w, map[string]any{"topics": topics, "due": due, "levels": study.Levels})
}

func (s *Server) handleStudyTopic(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	topic, err := s.brain.DB.StudyTopic(id)
	if errors.Is(err, store.ErrNoStudy) {
		fail(w, http.StatusNotFound, "That topic is not there.")

		return
	}

	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	cards, err := s.brain.DB.StudyCards(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"topic": topic, "questions": questionsOf(cards)})
}

// tutorFailed says why a lesson or a quiz could not be written.
func tutorFailed(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, brain.ErrNoModel), errors.Is(err, tools.ErrNoTutorModel):
		fail(w, http.StatusServiceUnavailable, "No language model is available to write lessons and questions. "+
			"Questions already kept can still be practised.")
	case errors.Is(err, study.ErrLevel):
		fail(w, http.StatusBadRequest, err.Error())
	default:
		fail(w, http.StatusBadGateway, err.Error())
	}
}

func (s *Server) handleStudyLesson(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Topic    string `json:"topic"`
		Level    string `json:"level"`
		Language string `json:"language"`
		Minutes  int    `json:"minutes"`
	}

	if !decodeUpTo(w, r, &body, 1<<12) {
		return
	}

	if _, good := study.LevelNamed(body.Level); !good {
		fail(w, http.StatusBadRequest, study.ErrLevel.Error())

		return
	}

	if len([]rune(body.Topic)) == 0 || len([]rune(body.Topic)) > 200 {
		fail(w, http.StatusBadRequest, "Say what to learn, in at most 200 characters.")

		return
	}

	done, err := s.brain.Tutoring().Lesson(r.Context(), body.Topic, body.Level, body.Language, body.Minutes)
	if err != nil {
		tutorFailed(w, err)

		return
	}

	ok(w, done)
}

func (s *Server) handleStudyQuiz(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Topic    string `json:"topic"`
		Level    string `json:"level"`
		Language string `json:"language"`
		Count    int    `json:"count"`
		Review   bool   `json:"review"`
	}

	if !decodeUpTo(w, r, &body, 1<<12) {
		return
	}

	if len([]rune(body.Topic)) > 200 {
		fail(w, http.StatusBadRequest, "Say what to learn, in at most 200 characters.")

		return
	}

	if !body.Review && len(body.Topic) == 0 {
		fail(w, http.StatusBadRequest, "Say what the questions should be about.")

		return
	}

	made, err := s.brain.Tutoring().Quiz(r.Context(), body.Topic, body.Level, body.Language, body.Count, body.Review)
	if err != nil {
		if body.Review {
			fail(w, http.StatusNotFound, err.Error())

			return
		}

		tutorFailed(w, err)

		return
	}

	ok(w, map[string]any{
		"topic": made.Topic, "questions": questionsOf(made.Cards), "dropped": made.Dropped,
		"model": made.Model, "review": made.Review,
	})
}

func (s *Server) handleStudyAnswer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Card   int64  `json:"card"`
		Answer string `json:"answer"`
		Self   *bool  `json:"self"`
	}

	if !decodeUpTo(w, r, &body, 1<<13) {
		return
	}

	marked, err := s.brain.Tutoring().Answer(r.Context(), body.Card, body.Answer, body.Self)
	if errors.Is(err, store.ErrNoStudy) {
		fail(w, http.StatusNotFound, "That question is not there.")

		return
	}

	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"right": marked.Right, "sure": marked.Sure, "expected": marked.Expected, "explain": marked.Explain,
		"by": marked.By, "learnt": marked.Learnt, "question": questionsOf([]store.StudyCard{marked.Card})[0],
	})
}

func (s *Server) handleStudyRemove(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	if err := s.brain.DB.RemoveStudyTopic(id); errors.Is(err, store.ErrNoStudy) {
		fail(w, http.StatusNotFound, "That topic is not there.")

		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"removed": id})
}
