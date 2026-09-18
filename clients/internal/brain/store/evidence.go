package store

import (
	"pn-scripts-assistant/internal/brain/redact"

	"strings"
	"time"
)

/*
 * Evidence: what a task can prove it did.
 *
 * A row per thing, written as it happens rather than summarised afterwards.
 * The kinds are few and closed, because they are what a package's rule for
 * "finished" is written against — a book is finished when there is a
 * document, a game when there is a check, a run and the versions it ran on.
 */

// Kinds of evidence.
const (
	EvidenceCommand  = "command" // exact argv, exit status, how long
	EvidenceFile     = "file"    // a path written, and whether it was new
	EvidenceCheck    = "check"   // a validation or headless check
	EvidenceBuild    = "build"   // a build, and where it went
	EvidenceTest     = "test"    // tests, and what they said
	EvidenceExport   = "export"  // a packaged artifact
	EvidenceSmoke    = "smoke"   // it started and ran
	EvidenceShot     = "screenshot"
	EvidenceDocument = "document"
	EvidenceSource   = "source"   // where a claim came from
	EvidenceQuestion = "question" // what to put to a qualified person
	EvidenceVersion  = "version"  // which software, at which version
	EvidenceAgent    = "agent"    // who did it, under which packages
	EvidenceApproval = "approval" // what its owner said yes to
	EvidenceWarning  = "warning"  // something left unresolved
	EvidenceNext     = "next"     // what a person must do next
	EvidenceSettings = "settings" // a project's settings as its task found them
	EvidenceDecision = "decision" // how the work was to be done, and why
	EvidenceSwitch   = "switch"   // one resource replaced by another, and why
	EvidenceInstall  = "install"  // something put on this machine, and its proof
)

type Evidence struct {
	ID      int64     `json:"id"`
	TaskID  int64     `json:"task_id"`
	StepID  int64     `json:"step_id,omitempty"`
	Kind    string    `json:"kind"`
	Subject string    `json:"subject"`
	Detail  string    `json:"detail,omitempty"`
	OK      bool      `json:"ok"`
	At      time.Time `json:"at"`
}

// Record keeps one piece of evidence.
func (d *DB) Record(e Evidence) error {
	_, err := d.sql().Exec(`INSERT INTO evidence (task_id, step_id, kind, subject, detail, ok, created_at)
		VALUES (?,?,?,?,?,?,?)`, e.TaskID, nullable(e.StepID), e.Kind,
		strings.TrimSpace(redact.Text(e.Subject)), strings.TrimSpace(redact.Text(e.Detail)), e.OK,
		time.Now().UTC().Format(time.RFC3339))

	return err
}

// EvidenceFor is everything a task has recorded, oldest first.
func (d *DB) EvidenceFor(taskID int64) ([]Evidence, error) {
	rows, err := d.sql().Query(`SELECT id, task_id, COALESCE(step_id,0), kind, subject, detail, ok, created_at
		FROM evidence WHERE task_id = ? ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Evidence{}

	for rows.Next() {
		var (
			e  Evidence
			at string
		)

		if err := rows.Scan(&e.ID, &e.TaskID, &e.StepID, &e.Kind, &e.Subject, &e.Detail, &e.OK, &at); err != nil {
			return nil, err
		}

		e.At = atTime(at)
		out = append(out, e)
	}

	return out, rows.Err()
}

// IntegrationEvent is one thing an outside server did, or had done to it.
type IntegrationEvent struct {
	ID     int64     `json:"id"`
	Server string    `json:"server"`
	Event  string    `json:"event"`
	Tool   string    `json:"tool,omitempty"`
	Agent  string    `json:"agent,omitempty"`
	TaskID int64     `json:"task_id,omitempty"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

// NoteIntegration records one.
func (d *DB) NoteIntegration(e IntegrationEvent) error {
	_, err := d.sql().Exec(`INSERT INTO integration_events (server, event, tool, agent, task_id, detail, created_at)
		VALUES (?,?,?,?,?,?,?)`, e.Server, e.Event, nullText(e.Tool), nullText(e.Agent),
		nullable(e.TaskID), e.Detail, time.Now().UTC().Format(time.RFC3339))

	return err
}

// IntegrationEvents is the newest first, for one server or all of them.
func (d *DB) IntegrationEvents(server string, limit int) ([]IntegrationEvent, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, server, event, COALESCE(tool,''), COALESCE(agent,''), COALESCE(task_id,0),
		detail, created_at FROM integration_events`
	args := []any{}

	if server != "" {
		query += ` WHERE server = ?`
		args = append(args, server)
	}

	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.sql().Query(query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []IntegrationEvent{}

	for rows.Next() {
		var (
			e  IntegrationEvent
			at string
		)

		if err := rows.Scan(&e.ID, &e.Server, &e.Event, &e.Tool, &e.Agent, &e.TaskID, &e.Detail, &at); err != nil {
			return nil, err
		}

		e.At = atTime(at)
		out = append(out, e)
	}

	return out, rows.Err()
}
