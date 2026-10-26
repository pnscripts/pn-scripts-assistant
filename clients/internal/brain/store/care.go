package store

import (
	"errors"
	"strings"
	"time"
)

/*
 * A business's answer book and how it wants to sound. The care package
 * decides what to do with them; this keeps them, sealed.
 */

// CareAnswer is one question the business has answered, as it wants it
// answered.
type CareAnswer struct {
	ID        int64     `json:"id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ErrNoCareAnswer is an answer that is not there.
var ErrNoCareAnswer = errors.New("there is no such answer in the answer book")

func cleanCare(question, answer string) (string, string, error) {
	question, answer = strings.TrimSpace(question), strings.TrimSpace(answer)
	if question == "" || answer == "" {
		return "", "", errors.New("an answer needs both the question and the answer")
	}

	if len([]rune(question)) > 1000 || len([]rune(answer)) > 8000 {
		return "", "", errors.New("keep the question under 1,000 characters and the answer under 8,000")
	}

	return question, answer, nil
}

// AddCareAnswer keeps a new answer.
func (d *DB) AddCareAnswer(question, answer string) (CareAnswer, error) {
	question, answer, err := cleanCare(question, answer)
	if err != nil {
		return CareAnswer{}, err
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)

	res, err := d.sql().Exec(`INSERT INTO care_answers (question, answer, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		d.seal(question), d.seal(answer), stamp, stamp)
	if err != nil {
		return CareAnswer{}, err
	}

	id, _ := res.LastInsertId()

	return CareAnswer{ID: id, Question: question, Answer: answer, CreatedAt: now, UpdatedAt: now}, nil
}

// ChangeCareAnswer rewrites an answer.
func (d *DB) ChangeCareAnswer(id int64, question, answer string) (CareAnswer, error) {
	question, answer, err := cleanCare(question, answer)
	if err != nil {
		return CareAnswer{}, err
	}

	now := time.Now().UTC()

	res, err := d.sql().Exec(`UPDATE care_answers SET question = ?, answer = ?, updated_at = ? WHERE id = ?`,
		d.seal(question), d.seal(answer), now.Format(time.RFC3339Nano), id)
	if err != nil {
		return CareAnswer{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return CareAnswer{}, ErrNoCareAnswer
	}

	return CareAnswer{ID: id, Question: question, Answer: answer, UpdatedAt: now}, nil
}

// RemoveCareAnswer takes an answer out of the book.
func (d *DB) RemoveCareAnswer(id int64) error {
	res, err := d.sql().Exec(`DELETE FROM care_answers WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoCareAnswer
	}

	return nil
}

// CareAnswers is the whole answer book, most recently changed first.
func (d *DB) CareAnswers() ([]CareAnswer, error) {
	rows, err := d.sql().Query(`SELECT id, question, answer, created_at, updated_at FROM care_answers ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []CareAnswer{}

	for rows.Next() {
		var (
			a                CareAnswer
			created, updated string
		)

		if err := rows.Scan(&a.ID, &a.Question, &a.Answer, &created, &updated); err != nil {
			return nil, err
		}

		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		a.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, a)
	}

	return out, rows.Err()
}

// CareProfile is how the business wants to sound, as names and values.
func (d *DB) CareProfile() (map[string]string, error) {
	rows, err := d.sql().Query(`SELECT key, value FROM care_profile`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := map[string]string{}

	for rows.Next() {
		var k, v string

		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}

		out[k] = v
	}

	return out, rows.Err()
}

// SetCareProfile keeps the values given; an empty value forgets one.
func (d *DB) SetCareProfile(values map[string]string) error {
	tx, err := d.sql().Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	for k, v := range values {
		v = strings.TrimSpace(v)

		if v == "" {
			if _, err := tx.Exec(`DELETE FROM care_profile WHERE key = ?`, k); err != nil {
				return err
			}

			continue
		}

		if _, err := tx.Exec(`INSERT INTO care_profile (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, d.seal(v)); err != nil {
			return err
		}
	}

	return tx.Commit()
}
