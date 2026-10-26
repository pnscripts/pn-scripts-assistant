package tools

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

// tutorModel answers every brief with what the test gave it, and remembers
// what it was asked.
type tutorModel struct {
	lesson, quiz, judge string
	asked               []string
}

func (m *tutorModel) ask(_ context.Context, system, user string) (string, string, error) {
	m.asked = append(m.asked, system+"\n"+user)

	switch {
	case strings.Contains(system, "You mark a learner"):
		return m.judge, "judge-model", nil
	case strings.Contains(system, "JSON array"):
		return m.quiz, "quiz-model", nil
	}

	return m.lesson, "lesson-model", nil
}

func tutorFor(t *testing.T, m *tutorModel, now time.Time) Tutoring {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return Tutoring{Book: db, Ask: m.ask, Now: func() time.Time { return now }}
}

const fractionQuiz = `[` +
	`{"kind":"choice","question":"Which is bigger?","options":["1/2","1/3","1/4"],"answer":"A","explain":"Fewer pieces are bigger pieces."},` +
	`{"kind":"number","question":"1/2 + 1/4 as a decimal?","answer":"0.75","explain":"Two quarters and one quarter."},` +
	`{"kind":"short","question":"What is the top number of a fraction called?","answer":"numerator","explain":"It counts the parts."},` +
	`{"kind":"essay","question":"Discuss fractions.","answer":"…"}]`

/*
 * The whole round: a lesson kept with its topic, questions written without
 * their answers, answers marked by rule, by the judge, and by the learner,
 * and the missed one coming back.
 */
func TestTutorLessonQuizAndAnswers(t *testing.T) {
	now := time.Date(2026, 10, 26, 17, 0, 0, 0, time.UTC)
	m := &tutorModel{lesson: "# Fractions\n## Goal\n…", quiz: fractionQuiz, judge: "WRONG — the denominator is the bottom number."}
	tutor := tutorFor(t, m, now)
	ctx := context.Background()

	said, err := LessonTool{With: tutor}.Execute(ctx, json.RawMessage(`{"topic":"Дроби","level":"child"}`))
	if err != nil || !strings.HasPrefix(said, "# Fractions") || !strings.Contains(said, "topic 1 at the child level") {
		t.Fatalf("%v %q", err, said)
	}

	if !strings.Contains(m.asked[0], "Bulgarian") || !strings.Contains(m.asked[0], "child of about 8") {
		t.Errorf("a Bulgarian topic for a child was not briefed so: %s", m.asked[0])
	}

	if _, err := (LessonTool{With: tutor}).Execute(ctx, json.RawMessage(`{"topic":"x","level":"genius"}`)); err == nil {
		t.Error("an unknown level was accepted")
	}

	said, err = QuizTool{With: tutor}.Execute(ctx, json.RawMessage(`{"topic":"дроби","level":"child","count":5}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "3 new questions") || !strings.Contains(said, "A) 1/2") ||
		strings.Contains(said, "0.75") || strings.Contains(said, "numerator") {
		t.Errorf("the questions came with their answers, or not at all: %s", said)
	}

	topics, _ := tutor.Book.StudyTopics(now)
	if len(topics) != 1 || topics[0].Cards != 3 {
		t.Fatalf("the quiz did not join the lesson's topic: %+v", topics)
	}

	cards, _ := tutor.Book.DueStudyCards(0, now, 0)

	right, err := tutor.Answer(ctx, cards[0].ID, "a", nil)
	if err != nil || !right.Right || right.By != "rule" || right.Card.Box != 2 {
		t.Errorf("a right letter: %v %+v", err, right)
	}

	wrong, _ := tutor.Answer(ctx, cards[1].ID, "0,5", nil)
	if wrong.Right || !wrong.Sure || wrong.Expected != "0.75" || wrong.Card.Wrong != 1 {
		t.Errorf("a wrong number: %+v", wrong)
	}

	judged, _ := tutor.Answer(ctx, cards[2].ID, "the bottom one", nil)
	if judged.Right || !judged.Sure || judged.By != "judge-model" || !strings.Contains(judged.Explain, "denominator") {
		t.Errorf("an answer no rule could mark goes to the judge: %+v", judged)
	}

	if !strings.Contains(m.asked[len(m.asked)-1], "Learner's answer: the bottom one") {
		t.Errorf("the judge was not shown the answer: %s", m.asked[len(m.asked)-1])
	}

	yes := true

	self, _ := tutor.Answer(ctx, cards[2].ID, "numerator!", &yes)
	if !self.Right || self.By != "learner" {
		t.Errorf("the learner's own mark: %+v", self)
	}

	// An hour later only the missed number is due again.
	later := tutor
	later.Now = func() time.Time { return now.Add(time.Hour) }

	said, _ = QuizTool{With: later}.Execute(ctx, json.RawMessage(`{"review":true}`))
	if !strings.Contains(said, "1 questions due again") || !strings.Contains(said, "1/2 + 1/4") {
		t.Errorf("review: %s", said)
	}

	if said, _ = (QuizTool{With: later}).Execute(ctx, json.RawMessage(`{"review":true,"topic":"geography"}`)); said != "" {
		t.Errorf("a topic never studied was reviewed: %s", said)
	}

	said, _ = StudyProgress{With: later}.Execute(ctx, nil)
	if !strings.Contains(said, "Дроби (child): 3 questions") || !strings.Contains(said, "2 of 4 answers right (50%)") ||
		!strings.Contains(said, "1 due now") {
		t.Errorf("progress: %s", said)
	}

	said, _ = CheckAnswer{With: tutor}.Execute(ctx, json.RawMessage(`{"question":1,"answer":"1/2"}`))
	if !strings.HasPrefix(said, "Right.") || !strings.Contains(said, "comes back in 3 days") {
		t.Errorf("check_answer: %s", said)
	}

	if _, err := (CheckAnswer{With: tutor}).Execute(ctx, json.RawMessage(`{"question":99,"answer":"x"}`)); err == nil ||
		!strings.Contains(err.Error(), "no question 99") {
		t.Errorf("a missing question: %v", err)
	}
}

// With no model, nothing is guessed: a short answer in other words is not
// marked, nothing is recorded, and the expected answer comes back.
func TestTutorWithoutAModel(t *testing.T) {
	now := time.Now()
	tutor := tutorFor(t, &tutorModel{}, now)
	tutor.Ask = nil

	if _, err := tutor.Lesson(context.Background(), "fractions", "", "", 0); !errors.Is(err, ErrNoTutorModel) {
		t.Errorf("a lesson with no model: %v", err)
	}

	topic, _ := tutor.Book.SaveStudyTopic("fractions", "beginner", "en", "")
	cards, _ := tutor.Book.AddStudyCards(topic.ID, []store.StudyCard{{Kind: "short", Question: "Top number?", Answer: "numerator"}})

	m, err := tutor.Answer(context.Background(), cards[0].ID, "the one on top", nil)
	if err != nil || m.Sure || m.Expected != "numerator" {
		t.Errorf("%v %+v", err, m)
	}

	if back, _ := tutor.Book.StudyCard(cards[0].ID); back.Right+back.Wrong != 0 {
		t.Errorf("an unmarked answer was recorded: %+v", back)
	}

	said, _ := StudyProgress{With: tutor}.Execute(context.Background(), nil)
	if !strings.Contains(said, "not practised yet") {
		t.Errorf("%s", said)
	}
}

func TestJudged(t *testing.T) {
	for reply, want := range map[string]bool{"RIGHT. Same meaning.": true, "**Wrong**: no": false, "Вярно — да.": true, "Грешно.": false} {
		if right, _, ok := judged(reply); !ok || right != want {
			t.Errorf("%q read as %v %v", reply, right, ok)
		}
	}

	if _, _, ok := judged("Maybe, it depends."); ok {
		t.Error("a hedge was read as a mark")
	}
}
