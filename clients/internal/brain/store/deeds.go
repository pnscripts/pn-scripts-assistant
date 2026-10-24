package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/redact"
)

/*
 * What the assistant did, and why — kept so the question can be asked later.
 *
 * The conversation already holds what a tool returned, mixed in with
 * everything else that was said, and a task keeps its own steps. Neither
 * answers the question somebody actually has the next morning: what did it
 * do on my machine yesterday, what was it asked that made it do that, and did
 * anything leave. A deed is one call with exactly those answers beside it.
 */

// Deed is one call the assistant made, put to its owner, or refused.
type Deed struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id,omitempty"`
	TaskID         int64     `json:"task_id,omitempty"`
	TaskName       string    `json:"task_name,omitempty"`
	Tool           string    `json:"tool"`
	Summary        string    `json:"summary"`
	Asked          string    `json:"asked,omitempty"`
	Request        string    `json:"request,omitempty"`
	Result         string    `json:"result,omitempty"`
	Specialist     string    `json:"specialist,omitempty"`
	Model          string    `json:"model,omitempty"`
	ModelWhy       string    `json:"model_why,omitempty"`
	Outcome        string    `json:"outcome"`
	Level          string    `json:"level,omitempty"`
	Changes        bool      `json:"changes_something"`
	Leaves         string    `json:"leaves,omitempty"`
	Millis         int64     `json:"millis"`
	CreatedAt      time.Time `json:"created_at"`
}

// How much of a call's arguments, result and request is kept: enough to
// recognise, not a second copy of the conversation.
const (
	deedAskedLimit   = 600
	deedResultLimit  = 600
	deedRequestLimit = 400
)

// AddDeed records one call. Secrets are taken out first, as everywhere text
// from a tool is kept.
func (d *DB) AddDeed(deed Deed) (int64, error) {
	now := deed.CreatedAt
	if now.IsZero() {
		now = time.Now()
	}

	changes := 0
	if deed.Changes {
		changes = 1
	}

	res, err := d.sql().Exec(`
		INSERT INTO deeds (conversation_id, task_id, tool, summary, asked, request, result,
			specialist, model, model_why, outcome, level, changes, leaves, millis, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullID(deed.ConversationID), nullID(deed.TaskID), deed.Tool,
		d.seal(redact.Text(truncate(deed.Summary, deedAskedLimit))),
		d.seal(redact.Text(truncate(deed.Asked, deedAskedLimit))),
		d.seal(redact.Text(truncate(deed.Request, deedRequestLimit))),
		d.seal(redact.Text(truncate(deed.Result, deedResultLimit))),
		deed.Specialist, deed.Model, deed.ModelWhy, deed.Outcome, deed.Level, changes,
		d.seal(deed.Leaves), deed.Millis, now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("recording what was done: %w", err)
	}

	id, err := res.LastInsertId()

	// Kept for half a year, tidied now and then rather than on every call.
	if err == nil && id%200 == 0 {
		d.PruneDeeds(time.Now().Add(-DeedsKept))
	}

	return id, err
}

// DeedsKept is how long the record of what was done is kept.
const DeedsKept = 183 * 24 * time.Hour

// PruneDeeds removes what was done before a moment.
func (d *DB) PruneDeeds(before time.Time) (int64, error) {
	res, err := d.sql().Exec(`DELETE FROM deeds WHERE created_at < ?`, before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}

	return id
}

// DeedFilter narrows the record to what somebody is looking for.
type DeedFilter struct {
	// Since is the oldest moment wanted; zero is everything.
	Since time.Time

	// Changes keeps only calls that change something.
	Changes bool

	// Leaves keeps only calls that sent something off this machine.
	Leaves bool

	// Conversation keeps one conversation's calls; zero is every one.
	Conversation int64

	// Limit is the most returned, newest first; zero is a hundred.
	Limit int
}

/*
 * Deeds is the record, newest first.
 *
 * A call made inside a task is shown with the task it belongs to, looked up
 * through the task's own conversation rather than stored twice, so a task
 * renamed later is named as it is now.
 */
func (d *DB) Deeds(f DeedFilter) ([]Deed, error) {
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	where := []string{"1 = 1"}
	args := []any{}

	if !f.Since.IsZero() {
		where = append(where, "d.created_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}

	if f.Changes {
		where = append(where, "d.changes = 1")
	}

	if f.Leaves {
		where = append(where, "d.leaves <> ''")
	}

	if f.Conversation != 0 {
		where = append(where, "d.conversation_id = ?")
		args = append(args, f.Conversation)
	}

	args = append(args, limit)

	rows, err := d.sql().Query(`
		SELECT d.id, COALESCE(d.conversation_id, 0), COALESCE(t.id, d.task_id, 0), COALESCE(t.name, ''),
			d.tool, d.summary, d.asked, d.request, d.result, d.specialist, d.model, d.model_why,
			d.outcome, d.level, d.changes, d.leaves, d.millis, d.created_at
		FROM deeds d
		LEFT JOIN tasks t ON t.work_conversation_id = d.conversation_id AND d.conversation_id IS NOT NULL
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY d.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading what was done: %w", err)
	}
	defer rows.Close()

	out := []Deed{}

	for rows.Next() {
		var deed Deed
		var changes int
		var created string

		if err := rows.Scan(&deed.ID, &deed.ConversationID, &deed.TaskID, &deed.TaskName,
			&deed.Tool, &deed.Summary, &deed.Asked, &deed.Request, &deed.Result,
			&deed.Specialist, &deed.Model, &deed.ModelWhy, &deed.Outcome, &deed.Level,
			&changes, &deed.Leaves, &deed.Millis, &created); err != nil {
			return nil, err
		}

		deed.Changes = changes == 1
		deed.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)

		out = append(out, deed)
	}

	return out, rows.Err()
}

/*
 * ForgetDeedsOf takes a deleted conversation's words out of the record.
 *
 * The calls stay — what was done on this machine is not undone by tidying a
 * transcript away, and the record of it is what makes the assistant
 * accountable — but what was asked, what the call was handed and what came
 * back go with the conversation they were said in.
 */
func (d *DB) ForgetDeedsOf(conversationID int64) error {
	_, err := d.sql().Exec(`
		UPDATE deeds SET request = '', asked = '', result = '', conversation_id = NULL
		WHERE conversation_id = ?`, conversationID)

	return err
}

// SetSources keeps an answer's numbered sources with the message.
func (d *DB) SetSources(messageID int64, sources any) error {
	raw, err := json.Marshal(sources)
	if err != nil {
		return err
	}

	_, err = d.sql().Exec(`
		INSERT INTO message_sources (message_id, sources) VALUES (?, ?)
		ON CONFLICT(message_id) DO UPDATE SET sources = excluded.sources`,
		messageID, d.seal(string(raw)))

	return err
}

// Sources is a message's numbered sources, or nil when it has none.
func (d *DB) Sources(messageID int64) (json.RawMessage, error) {
	var raw string

	err := d.sql().QueryRow(`SELECT sources FROM message_sources WHERE message_id = ?`, messageID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return json.RawMessage(raw), nil
}
