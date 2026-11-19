package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * Sealing a brain that was written in the clear, in place (ADR 0008).
 *
 * Power is cut here often, so this is built to be stopped at any moment:
 *
 *  1. A sealed copy of the whole file is written beside it and read back
 *     until it matches byte for byte. Only then is anything changed.
 *  2. Rows are sealed a few hundred at a time, each batch one transaction.
 *     A cut leaves every row either as it was or sealed, never half; a value
 *     is told sealed by looking at it, so starting again simply carries on.
 *  3. Once every row is sealed the file is rebuilt (VACUUM) and the journal
 *     emptied, so the plain text the rows held before is not left behind in
 *     free pages. VACUUM is itself all-or-nothing.
 *
 * Progress is written down in vault_state, a table of its own, which is also
 * how a later start knows which key the brain was sealed with.
 */

// sealedColumns are the personal columns: text first, then blobs.
var sealedColumns = []struct {
	table string
	text  []string
	blob  []string
}{
	{"conversations", []string{"title"}, nil},
	{"messages", []string{"content"}, nil},
	{"lessons", []string{"content"}, []string{"embedding"}},
	{"knowledge_facts", []string{"content"}, []string{"embedding"}},
	{"tool_invocations", []string{"arguments", "summary", "result", "program_version", "summary_bg", "asked_by"}, nil},
	{"reminders", []string{"what"}, nil},
	{"tasks", []string{"name", "goal", "done_when", "report"}, nil},
	{"task_steps", []string{"instruction", "done_when", "answer", "evidence"}, nil},
	{"goals", []string{"name", "why"}, nil},
	{"events", []string{"title", "place", "notes"}, nil},
	{"agent_memories", []string{"content"}, nil},
	{"proposals", []string{"request", "body", "outcome"}, nil},
	{"evidence", []string{"subject", "detail"}, nil},
	{"activity", []string{"said", "data"}, nil},
	{"message_sources", []string{"sources"}, nil},
	{"deeds", []string{"summary", "asked", "request", "result", "leaves", "summary_bg", "asked_by"}, nil},
	{"mail_drafts", []string{"to_addr", "subject", "body", "answers", "warning", "note"}, nil},
	{"todos", []string{"list", "text"}, nil},
	{"study_topics", []string{"topic", "lesson"}, nil},
	{"study_cards", []string{"question", "options", "answer", "explain"}, nil},
	{"research_reports", []string{"question", "report", "sources", "claims"}, nil},
	{"care_answers", []string{"question", "answer"}, nil},
	{"care_profile", []string{"value"}, nil},
	{"invoices", []string{"number", "party", "party_vat", "net", "vat", "gross", "note"}, nil},
	{"books_settings", []string{"value"}, nil},
	{"rules", []string{"text", "words", "source"}, nil},
	{"programs", []string{"ident", "name", "version", "path", "exec", "previous_version"}, nil},
	{"program_permits", []string{"version"}, nil},
}

// SealBatch is how many rows one transaction seals.
const SealBatch = 200

// afterSealBatch is called after each batch commits: for the test that cuts
// the power part way.
var afterSealBatch func(batches int)

// BackupBeforeSealing is where the sealed copy taken first is kept.
func BackupBeforeSealing(path string) string { return path + ".before-sealing.pnsealed" }

func (d *DB) state(key string) string {
	var v string

	d.sql().QueryRow(`SELECT v FROM vault_state WHERE k = ?`, key).Scan(&v)

	return v
}

func (d *DB) setState(key, value string) error {
	_, err := d.sql().Exec(`INSERT INTO vault_state (k, v) VALUES (?, ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, key, value)

	return err
}

// SealProgress is what sealing in place did.
type SealProgress struct {
	Rows      int
	BackedUp  bool
	AlreadyOK bool

	// PutRight is how many records an older version had left unreadable and
	// this start sealed properly (see putRightOnce).
	PutRight int
}

// lookalikesPutRight names the one-time tidy of putRightOnce in chores.
const lookalikesPutRight = "sealed-lookalikes-put-right"

/*
 * SealInPlace seals every personal value still in the clear, with a verified
 * sealed copy taken first. Safe to call on every start: on a sealed brain it
 * checks the key and returns.
 */
func (d *DB) SealInPlace(ctx context.Context) (SealProgress, error) {
	var p SealProgress

	if d.key == nil {
		return p, errors.New("there is no key to seal with")
	}

	if _, err := d.sql().Exec(`CREATE TABLE IF NOT EXISTS vault_state (k TEXT PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		return p, err
	}

	if id := d.state("key_id"); id != "" && id != d.key.ID() {
		return p, fmt.Errorf("this brain was sealed with key %s, not %s: %w", id, d.key.ID(), vault.ErrWrongKey)
	}

	if d.state("sealed") == "done" {
		p.AlreadyOK = true

		n, err := d.putRightOnce(ctx)
		p.PutRight = n

		return p, err
	}

	if err := d.setState("key_id", d.key.ID()); err != nil {
		return p, err
	}

	// 1. The copy, proved before anything is touched.
	if d.state("backup") != "verified" {
		if _, err := d.sql().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return p, fmt.Errorf("settling the journal before the copy: %w", err)
		}

		if err := vault.SealedCopy(d.key, d.path, BackupBeforeSealing(d.path)); err != nil {
			return p, fmt.Errorf("the copy taken before sealing: %w", err)
		}

		if err := d.setState("backup", "verified"); err != nil {
			return p, err
		}

		p.BackedUp = true
	}

	// 2. The rows, a batch at a time.
	n, err := d.sealRows(ctx)
	p.Rows = n

	if err != nil {
		return p, err
	}

	// 3. Nothing of the old text left in free pages or the journal.
	if err := d.setState("sealed", "rows"); err != nil {
		return p, err
	}

	if _, err := d.sql().Exec(`VACUUM`); err != nil {
		return p, fmt.Errorf("rebuilding the file without the plain text: %w", err)
	}

	if _, err := d.sql().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return p, err
	}

	if err := d.setState("sealed", "done"); err != nil {
		return p, err
	}

	// Every row was just looked at: nothing is left for putRightOnce.
	return p, d.choreDone(lookalikesPutRight)
}

// sealRows goes through every personal column and seals what is not, a batch
// at a time; it returns how many rows it changed.
func (d *DB) sealRows(ctx context.Context) (int, error) {
	changed, batches := 0, 0

	for _, t := range sealedColumns {
		cols := d.existing(t.table, append(append([]string{}, t.text...), t.blob...))
		if len(cols) == 0 {
			continue
		}

		blob := map[string]bool{}
		for _, b := range t.blob {
			blob[b] = true
		}

		after := int64(0)

		for {
			if err := ctx.Err(); err != nil {
				return changed, err
			}

			n, last, err := d.sealBatch(t.table, cols, blob, after)
			if err != nil {
				return changed, fmt.Errorf("sealing %s: %w", t.table, err)
			}

			changed += n

			if last == 0 {
				break
			}

			after = last
			batches++

			if afterSealBatch != nil {
				afterSealBatch(batches)
			}
		}
	}

	return changed, nil
}

/*
 * putRightOnce seals what an older version left unreadable in a sealed brain.
 *
 * Until 1.2.0 text that began with the mark of sealed text was taken for
 * sealed and written as it came: a message, a page's words, a mail's
 * subject. It could not be opened, and no query that met it could be
 * answered. Such text is not shaped like anything this program seals
 * (vault.ShapedString), which is how it is told from a sealed value somebody
 * changed: that one is left alone and goes on failing the read, as it
 * should. Text built to have the full shape stays unreadable for the same
 * reason; nothing can tell it from a damaged value.
 *
 * Once per brain, by name in chores. Each batch is a transaction and a row
 * put right is not touched again, so a power cut part way costs nothing: the
 * next start carries on and then writes the name down.
 */
func (d *DB) putRightOnce(ctx context.Context) (int, error) {
	var done int

	if err := d.sql().QueryRow(`SELECT COUNT(*) FROM chores WHERE name = ?`, lookalikesPutRight).Scan(&done); err != nil {
		return 0, fmt.Errorf("asking whether %s was done: %w", lookalikesPutRight, err)
	}

	if done > 0 {
		return 0, nil
	}

	n, err := d.sealRows(ctx)
	if err != nil {
		return n, err
	}

	// What was put right lay in the clear until now: not in the journal
	// either, from here on.
	if n > 0 {
		if _, err := d.sql().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return n, err
		}
	}

	return n, d.choreDone(lookalikesPutRight)
}

func (d *DB) choreDone(name string) error {
	_, err := d.sql().Exec(`INSERT OR IGNORE INTO chores (name, done_at) VALUES (?, ?)`,
		name, time.Now().UTC().Format(time.RFC3339))

	return err
}

// existing is which of the columns a table has: an older brain may predate
// some of them, and a table may not exist at all yet.
func (d *DB) existing(table string, want []string) []string {
	rows, err := d.sql().Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil
	}
	defer rows.Close()

	have := map[string]bool{}

	for rows.Next() {
		var name string

		if rows.Scan(&name) == nil {
			have[name] = true
		}
	}

	var out []string

	for _, c := range want {
		if have[c] {
			out = append(out, c)
		}
	}

	return out
}

// sealBatch seals one batch after rowid after, and returns how many rows it
// changed and the last rowid it looked at (0 when there were none).
func (d *DB) sealBatch(table string, cols []string, blob map[string]bool, after int64) (int, int64, error) {
	tx, err := d.sql().Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	/*
	 * Read raw, not through the connection's opening (sealed.go): hex() hands
	 * the stored bytes back as hex, which nothing mistakes for a sealed value,
	 * so what is already sealed is seen to be and left alone.
	 */
	selects := make([]string, len(cols))
	for i, c := range cols {
		selects[i] = "(" + c + " IS NULL), hex(" + c + ")"
	}

	rows, err := tx.Query(`SELECT rowid, `+strings.Join(selects, ", ")+` FROM `+table+
		` WHERE rowid > ? ORDER BY rowid LIMIT ?`, after, SealBatch)
	if err != nil {
		return 0, 0, err
	}

	type change struct {
		rowid  int64
		values []any
	}

	var (
		changes []change
		last    int64
	)

	for rows.Next() {
		dest := make([]any, 2*len(cols)+1)
		null := make([]bool, len(cols))
		hexed := make([]string, len(cols))

		dest[0] = &last
		for i := range cols {
			dest[1+2*i] = &null[i]
			dest[2+2*i] = &hexed[i]
		}

		if err := rows.Scan(dest...); err != nil {
			rows.Close()

			return 0, 0, err
		}

		values := make([]any, len(cols))
		changed := false

		for i, c := range cols {
			if null[i] {
				values[i] = nil

				continue
			}

			v, err := hex.DecodeString(hexed[i])
			if err != nil {
				rows.Close()

				return 0, 0, err
			}

			/*
			 * Every value goes back exactly as given here (storedText,
			 * storedBlob). What has the shape of a sealed value is left as
			 * it is, which is what lets this carry on after a power cut.
			 * Anything else is somebody's text or bytes: when it begins with
			 * the sealed mark all the same, an older version wrote it as it
			 * came, and it is kept apart before it is sealed, as it would be
			 * written today.
			 */
			if blob[c] {
				values[i] = storedBlob(v)

				if len(v) > 0 && !vault.Shaped(v) {
					if vault.IsSealed(v) {
						v = vault.KeepBytes(v)
					}

					values[i] = storedBlob(d.key.Seal(v))
					changed = true
				}

				continue
			}

			text := string(v)
			values[i] = storedText(text)

			if len(v) > 0 && !vault.ShapedString(text) {
				if vault.IsSealedString(text) {
					text = vault.Keep(text)
				}

				values[i] = storedText(d.key.SealString(text))
				changed = true
			}
		}

		if changed {
			changes = append(changes, change{rowid: last, values: values})
		}
	}

	if err := rows.Close(); err != nil {
		return 0, 0, err
	}

	sets := make([]string, len(cols))
	for i, c := range cols {
		sets[i] = c + " = ?"
	}

	update := `UPDATE ` + table + ` SET ` + strings.Join(sets, ", ") + ` WHERE rowid = ?`

	for _, ch := range changes {
		if _, err := tx.Exec(update, append(ch.values, ch.rowid)...); err != nil {
			return 0, 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}

	return len(changes), last, nil
}

/*
 * SealedWith is the key a brain's database was sealed with, read without
 * opening anything: empty for a brain never sealed, or no database at all.
 * It is how a missing vault.json is told from a brain that never had one.
 */
func SealedWith(path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return ""
	}
	defer db.Close()

	var id string

	db.QueryRow(`SELECT v FROM vault_state WHERE k = 'key_id'`).Scan(&id)

	return id
}
