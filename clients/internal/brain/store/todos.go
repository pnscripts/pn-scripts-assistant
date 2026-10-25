package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

/*
 * To-do lists: things to do with no hour of their own.
 *
 * The list name is sealed with the text, so filtering by list happens here
 * in Go rather than in SQL — a household has dozens of open items, not
 * millions. Ticked items are kept for a fortnight, so "what did I get done
 * this week" has an answer, and then let go.
 */

// Todo is one thing to do.
type Todo struct {
	ID        int64      `json:"id"`
	List      string     `json:"list"`
	Text      string     `json:"text"`
	Due       *time.Time `json:"due,omitempty"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ErrNoTodo is an item that is not there.
var ErrNoTodo = errors.New("there is no such item on the lists")

// DefaultList is where an item goes when no list is named.
const DefaultList = "to do"

// CleanList is a list name as it is kept: trimmed, lower case, never empty.
func CleanList(name string) string {
	name = strings.ToLower(strings.Join(strings.Fields(name), " "))
	if name == "" {
		return DefaultList
	}

	return name
}

// AddTodo puts something on a list.
func (d *DB) AddTodo(list, text string, due *time.Time) (Todo, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Todo{}, errors.New("there is nothing to put on the list")
	}

	list = CleanList(list)
	now := time.Now().UTC()

	var dueText any
	if due != nil {
		dueText = due.Format("2006-01-02")
	}

	res, err := d.sql().Exec(`INSERT INTO todos (list, text, due, created_at) VALUES (?, ?, ?, ?)`,
		d.seal(list), d.seal(text), dueText, now.Format(time.RFC3339Nano))
	if err != nil {
		return Todo{}, fmt.Errorf("putting it on the list: %w", err)
	}

	id, _ := res.LastInsertId()

	d.forgetOldTodos(now)

	return Todo{ID: id, List: list, Text: text, Due: due, CreatedAt: now}, nil
}

// Todos is the items on a list, or on every list for "", open ones first
// (due soonest, then oldest), then the recently ticked.
func (d *DB) Todos(list string, withDone bool) ([]Todo, error) {
	rows, err := d.sql().Query(`SELECT id, list, text, due, done_at, created_at FROM todos`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	want := ""
	if strings.TrimSpace(list) != "" {
		want = CleanList(list)
	}

	out := []Todo{}

	for rows.Next() {
		var (
			t             Todo
			due, done     sql.NullString
			createdAtText string
		)

		if err := rows.Scan(&t.ID, &t.List, &t.Text, &due, &done, &createdAtText); err != nil {
			return nil, err
		}

		if want != "" && t.List != want {
			continue
		}

		if done.Valid {
			if !withDone {
				continue
			}

			if at, err := time.Parse(time.RFC3339Nano, done.String); err == nil {
				t.DoneAt = &at
			}
		}

		if due.Valid {
			if at, err := time.ParseInLocation("2006-01-02", due.String, time.Local); err == nil {
				t.Due = &at
			}
		}

		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtText)
		out = append(out, t)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]

		if (a.DoneAt == nil) != (b.DoneAt == nil) {
			return a.DoneAt == nil
		}

		if a.DoneAt != nil {
			return a.DoneAt.After(*b.DoneAt)
		}

		if (a.Due == nil) != (b.Due == nil) {
			return a.Due != nil
		}

		if a.Due != nil && !a.Due.Equal(*b.Due) {
			return a.Due.Before(*b.Due)
		}

		return a.ID < b.ID
	})

	return out, rows.Err()
}

// TodoLists is every list name with open items, and how many.
func (d *DB) TodoLists() (map[string]int, error) {
	all, err := d.Todos("", false)
	if err != nil {
		return nil, err
	}

	out := map[string]int{}

	for _, t := range all {
		out[t.List]++
	}

	return out, nil
}

// TickTodo marks an item done, or not done again.
func (d *DB) TickTodo(id int64, done bool) (Todo, error) {
	var at any
	if done {
		at = time.Now().UTC().Format(time.RFC3339Nano)
	}

	res, err := d.sql().Exec(`UPDATE todos SET done_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return Todo{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return Todo{}, ErrNoTodo
	}

	return d.todo(id)
}

// RemoveTodo takes an item off its list.
func (d *DB) RemoveTodo(id int64) error {
	res, err := d.sql().Exec(`DELETE FROM todos WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoTodo
	}

	return nil
}

func (d *DB) todo(id int64) (Todo, error) {
	all, err := d.Todos("", true)
	if err != nil {
		return Todo{}, err
	}

	for _, t := range all {
		if t.ID == id {
			return t, nil
		}
	}

	return Todo{}, ErrNoTodo
}

func (d *DB) forgetOldTodos(now time.Time) {
	_, _ = d.sql().Exec(`DELETE FROM todos WHERE done_at IS NOT NULL AND done_at < ?`,
		now.AddDate(0, 0, -14).Format(time.RFC3339Nano))
}
