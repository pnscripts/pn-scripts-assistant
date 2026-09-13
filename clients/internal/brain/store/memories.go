package store

import (
	"strings"
	"time"
	"unicode"
)

// The kinds of thing an agent remembers.
const (
	// Found is what a step of its established, checked against what the tools
	// returned.
	Found = "found"

	// Learned is what went wrong: a review that found its work wrong, a step it
	// could not do and handed on. The memory worth most, and the one a person
	// would be least likely to write down.
	Learned = "learned"
)

// Memory is one thing one agent remembers from its own work.
type Memory struct {
	ID      int64     `json:"id"`
	Agent   string    `json:"agent"`
	Kind    string    `json:"kind"`
	Content string    `json:"content"`
	TaskID  int64     `json:"task_id,omitempty"`
	StepID  int64     `json:"step_id,omitempty"`
	At      time.Time `json:"at"`
}

// Remember writes one memory for one agent.
func (d *DB) Remember(m Memory) error {
	_, err := d.sql().Exec(`INSERT INTO agent_memories (agent, kind, content, task_id, step_id, created_at)
		VALUES (?,?,?,?,?,?)`, m.Agent, m.Kind, strings.TrimSpace(m.Content),
		nullable(m.TaskID), nullable(m.StepID), time.Now().UTC().Format(time.RFC3339))

	return err
}

// Memories is what one agent remembers, newest first.
func (d *DB) Memories(agent string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := d.sql().Query(`
		SELECT id, agent, kind, content, COALESCE(task_id,0), COALESCE(step_id,0), created_at
		FROM agent_memories WHERE agent = ? ORDER BY id DESC LIMIT ?`, agent, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Memory{}

	for rows.Next() {
		var (
			m  Memory
			at string
		)

		if err := rows.Scan(&m.ID, &m.Agent, &m.Kind, &m.Content, &m.TaskID, &m.StepID, &at); err != nil {
			return nil, err
		}

		m.At = atTime(at)
		out = append(out, m)
	}

	return out, rows.Err()
}

// HowMuchRemembered is how many memories each agent has, for the registry.
func (d *DB) HowMuchRemembered() (map[string]int, error) {
	rows, err := d.sql().Query(`SELECT agent, COUNT(*) FROM agent_memories GROUP BY agent`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := map[string]int{}

	for rows.Next() {
		var (
			agent string
			n     int
		)

		if err := rows.Scan(&agent, &n); err != nil {
			return nil, err
		}

		out[agent] = n
	}

	return out, rows.Err()
}

/*
 * Recall is what an agent remembers that bears on a piece of work, best first.
 *
 * By the words the two share, in Go — the same reason the job search is:
 * SQLite does not know Bulgarian — and only memories sharing a word worth
 * sharing. Lessons before findings at the same score, because what went wrong
 * last time is the thing most worth being told before trying again.
 *
 * No embedding. An agent's memory is tens of rows, the step it is recalled for
 * is one sentence, and a model call on this machine to rank them would cost
 * more than the step it was meant to help.
 */
func (d *DB) Recall(agent, work string, most int) ([]Memory, error) {
	all, err := d.Memories(agent, 200)
	if err != nil {
		return nil, err
	}

	wanted := significant(work)

	type scored struct {
		m     Memory
		score int
	}

	found := []scored{}

	for _, m := range all {
		score := 0

		for word := range significant(m.Content) {
			if wanted[word] {
				score += 2
			}
		}

		if score == 0 {
			continue
		}

		if m.Kind == Learned {
			score++
		}

		found = append(found, scored{m: m, score: score})
	}

	// Stable, so equal scores keep newest first.
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && found[j].score > found[j-1].score; j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}

	out := []Memory{}

	for _, s := range found {
		if len(out) >= most {
			break
		}

		out = append(out, s.m)
	}

	return out, nil
}

// significant is the words of a text long enough to mean something, folded,
// by their first five letters so "queries" meets "query".
func significant(text string) map[string]bool {
	out := map[string]bool{}

	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		r := []rune(w)

		if len(r) < 4 {
			continue
		}

		if len(r) > 5 {
			r = r[:5]
		}

		out[string(r)] = true
	}

	return out
}
