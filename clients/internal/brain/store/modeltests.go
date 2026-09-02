package store

import "time"

// ModelTest is what one model was measured doing on this machine.
type ModelTest struct {
	Name     string    `json:"name"`
	Seconds  float64   `json:"seconds"`
	ToolCall string    `json:"tool_call"`
	Note     string    `json:"note"`
	TestedAt time.Time `json:"tested_at"`
}

/*
 * RecordModelTest stores a measurement, replacing any earlier one.
 *
 * Replacing rather than accumulating: what matters is what this model does on
 * this machine now, and a history of that would be a table nobody reads.
 */
func (d *DB) RecordModelTest(t ModelTest) error {
	_, err := d.sql().Exec(
		`INSERT INTO model_tests (name, seconds, tool_call, note, tested_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   seconds = excluded.seconds,
		   tool_call = excluded.tool_call,
		   note = excluded.note,
		   tested_at = excluded.tested_at`,
		t.Name, t.Seconds, t.ToolCall, t.Note,
		time.Now().UTC().Format(time.RFC3339))

	return err
}

// ModelTests returns every measurement, by model name.
func (d *DB) ModelTests() (map[string]ModelTest, error) {
	rows, err := d.sql().Query(
		`SELECT name, seconds, tool_call, COALESCE(note, ''), tested_at FROM model_tests`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := map[string]ModelTest{}

	for rows.Next() {
		var (
			t  ModelTest
			at string
		)

		if err := rows.Scan(&t.Name, &t.Seconds, &t.ToolCall, &t.Note, &at); err != nil {
			return nil, err
		}

		t.TestedAt, _ = time.Parse(time.RFC3339, at)
		out[t.Name] = t
	}

	return out, rows.Err()
}
