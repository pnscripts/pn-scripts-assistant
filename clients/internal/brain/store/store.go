// Package store is the brain's memory on disk.
//
// It is SQLite, opened through a pure-Go driver so the finished binary needs no
// C toolchain, no system libraries and no database server. That last point is
// the whole reason this package exists: everything the brain knows lives in one
// SQLite file inside its own folder, opened directly by the program. There is
// no database server, no container, and nothing to start first — a personal
// assistant that cannot begin without another stack running is not one.
//
// Vectors are kept as raw float32 blobs rather than a vector extension.
// Similarity is then computed in Go over the rows. That is a linear scan, and
// it is the right trade at this size: 79 facts is 240KB of vectors and the scan
// costs well under a millisecond, while an index would add a dependency, a
// build step and a failure mode. See Search for where that stops being true.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

/*
 * DB wraps the connection with the brain's own queries.
 *
 * The pool is held behind a pointer that can be swapped rather than as a plain
 * field, so the database can be pointed at the file that is there now without
 * restarting the program. That is not a nicety: this brain lives on a drive
 * that gets unplugged, and an open SQLite connection keeps writing to the
 * inode it opened, not to the path. When the drive comes back the file at the
 * path is a different object from the one the connection is holding, and
 * carrying on means half the writes go to a file nobody will ever read again
 * while the other half go to the real one. See Reopen.
 */
type DB struct {
	pool atomic.Pointer[sql.DB]

	// Where it was opened from, so it can be opened again.
	path string
}

// sql hands over the pool in use at this moment.
func (d *DB) sql() *sql.DB { return d.pool.Load() }

// Open prepares the database at path, creating it and its schema if absent.
//
// The pragmas are not decoration. WAL lets the learning worker write while a
// conversation reads, which with the default journal would block. busy_timeout
// turns the remaining contention into a short wait instead of an immediate
// "database is locked" error, which is the single most common way a
// multi-goroutine SQLite program fails in production.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	handle, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	if err := handle.Ping(); err != nil {
		handle.Close()
		return nil, fmt.Errorf("reaching %s: %w", path, err)
	}

	db := &DB{path: path}
	db.pool.Store(handle)

	if err := db.migrate(); err != nil {
		handle.Close()
		return nil, err
	}

	return db, nil
}

/*
 * Reopen points the database at whatever is at the path now.
 *
 * For one situation: the drive was unplugged and plugged back in. Until this
 * existed the program said the drive was back and carried on with the
 * connection it already had — which refers to the file on the disk that left,
 * by inode. Every write on it went to an object with no name, and any
 * connection the pool opened afterwards went to the real file, so the brain
 * held two databases at once and disagreed with itself about what it knew.
 *
 * The old pool is closed rather than abandoned; queries in flight on it finish
 * against the file they started with, which is the best available answer for
 * work that began before the disk moved.
 */
func (d *DB) Reopen() error {
	fresh, err := Open(d.path)
	if err != nil {
		return err
	}

	old := d.pool.Swap(fresh.pool.Load())

	if old != nil {
		old.Close()
	}

	return nil
}

func (d *DB) Close() error { return d.sql().Close() }

// SQL exposes the handle for the few places that genuinely need it, such as
// tests that want to assert on raw rows.
func (d *DB) SQL() *sql.DB { return d.sql() }

// migrate brings the schema up to date.
//
// Migrations are a numbered list applied in order, with the high-water mark
// recorded in the database itself. Laravel's migration table did the same job;
// this keeps the idea and drops the framework. Each entry must be safe to run
// against a database that has already had every earlier entry applied, because
// that is the only order it will ever see.
func (d *DB) migrate() error {
	if _, err := d.sql().Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("creating schema_version: %w", err)
	}

	var current int
	err := d.sql().QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	for i, stmt := range migrations {
		version := i + 1

		if version <= current {
			continue
		}

		tx, err := d.sql().Begin()
		if err != nil {
			return fmt.Errorf("starting migration %d: %w", version, err)
		}

		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", version, err)
		}

		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", version, err)
		}
	}

	return nil
}

// migrations is append-only. Editing an existing entry changes nothing on a
// database that has already applied it, so a correction has to arrive as a new
// entry rather than an edit to an old one.
var migrations = []string{
	// 1: the tables everything else is built on.
	//
	// Timestamps are TEXT in RFC3339 rather than SQLite's numeric time, because
	// they are read by humans in exports and compared as strings in queries,
	// and RFC3339 sorts correctly as text.
	//
	// embedding is BLOB: little-endian float32, one after another. Dimension is
	// stored alongside so a change of embedding model is detectable rather than
	// silently producing nonsense similarity scores.
	`
	CREATE TABLE conversations (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		title      TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE TABLE messages (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		role            TEXT NOT NULL,
		provider        TEXT,
		model           TEXT,
		content         TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL
	);
	CREATE INDEX idx_messages_conversation ON messages(conversation_id, id);

	CREATE TABLE lessons (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER REFERENCES conversations(id) ON DELETE SET NULL,
		content         TEXT NOT NULL,
		status          TEXT NOT NULL DEFAULT 'proposed',
		confidence      TEXT,
		source          TEXT,
		embedding       BLOB,
		dimensions      INTEGER,
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL
	);
	CREATE INDEX idx_lessons_status ON lessons(status, id);

	CREATE TABLE knowledge_facts (
		id                      INTEGER PRIMARY KEY AUTOINCREMENT,
		promoted_from_lesson_id INTEGER,
		category                TEXT,
		content                 TEXT NOT NULL,
		embedding               BLOB,
		dimensions              INTEGER,
		created_at              TEXT NOT NULL,
		updated_at              TEXT NOT NULL
	);
	CREATE INDEX idx_facts_category ON knowledge_facts(category);

	CREATE TABLE tool_invocations (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		tool         TEXT NOT NULL,
		arguments    TEXT NOT NULL,
		summary      TEXT,
		risk         TEXT NOT NULL,
		status       TEXT NOT NULL DEFAULT 'pending',
		result       TEXT,
		decided_at   TEXT,
		created_at   TEXT NOT NULL,
		updated_at   TEXT NOT NULL
	);
	CREATE INDEX idx_invocations_status ON tool_invocations(status, id);
	`,

	// 2: things the brain has to be somewhere for.
	//
	// Kept here rather than in a calendar service because the promise is that
	// nothing leaves this machine, and an appointment is exactly the kind of
	// thing that quietly would. at is RFC3339 like every other timestamp, so it
	// sorts as text and reads correctly in an export.
	//
	// said_at, not a boolean: knowing when somebody was told is the difference
	// between a reminder that fires once and one that repeats itself forever
	// after a restart.
	`
	CREATE TABLE reminders (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		what       TEXT NOT NULL,
		at         TEXT NOT NULL,
		said_at    TEXT,
		created_at TEXT NOT NULL
	);
	CREATE INDEX idx_reminders_at ON reminders(said_at, at);
	`,

	// 3: what each model was measured doing on this machine.
	//
	// Kept because the measurements are about this processor and nowhere else:
	// they are minutes of work to produce, they are the only honest basis for
	// choosing a model, and they were previously held in the page's memory —
	// so every reload threw them away and the list said "not tested here yet"
	// about models that had been tested at length.
	`
	CREATE TABLE model_tests (
		name       TEXT PRIMARY KEY,
		seconds    REAL NOT NULL,
		tool_call  TEXT NOT NULL,
		note       TEXT,
		tested_at  TEXT NOT NULL
	);
	`,
}

/*
 * Snapshot writes a complete copy of the database to path.
 *
 * VACUUM INTO rather than copying the file: the brain is running while this
 * happens — the learning worker writes, conversations are saved — and copying
 * bytes out from under an open SQLite database produces a file that is
 * plausible, opens without complaint, and is corrupt in the middle where the
 * two halves came from different moments. VACUUM INTO takes a consistent
 * snapshot of one transaction's view, and compacts it on the way out.
 *
 * The destination must not exist; SQLite refuses rather than overwriting,
 * which is the behaviour worth having when the thing at risk is a backup.
 */
func (d *DB) Snapshot(ctx context.Context, path string) error {
	if _, err := d.sql().ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("copying the database to %s: %w", path, err)
	}

	return nil
}
