package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * What other programs may ask about (package otherprograms): one row for each
 * thing its person allowed, with the time it was allowed. No row is not
 * allowed. Nothing personal is kept here: a subject is one of the
 * program's own words.
 */

// AllowOutside lets other programs ask about subject from at on. Allowed
// again while allowed, it keeps the first time.
func (d *DB) AllowOutside(subject string, at time.Time) error {
	if subject == "" {
		return errors.New("nothing named to allow other programs to ask about")
	}

	_, err := d.sql().Exec(`INSERT INTO outside_may (subject, since) VALUES (?, ?) ON CONFLICT (subject) DO NOTHING`,
		subject, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("allowing other programs to ask about %s: %w", subject, err)
	}

	return nil
}

// DisallowOutside takes AllowOutside back; taking back what is not allowed
// is fine.
func (d *DB) DisallowOutside(subject string) error {
	if _, err := d.sql().Exec(`DELETE FROM outside_may WHERE subject = ?`, subject); err != nil {
		return fmt.Errorf("no longer allowing other programs to ask about %s: %w", subject, err)
	}

	return nil
}

/*
 * OutsideAllowed is everything other programs may ask about, with since
 * when. A row that cannot be read is an error and nothing is returned
 * with it: whoever asks treats that as not allowed.
 */
func (d *DB) OutsideAllowed() (map[string]time.Time, error) {
	rows, err := d.sql().Query(`SELECT subject, since FROM outside_may`)
	if err != nil {
		return nil, fmt.Errorf("reading what other programs may ask: %w", err)
	}
	defer rows.Close()

	out := map[string]time.Time{}

	for rows.Next() {
		var subject, since string

		if err := rows.Scan(&subject, &since); err != nil {
			return nil, err
		}

		at, err := time.Parse(time.RFC3339Nano, since)
		if err != nil {
			return nil, fmt.Errorf("reading since when other programs may ask about %s: %w", subject, err)
		}

		out[subject] = at
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// NoName is kept for a program whose name leaves nothing to keep (see
// askedByName; otherprograms.Unnamed is the same word).
const NoName = "unnamed"

/*
 * askedByName is the name of a program that asked, as it may be written.
 *
 * The name is whatever another program chose to send. Text that begins
 * with the mark of sealed text is written as it is, because it is taken
 * for already sealed, and then cannot be read back: one such name would
 * make the whole record unreadable. So the mark is taken off the front,
 * however often it is repeated, and a name that was nothing else is kept
 * as NoName. An empty name stays empty: nobody else asked.
 */
func askedByName(by string) string {
	if by == "" {
		return ""
	}

	by = strings.TrimSpace(by)

	for vault.IsSealedString(by) {
		by = strings.TrimSpace(strings.TrimPrefix(by, vault.Prefix))
	}

	if by == "" {
		return NoName
	}

	return by
}
