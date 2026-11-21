package store

import (
	"errors"
	"fmt"
	"time"
)

/*
 * The actions its person wrote for other programs to run (package
 * otherprograms, the "actions" switch): a name, and the command that name
 * stands for. Both are sealed, so a name is looked for here in Go and not
 * in SQL; there are a handful of them.
 */

// OutsideAction is one action as its person wrote it.
type OutsideAction struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Command   string    `json:"command"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	// ErrActionExists is a second action under a name already taken.
	ErrActionExists = errors.New("there is an action with that name already")

	// ErrNoAction is an action that is not there.
	ErrNoAction = errors.New("there is no such action")

	// ErrActionsFull is one action more than are kept.
	ErrActionsFull = errors.New("no more actions are kept than there are already")
)

/*
 * AddOutsideAction keeps a new action, with its line on the record of what
 * was done, as one thing: both are written or neither is. At most most of
 * them, and never two under one name.
 */
func (d *DB) AddOutsideAction(name, command string, most int, written Deed) (OutsideAction, error) {
	if name == "" || command == "" {
		return OutsideAction{}, errors.New("an action is a name and a command")
	}

	d.actions.Lock()
	defer d.actions.Unlock()

	have, err := d.OutsideActions()
	if err != nil {
		return OutsideAction{}, err
	}

	if len(have) >= most {
		return OutsideAction{}, ErrActionsFull
	}

	for _, a := range have {
		if a.Name == name {
			return OutsideAction{}, ErrActionExists
		}
	}

	now := time.Now().UTC()

	tx, err := d.sql().Begin()
	if err != nil {
		return OutsideAction{}, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO outside_actions (name, command, created_at) VALUES (?, ?, ?)`,
		d.seal(name), d.seal(command), now.Format(time.RFC3339Nano))
	if err != nil {
		return OutsideAction{}, fmt.Errorf("keeping the action: %w", err)
	}

	if _, err := d.insertDeed(tx, written); err != nil {
		return OutsideAction{}, err
	}

	if err := tx.Commit(); err != nil {
		return OutsideAction{}, err
	}

	id, _ := res.LastInsertId()

	return OutsideAction{ID: id, Name: name, Command: command, CreatedAt: now}, nil
}

// OutsideActions is every action, in the order they were written.
func (d *DB) OutsideActions() ([]OutsideAction, error) {
	rows, err := d.sql().Query(`SELECT id, name, command, created_at FROM outside_actions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading the actions: %w", err)
	}
	defer rows.Close()

	out := []OutsideAction{}

	for rows.Next() {
		var (
			a       OutsideAction
			created string
		)

		if err := rows.Scan(&a.ID, &a.Name, &a.Command, &created); err != nil {
			return nil, err
		}

		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)

		out = append(out, a)
	}

	return out, rows.Err()
}

/*
 * RemoveOutsideAction takes one action away, with its line on the record,
 * as one thing. removed is given the action before it goes, to say what was
 * taken away.
 */
func (d *DB) RemoveOutsideAction(id int64, removed func(OutsideAction) Deed) error {
	d.actions.Lock()
	defer d.actions.Unlock()

	have, err := d.OutsideActions()
	if err != nil {
		return err
	}

	for _, a := range have {
		if a.ID != id {
			continue
		}

		tx, err := d.sql().Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		if _, err := tx.Exec(`DELETE FROM outside_actions WHERE id = ?`, id); err != nil {
			return fmt.Errorf("taking the action away: %w", err)
		}

		if _, err := d.insertDeed(tx, removed(a)); err != nil {
			return err
		}

		return tx.Commit()
	}

	return ErrNoAction
}
