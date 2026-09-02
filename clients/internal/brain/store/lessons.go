package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Lesson is a candidate fact held in quarantine.
type Lesson struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id,omitempty"`
	Content        string    `json:"content"`
	Status         string    `json:"status"`
	Confidence     string    `json:"confidence,omitempty"`
	Source         string    `json:"source,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// AddLesson records a proposal.
func (d *DB) AddLesson(conversationID int64, content, status, confidence, source string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql().Exec(
		`INSERT INTO lessons (conversation_id, content, status, confidence, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullifyID(conversationID), content, status, nullify(confidence), nullify(source), now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("recording lesson: %w", err)
	}

	return res.LastInsertId()
}

// LessonsByStatus returns quarantined lessons at one stage.
func (d *DB) LessonsByStatus(status string, limit int) ([]Lesson, error) {
	rows, err := d.sql().Query(`
		SELECT id, COALESCE(conversation_id,0), content, status,
		       COALESCE(confidence,''), COALESCE(source,''), created_at
		FROM lessons WHERE status = ? ORDER BY id LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Lesson{}

	for rows.Next() {
		var l Lesson
		var created string

		if err := rows.Scan(&l.ID, &l.ConversationID, &l.Content, &l.Status,
			&l.Confidence, &l.Source, &created); err != nil {
			return nil, err
		}

		l.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, l)
	}

	return out, rows.Err()
}

// SetLessonStatus moves a lesson through the pipeline.
func (d *DB) SetLessonStatus(id int64, status string) error {
	_, err := d.sql().Exec(
		`UPDATE lessons SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(time.RFC3339), id,
	)

	return err
}

// PromoteLesson stores a lesson's content as durable knowledge.
//
// The link back to the lesson is kept so a fact can always be traced to what
// proposed it — without that, a wrong fact cannot be explained, only deleted.
func (d *DB) PromoteLesson(lessonID int64, category, content string, embedding []float32) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql().Exec(
		`INSERT INTO knowledge_facts
		 (promoted_from_lesson_id, category, content, embedding, dimensions, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		lessonID, category, content, EncodeVector(embedding), len(embedding), now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("promoting lesson %d: %w", lessonID, err)
	}

	return res.LastInsertId()
}

func nullifyID(id int64) any {
	if id == 0 {
		return nil
	}

	return id
}

// Lesson loads one by id.
func (d *DB) Lesson(id int64) (*Lesson, error) {
	var l Lesson
	var created string

	err := d.sql().QueryRow(`
		SELECT id, COALESCE(conversation_id,0), content, status,
		       COALESCE(confidence,''), COALESCE(source,''), created_at
		FROM lessons WHERE id = ?`, id).
		Scan(&l.ID, &l.ConversationID, &l.Content, &l.Status, &l.Confidence, &l.Source, &created)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	l.CreatedAt, _ = time.Parse(time.RFC3339, created)

	return &l, nil
}
