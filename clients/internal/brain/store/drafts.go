package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

/*
 * Replies waiting for their owner's yes.
 *
 * The assistant may write as many drafts as it likes — writing one sends
 * nothing — and none of them leaves until its owner sends that one, as it
 * stands. Sent and discarded drafts are kept as a record for a month and
 * then let go.
 */

// Draft states.
const (
	DraftWaiting   = "waiting"
	DraftSent      = "sent"
	DraftDiscarded = "discarded"
	DraftFailed    = "failed"
)

// Draft is one message waiting to be sent.
type Draft struct {
	ID      int64  `json:"id"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`

	// Answers is the message it replies to, as its owner would recognise
	// it: "Anna Petrova <anna@example.com> — Offer for March, 3 May".
	Answers string `json:"answers,omitempty"`

	// Warning is the scam check's verdict on the message it answers, when
	// that verdict warned.
	Warning string `json:"warning,omitempty"`

	Status    string     `json:"status"`
	Note      string     `json:"note,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
}

// ErrNoDraft is a draft that is not there, or no longer waiting.
var ErrNoDraft = errors.New("there is no such draft waiting")

// AddDraft keeps a new draft, waiting.
func (d *DB) AddDraft(to, subject, body, answers, warning string) (Draft, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return Draft{}, errors.New("a draft needs somebody to send it to")
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)

	res, err := d.sql().Exec(`
		INSERT INTO mail_drafts (to_addr, subject, body, answers, warning, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.seal(to), d.seal(subject), d.seal(body), d.seal(answers), d.seal(warning), DraftWaiting, stamp, stamp)
	if err != nil {
		return Draft{}, fmt.Errorf("keeping the draft: %w", err)
	}

	id, _ := res.LastInsertId()

	d.forgetOldDrafts(now)

	return Draft{ID: id, To: to, Subject: subject, Body: body, Answers: answers, Warning: warning,
		Status: DraftWaiting, CreatedAt: now, UpdatedAt: now}, nil
}

const draftColumns = `id, to_addr, subject, body, answers, warning, status, note, created_at, updated_at, sent_at`

func scanDraft(row interface{ Scan(...any) error }) (Draft, error) {
	var (
		dr               Draft
		created, updated string
		sent             sql.NullString
	)

	if err := row.Scan(&dr.ID, &dr.To, &dr.Subject, &dr.Body, &dr.Answers, &dr.Warning, &dr.Status, &dr.Note,
		&created, &updated, &sent); err != nil {
		return Draft{}, err
	}

	dr.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	dr.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)

	if sent.Valid {
		if at, err := time.Parse(time.RFC3339Nano, sent.String); err == nil {
			dr.SentAt = &at
		}
	}

	return dr, nil
}

// Draft is one draft, in whatever state.
func (d *DB) Draft(id int64) (Draft, error) {
	dr, err := scanDraft(d.sql().QueryRow(`SELECT `+draftColumns+` FROM mail_drafts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, ErrNoDraft
	}

	return dr, err
}

// Drafts lists drafts in one state, or all with "", newest first.
func (d *DB) Drafts(status string, limit int) ([]Draft, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT ` + draftColumns + ` FROM mail_drafts`
	args := []any{}

	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.sql().Query(query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Draft{}

	for rows.Next() {
		dr, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, dr)
	}

	return out, rows.Err()
}

// ChangeDraft rewrites a waiting draft.
func (d *DB) ChangeDraft(id int64, to, subject, body string) (Draft, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return Draft{}, errors.New("a draft needs somebody to send it to")
	}

	res, err := d.sql().Exec(`
		UPDATE mail_drafts SET to_addr = ?, subject = ?, body = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		d.seal(to), d.seal(subject), d.seal(body), time.Now().UTC().Format(time.RFC3339Nano),
		id, DraftWaiting, DraftFailed)
	if err != nil {
		return Draft{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return Draft{}, ErrNoDraft
	}

	return d.Draft(id)
}

/*
 * SetDraftStatus records what became of a draft: sent, discarded, or failed
 * with the reason. Only a waiting or failed draft can change, so a draft is
 * never sent twice by two presses.
 */
func (d *DB) SetDraftStatus(id int64, status, note string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	var sent any
	if status == DraftSent {
		sent = now
	}

	res, err := d.sql().Exec(`
		UPDATE mail_drafts SET status = ?, note = ?, updated_at = ?, sent_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		status, d.seal(note), now, sent, id, DraftWaiting, DraftFailed)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoDraft
	}

	return nil
}

// ClaimDraft marks a draft as being sent, so a second press finds nothing
// to send. It returns the draft as it was claimed.
func (d *DB) ClaimDraft(id int64) (Draft, error) {
	// A draft left "sending" by a program that stopped half way is claimable
	// again after ten minutes — far longer than any server takes to answer.
	now := time.Now().UTC()

	res, err := d.sql().Exec(`
		UPDATE mail_drafts SET status = 'sending', updated_at = ?
		WHERE id = ? AND (status IN (?, ?) OR (status = 'sending' AND updated_at < ?))`,
		now.Format(time.RFC3339Nano), id, DraftWaiting, DraftFailed,
		now.Add(-10*time.Minute).Format(time.RFC3339Nano))
	if err != nil {
		return Draft{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return Draft{}, ErrNoDraft
	}

	return d.Draft(id)
}

// SentDraft and FailedDraft finish a claimed draft.
func (d *DB) SentDraft(id int64, note string) error { return d.finishDraft(id, DraftSent, note) }

func (d *DB) FailedDraft(id int64, note string) error { return d.finishDraft(id, DraftFailed, note) }

func (d *DB) finishDraft(id int64, status, note string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	var sent any
	if status == DraftSent {
		sent = now
	}

	_, err := d.sql().Exec(`UPDATE mail_drafts SET status = ?, note = ?, updated_at = ?, sent_at = ?
		WHERE id = ? AND status = 'sending'`, status, d.seal(note), now, sent, id)

	return err
}

// forgetOldDrafts lets sent and discarded drafts go after a month.
func (d *DB) forgetOldDrafts(now time.Time) {
	_, _ = d.sql().Exec(`DELETE FROM mail_drafts WHERE status IN (?, ?) AND updated_at < ?`,
		DraftSent, DraftDiscarded, now.AddDate(0, -1, 0).Format(time.RFC3339Nano))
}
