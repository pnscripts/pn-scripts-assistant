package store

import (
	"bytes"
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
	// this start made readable again (see putRightOnce).
	PutRight int

	// LeftAsFound names the tables that tidy could not go through, each
	// with why. They are tried again at the next start.
	LeftAsFound []string
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
		var err error

		p.AlreadyOK = true

		p.PutRight, p.LeftAsFound, err = d.putRightOnce(ctx)

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

	// And such text in the columns that are not sealed.
	if p.PutRight, p.LeftAsFound, err = d.putRight(ctx); err != nil {
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

	// Every table was just looked at: nothing is left for putRightOnce,
	// unless one could not be gone through.
	if len(p.LeftAsFound) > 0 {
		return p, nil
	}

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
 * putRightOnce makes readable what an older version left unreadable in a
 * sealed brain.
 *
 * Until 1.2.0 text that began with the mark of sealed text was taken for
 * sealed and written as it came: a message, a page's words, a mail's
 * subject, the name of a model. It could not be opened, and no query that
 * met it could be answered. See putRight for what is done about it.
 *
 * Once per brain, by name in chores. A table is one transaction and a row
 * put right is not found again, so a power cut part way costs nothing: the
 * next start does it from the beginning and then writes the name down. The
 * journal is emptied before that, whatever was found, so the text that lay
 * in the clear is not left there by a start that was cut off after its work.
 *
 * A table it cannot go through is named and left, and the name of the tidy
 * is then not written down: the next start tries again. The program starts
 * all the same; a tidy is not worth a brain that will not open.
 */
func (d *DB) putRightOnce(ctx context.Context) (int, []string, error) {
	var done int

	if err := d.sql().QueryRow(`SELECT COUNT(*) FROM chores WHERE name = ?`, lookalikesPutRight).Scan(&done); err != nil {
		return 0, nil, fmt.Errorf("asking whether %s was done: %w", lookalikesPutRight, err)
	}

	if done > 0 {
		return 0, nil, nil
	}

	n, left, err := d.putRight(ctx)
	if err != nil {
		return n, left, err
	}

	if _, err := d.sql().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return n, left, err
	}

	if len(left) > 0 {
		return n, left, nil
	}

	return n, left, d.choreDone(lookalikesPutRight)
}

/*
 * putRight looks through every column of every table for text that begins
 * with the sealed mark and was stored as it came, and stores it as it would
 * be stored today. It returns how many rows it changed.
 *
 * In a sealed column that is text with the mark and not the rest of what
 * sealing writes (vault.BeginsSealed): it is kept apart and sealed. A value
 * that does begin like a sealed one is left alone, whether it opens or not:
 * changed, cut short or built to look the part, nothing here can tell which,
 * and a damaged value has to go on failing the read.
 *
 * In any other column nothing is ever sealed, so whatever begins with the
 * mark there is text and is kept apart, a value sealed with this brain's own
 * key included: left where it does not belong, it would be opened for
 * whoever put it there.
 *
 * The rows are found in SQL, by the marks written into the query itself, so
 * a brain of any size costs one pass inside SQLite and only the rows
 * concerned come out of it, read as hex, which nothing takes for a sealed
 * value.
 */
func (d *DB) putRight(ctx context.Context) (int, []string, error) {
	sealed := map[string]map[string]bool{}

	for _, t := range sealedColumns {
		sealed[t.table] = map[string]bool{}

		for _, c := range append(append([]string{}, t.text...), t.blob...) {
			sealed[t.table][c] = true
		}
	}

	tables, err := d.names(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return 0, nil, err
	}

	var (
		changed int
		left    []string
	)

	for _, table := range tables {
		if err := ctx.Err(); err != nil {
			return changed, left, err
		}

		n, err := d.putRightIn(table, sealed[table])
		if err != nil {
			left = append(left, table+": "+err.Error())

			continue
		}

		changed += n
	}

	return changed, left, nil
}

func (d *DB) names(query string, args ...any) ([]string, error) {
	rows, err := d.sql().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string

	for rows.Next() {
		var name string

		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		out = append(out, name)
	}

	return out, rows.Err()
}

// The marks as SQL sees them.
var (
	markInSQL  = "'" + vault.Prefix + "'"
	magicInSQL = "'" + strings.ToUpper(hex.EncodeToString(vault.Seal0())) + "'"
)

func (d *DB) putRightIn(table string, sealed map[string]bool) (int, error) {
	cols, err := d.names(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil || len(cols) == 0 {
		return 0, err
	}

	quoted := func(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

	var selects, wheres []string

	for _, c := range cols {
		q := quoted(c)
		selects = append(selects, "typeof("+q+"), hex("+q+")")

		text := "typeof(" + q + ") = 'text' AND substr(" + q + ", 1, " + fmt.Sprint(len(vault.Prefix)) + ") = " + markInSQL

		if sealed[c] {
			// Nearly everything in a sealed column begins with the mark:
			// only what does not go on as sealing writes is asked for.
			text += " AND substr(" + q + ", 1, " + fmt.Sprint(len(vault.SealedHead())) + ") <> '" + vault.SealedHead() + "'"
		}

		wheres = append(wheres, "("+text+")")

		if !sealed[c] {
			wheres = append(wheres, "(typeof("+q+") = 'blob' AND (hex(substr("+q+", 1, "+fmt.Sprint(len(vault.Seal0()))+")) = "+magicInSQL+
				" OR hex(substr("+q+", 1, "+fmt.Sprint(len(vault.Prefix))+")) = '"+strings.ToUpper(hex.EncodeToString([]byte(vault.Prefix)))+"'))")
		}
	}

	tx, err := d.sql().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT rowid, ` + strings.Join(selects, ", ") + ` FROM ` + quoted(table) +
		` WHERE ` + strings.Join(wheres, " OR "))
	if err != nil {
		return 0, err
	}

	type change struct {
		rowid  int64
		column string
		value  any
	}

	var (
		changes []change
		touched = map[int64]bool{}
	)

	for rows.Next() {
		var rowid int64

		kind := make([]string, len(cols))
		hexed := make([]string, len(cols))
		dest := []any{&rowid}

		for i := range cols {
			dest = append(dest, &kind[i], &hexed[i])
		}

		if err := rows.Scan(dest...); err != nil {
			rows.Close()

			return 0, err
		}

		for i, c := range cols {
			if kind[i] != "text" && kind[i] != "blob" {
				continue
			}

			v, err := hex.DecodeString(hexed[i])
			if err != nil {
				rows.Close()

				return 0, err
			}

			text := string(v)

			switch {
			case kind[i] == "text" && sealed[c]:
				if vault.IsSealedString(text) && !vault.BeginsSealed(text) {
					changes = append(changes, change{rowid, c, storedText(d.key.SealString(vault.Keep(text)))})
				}
			case kind[i] == "text":
				if vault.IsSealedString(text) {
					changes = append(changes, change{rowid, c, storedText(vault.Keep(text))})
				}
			case !sealed[c]:
				if vault.IsSealed(v) || bytes.HasPrefix(v, []byte(vault.Prefix)) {
					changes = append(changes, change{rowid, c, storedBlob(vault.KeepBytes(v))})
				}
			}
		}
	}

	if err := rows.Close(); err != nil {
		return 0, err
	}

	for _, ch := range changes {
		if _, err := tx.Exec(`UPDATE `+quoted(table)+` SET `+quoted(ch.column)+` = ? WHERE rowid = ?`, ch.value, ch.rowid); err != nil {
			return 0, err
		}

		touched[ch.rowid] = true
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return len(touched), nil
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
			 * storedBlob). What begins the way sealing writes is left as
			 * it is, which is what lets this carry on after a power cut.
			 * Anything else is somebody's text or bytes: text that carries
			 * the sealed mark all the same was written as it came by an
			 * older version, and is kept apart before it is sealed, as it
			 * would be written today.
			 */
			if blob[c] {
				values[i] = storedBlob(v)

				if len(v) > 0 && !vault.IsSealed(v) {
					values[i] = storedBlob(d.key.Seal(v))
					changed = true
				}

				continue
			}

			text := string(v)
			values[i] = storedText(text)

			if len(v) > 0 && !vault.BeginsSealed(text) {
				// With the mark here it came from an older version, as it
				// was; text this version wrote with no key is kept apart
				// already and is sealed as it lies.
				if vault.IsSealedString(text) {
					text = vault.Kept + text
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
