package store

import (
	"errors"
	"strings"
	"time"
)

/*
 * The invoice register and the VAT helper's settings. The books package
 * decides what an invoice must say and what a period comes to; this keeps
 * them, sealed.
 */

// Invoice is one invoice in the register.
type Invoice struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Number   string `json:"number"`
	Date     string `json:"date"`
	Party    string `json:"party"`
	PartyVAT string `json:"party_vat,omitempty"`
	Net      string `json:"net"`
	Rate     string `json:"rate"`
	VAT      string `json:"vat"`
	Gross    string `json:"gross"`
	Currency string `json:"currency,omitempty"`
	Reverse  bool   `json:"reverse,omitempty"`
	Note     string `json:"note,omitempty"`
}

// ErrNoInvoice is an invoice that is not there.
var ErrNoInvoice = errors.New("there is no such invoice in the register")

// AddInvoice keeps an invoice.
func (d *DB) AddInvoice(inv Invoice) (Invoice, error) {
	if inv.Kind != "sale" && inv.Kind != "purchase" {
		return Invoice{}, errors.New("an invoice is a sale or a purchase")
	}

	if _, err := time.Parse("2006-01-02", inv.Date); err != nil {
		return Invoice{}, errors.New("an invoice needs its date, as YYYY-MM-DD")
	}

	reverse := 0
	if inv.Reverse {
		reverse = 1
	}

	res, err := d.sql().Exec(`INSERT INTO invoices (kind, number, date, party, party_vat, net, rate, vat, gross, currency, reverse, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.Kind, d.seal(strings.TrimSpace(inv.Number)), inv.Date, d.seal(inv.Party), d.seal(inv.PartyVAT),
		d.seal(inv.Net), inv.Rate, d.seal(inv.VAT), d.seal(inv.Gross), inv.Currency, reverse, d.seal(inv.Note),
		time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Invoice{}, err
	}

	inv.ID, _ = res.LastInsertId()

	return inv, nil
}

// Invoices is the register, oldest first; from and to (YYYY-MM-DD, either
// empty) narrow it to a span of dates.
func (d *DB) Invoices(from, to string) ([]Invoice, error) {
	q := `SELECT id, kind, number, date, party, party_vat, net, rate, vat, gross, currency, reverse, note FROM invoices`

	var (
		where []string
		args  []any
	)

	if from != "" {
		where, args = append(where, "date >= ?"), append(args, from)
	}

	if to != "" {
		where, args = append(where, "date <= ?"), append(args, to)
	}

	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}

	rows, err := d.sql().Query(q+" ORDER BY date, id", args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Invoice{}

	for rows.Next() {
		var (
			inv     Invoice
			reverse int
		)

		if err := rows.Scan(&inv.ID, &inv.Kind, &inv.Number, &inv.Date, &inv.Party, &inv.PartyVAT, &inv.Net, &inv.Rate,
			&inv.VAT, &inv.Gross, &inv.Currency, &reverse, &inv.Note); err != nil {
			return nil, err
		}

		inv.Reverse = reverse == 1
		out = append(out, inv)
	}

	return out, rows.Err()
}

// RemoveInvoice takes an invoice out of the register.
func (d *DB) RemoveInvoice(id int64) error {
	res, err := d.sql().Exec(`DELETE FROM invoices WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoInvoice
	}

	return nil
}

// BooksSettings are the VAT helper's settings, as names and values.
func (d *DB) BooksSettings() (map[string]string, error) {
	rows, err := d.sql().Query(`SELECT key, value FROM books_settings`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := map[string]string{}

	for rows.Next() {
		var k, v string

		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}

		out[k] = v
	}

	return out, rows.Err()
}

// SetBooksSettings keeps the values given; an empty value forgets one.
func (d *DB) SetBooksSettings(values map[string]string) error {
	tx, err := d.sql().Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	for k, v := range values {
		v = strings.TrimSpace(v)

		if v == "" {
			if _, err := tx.Exec(`DELETE FROM books_settings WHERE key = ?`, k); err != nil {
				return err
			}

			continue
		}

		if _, err := tx.Exec(`INSERT INTO books_settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, d.seal(v)); err != nil {
			return err
		}
	}

	return tx.Commit()
}
