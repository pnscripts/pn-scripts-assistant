package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

/*
 * Proposals: what somebody is asked to agree to, kept exactly as it was shown.
 *
 * The body is the proposal itself, as JSON, and it is what is carried out when
 * the answer is yes — never something rebuilt at that moment. Rebuilding would
 * mean the thing approved and the thing done could differ: a tool installed in
 * between, a package edited, a job imported. What somebody read is what
 * happens, or nothing does.
 */

// Kinds of proposal.
const (
	ProposeHire    = "hire"
	ProposeProject = "project"
)

// Where a proposal has got to. Open moves once, to one of the others.
const (
	ProposalOpen       = "open"
	ProposalAccepted   = "accepted"
	ProposalDeclined   = "declined"
	ProposalSuperseded = "superseded"
)

type Proposal struct {
	ID             int64     `json:"id"`
	Kind           string    `json:"kind"`
	ConversationID int64     `json:"conversation_id,omitempty"`
	Request        string    `json:"request"`
	Body           string    `json:"body"`
	State          string    `json:"state"`
	Outcome        string    `json:"outcome,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	DecidedAt      time.Time `json:"decided_at,omitempty"`
}

// Propose records a proposal, open.
func (d *DB) Propose(p Proposal) (int64, error) {
	res, err := d.sql().Exec(`INSERT INTO proposals (kind, conversation_id, request, body, state, created_at)
		VALUES (?,?,?,?,?,?)`, p.Kind, nullable(p.ConversationID), p.Request, p.Body,
		ProposalOpen, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("recording the proposal: %w", err)
	}

	return res.LastInsertId()
}

const proposalColumns = `id, kind, COALESCE(conversation_id,0), request, body, state,
	COALESCE(outcome,''), created_at, COALESCE(decided_at,'')`

func scanProposal(row interface{ Scan(...any) error }) (Proposal, error) {
	var (
		p                Proposal
		created, decided string
	)

	err := row.Scan(&p.ID, &p.Kind, &p.ConversationID, &p.Request, &p.Body, &p.State,
		&p.Outcome, &created, &decided)
	if err != nil {
		return p, err
	}

	p.CreatedAt, p.DecidedAt = atTime(created), atTime(decided)

	return p, nil
}

// ProposalByID is one proposal, or nil.
func (d *DB) ProposalByID(id int64) (*Proposal, error) {
	p, err := scanProposal(d.sql().QueryRow(`SELECT `+proposalColumns+` FROM proposals WHERE id = ?`, id))

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &p, nil
}

/*
 * OpenProposal is the newest proposal of a kind still waiting for an answer in
 * one conversation, if it is recent.
 *
 * Recent, because "permanent" said an hour after a proposal is more likely to
 * be about something else than an answer to it.
 */
func (d *DB) OpenProposal(conversationID int64, kind string, within time.Duration) (*Proposal, error) {
	since := time.Now().Add(-within).UTC().Format(time.RFC3339)

	p, err := scanProposal(d.sql().QueryRow(`SELECT `+proposalColumns+` FROM proposals
		WHERE conversation_id = ? AND kind = ? AND state = ? AND created_at >= ?
		ORDER BY id DESC LIMIT 1`, conversationID, kind, ProposalOpen, since))

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &p, nil
}

// Proposals is the newest first, of a kind or all of them.
func (d *DB) Proposals(kind string, limit int) ([]Proposal, error) {
	if limit <= 0 {
		limit = 30
	}

	query := `SELECT ` + proposalColumns + ` FROM proposals`
	args := []any{}

	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}

	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.sql().Query(query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Proposal{}

	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, p)
	}

	return out, rows.Err()
}

// UpdateProposal replaces an open proposal's body — its owner answered one of
// its questions — and leaves it open. Nothing else can be changed.
func (d *DB) UpdateProposal(id int64, body string) error {
	res, err := d.sql().Exec(`UPDATE proposals SET body = ? WHERE id = ? AND state = ?`,
		body, id, ProposalOpen)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("proposal %d has already been decided", id)
	}

	return nil
}

/*
 * DecideProposal moves an open proposal to its answer, once.
 *
 * The same shape as deciding an action: the move is the check. A proposal
 * already decided is not decided again, so a second press of a button or a
 * replayed request cannot hire somebody twice.
 */
func (d *DB) DecideProposal(id int64, state, outcome string) (bool, error) {
	switch state {
	case ProposalAccepted, ProposalDeclined, ProposalSuperseded:
	default:
		return false, fmt.Errorf("%q is not an answer to a proposal", state)
	}

	res, err := d.sql().Exec(`UPDATE proposals SET state = ?, outcome = ?, decided_at = ?
		WHERE id = ? AND state = ?`, state, strings.TrimSpace(outcome),
		time.Now().UTC().Format(time.RFC3339), id, ProposalOpen)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n == 1, err
}

// NoteOutcome adds what came of a proposal after it was decided — the task it
// started, what failed — without changing its answer.
func (d *DB) NoteOutcome(id int64, outcome string) error {
	_, err := d.sql().Exec(`UPDATE proposals SET outcome = ? WHERE id = ?`, strings.TrimSpace(outcome), id)

	return err
}

// SetTaskProject records the folder a task works in and what it works under.
func (d *DB) SetTaskProject(taskID int64, project, packages string) error {
	_, err := d.sql().Exec(`UPDATE tasks SET project = ?, packages = ? WHERE id = ?`,
		nullText(project), nullText(packages), taskID)

	return err
}
