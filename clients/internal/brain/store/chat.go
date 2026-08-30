package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Conversation is one thread.
type Conversation struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}

// Message is one turn within a conversation.
type Message struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Provider  string    `json:"provider,omitempty"`
	Model     string    `json:"model,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// NewConversation starts a thread titled from its opening message.
func (d *DB) NewConversation(title string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql.Exec(
		`INSERT INTO conversations (title, created_at, updated_at) VALUES (?, ?, ?)`,
		truncate(title, 60), now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("starting conversation: %w", err)
	}

	return res.LastInsertId()
}

// ConversationExists reports whether an id refers to a real thread. Used to
// validate input before anything is written against it.
func (d *DB) ConversationExists(id int64) (bool, error) {
	var n int

	err := d.sql.QueryRow(`SELECT COUNT(*) FROM conversations WHERE id = ?`, id).Scan(&n)

	return n > 0, err
}

// AddMessage appends a turn.
func (d *DB) AddMessage(conversationID int64, role, provider, model, content string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql.Exec(
		`INSERT INTO messages (conversation_id, role, provider, model, content, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		conversationID, role, nullify(provider), nullify(model), content, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("recording message: %w", err)
	}

	// Touch the conversation so "latest" means latest activity, not creation.
	d.sql.Exec(`UPDATE conversations SET updated_at = ? WHERE id = ?`, now, conversationID)

	return res.LastInsertId()
}

// History returns every turn in order, oldest first.
func (d *DB) History(conversationID int64) ([]Message, error) {
	rows, err := d.sql.Query(`
		SELECT id, role, COALESCE(provider,''), COALESCE(model,''), content, created_at
		FROM messages WHERE conversation_id = ? ORDER BY id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message

	for rows.Next() {
		var m Message
		var created string

		if err := rows.Scan(&m.ID, &m.Role, &m.Provider, &m.Model, &m.Content, &created); err != nil {
			return nil, err
		}

		m.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, m)
	}

	return out, rows.Err()
}

// LatestConversation returns the most recently active thread, if any.
func (d *DB) LatestConversation() (*Conversation, error) {
	var c Conversation
	var title sql.NullString
	var created string

	err := d.sql.QueryRow(`
		SELECT id, title, created_at FROM conversations
		ORDER BY updated_at DESC, id DESC LIMIT 1`).Scan(&c.ID, &title, &created)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	c.Title = title.String
	c.CreatedAt, _ = time.Parse(time.RFC3339, created)

	return &c, nil
}

// RecentActivity summarises what the brain has been doing, for the interface.
type Activity struct {
	When    time.Time `json:"when"`
	Kind    string    `json:"kind"`
	Summary string    `json:"summary"`
}

// RecentActivity returns the newest few events across messages and learning.
func (d *DB) RecentActivity(limit int) ([]Activity, error) {
	rows, err := d.sql.Query(`
		SELECT created_at, 'message', role || ': ' || substr(content, 1, 80)
		FROM messages WHERE role IN ('user','assistant')
		UNION ALL
		SELECT created_at, 'lesson', status || ': ' || substr(content, 1, 80)
		FROM lessons
		ORDER BY 1 DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Activity{}

	for rows.Next() {
		var a Activity
		var when string

		if err := rows.Scan(&when, &a.Kind, &a.Summary); err != nil {
			return nil, err
		}

		a.When, _ = time.Parse(time.RFC3339, when)
		out = append(out, a)
	}

	return out, rows.Err()
}

// nullify keeps empty optional columns NULL rather than empty strings, so
// "unset" and "set to nothing" stay distinguishable.
func nullify(s string) any {
	if s == "" {
		return nil
	}

	return s
}

// Recent is one conversation as a list shows it.
type Recent struct {
	ID   int64     `json:"id"`
	When time.Time `json:"when"`

	// Opening is the first thing the person said, which is what a conversation
	// is actually remembered by — far better than a title nobody wrote.
	Opening string `json:"opening"`

	// Turns is how much was said, so a real exchange is distinguishable from
	// a greeting that went nowhere.
	Turns int `json:"turns"`
}

/*
 * RecentConversations lists what was talked about, newest first.
 *
 * Ordered by the last message rather than when the conversation was created:
 * one that was returned to an hour later is more recent than one started after
 * it and abandoned.
 *
 * Conversations with nothing in them are left out. They are created by opening
 * the program and never saying anything, and a list mostly made of those is
 * not a history of anything.
 */
func (d *DB) RecentConversations(limit int) ([]Recent, error) {
	rows, err := d.sql.Query(`
		SELECT c.id,
		       MAX(m.created_at) AS last_at,
		       COUNT(m.id)       AS turns,
		       (SELECT content FROM messages
		         WHERE conversation_id = c.id AND role = 'user'
		         ORDER BY id LIMIT 1) AS opening
		FROM conversations c
		JOIN messages m ON m.conversation_id = c.id
		GROUP BY c.id
		HAVING opening IS NOT NULL AND TRIM(opening) <> ''
		ORDER BY last_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []Recent

	for rows.Next() {
		var (
			r      Recent
			lastAt string
		)

		if err := rows.Scan(&r.ID, &lastAt, &r.Turns, &r.Opening); err != nil {
			return nil, err
		}

		r.When, _ = time.Parse(time.RFC3339, lastAt)

		out = append(out, r)
	}

	return out, rows.Err()
}
