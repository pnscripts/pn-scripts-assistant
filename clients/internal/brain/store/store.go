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
		// 0700: the folder holds the database, and the database is the whole
		// of what this program knows about somebody.
		if err := os.MkdirAll(dir, 0o700); err != nil {
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

	ours(path)

	return db, nil
}

/*
 * ours makes the database readable by its owner and nobody else.
 *
 * SQLite creates it with whatever the umask allows, which on Ubuntu is 0644 —
 * so on a shared machine every other account could read every conversation,
 * every memory and every piece of evidence. The settings beside it have been
 * 0600 since the first week, and the settings are the less revealing file of
 * the two.
 *
 * The journal and shared-memory files as well: in WAL mode the most recent
 * writes are in the -wal file and nowhere else, so leaving that one open
 * would leave the newest conversation open.
 *
 * Quietly, and not as an error: a database on a drive that cannot hold
 * permissions — a FAT stick, somebody's NAS — still works, and refusing to
 * start over it would be the wrong trade.
 */
func ours(path string) {
	for _, at := range []string{path, path + "-wal", path + "-shm"} {
		if info, err := os.Stat(at); err == nil && info.Mode().Perm()&0o077 != 0 {
			os.Chmod(at, 0o600)
		}
	}
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

	/*
	 * 4: work that outlives the sentence that asked for it.
	 *
	 * A conversation records what was said. A task records what is being done,
	 * and the difference that matters is that a task has to survive the
	 * program being closed halfway through it. So everything a task needs to
	 * pick itself up — which step it is on, how much budget is left, what it
	 * is waiting on — is a column here rather than a field in memory. A budget
	 * held in memory refills itself on restart, and a task that resumes with a
	 * full purse has no budget at all.
	 *
	 * work_conversation_id is the task's own thread. The agent writes tool
	 * output into whichever conversation it is handed, and a task with ten
	 * steps would otherwise put ten steps of it in front of the owner's next
	 * question — where it is read again on every round, on a machine where
	 * that reading is most of the wait.
	 *
	 * None of the added columns is NOT NULL because ALTER TABLE ADD COLUMN is
	 * the only form SQLite will do without rewriting the table, and it will not
	 * do that one with a NULL default.
	 */
	`
	CREATE TABLE tasks (
		id                   INTEGER PRIMARY KEY AUTOINCREMENT,
		name                 TEXT NOT NULL,
		goal                 TEXT NOT NULL,
		done_when            TEXT NOT NULL DEFAULT '',
		state                TEXT NOT NULL DEFAULT 'planning',
		conversation_id      INTEGER REFERENCES conversations(id) ON DELETE SET NULL,
		work_conversation_id INTEGER REFERENCES conversations(id) ON DELETE SET NULL,
		provider             TEXT NOT NULL DEFAULT '',
		job_id               INTEGER,
		steps_left           INTEGER NOT NULL DEFAULT 0,
		calls_left           INTEGER NOT NULL DEFAULT 0,
		replans_left         INTEGER NOT NULL DEFAULT 0,
		deadline             TEXT,
		report               TEXT,
		blocked_because      TEXT,
		created_at           TEXT NOT NULL,
		updated_at           TEXT NOT NULL,
		finished_at          TEXT
	);
	CREATE INDEX idx_tasks_state ON tasks(state, id);

	CREATE TABLE task_steps (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id       INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		position      INTEGER NOT NULL,
		instruction   TEXT NOT NULL,
		done_when     TEXT NOT NULL DEFAULT '',
		kind          TEXT NOT NULL DEFAULT 'do',
		changes       INTEGER NOT NULL DEFAULT 0,
		assignee      TEXT NOT NULL DEFAULT '',
		provider      TEXT NOT NULL DEFAULT '',
		model         TEXT NOT NULL DEFAULT '',
		state         TEXT NOT NULL DEFAULT 'waiting',
		attempts      INTEGER NOT NULL DEFAULT 0,
		answer        TEXT,
		evidence      TEXT,
		checked_by    TEXT,
		verdict       TEXT,
		why           TEXT,
		started_at    TEXT,
		ended_at      TEXT,
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	);
	CREATE INDEX idx_task_steps_task ON task_steps(task_id, position);

	ALTER TABLE tool_invocations ADD COLUMN task_id INTEGER;
	ALTER TABLE tool_invocations ADD COLUMN step_id INTEGER;
	ALTER TABLE tool_invocations ADD COLUMN conversation_id INTEGER;
	CREATE INDEX idx_invocations_task ON tool_invocations(task_id, status);

	ALTER TABLE conversations ADD COLUMN kind TEXT;
	`,

	/*
	 * 5: what the work is for.
	 *
	 * A task is finite and a goal is not, and the difference is the whole
	 * reason this is a second table rather than a flag on the first. "Go
	 * through my projects and tell me what is broken" finishes. "Keep my
	 * projects building" does not — it comes round again, and the useful
	 * question about it is not whether it is done but when it was last looked
	 * at and what happened that time.
	 *
	 * every_days is how often it is worth coming back to, and zero means never
	 * on its own. starts_itself is off by default and deliberately separate:
	 * a goal that quietly begins work while nobody is at the machine is a
	 * different thing from one that says it is due, and the second should not
	 * become the first by accident.
	 */
	`
	CREATE TABLE goals (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT NOT NULL,
		why           TEXT NOT NULL DEFAULT '',
		state         TEXT NOT NULL DEFAULT 'active',
		every_days    INTEGER NOT NULL DEFAULT 0,
		starts_itself INTEGER NOT NULL DEFAULT 0,
		last_worked   TEXT,
		next_due      TEXT,
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	);
	CREATE INDEX idx_goals_state ON goals(state, id);

	ALTER TABLE tasks ADD COLUMN goal_id INTEGER;
	CREATE INDEX idx_tasks_goal ON tasks(goal_id, id);
	`,

	/*
	 * 6: a diary, kept here.
	 *
	 * A calendar was a deliberate non-goal for a long time, and the reason
	 * stands: it is the kind of thing that quietly ends up on somebody else's
	 * server, and then every appointment somebody has is a row in a company's
	 * database. This one cannot do that. It is a table in the same file as
	 * everything else the brain knows, it travels on the same drive, and the
	 * only way anything leaves is somebody exporting it on purpose.
	 *
	 * Separate from reminders, which are a different thing wearing a similar
	 * hat: a reminder speaks at a moment and is then done with. An event
	 * occupies time, has an end, and is the answer to "am I free on Thursday".
	 *
	 * uid is the iCalendar identity, so a file imported twice updates its
	 * events rather than doubling them.
	 */
	`
	CREATE TABLE events (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		uid        TEXT NOT NULL DEFAULT '',
		title      TEXT NOT NULL,
		starts_at  TEXT NOT NULL,
		ends_at    TEXT NOT NULL,
		all_day    INTEGER NOT NULL DEFAULT 0,
		place      TEXT NOT NULL DEFAULT '',
		notes      TEXT NOT NULL DEFAULT '',
		came_from  TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE INDEX idx_events_when ON events(starts_at, id);
	CREATE UNIQUE INDEX idx_events_uid ON events(uid) WHERE uid <> '';
	`,

	/*
	 * 7: memory that can be wrong, and stop being wrong.
	 *
	 * Until now a fact, once promoted, was permanent and equal. Nothing ever
	 * replaced one, nothing retired one, and recall ranked purely on how
	 * similar the words were — so a document read in March and rewritten in
	 * June left both versions in memory, equally confident, and the brain
	 * would quote whichever happened to be worded more like the question.
	 * "Gets sharper the more you use it" cannot be true of a store that only
	 * ever grows.
	 *
	 * source is copied here at promotion rather than reached through the
	 * lesson it came from. A fact outlives its lesson — deleting a
	 * conversation nulls the link on purpose — and a fact that cannot say
	 * where it came from cannot be retired when that place is gone.
	 *
	 * superseded_by points at what replaced it. Kept rather than deleted,
	 * because "what did it used to think" is a real question, and because a
	 * replacement made on a bad reading should be undoable.
	 *
	 * used_count and last_used are what makes recall able to learn. A fact
	 * that keeps proving relevant is more likely to be relevant again, and
	 * that is the only feedback available without asking somebody to rate
	 * their own assistant.
	 */
	`
	ALTER TABLE knowledge_facts ADD COLUMN source TEXT;
	ALTER TABLE knowledge_facts ADD COLUMN superseded_by INTEGER;
	ALTER TABLE knowledge_facts ADD COLUMN retired_at TEXT;
	ALTER TABLE knowledge_facts ADD COLUMN why_retired TEXT;
	ALTER TABLE knowledge_facts ADD COLUMN used_count INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE knowledge_facts ADD COLUMN last_used TEXT;

	UPDATE knowledge_facts SET source = (
		SELECT source FROM lessons WHERE lessons.id = knowledge_facts.promoted_from_lesson_id
	) WHERE promoted_from_lesson_id IS NOT NULL;

	CREATE INDEX idx_facts_living ON knowledge_facts(retired_at, superseded_by);
	CREATE INDEX idx_facts_source ON knowledge_facts(source);
	`,

	/*
	 * 8: what jobs there are, as against who does them.
	 *
	 * The roster has always been six people in a Go slice, and every one of
	 * them *was* its job: the developer is the thing that writes code, and
	 * there is no separate idea of what writing code involves. That holds
	 * until somebody wants two backend engineers who are not the same person,
	 * or a specialist this program has never heard of, or an answer to "who
	 * here knows PostgreSQL" that is a query rather than a guess.
	 *
	 * So a job is a definition and an agent is somebody who holds one. The
	 * definitions live in rows rather than in files because there are
	 * thousands of them and nobody edits them by hand — which is the split
	 * this program already makes everywhere else, where what happened is a row
	 * and what somebody maintains is a file in the brain's own folder.
	 *
	 * Capabilities are kept apart from jobs on purpose. "API design" belongs
	 * to a backend engineer and to a technical product manager both, and
	 * copying it into each job is how a list of skills becomes a list of
	 * spellings of skills. They are kept apart from tools for a different
	 * reason: a tool is something this program can do, a capability is
	 * something somebody is good at, and most capabilities map to no tool at
	 * all.
	 *
	 * The ids are text and hierarchical — engineering.backend.api_engineer —
	 * because they are written into agent files that people read and edit, and
	 * an integer there would make every one of those files meaningless on its
	 * own. Titles change; ids do not.
	 *
	 * came_from says where a row arrived from, so an import can be run twice,
	 * or a second source imported over the first, without either of them
	 * quietly overwriting something hand-written.
	 *
	 * The last line has nothing to do with jobs and everything to do with them
	 * being asked about. task_steps.assignee has been written on every step
	 * since migration 4 and never indexed, because until now it had six
	 * possible values and the one query that read it read all of them anyway.
	 */
	`
	CREATE TABLE occupations (
		id          TEXT PRIMARY KEY,
		title       TEXT NOT NULL,
		aliases     TEXT NOT NULL DEFAULT '',
		category    TEXT NOT NULL DEFAULT '',
		isco        TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'established',
		risk        TEXT NOT NULL DEFAULT 'low',
		oversight   INTEGER NOT NULL DEFAULT 0,
		came_from   TEXT NOT NULL DEFAULT 'seed',
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);
	CREATE INDEX idx_occupations_category ON occupations(category, id);

	CREATE TABLE capabilities (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		aliases     TEXT NOT NULL DEFAULT '',
		kind        TEXT NOT NULL DEFAULT 'skill',
		description TEXT NOT NULL DEFAULT '',
		came_from   TEXT NOT NULL DEFAULT 'seed',
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);

	CREATE TABLE job_capabilities (
		job_id        TEXT NOT NULL,
		capability_id TEXT NOT NULL,
		essential     INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (job_id, capability_id)
	);
	CREATE INDEX idx_job_capabilities_capability ON job_capabilities(capability_id);

	CREATE INDEX idx_task_steps_assignee ON task_steps(assignee, id);
	`,

	/*
	 * 9: what a job actually involves, as against what it is called.
	 *
	 * Eight of these went in as one migration because they are one idea: a job
	 * definition that is a title and a sentence is a label, and a label is not
	 * enough to brief somebody with. What separates a backend engineer from a
	 * frontend one, told to a model, is the responsibilities and the outputs —
	 * not the name, which it already knew.
	 *
	 * ladder is here rather than in one shared list because career ladders are
	 * not shared. "Intern, junior, mid, senior, staff, principal" is a software
	 * ladder and nothing else's: a surgeon is a resident and then a consultant,
	 * an accountant is qualified or is not. A single ladder applied to every
	 * occupation would be wrong for most of them and would look authoritative
	 * while being so.
	 *
	 * near is other jobs worth considering instead of this one, which is what
	 * makes "we have nobody for that" answerable with something better than no.
	 *
	 * sources records which classification a row came out of — an ISCO unit
	 * group, an O*NET-SOC code — so an imported row can say where it got its
	 * authority from and a hand-written one can honestly say it has none.
	 *
	 * All nullable, because ALTER TABLE ADD COLUMN is the only form SQLite will
	 * do without rewriting the table, and it will not do that one with a NULL
	 * default.
	 */
	`
	ALTER TABLE occupations ADD COLUMN responsibilities TEXT;
	ALTER TABLE occupations ADD COLUMN knows TEXT;
	ALTER TABLE occupations ADD COLUMN makes TEXT;
	ALTER TABLE occupations ADD COLUMN measured_by TEXT;
	ALTER TABLE occupations ADD COLUMN ladder TEXT;
	ALTER TABLE occupations ADD COLUMN near TEXT;
	ALTER TABLE occupations ADD COLUMN sources TEXT;
	`,

	/*
	 * 10: how serious each piece of work is.
	 *
	 * On the task and on every step, because they are different claims: a
	 * task is as serious as its most serious step, and the view should be
	 * able to say which step that was rather than only that there is one.
	 *
	 * acted is what ran at high or critical without anybody being asked — one
	 * summary a line. Only ever written on never stop, never refuse, where
	 * nothing asks by its owner's choice, and it is the half of that choice
	 * that makes it reviewable: the task's account names each one.
	 *
	 * Nullable, like every added column. A row from before this reads as low,
	 * which is what it was treated as when it ran.
	 */
	`
	ALTER TABLE tasks ADD COLUMN risk TEXT;
	ALTER TABLE task_steps ADD COLUMN risk TEXT;
	ALTER TABLE task_steps ADD COLUMN acted TEXT;
	`,

	/*
	 * 11: work handed from one agent to another, and back.
	 *
	 * A task can be a child of another task's step: the specialist's work,
	 * drawn from the parent's budget and never able to do more than the agent
	 * that handed it on. depth is how far down a chain of handings-on this
	 * is, so a chain can be capped in a query rather than by walking it.
	 *
	 * within is the tools the child may use at most, as a list — NULL for no
	 * limit and an empty string for none at all, which are different answers
	 * and must not collapse into one. never is added to whatever else a step
	 * may not do.
	 *
	 * On a step: handed_to is the child task doing it; escalated_from is the
	 * step whose failure this one is the manager's answer to; review_of is
	 * the step this one reviews. Each is how the view draws the chain, and
	 * how the conductor refuses to escalate an escalation or review a review.
	 */
	`
	ALTER TABLE tasks ADD COLUMN parent_task_id INTEGER;
	ALTER TABLE tasks ADD COLUMN parent_step_id INTEGER;
	ALTER TABLE tasks ADD COLUMN depth INTEGER;
	ALTER TABLE tasks ADD COLUMN within TEXT;
	ALTER TABLE tasks ADD COLUMN never TEXT;
	ALTER TABLE task_steps ADD COLUMN handed_to INTEGER;
	ALTER TABLE task_steps ADD COLUMN escalated_from INTEGER;
	ALTER TABLE task_steps ADD COLUMN review_of INTEGER;
	CREATE INDEX idx_tasks_parent ON tasks(parent_task_id);
	`,

	/*
	 * 12: steps that can be done at the same time as the one before.
	 *
	 * Said by the planner, and only ever a permission: the conductor still
	 * runs them one model call at a time on this machine's lane, and side by
	 * side only where the lanes have room. A step that needs what the step
	 * before it found is never marked, so marking one wrongly costs a step
	 * that could not see something it would have been told — which is why
	 * the planner is asked to mark only what plainly stands alone.
	 */
	`
	ALTER TABLE task_steps ADD COLUMN together INTEGER;
	`,

	/*
	 * 13: who was hired for a task.
	 *
	 * One line each, on the root task, written when the hire happens — not
	 * worked out at the end from the roster, because by the end a temporary
	 * hire has been let go and the roster no longer knows they existed. The
	 * account a task gives of itself names every one.
	 */
	`
	ALTER TABLE tasks ADD COLUMN hired TEXT;
	`,

	/*
	 * 14: importing a job classification.
	 *
	 * A row per run, because an import of three thousand jobs is minutes and
	 * the program may be closed in the middle: what was running, how far it
	 * got and why it stopped all have to be readable afterwards, and "it was
	 * importing" is not something memory can answer after a restart.
	 *
	 * notes is what it could not settle on its own — a label that matched two
	 * different jobs already here — listed rather than silently decided.
	 */
	`
	CREATE TABLE imports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT NOT NULL,
		from_path TEXT NOT NULL,
		state TEXT NOT NULL,
		stage TEXT NOT NULL DEFAULT '',
		done INTEGER NOT NULL DEFAULT 0,
		of INTEGER NOT NULL DEFAULT 0,
		jobs INTEGER NOT NULL DEFAULT 0,
		capabilities INTEGER NOT NULL DEFAULT 0,
		links INTEGER NOT NULL DEFAULT 0,
		filled INTEGER NOT NULL DEFAULT 0,
		notes TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		finished_at TEXT
	);
	CREATE INDEX idx_imports_started ON imports(id DESC);
	`,

	/*
	 * 15: what each agent remembers, and who handed a step on.
	 *
	 * Memory apart from knowledge, which is the distinction the organisation
	 * rests on: what a backend engineer knows is the job's, shared by everyone
	 * who holds it, and lives in the taxonomy. What this backend engineer did
	 * last Tuesday, and what a review said was wrong with it, is this one's —
	 * two agents in the same job remember different things.
	 *
	 * Not the owner's facts table. Those are what the assistant knows about
	 * the person it works for, counted and said aloud as that, and a step's
	 * finding about a slow query is not a thing it knows about anybody.
	 *
	 * handed_by keeps who handed a step on, because finishing the step records
	 * the specialist who did it — and a delegation rate needs the other name.
	 */
	`
	CREATE TABLE agent_memories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent TEXT NOT NULL,
		kind TEXT NOT NULL,
		content TEXT NOT NULL,
		task_id INTEGER,
		step_id INTEGER,
		created_at TEXT NOT NULL
	);
	CREATE INDEX idx_agent_memories_agent ON agent_memories(agent, id DESC);
	ALTER TABLE task_steps ADD COLUMN handed_by TEXT;
	`,

	/*
	 * 16: proposals, evidence, what integrations did, and a task's project.
	 *
	 * A proposal is what somebody is asked to agree to before anything
	 * material happens — a hire, a project — kept whole, so what was approved
	 * is exactly what was shown and can be read back afterwards. Decided once:
	 * its state moves from open, and never back.
	 *
	 * Evidence is what a task can prove it did, a row per thing: a command and
	 * its exit status, a file and whether it was new, a build and where it
	 * went. Kept apart from the steps' prose, because "the tests passed" in a
	 * sentence and a row saying which command exited zero are different kinds
	 * of claim, and only one of them decides whether a job is finished.
	 *
	 * integration_events is every time an outside server was switched on, read
	 * from or called, and by whom. What went in is kept as its size and a
	 * digest rather than its text, so the record of a call cannot become a
	 * second copy of whatever was private in it.
	 *
	 * project is the folder a task works in, and packages what it works
	 * under; both empty for everything before.
	 */
	`
	CREATE TABLE proposals (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL,
		conversation_id INTEGER,
		request TEXT NOT NULL,
		body TEXT NOT NULL,
		state TEXT NOT NULL,
		outcome TEXT,
		created_at TEXT NOT NULL,
		decided_at TEXT
	);
	CREATE INDEX idx_proposals_conversation ON proposals(conversation_id, id DESC);

	CREATE TABLE evidence (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL,
		step_id INTEGER,
		kind TEXT NOT NULL,
		subject TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		ok INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	);
	CREATE INDEX idx_evidence_task ON evidence(task_id, id);

	CREATE TABLE integration_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server TEXT NOT NULL,
		event TEXT NOT NULL,
		tool TEXT,
		agent TEXT,
		task_id INTEGER,
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	);
	CREATE INDEX idx_integration_events_server ON integration_events(server, id DESC);

	ALTER TABLE tasks ADD COLUMN project TEXT;
	ALTER TABLE tasks ADD COLUMN packages TEXT;
	`,

	/*
	 * 17: choosing how work is done, and remembering how it went.
	 *
	 * resource_runs is every time a resource — a coding agent, a model, an
	 * engine — was given work, and what came of it: the history the
	 * orchestrator breaks ties with and the record of every switch.
	 * resource_usage is the last reading of how much of a subscription is
	 * left, when its service says; a row with no remaining is a service that
	 * would not say, and nothing is invented in its place.
	 *
	 * A step's action is work done by this program rather than asked of a
	 * model — checking, running and building a project — and a task's
	 * resources is what it was told about how it may be done.
	 */
	`
	CREATE TABLE resource_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		resource TEXT NOT NULL,
		kind TEXT NOT NULL,
		work TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		task_id INTEGER,
		step_id INTEGER,
		result TEXT NOT NULL,
		failure TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		millis INTEGER NOT NULL DEFAULT 0,
		verified INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);
	CREATE INDEX idx_resource_runs_resource ON resource_runs(resource, id DESC);

	CREATE TABLE resource_usage (
		resource TEXT PRIMARY KEY,
		state TEXT NOT NULL,
		remaining REAL,
		window TEXT NOT NULL DEFAULT '',
		resets_at TEXT,
		plan TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		observed_at TEXT NOT NULL
	);

	ALTER TABLE task_steps ADD COLUMN action TEXT;
	ALTER TABLE tasks ADD COLUMN resources TEXT;
	`,

	/*
	 * 18: activity, which is everything that happened, in the order it
	 * happened.
	 *
	 * Not the diary — that is `events`, and it is about the owner's day. This
	 * is the program's own account of itself: a task started, a tool ran, an
	 * approval was asked for. It exists because a phone that was asleep for an
	 * hour has to be able to ask "what happened after number 1042" and be told
	 * exactly, and because a task nobody watched should still be answerable
	 * afterwards.
	 *
	 * seq is the primary key and nothing else orders it. Timestamps are for
	 * reading; two events can share a second, and a clock can go backwards.
	 */
	`
	CREATE TABLE activity (
		seq        INTEGER PRIMARY KEY AUTOINCREMENT,
		id         TEXT NOT NULL,
		type       TEXT NOT NULL,
		task_id    INTEGER,
		step_id    INTEGER,
		device     TEXT NOT NULL DEFAULT '',
		said       TEXT NOT NULL DEFAULT '',
		data       TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	);
	CREATE INDEX idx_activity_task ON activity(task_id, seq);
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
