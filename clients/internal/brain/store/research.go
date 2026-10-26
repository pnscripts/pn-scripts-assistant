package store

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

/*
 * Research reports, kept so they can be read again, downloaded and checked
 * again. The sources and the claims are kept as the research package wrote
 * them; the store only seals and counts.
 */

// ResearchReport is one finished piece of research.
type ResearchReport struct {
	ID       int64           `json:"id"`
	Question string          `json:"question"`
	Lang     string          `json:"lang"`
	Depth    string          `json:"depth"`
	Report   string          `json:"report,omitempty"`
	Sources  json.RawMessage `json:"sources,omitempty"`
	Claims   json.RawMessage `json:"claims,omitempty"`
	Model    string          `json:"model,omitempty"`

	NSources int       `json:"n_sources"`
	NClaims  int       `json:"n_claims"`
	NFlagged int       `json:"n_flagged"`
	Created  time.Time `json:"created_at"`
}

// ErrNoResearch is a report that is not there.
var ErrNoResearch = errors.New("there is no such research report")

// SaveResearch keeps a finished report.
func (d *DB) SaveResearch(r ResearchReport) (ResearchReport, error) {
	r.Question = strings.TrimSpace(r.Question)
	if r.Question == "" {
		return ResearchReport{}, errors.New("a report needs its question")
	}

	if len(r.Sources) == 0 {
		r.Sources = json.RawMessage("[]")
	}

	if len(r.Claims) == 0 {
		r.Claims = json.RawMessage("[]")
	}

	r.Created = time.Now().UTC()

	res, err := d.sql().Exec(`INSERT INTO research_reports
		(question, lang, depth, report, sources, claims, model, n_sources, n_claims, n_flagged, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.seal(r.Question), r.Lang, r.Depth, d.seal(r.Report), d.seal(string(r.Sources)), d.seal(string(r.Claims)),
		r.Model, r.NSources, r.NClaims, r.NFlagged, r.Created.Format(time.RFC3339Nano))
	if err != nil {
		return ResearchReport{}, err
	}

	r.ID, _ = res.LastInsertId()

	return r, nil
}

// ResearchReports is every report, newest first, without its text.
func (d *DB) ResearchReports() ([]ResearchReport, error) {
	rows, err := d.sql().Query(`SELECT id, question, lang, depth, model, n_sources, n_claims, n_flagged, created_at
		FROM research_reports ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []ResearchReport{}

	for rows.Next() {
		var (
			r       ResearchReport
			created string
		)

		if err := rows.Scan(&r.ID, &r.Question, &r.Lang, &r.Depth, &r.Model, &r.NSources, &r.NClaims, &r.NFlagged, &created); err != nil {
			return nil, err
		}

		r.Created, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, r)
	}

	return out, rows.Err()
}

// Research is one report in full.
func (d *DB) Research(id int64) (ResearchReport, error) {
	var (
		r                        ResearchReport
		created, sources, claims string
	)

	err := d.sql().QueryRow(`SELECT id, question, lang, depth, report, sources, claims, model, n_sources, n_claims, n_flagged, created_at
		FROM research_reports WHERE id = ?`, id).
		Scan(&r.ID, &r.Question, &r.Lang, &r.Depth, &r.Report, &sources, &claims, &r.Model, &r.NSources, &r.NClaims, &r.NFlagged, &created)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return ResearchReport{}, ErrNoResearch
		}

		return ResearchReport{}, err
	}

	r.Sources, r.Claims = json.RawMessage(sources), json.RawMessage(claims)
	r.Created, _ = time.Parse(time.RFC3339Nano, created)

	return r, nil
}

// RemoveResearch takes a report away.
func (d *DB) RemoveResearch(id int64) error {
	res, err := d.sql().Exec(`DELETE FROM research_reports WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoResearch
	}

	return nil
}
