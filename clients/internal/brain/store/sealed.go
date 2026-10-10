package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"

	"modernc.org/sqlite"

	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * The database's personal columns, sealed (ADR 0008).
 *
 * Writing seals explicitly: d.seal(text) at every place a message, a memory,
 * a tool call or anything else of its owner's is written. Reading opens
 * implicitly: the connection this package uses opens every sealed value on
 * the way out, so a query reads exactly as it did before and the hundred
 * places that read do not each have to remember. A value that will not open —
 * another brain's key, a changed byte — fails the query rather than being
 * handed on as gibberish.
 *
 * Why columns and not the file: modernc's SQLite has no page encryption, a
 * whole-file approach means a plaintext working copy somewhere and losing
 * what was said since the last re-encryption to a power cut, and power cuts
 * are frequent here. Columns cost the few queries that did text work inside
 * SQL on a personal column; those now do it in Go.
 */

// connector opens modernc connections that open sealed values.
type connector struct {
	dsn string
	key *vault.Key
}

func (c connector) Connect(context.Context) (driver.Conn, error) {
	inner, err := (&sqlite.Driver{}).Open(c.dsn)
	if err != nil {
		return nil, err
	}

	return &sealedConn{Conn: inner, key: c.key}, nil
}

func (c connector) Driver() driver.Driver { return &sqlite.Driver{} }

func openPool(path string, key *vault.Key) *sql.DB {
	// secure_delete: a page freed by an update is zeroed, so the plain text a
	// value held before it was sealed is not left behind in the file.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)" +
		"&_pragma=secure_delete(ON)"

	return sql.OpenDB(connector{dsn: dsn, key: key})
}

type sealedConn struct {
	driver.Conn
	key *vault.Key
}

func (c *sealedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

func (c *sealedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	s, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}

	return &sealedStmt{Stmt: s, key: c.key}, nil
}

func (c *sealedConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *sealedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (c *sealedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}

	return &sealedRows{Rows: rows, key: c.key}, nil
}

func (c *sealedConn) Ping(ctx context.Context) error { return c.Conn.(driver.Pinger).Ping(ctx) }

func (c *sealedConn) ResetSession(ctx context.Context) error {
	return c.Conn.(driver.SessionResetter).ResetSession(ctx)
}

func (c *sealedConn) IsValid() bool { return c.Conn.(driver.Validator).IsValid() }

type sealedStmt struct {
	driver.Stmt
	key *vault.Key
}

func (s *sealedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.Stmt.(driver.StmtExecContext).ExecContext(ctx, args)
}

func (s *sealedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := s.Stmt.(driver.StmtQueryContext).QueryContext(ctx, args)
	if err != nil {
		return nil, err
	}

	return &sealedRows{Rows: rows, key: s.key}, nil
}

type sealedRows struct {
	driver.Rows
	key *vault.Key
}

func (r *sealedRows) Next(dest []driver.Value) error {
	if err := r.Rows.Next(dest); err != nil {
		return err
	}

	for i, v := range dest {
		switch x := v.(type) {
		case string:
			if vault.IsSealedString(x) {
				if r.key == nil {
					return fmt.Errorf("column %d: %w", i, vault.ErrNoKey)
				}

				plain, err := r.key.OpenString(x)
				if err != nil {
					return fmt.Errorf("column %d: %w", i, err)
				}

				dest[i] = plain
			}
		case []byte:
			if vault.IsSealed(x) {
				if r.key == nil {
					return fmt.Errorf("column %d: %w", i, vault.ErrNoKey)
				}

				plain, err := r.key.Open(x)
				if err != nil {
					return fmt.Errorf("column %d: %w", i, err)
				}

				dest[i] = plain
			} else if vault.IsSealedString(string(x)) {
				// Text that came back as bytes.
				if r.key == nil {
					return fmt.Errorf("column %d: %w", i, vault.ErrNoKey)
				}

				plain, err := r.key.OpenString(string(x))
				if err != nil {
					return fmt.Errorf("column %d: %w", i, err)
				}

				dest[i] = []byte(plain)
			}
		}
	}

	return nil
}

// seal is a personal value as it is written: sealed when this database has a
// key, as it is when it does not (a test, or a brain opened by a tool that
// reads nothing personal).
func (d *DB) seal(text string) any {
	if d.key == nil || text == "" || vault.IsSealedString(text) {
		return text
	}

	return d.key.SealString(text)
}

// sealNull is seal for a column that may be NULL.
func (d *DB) sealNull(text sql.NullString) any {
	if !text.Valid {
		return nil
	}

	return d.seal(text.String)
}

// sealBlob is seal for bytes — an embedding of somebody's words is their words.
func (d *DB) sealBlob(b []byte) any {
	if d.key == nil || b == nil || vault.IsSealed(b) {
		return b
	}

	return d.key.Seal(b)
}

// Sealed reports whether this database seals what it writes.
func (d *DB) Sealed() bool { return d.key != nil }
