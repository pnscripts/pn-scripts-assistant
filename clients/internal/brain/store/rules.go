package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/rules"
)

/*
 * Rules: what its user told it always or never to do (package rules).
 *
 * A rule is proposed when it is noticed and lives only once its user has
 * said yes; nothing here turns a proposal into a rule by itself. Declined
 * proposals are kept, so the same sentence said again is not proposed again,
 * and the oldest of them are let go once there are more than MaxDeclined.
 * A household has dozens of rules, not thousands, so they are read whole and
 * filtered in Go — the text is sealed and cannot be searched in SQL anyway.
 */

// ErrNoRule is a rule that is not there.
var ErrNoRule = errors.New("there is no such rule")

// MaxRules is how many rules, kept or proposed, one brain holds. Past that
// the prompt that carries them stops being read, and a list nobody can read
// is not a list of rules.
const MaxRules = 200

// MaxDeclined is how many declined proposals are remembered.
const MaxDeclined = 200

// AddRule keeps a rule as it is given — usually a proposal.
func (d *DB) AddRule(r rules.Rule) (rules.Rule, error) {
	r = rules.Normalise(r)

	if r.Text == "" {
		return rules.Rule{}, errors.New("a rule needs words")
	}

	if r.Status == "" {
		r.Status = rules.Proposed
	}

	if n, err := d.countRules(); err != nil {
		return rules.Rule{}, err
	} else if n >= MaxRules && r.Status != rules.Declined {
		return rules.Rule{}, fmt.Errorf("there are already %d rules; switch off or delete some first", n)
	}

	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}

	r.UpdatedAt = now

	words, _ := json.Marshal(r.Words)

	res, err := d.sql().Exec(`
		INSERT INTO rules (text, kind, actions, words, days, is_on, status, source, conversation_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.seal(r.Text), string(r.Kind), strings.Join(r.Actions, ","), d.seal(string(words)), daysText(r.Days),
		boolInt(r.On), r.Status, d.seal(r.Source), nullifyID(r.ConversationID),
		r.CreatedAt.Format(time.RFC3339Nano), r.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return rules.Rule{}, fmt.Errorf("keeping a rule: %w", err)
	}

	r.ID, _ = res.LastInsertId()

	d.forgetOldDeclined()

	return r, nil
}

func (d *DB) countRules() (int, error) {
	var n int

	err := d.sql().QueryRow(`SELECT COUNT(*) FROM rules WHERE status != ?`, rules.Declined).Scan(&n)

	return n, err
}

// Rules is every rule, proposals first, then kept ones in the order they
// were made. Declined proposals are left out unless withDeclined.
func (d *DB) Rules(withDeclined bool) ([]rules.Rule, error) {
	rows, err := d.sql().Query(`
		SELECT id, text, kind, actions, words, days, is_on, status, source,
		       COALESCE(conversation_id, 0), created_at, updated_at
		FROM rules ORDER BY CASE status WHEN ? THEN 0 ELSE 1 END, id`, rules.Proposed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []rules.Rule{}

	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}

		if r.Status == rules.Declined && !withDeclined {
			continue
		}

		out = append(out, r)
	}

	return out, rows.Err()
}

// LiveRules is the rules being followed now.
func (d *DB) LiveRules() ([]rules.Rule, error) {
	all, err := d.Rules(false)
	if err != nil {
		return nil, err
	}

	live := all[:0]

	for _, r := range all {
		if r.Live() {
			live = append(live, r)
		}
	}

	return live, nil
}

// Rule is one rule by its number.
func (d *DB) Rule(id int64) (rules.Rule, error) {
	row := d.sql().QueryRow(`
		SELECT id, text, kind, actions, words, days, is_on, status, source,
		       COALESCE(conversation_id, 0), created_at, updated_at
		FROM rules WHERE id = ?`, id)

	r, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return rules.Rule{}, ErrNoRule
	}

	return r, err
}

type scanner interface{ Scan(dest ...any) error }

func scanRule(row scanner) (rules.Rule, error) {
	var (
		r                      rules.Rule
		kind, actions, words   string
		days, created, updated string
		on                     int
	)

	if err := row.Scan(&r.ID, &r.Text, &kind, &actions, &words, &days, &on, &r.Status, &r.Source,
		&r.ConversationID, &created, &updated); err != nil {
		return rules.Rule{}, err
	}

	r.Kind = rules.Kind(kind)
	r.On = on != 0
	r.Actions = splitCommas(actions)
	r.Days = parseDays(days)
	r.Words = []string{}

	if words != "" {
		_ = json.Unmarshal([]byte(words), &r.Words)
	}

	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)

	return r, nil
}

/*
 * ChangeRule writes a rule's words, kind, actions, words to look for, days
 * and switch. The status is not changed here: keeping and declining are
 * their own decisions (Decide), so an edit can never turn a proposal into a
 * rule nobody agreed to.
 */
func (d *DB) ChangeRule(r rules.Rule) (rules.Rule, error) {
	was, err := d.Rule(r.ID)
	if err != nil {
		return rules.Rule{}, err
	}

	r = rules.Normalise(r)
	if r.Text == "" {
		return rules.Rule{}, errors.New("a rule needs words")
	}

	r.Status, r.Source, r.ConversationID, r.CreatedAt = was.Status, was.Source, was.ConversationID, was.CreatedAt

	if r.Status != rules.Kept {
		r.On = false
	}

	r.UpdatedAt = time.Now().UTC()

	words, _ := json.Marshal(r.Words)

	_, err = d.sql().Exec(`
		UPDATE rules SET text = ?, kind = ?, actions = ?, words = ?, days = ?, is_on = ?, updated_at = ?
		WHERE id = ?`,
		d.seal(r.Text), string(r.Kind), strings.Join(r.Actions, ","), d.seal(string(words)), daysText(r.Days),
		boolInt(r.On), r.UpdatedAt.Format(time.RFC3339Nano), r.ID)
	if err != nil {
		return rules.Rule{}, fmt.Errorf("changing a rule: %w", err)
	}

	return r, nil
}

// DecideRule is its user's answer to a proposal: kept (and on) or declined.
// Only a proposal can be decided, and only once.
func (d *DB) DecideRule(id int64, keep bool) (rules.Rule, error) {
	status, on := rules.Declined, 0
	if keep {
		status, on = rules.Kept, 1
	}

	res, err := d.sql().Exec(`UPDATE rules SET status = ?, is_on = ?, updated_at = ? WHERE id = ? AND status = ?`,
		status, on, time.Now().UTC().Format(time.RFC3339Nano), id, rules.Proposed)
	if err != nil {
		return rules.Rule{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		r, err := d.Rule(id)
		if err != nil {
			return rules.Rule{}, err
		}

		return r, fmt.Errorf("that rule was already %s", r.Status)
	}

	d.forgetOldDeclined()

	return d.Rule(id)
}

// SwitchRule turns a kept rule on or off.
func (d *DB) SwitchRule(id int64, on bool) (rules.Rule, error) {
	res, err := d.sql().Exec(`UPDATE rules SET is_on = ?, updated_at = ? WHERE id = ? AND status = ?`,
		boolInt(on), time.Now().UTC().Format(time.RFC3339Nano), id, rules.Kept)
	if err != nil {
		return rules.Rule{}, err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := d.Rule(id); err != nil {
			return rules.Rule{}, err
		}

		return rules.Rule{}, errors.New("only a kept rule can be switched on or off")
	}

	return d.Rule(id)
}

// RemoveRule deletes a rule for good.
func (d *DB) RemoveRule(id int64) error {
	res, err := d.sql().Exec(`DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoRule
	}

	return nil
}

// KnownRule reports whether a rule with these words is already kept,
// proposed or declined — the same sentence is never proposed twice.
func (d *DB) KnownRule(text string) (bool, error) {
	all, err := d.Rules(true)
	if err != nil {
		return false, err
	}

	want := strings.ToLower(rules.Clean(text))

	for _, r := range all {
		if strings.ToLower(r.Text) == want {
			return true, nil
		}
	}

	return false, nil
}

func (d *DB) forgetOldDeclined() {
	_, _ = d.sql().Exec(`
		DELETE FROM rules WHERE status = ? AND id NOT IN (
			SELECT id FROM rules WHERE status = ? ORDER BY id DESC LIMIT ?)`,
		rules.Declined, rules.Declined, MaxDeclined)
}

// splitCommas is a list kept as "a,b,c", never nil.
func splitCommas(s string) []string {
	out := []string{}

	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}

	return out
}

func daysText(days []time.Weekday) string {
	parts := make([]string, 0, len(days))

	for _, d := range days {
		parts = append(parts, strconv.Itoa(int(d)))
	}

	return strings.Join(parts, ",")
}

func parseDays(s string) []time.Weekday {
	out := []time.Weekday{}

	for _, x := range splitCommas(s) {
		if n, err := strconv.Atoi(x); err == nil && n >= 0 && n <= 6 {
			out = append(out, time.Weekday(n))
		}
	}

	return out
}
