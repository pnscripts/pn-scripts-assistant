package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

/*
 * What somebody is learning: topics and their practice questions.
 *
 * The topic is sealed, so finding one by name happens here in Go rather than
 * in SQL — a learner has tens of topics, not thousands. Marking an answer and
 * deciding when a question comes back is the study package's; this only
 * keeps what it decided.
 */

// StudyTopic is one thing being learnt, at a level.
type StudyTopic struct {
	ID        int64     `json:"id"`
	Topic     string    `json:"topic"`
	Level     string    `json:"level"`
	Lang      string    `json:"lang"`
	Lesson    string    `json:"lesson,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Counted from its cards when topics are listed.
	Cards  int `json:"cards"`
	Due    int `json:"due"`
	Learnt int `json:"learnt"`
	Right  int `json:"right"`
	Wrong  int `json:"wrong"`
}

// StudyCard is one practice question and where it stands.
type StudyCard struct {
	ID       int64    `json:"id"`
	TopicID  int64    `json:"topic_id"`
	Kind     string   `json:"kind"`
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
	Answer   string   `json:"answer"`
	Explain  string   `json:"explain,omitempty"`

	Box    int        `json:"box"`
	Due    time.Time  `json:"due"`
	Right  int        `json:"right"`
	Wrong  int        `json:"wrong"`
	LastAt *time.Time `json:"last_at,omitempty"`
}

// ErrNoStudy is a topic or a question that is not there.
var ErrNoStudy = errors.New("there is no such topic or question")

func sameTopic(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

/*
 * SaveStudyTopic finds the topic at this level, or starts it, and keeps a
 * new lesson for it when one is given. The same topic at another level is
 * another topic: a child's fractions and an engineer's are not one record.
 */
func (d *DB) SaveStudyTopic(topic, level, lang, lesson string) (StudyTopic, error) {
	topic = strings.Join(strings.Fields(topic), " ")
	if topic == "" {
		return StudyTopic{}, errors.New("there is no topic to learn")
	}

	if len([]rune(topic)) > 200 {
		topic = string([]rune(topic)[:200])
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)

	all, err := d.studyTopics()
	if err != nil {
		return StudyTopic{}, err
	}

	for _, t := range all {
		if sameTopic(t.Topic, topic) && t.Level == level {
			if lesson == "" {
				return t, nil
			}

			if _, err := d.sql().Exec(`UPDATE study_topics SET lesson = ?, lang = ?, updated_at = ? WHERE id = ?`,
				d.seal(lesson), lang, stamp, t.ID); err != nil {
				return StudyTopic{}, err
			}

			t.Lesson, t.Lang, t.UpdatedAt = lesson, lang, now

			return t, nil
		}
	}

	res, err := d.sql().Exec(`INSERT INTO study_topics (topic, level, lang, lesson, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		d.seal(topic), level, lang, d.seal(lesson), stamp, stamp)
	if err != nil {
		return StudyTopic{}, fmt.Errorf("keeping the topic: %w", err)
	}

	id, _ := res.LastInsertId()

	return StudyTopic{ID: id, Topic: topic, Level: level, Lang: lang, Lesson: lesson, CreatedAt: now, UpdatedAt: now}, nil
}

func (d *DB) studyTopics() ([]StudyTopic, error) {
	rows, err := d.sql().Query(`SELECT id, topic, level, lang, lesson, created_at, updated_at FROM study_topics`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []StudyTopic{}

	for rows.Next() {
		var (
			t                StudyTopic
			created, updated string
		)

		if err := rows.Scan(&t.ID, &t.Topic, &t.Level, &t.Lang, &t.Lesson, &created, &updated); err != nil {
			return nil, err
		}

		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, t)
	}

	return out, rows.Err()
}

// StudyTopics is every topic with its counts, the most recently studied
// first. The lessons are left out; StudyTopic has them.
func (d *DB) StudyTopics(now time.Time) ([]StudyTopic, error) {
	topics, err := d.studyTopics()
	if err != nil {
		return nil, err
	}

	cards, err := d.StudyCards(0)
	if err != nil {
		return nil, err
	}

	at := map[int64]int{}
	for i := range topics {
		at[topics[i].ID] = i
		topics[i].Lesson = ""
	}

	for _, c := range cards {
		i, ok := at[c.TopicID]
		if !ok {
			continue
		}

		t := &topics[i]
		t.Cards++
		t.Right += c.Right
		t.Wrong += c.Wrong

		if !c.Due.After(now) {
			t.Due++
		}

		if c.Box >= 5 {
			t.Learnt++
		}

		if c.LastAt != nil && c.LastAt.After(t.UpdatedAt) {
			t.UpdatedAt = *c.LastAt
		}
	}

	sort.SliceStable(topics, func(i, j int) bool { return topics[i].UpdatedAt.After(topics[j].UpdatedAt) })

	return topics, nil
}

// StudyTopic is one topic with its lesson.
func (d *DB) StudyTopic(id int64) (StudyTopic, error) {
	all, err := d.studyTopics()
	if err != nil {
		return StudyTopic{}, err
	}

	for _, t := range all {
		if t.ID == id {
			return t, nil
		}
	}

	return StudyTopic{}, ErrNoStudy
}

// FindStudyTopic is the topic of this name at this level, or at any level
// when level is empty (the most recent).
func (d *DB) FindStudyTopic(topic, level string) (StudyTopic, error) {
	all, err := d.StudyTopics(time.Now())
	if err != nil {
		return StudyTopic{}, err
	}

	for _, t := range all {
		if sameTopic(t.Topic, topic) && (level == "" || t.Level == level) {
			return d.StudyTopic(t.ID)
		}
	}

	return StudyTopic{}, ErrNoStudy
}

// RemoveStudyTopic forgets a topic and its questions.
func (d *DB) RemoveStudyTopic(id int64) error {
	if _, err := d.sql().Exec(`DELETE FROM study_cards WHERE topic_id = ?`, id); err != nil {
		return err
	}

	res, err := d.sql().Exec(`DELETE FROM study_topics WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoStudy
	}

	return nil
}

// AddStudyCards keeps new questions for a topic, due now.
func (d *DB) AddStudyCards(topicID int64, cards []StudyCard) ([]StudyCard, error) {
	if _, err := d.StudyTopic(topicID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	out := make([]StudyCard, 0, len(cards))

	for _, c := range cards {
		options := ""
		if len(c.Options) > 0 {
			b, _ := json.Marshal(c.Options)
			options = string(b)
		}

		res, err := d.sql().Exec(`INSERT INTO study_cards (topic_id, kind, question, options, answer, explain, box, due, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			topicID, c.Kind, d.seal(c.Question), d.seal(options), d.seal(c.Answer), d.seal(c.Explain), stamp, stamp)
		if err != nil {
			return out, fmt.Errorf("keeping a question: %w", err)
		}

		c.ID, _ = res.LastInsertId()
		c.TopicID, c.Box, c.Due, c.Right, c.Wrong, c.LastAt = topicID, 1, now, 0, 0, nil
		out = append(out, c)
	}

	return out, nil
}

// StudyCards is a topic's questions, or every topic's for 0, oldest first.
func (d *DB) StudyCards(topicID int64) ([]StudyCard, error) {
	query := `SELECT id, topic_id, kind, question, options, answer, explain, box, due, right_count, wrong_count, last_at FROM study_cards`

	var args []any

	if topicID > 0 {
		query += ` WHERE topic_id = ?`
		args = append(args, topicID)
	}

	rows, err := d.sql().Query(query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []StudyCard{}

	for rows.Next() {
		var (
			c       StudyCard
			options string
			due     string
			last    sql.NullString
		)

		if err := rows.Scan(&c.ID, &c.TopicID, &c.Kind, &c.Question, &options, &c.Answer, &c.Explain,
			&c.Box, &due, &c.Right, &c.Wrong, &last); err != nil {
			return nil, err
		}

		if options != "" {
			_ = json.Unmarshal([]byte(options), &c.Options)
		}

		c.Due, _ = time.Parse(time.RFC3339Nano, due)

		if last.Valid {
			if at, err := time.Parse(time.RFC3339Nano, last.String); err == nil {
				c.LastAt = &at
			}
		}

		out = append(out, c)
	}

	return out, rows.Err()
}

// StudyCard is one question.
func (d *DB) StudyCard(id int64) (StudyCard, error) {
	all, err := d.StudyCards(0)
	if err != nil {
		return StudyCard{}, err
	}

	for _, c := range all {
		if c.ID == id {
			return c, nil
		}
	}

	return StudyCard{}, ErrNoStudy
}

// DueStudyCards is the questions due by now — of one topic, or of all for 0 —
// the longest overdue first, at most limit of them.
func (d *DB) DueStudyCards(topicID int64, now time.Time, limit int) ([]StudyCard, error) {
	all, err := d.StudyCards(topicID)
	if err != nil {
		return nil, err
	}

	out := []StudyCard{}

	for _, c := range all {
		if !c.Due.After(now) {
			out = append(out, c)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Due.Before(out[j].Due) })

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

// MarkStudyCard keeps an answer's outcome: the box and due time the
// schedule chose, and one more right or wrong.
func (d *DB) MarkStudyCard(id int64, right bool, box int, due, now time.Time) (StudyCard, error) {
	column := "wrong_count"
	if right {
		column = "right_count"
	}

	res, err := d.sql().Exec(`UPDATE study_cards SET box = ?, due = ?, last_at = ?, `+column+` = `+column+` + 1 WHERE id = ?`,
		box, due.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return StudyCard{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return StudyCard{}, ErrNoStudy
	}

	return d.StudyCard(id)
}
