package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ImportReport records what a migration actually moved, per table.
type ImportReport struct {
	Conversations int
	Messages      int
	Lessons       int
	Facts         int
	Invocations   int
	Skipped       []string
}

// ImportPostgresExport loads the JSON produced from the old Postgres database.
//
// The export is one file per table of `json_agg(row_to_json(...))`, so every
// value arrives as JSON and the pgvector columns arrive as their text form —
// "[0.41,0.11,...]". Parsing that back into float32 is the only real work here.
//
// Existing ids are preserved. The brain map, recall results and the interface
// all refer to facts by id, so renumbering would quietly invalidate anything
// that had already been recorded about them.
//
// Importing into a non-empty table is refused rather than merged. A merge would
// have to decide what to do about colliding ids, and every answer to that is a
// silent way to lose data; refusing makes the operator choose.
func (d *DB) ImportPostgresExport(dir string) (ImportReport, error) {
	var rep ImportReport

	for _, t := range []string{"conversations", "messages", "lessons", "knowledge_facts", "tool_invocations"} {
		var n int

		if err := d.sql().QueryRow(`SELECT COUNT(*) FROM ` + t).Scan(&n); err != nil {
			return rep, fmt.Errorf("checking %s: %w", t, err)
		}

		if n > 0 {
			return rep, fmt.Errorf("%s already holds %d rows; import expects an empty database", t, n)
		}
	}

	tx, err := d.sql().Begin()
	if err != nil {
		return rep, err
	}
	defer tx.Rollback()

	// Order matters: messages and lessons reference conversations.
	rows, err := readTable(dir, "conversations")
	if err != nil {
		return rep, err
	}

	for _, r := range rows {
		_, err := tx.Exec(
			`INSERT INTO conversations (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			num(r["id"]), text(r["title"]), stamp(r["created_at"]), stamp(r["updated_at"]),
		)
		if err != nil {
			return rep, fmt.Errorf("conversation %v: %w", r["id"], err)
		}

		rep.Conversations++
	}

	if rows, err = readTable(dir, "messages"); err != nil {
		return rep, err
	}

	for _, r := range rows {
		_, err := tx.Exec(
			`INSERT INTO messages (id, conversation_id, role, provider, model, content, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			num(r["id"]), num(r["conversation_id"]), text(r["role"]), text(r["provider"]),
			text(r["model"]), text(r["content"]), stamp(r["created_at"]), stamp(r["updated_at"]),
		)
		if err != nil {
			return rep, fmt.Errorf("message %v: %w", r["id"], err)
		}

		rep.Messages++
	}

	if rows, err = readTable(dir, "lessons"); err != nil {
		return rep, err
	}

	for _, r := range rows {
		vec, dims := vector(r["embedding"])

		_, err := tx.Exec(
			`INSERT INTO lessons (id, conversation_id, content, status, confidence, source, embedding, dimensions, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			num(r["id"]), num(r["conversation_id"]), text(r["content"]), text(r["status"]),
			text(r["confidence"]), text(r["source"]), vec, dims,
			stamp(r["created_at"]), stamp(r["updated_at"]),
		)
		if err != nil {
			return rep, fmt.Errorf("lesson %v: %w", r["id"], err)
		}

		rep.Lessons++
	}

	if rows, err = readTable(dir, "knowledge_facts"); err != nil {
		return rep, err
	}

	for _, r := range rows {
		vec, dims := vector(r["embedding"])

		if vec == nil {
			rep.Skipped = append(rep.Skipped, fmt.Sprintf("fact %v had no usable embedding", r["id"]))
		}

		_, err := tx.Exec(
			`INSERT INTO knowledge_facts (id, promoted_from_lesson_id, category, content, embedding, dimensions, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			num(r["id"]), num(r["promoted_from_lesson_id"]), text(r["category"]), text(r["content"]),
			vec, dims, stamp(r["created_at"]), stamp(r["updated_at"]),
		)
		if err != nil {
			return rep, fmt.Errorf("fact %v: %w", r["id"], err)
		}

		rep.Facts++
	}

	// Optional: this table did not exist in every version of the old schema.
	if rows, err = readTable(dir, "tool_invocations"); err == nil {
		for _, r := range rows {
			_, err := tx.Exec(
				`INSERT INTO tool_invocations (id, tool, arguments, summary, risk, status, result, decided_at, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				num(r["id"]), text(r["tool"]), text(r["arguments"]), text(r["summary"]),
				orDefault(text(r["risk"]), "safe"), orDefault(text(r["status"]), "pending"),
				text(r["result"]), stamp(r["decided_at"]),
				stamp(r["created_at"]), stamp(r["updated_at"]),
			)
			if err != nil {
				return rep, fmt.Errorf("invocation %v: %w", r["id"], err)
			}

			rep.Invocations++
		}
	}

	return rep, tx.Commit()
}

func readTable(dir, name string) ([]map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, fmt.Errorf("reading %s export: %w", name, err)
	}

	var rows []map[string]any

	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parsing %s export: %w", name, err)
	}

	return rows, nil
}

// vector parses pgvector's text form into the stored blob.
//
// Returns nil for anything unparseable rather than a zero vector: a zero vector
// scores 0 against everything, which looks like "unrelated" instead of like
// "missing", and would hide the loss.
func vector(v any) (any, any) {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil, nil
	}

	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")

	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))

	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil, nil
		}

		out = append(out, float32(f))
	}

	return EncodeVector(out), len(out)
}

func text(v any) any {
	if v == nil {
		return nil
	}

	if s, ok := v.(string); ok {
		return s
	}

	return fmt.Sprint(v)
}

func num(v any) any {
	switch n := v.(type) {
	case nil:
		return nil
	case float64:
		return int64(n)
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return nil
		}

		return i
	default:
		return nil
	}
}

// stamp normalises Postgres timestamps to the RFC3339 text this schema stores.
func stamp(v any) any {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}

	for _, layout := range []string{
		time.RFC3339Nano, time.RFC3339,
		"2006-01-02T15:04:05.999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}

	return s
}

func orDefault(v any, fallback string) any {
	if s, ok := v.(string); ok && s != "" {
		return s
	}

	return fallback
}
