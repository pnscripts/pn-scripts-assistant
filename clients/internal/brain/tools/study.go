package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"pn-scripts-assistant/internal/brain/proof"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/study"
)

/*
 * Learning: a lesson at the learner's level, practice questions, answers
 * marked, and missed questions coming back until they are known.
 *
 * The model writes lessons and questions; the study package decides what a
 * lesson must contain, keeps only the questions a rule can mark, marks the
 * answers wherever a rule can, and schedules the next time. Where no rule
 * can tell — a short answer in other words — a model is asked to judge it
 * against the expected answer, and when there is none the learner is shown
 * the answer and marks it themselves. A tutor that marks its own questions
 * generously teaches the wrong thing.
 *
 * Nothing here writes anywhere but the assistant's own record of what is
 * being learnt, so every one of these is Safe.
 */

// StudyBook is the record of what is being learnt, as much of the store as
// the tutor needs.
type StudyBook interface {
	SaveStudyTopic(topic, level, lang, lesson string) (store.StudyTopic, error)
	StudyTopics(now time.Time) ([]store.StudyTopic, error)
	StudyTopic(id int64) (store.StudyTopic, error)
	FindStudyTopic(topic, level string) (store.StudyTopic, error)
	AddStudyCards(topicID int64, cards []store.StudyCard) ([]store.StudyCard, error)
	StudyCard(id int64) (store.StudyCard, error)
	DueStudyCards(topicID int64, now time.Time, limit int) ([]store.StudyCard, error)
	MarkStudyCard(id int64, right bool, box int, due, now time.Time) (store.StudyCard, error)
}

// Tutoring is what the learning tools and the Learning page share.
type Tutoring struct {
	Book StudyBook

	// Ask has the model write something: the system prompt, the learner's
	// words, and back the reply and which model wrote it. Nil is no model.
	Ask func(ctx context.Context, system, user string) (string, string, error)

	// Now is the clock, replaceable in tests.
	Now func() time.Time
}

func (t Tutoring) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}

	return time.Now()
}

// ErrNoTutorModel is a lesson or a quiz asked for with no model to write it.
var ErrNoTutorModel = errors.New("no language model is available to write lessons and questions")

func studyLanguage(lang, topic string) string {
	if lang == "en" || lang == "bg" {
		return lang
	}

	return proof.Language(topic)
}

// Lesson is a lesson written and kept.
type Lesson struct {
	Topic store.StudyTopic `json:"topic"`
	Model string           `json:"model"`
}

// Lesson writes a lesson on a topic at a level and keeps it with the topic.
func (t Tutoring) Lesson(ctx context.Context, topic, level, lang string, minutes int) (Lesson, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return Lesson{}, errors.New("say what to learn")
	}

	lv, ok := study.LevelNamed(level)
	if !ok {
		return Lesson{}, study.ErrLevel
	}

	if t.Ask == nil {
		return Lesson{}, ErrNoTutorModel
	}

	lang = studyLanguage(lang, topic)

	text, model, err := t.Ask(ctx, study.LessonBrief(lv, lang, minutes), topic)
	if err != nil {
		return Lesson{}, err
	}

	kept, err := t.Book.SaveStudyTopic(topic, lv.Name, lang, text)
	if err != nil {
		return Lesson{}, err
	}

	return Lesson{Topic: kept, Model: model}, nil
}

// Quiz is questions to answer: new ones written for it, or ones due again.
type Quiz struct {
	Topic   store.StudyTopic  `json:"topic"`
	Cards   []store.StudyCard `json:"cards"`
	Dropped int               `json:"dropped"`
	Model   string            `json:"model,omitempty"`
	Review  bool              `json:"review"`
}

/*
 * Quiz has the model write count new questions on a topic, keeps the ones
 * that can be marked, and gives them back. With review, no model is asked:
 * the questions already kept that are due again come back instead — of this
 * topic, or of every topic when none is named.
 */
func (t Tutoring) Quiz(ctx context.Context, topic, level, lang string, count int, review bool) (Quiz, error) {
	topic = strings.TrimSpace(topic)

	if count <= 0 {
		count = 5
	}

	if count > study.MaxQuestions {
		count = study.MaxQuestions
	}

	if review {
		var (
			found store.StudyTopic
			id    int64
		)

		if topic != "" {
			lv := ""
			if level != "" {
				l, ok := study.LevelNamed(level)
				if !ok {
					return Quiz{}, study.ErrLevel
				}

				lv = l.Name
			}

			f, err := t.Book.FindStudyTopic(topic, lv)
			if err != nil {
				return Quiz{}, fmt.Errorf("%q has not been studied here yet", topic)
			}

			found, id = f, f.ID
			found.Lesson = ""
		}

		due, err := t.Book.DueStudyCards(id, t.now(), count)
		if err != nil {
			return Quiz{}, err
		}

		return Quiz{Topic: found, Cards: due, Review: true}, nil
	}

	if topic == "" {
		return Quiz{}, errors.New("say what the questions should be about")
	}

	lv, ok := study.LevelNamed(level)
	if !ok {
		return Quiz{}, study.ErrLevel
	}

	if t.Ask == nil {
		return Quiz{}, ErrNoTutorModel
	}

	lang = studyLanguage(lang, topic)

	reply, model, err := t.Ask(ctx, study.QuizBrief(lv, lang, count), topic)
	if err != nil {
		return Quiz{}, err
	}

	questions, dropped, err := study.ParseQuiz(reply)
	if err != nil {
		return Quiz{}, err
	}

	if len(questions) > count {
		dropped += len(questions) - count
		questions = questions[:count]
	}

	kept, err := t.Book.SaveStudyTopic(topic, lv.Name, lang, "")
	if err != nil {
		return Quiz{}, err
	}

	cards := make([]store.StudyCard, 0, len(questions))
	for _, q := range questions {
		cards = append(cards, store.StudyCard{
			Kind: q.Kind, Question: q.Question, Options: q.Options, Answer: q.Answer, Explain: q.Explain,
		})
	}

	cards, err = t.Book.AddStudyCards(kept.ID, cards)
	if err != nil {
		return Quiz{}, err
	}

	kept.Lesson = ""

	return Quiz{Topic: kept, Cards: cards, Dropped: dropped, Model: model}, nil
}

// Marked is an answer marked, and what it changed.
type Marked struct {
	Card     store.StudyCard `json:"card"`
	Right    bool            `json:"right"`
	Sure     bool            `json:"sure"`
	Expected string          `json:"expected"`
	Explain  string          `json:"explain,omitempty"`

	// By is who marked it: "rule", the model's name, or "learner".
	By     string `json:"by"`
	Learnt bool   `json:"learnt"`
}

func questionOf(c store.StudyCard) study.Question {
	return study.Question{Kind: c.Kind, Question: c.Question, Options: c.Options, Answer: c.Answer, Explain: c.Explain}
}

/*
 * Answer marks a learner's answer to a kept question and moves it in the
 * schedule.
 *
 * self is the learner's own mark, given after they were shown the answer
 * because nothing else could tell; it is taken as it is. Otherwise a rule
 * marks it, or failing that the model judges it against the expected
 * answer. When neither can, nothing is recorded and the expected answer
 * comes back for the learner to compare.
 */
func (t Tutoring) Answer(ctx context.Context, id int64, given string, self *bool) (Marked, error) {
	card, err := t.Book.StudyCard(id)
	if err != nil {
		return Marked{}, err
	}

	if len([]rune(given)) > 2000 {
		given = string([]rune(given)[:2000])
	}

	v := study.Check(questionOf(card), given)
	m := Marked{Card: card, Right: v.Right, Sure: v.Sure, Expected: v.Expected, Explain: card.Explain, By: "rule"}

	switch {
	case self != nil:
		m.Right, m.Sure, m.By = *self, true, "learner"

	case !v.Sure && t.Ask != nil:
		topic, _ := t.Book.StudyTopic(card.TopicID)

		lang := topic.Lang
		if lang == "" {
			lang = studyLanguage("", card.Question)
		}

		reply, model, err := t.Ask(ctx, study.JudgeBrief(lang),
			"Question: "+card.Question+"\nExpected answer: "+strings.ReplaceAll(card.Answer, "|", " or ")+
				"\nLearner's answer: "+given)
		if err == nil {
			if right, why, ok := judged(reply); ok {
				m.Right, m.Sure, m.By = right, true, model

				if why != "" {
					r := []rune(why)
					r[0] = unicode.ToUpper(r[0])
					m.Explain = strings.TrimSpace(string(r) + " " + card.Explain)
				}
			}
		}
	}

	if !m.Sure {
		return m, nil
	}

	now := t.now()
	box, due := study.Next(card.Box, m.Right, now)

	updated, err := t.Book.MarkStudyCard(card.ID, m.Right, box, due, now)
	if err != nil {
		return Marked{}, err
	}

	m.Card = updated
	m.Learnt = study.Learnt(updated.Box)

	return m, nil
}

// judged reads a judge's "RIGHT — because…" or "WRONG: …".
func judged(reply string) (bool, string, bool) {
	text := strings.TrimSpace(reply)
	fields := strings.Fields(text)

	if len(fields) == 0 {
		return false, "", false
	}

	first := strings.ToUpper(strings.Trim(fields[0], ".,:;!—-*"))
	why := strings.TrimLeft(strings.TrimSpace(text[len(fields[0]):]), ".,:;!—- ")

	switch first {
	case "RIGHT", "CORRECT", "ВЯРНО", "ПРАВИЛНО":
		return true, why, true
	case "WRONG", "INCORRECT", "ГРЕШНО", "НЕВЯРНО", "НЕПРАВИЛНО":
		return false, why, true
	}

	return false, "", false
}

// studyCues are the words that mean somebody wants to learn. Not "explain"
// or "what is": those are asked of every answer, and offering four more
// tools on every turn makes a small model worse at all of them.
var studyCues = []string{
	"learn", "teach me", "lesson", "tutor", "quiz", "test me", "practi", "homework", "study",
	"exam ", "exams", "revis", "flashcard", // not "exam": it is in "example"
	"уча", "науч", "урок", "тест", "изпитай", "изпит", "упражн", "домашн", "преговор", "учеб",
}

// LessonTool writes a lesson at the learner's level.
type LessonTool struct{ With Tutoring }

func (LessonTool) Name() string   { return "lesson" }
func (LessonTool) Risk() Risk     { return Safe }
func (LessonTool) Cues() []string { return studyCues }

func (LessonTool) Description() string {
	return "Teach a topic as a lesson pitched at the learner's level (child, beginner, intermediate, " +
		"advanced): the goal, what to know first, small steps each with a worked example, common " +
		"mistakes, questions to check yourself with their answers, and what to learn next. The lesson is " +
		"kept with the topic so practice questions and progress follow it. Use quiz afterwards to practise."
}

func (LessonTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"topic": {"type": "string", "description": "What to learn, in the learner's words: \"fractions\", \"how vaccines work\", \"Python loops\""},
			"level": {"type": "string", "enum": ["child", "beginner", "intermediate", "advanced"]},
			"language": {"type": "string", "enum": ["auto", "en", "bg"], "description": "Language of the lesson; auto follows the topic"},
			"minutes": {"type": "integer", "description": "About how long the lesson should take, 5 to 90"}
		},
		"required": ["topic"]
	}`)
}

type lessonArgs struct {
	Topic    string `json:"topic"`
	Level    string `json:"level"`
	Language string `json:"language"`
	Minutes  int    `json:"minutes"`
}

func (LessonTool) Summarize(raw json.RawMessage) string {
	var a lessonArgs

	_ = json.Unmarshal(raw, &a)

	return strings.TrimSpace("Lesson on " + a.Topic + " " + a.Level)
}

func (l LessonTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a lessonArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	done, err := l.With.Lesson(ctx, a.Topic, a.Level, a.Language, a.Minutes)
	if err != nil {
		return "", err
	}

	return done.Topic.Lesson + fmt.Sprintf("\n\n(Kept as topic %d at the %s level. Offer practice questions with quiz when the learner is ready.)",
		done.Topic.ID, done.Topic.Level), nil
}

// QuizTool writes practice questions, or brings back the ones due again.
type QuizTool struct{ With Tutoring }

func (QuizTool) Name() string   { return "quiz" }
func (QuizTool) Risk() Risk     { return Safe }
func (QuizTool) Cues() []string { return studyCues }

func (QuizTool) Description() string {
	return "Practice questions on a topic at the learner's level — choices, numbers and short answers that " +
		"can be marked — kept so missed ones come back later. With review true, no new questions are " +
		"written: the kept questions that are due again come back (of the topic, or of everything). Returns " +
		"each question with its number and never the answers: ask the learner, then pass each reply to " +
		"check_answer with that number."
}

func (QuizTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"topic": {"type": "string", "description": "What the questions are about"},
			"level": {"type": "string", "enum": ["child", "beginner", "intermediate", "advanced"]},
			"language": {"type": "string", "enum": ["auto", "en", "bg"]},
			"count": {"type": "integer", "description": "How many questions, 1 to 10 (5 when not given)"},
			"review": {"type": "boolean", "description": "Bring back the questions due again instead of writing new ones"}
		}
	}`)
}

type quizArgs struct {
	Topic    string `json:"topic"`
	Level    string `json:"level"`
	Language string `json:"language"`
	Count    int    `json:"count"`
	Review   bool   `json:"review"`
}

func (QuizTool) Summarize(raw json.RawMessage) string {
	var a quizArgs

	_ = json.Unmarshal(raw, &a)

	if a.Review {
		return strings.TrimSpace("Review the questions due again " + a.Topic)
	}

	return strings.TrimSpace(fmt.Sprintf("%d practice questions on %s %s", max(a.Count, 1), a.Topic, a.Level))
}

func (q QuizTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a quizArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	made, err := q.With.Quiz(ctx, a.Topic, a.Level, a.Language, a.Count, a.Review)
	if err != nil {
		return "", err
	}

	if len(made.Cards) == 0 {
		return "Nothing is due for review now. Offer new questions instead (quiz without review).", nil
	}

	var b strings.Builder

	if made.Review {
		fmt.Fprintf(&b, "%d questions due again.\n", len(made.Cards))
	} else {
		fmt.Fprintf(&b, "%d new questions on %s (%s).\n", len(made.Cards), made.Topic.Topic, made.Topic.Level)
	}

	b.WriteString("Ask them one at a time, without the answers. Pass each reply to check_answer with the question's number.\n")

	for _, c := range made.Cards {
		fmt.Fprintf(&b, "\nQuestion %d: %s\n", c.ID, c.Question)

		for i, o := range c.Options {
			if i < len(study.Letters) {
				fmt.Fprintf(&b, "  %c) %s\n", study.Letters[i], o)
			}
		}
	}

	return b.String(), nil
}

// CheckAnswer marks a learner's answer to a kept question.
type CheckAnswer struct{ With Tutoring }

func (CheckAnswer) Name() string   { return "check_answer" }
func (CheckAnswer) Risk() Risk     { return Safe }
func (CheckAnswer) Cues() []string { return studyCues }

func (CheckAnswer) Description() string {
	return "Mark the learner's answer to a question from quiz, by the question's number: says right or " +
		"wrong, the expected answer and why, and schedules the question again (soon when missed, later and " +
		"later when right). Pass the learner's own words; never answer for them."
}

func (CheckAnswer) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {"type": "integer", "description": "The question's number from quiz"},
			"answer": {"type": "string", "description": "The learner's answer, as they gave it"}
		},
		"required": ["question", "answer"]
	}`)
}

type checkArgs struct {
	Question int64  `json:"question"`
	Answer   string `json:"answer"`
}

func (CheckAnswer) Summarize(raw json.RawMessage) string {
	var a checkArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Mark the answer to question %d", a.Question)
}

func (c CheckAnswer) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a checkArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	m, err := c.With.Answer(ctx, a.Question, a.Answer, nil)
	if errors.Is(err, store.ErrNoStudy) {
		return "", fmt.Errorf("there is no question %d; use the numbers quiz gave", a.Question)
	}

	if err != nil {
		return "", err
	}

	var b strings.Builder

	switch {
	case !m.Sure:
		fmt.Fprintf(&b, "Could not be marked for certain. Expected: %s. Show the learner both answers and let them judge.", m.Expected)
	case m.Right:
		b.WriteString("Right.")
	default:
		fmt.Fprintf(&b, "Not right. The answer is: %s.", m.Expected)
	}

	if m.Explain != "" {
		b.WriteString(" Why: " + m.Explain)
	}

	if m.Sure {
		if m.Learnt {
			b.WriteString(" This question now counts as learnt.")
		} else {
			fmt.Fprintf(&b, " It comes back %s.", when(m.Card.Due, c.With.now()))
		}
	}

	return b.String(), nil
}

func when(due, now time.Time) string {
	d := due.Sub(now)

	switch {
	case d < time.Hour:
		return "in a few minutes"
	case d < 36*time.Hour:
		return "tomorrow"
	default:
		return fmt.Sprintf("in %d days", int(d.Hours()/24+0.5))
	}
}

// StudyProgress says what is being learnt and how it is going.
type StudyProgress struct{ With Tutoring }

func (StudyProgress) Name() string   { return "study_progress" }
func (StudyProgress) Risk() Risk     { return Safe }
func (StudyProgress) Cues() []string { return studyCues }

func (StudyProgress) Description() string {
	return "What the learner is studying and how it is going: every topic with its level, questions, " +
		"answers right and wrong, questions learnt, and how many are due for review now."
}

func (StudyProgress) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (StudyProgress) Summarize(json.RawMessage) string { return "Learning progress" }

func (s StudyProgress) Execute(context.Context, json.RawMessage) (string, error) {
	topics, err := s.With.Book.StudyTopics(s.With.now())
	if err != nil {
		return "", err
	}

	if len(topics) == 0 {
		return "Nothing has been studied here yet. Offer a lesson or practice questions on any topic.", nil
	}

	var b strings.Builder

	due := 0

	for _, t := range topics {
		answered := t.Right + t.Wrong

		score := "not practised yet"
		if answered > 0 {
			score = fmt.Sprintf("%d of %d answers right (%d%%)", t.Right, answered, t.Right*100/answered)
		}

		fmt.Fprintf(&b, "- %s (%s): %d questions, %d learnt, %s, %d due now.\n",
			t.Topic, t.Level, t.Cards, t.Learnt, score, t.Due)

		due += t.Due
	}

	if due > 0 {
		fmt.Fprintf(&b, "\n%d questions are due for review: quiz with review true brings them back.", due)
	}

	return b.String(), nil
}
