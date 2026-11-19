package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * Text that only looks sealed.
 *
 * Anything a person, a web page, a file, a mail or another program hands
 * over may begin with the mark sealed text carries. It is still their text:
 * it is kept, kept sealed where the brain is sealed, and read back exactly
 * as it came. It never makes a table unreadable and it is never opened as
 * if this program had sealed it.
 */

// shapedLikeSealed is text built to pass for a sealed value: the mark, then
// base64 of the blob's opening bytes, a key id, a nonce and a tag.
func shapedLikeSealed() string {
	blob := append([]byte("\x89PNSEAL\x01"), bytes.Repeat([]byte{0x41}, 8+12+16+5)...)

	return vault.Prefix + base64.RawStdEncoding.EncodeToString(blob)
}

// lookalikes is every kind of such text, by name. mine is a key of the brain
// under test (nil for a brain with none).
func lookalikes(t *testing.T, mine *vault.Key) map[string]string {
	t.Helper()

	other := sealKey(t)

	out := map[string]string{
		"the mark and a word":           "pnsealed1:x",
		"the mark alone":                "pnsealed1:",
		"the mark twice":                "pnsealed1:pnsealed1:AAAA",
		"the mark and Bulgarian":        "pnsealed1:Здравей, как си?",
		"the mark and a secret":         "pnsealed1:the code is LOOKALIKE-ZZQ-4471",
		"shaped like a sealed value":    shapedLikeSealed(),
		"sealed by another brain":       other.SealString("somebody else's words"),
		"the mark of kept text":         "pnplain1:hello",
		"the mark of kept text, twice":  "pnplain1:pnplain1:hello",
		"kept text then the sealed one": "pnplain1:pnsealed1:x",
		"the blob's opening bytes":      "\x89PNSEAL\x01 and then words",
		"the kept blob's opening bytes": "\x89PNPLAIN\x01 and then words",
		"ordinary words":                "Купи хляб and milk",
		"the mark later in the text":    "see pnsealed1:x",
	}

	if mine != nil {
		// A value this very brain sealed, handed back to it as text: it is
		// text, and reading it must not open it.
		out["sealed by this brain"] = mine.SealString("what this brain sealed before")
	}

	return out
}

func brains(t *testing.T) map[string]func(*testing.T) (*DB, *vault.Key, string) {
	t.Helper()

	return map[string]func(*testing.T) (*DB, *vault.Key, string){
		"a sealed brain": func(t *testing.T) (*DB, *vault.Key, string) {
			path := filepath.Join(t.TempDir(), "brain.sqlite")
			k := sealKey(t)

			d, err := OpenSealed(path, k)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { d.Close() })

			return d, k, path
		},
		"a brain with no key": func(t *testing.T) (*DB, *vault.Key, string) {
			path := filepath.Join(t.TempDir(), "brain.sqlite")

			d, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { d.Close() })

			return d, nil, path
		},
	}
}

// What a person says, a to-do, a reminder, a tool's result and the record of
// it: each comes back as it was written, and the others beside it stay
// readable.
func TestTextThatLooksSealedIsKeptAsText(t *testing.T) {
	for name, opened := range brains(t) {
		t.Run(name, func(t *testing.T) {
			d, key, _ := opened(t)

			conv, err := d.NewConversation("pnsealed1:a title")
			if err != nil {
				t.Fatal(err)
			}

			for kind, text := range lookalikes(t, key) {
				if _, err := d.AddMessage(conv, "user", "", "", text); err != nil {
					t.Fatalf("%s: %v", kind, err)
				}

				history, err := d.History(conv)
				if err != nil {
					t.Fatalf("%s: the conversation cannot be read any more: %v", kind, err)
				}

				if got := history[len(history)-1].Content; got != text {
					t.Errorf("%s: the message %q came back as %q", kind, text, got)
				}

				todo, err := d.AddTodo("pnsealed1:shopping", text, nil)
				if err != nil {
					t.Fatalf("%s: %v", kind, err)
				}

				todos, err := d.Todos("pnsealed1:shopping", true)
				if err != nil {
					t.Fatalf("%s: the to-dos cannot be read any more: %v", kind, err)
				}

				found := false

				for _, one := range todos {
					if one.ID == todo.ID {
						found = one.Text == text && one.List == "pnsealed1:shopping"
					}
				}

				if !found {
					t.Errorf("%s: the to-do %q did not come back as written: %+v", kind, text, todos)
				}

				if _, err := d.AddReminder(text, time.Now().Add(time.Hour)); err != nil {
					t.Fatalf("%s: %v", kind, err)
				}

				id, err := d.RecordInvocationIn(conv, "read_file", text, text, text, "safe")
				if err != nil {
					t.Fatalf("%s: %v", kind, err)
				}

				asked, err := d.Invocation(id)
				if err != nil {
					t.Fatalf("%s: the question cannot be read any more: %v", kind, err)
				}

				if asked.Arguments != text || asked.Summary != text || asked.SummaryBG != text {
					t.Errorf("%s: the question came back as %+v", kind, asked)
				}

				if _, err := d.AddDeed(Deed{ConversationID: conv, Tool: "fetch_page", Summary: text, SummaryBG: text,
					Asked: text, Request: text, Result: text, Outcome: "done"}); err != nil {
					t.Fatalf("%s: %v", kind, err)
				}

				deeds, err := d.Deeds(DeedFilter{Conversation: conv})
				if err != nil {
					t.Fatalf("%s: the record cannot be read any more: %v", kind, err)
				}

				if got := deeds[0]; got.Summary != text || got.SummaryBG != text || got.Result != text {
					t.Errorf("%s: the record came back as %+v", kind, got)
				}
			}

			if _, err := d.PendingInvocations(); err != nil {
				t.Errorf("the waiting questions cannot be read: %v", err)
			}

			latest, err := d.LatestConversation()
			if err != nil || latest == nil || latest.Title != "pnsealed1:a title" {
				t.Errorf("the conversation's title: %+v %v", latest, err)
			}
		})
	}
}

// In a sealed brain such text is sealed like any other: nothing of it can be
// read in the file.
func TestTextThatLooksSealedIsStillSealedOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	conv, _ := d.NewConversation("x")

	for _, text := range []string{"pnsealed1:the code is LOOKALIKE-ZZQ-4471", "pnplain1:the code is LOOKALIKE-ZZQ-4471"} {
		if _, err := d.AddMessage(conv, "user", "", "", text); err != nil {
			t.Fatal(err)
		}

		if _, err := d.AddTodo("list", text, nil); err != nil {
			t.Fatal(err)
		}
	}

	d.Close()

	for _, f := range []string{path, path + "-wal"} {
		raw, _ := os.ReadFile(f)

		if bytes.Contains(raw, []byte("LOOKALIKE-ZZQ")) {
			t.Errorf("text that began with the mark is readable in %s", filepath.Base(f))
		}
	}
}

/*
 * Every personal column of every table, one by one, so a table added later
 * is covered the day it is listed: a row is written with such text in the
 * column, read back, and the whole table read after it.
 */
func TestEverySealedColumnKeepsTextThatLooksSealed(t *testing.T) {
	for name, opened := range brains(t) {
		t.Run(name, func(t *testing.T) {
			d, key, _ := opened(t)
			ctx := context.Background()

			// One connection with the links between tables not enforced:
			// this is about columns, not about which row belongs to which.
			conn, err := d.sql().Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
				t.Fatal(err)
			}

			if _, err := conn.ExecContext(ctx, `PRAGMA ignore_check_constraints = ON`); err != nil {
				t.Fatal(err)
			}

			defer conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
			defer conn.ExecContext(ctx, `PRAGMA ignore_check_constraints = OFF`)

			texts := lookalikes(t, key)
			checked := 0

			for _, table := range sealedColumns {
				type column struct {
					name, kind string
					key        bool
				}

				var columns []column

				rows, err := conn.QueryContext(ctx, `SELECT name, type, pk FROM pragma_table_info(?)`, table.table)
				if err != nil {
					t.Fatal(err)
				}

				for rows.Next() {
					var (
						c  column
						pk int
					)

					if err := rows.Scan(&c.name, &c.kind, &pk); err != nil {
						t.Fatal(err)
					}

					c.key = pk > 0 && strings.EqualFold(c.kind, "INTEGER")
					columns = append(columns, c)
				}

				rows.Close()

				if len(columns) == 0 {
					t.Fatalf("the table %s is listed as sealed and does not exist", table.table)
				}

				personal := map[string]bool{}
				for _, c := range append(append([]string{}, table.text...), table.blob...) {
					personal[c] = true
				}

				number := 0

				write := func(target string, value any) {
					number++

					var (
						names  []string
						values []any
					)

					for _, c := range columns {
						if c.key {
							continue
						}

						names = append(names, c.name)

						switch {
						case c.name == target:
							values = append(values, value)
						case personal[c.name]:
							values = append(values, d.seal(fmt.Sprintf("plain %d", number)))
						case strings.Contains(strings.ToUpper(c.kind), "INT"), strings.Contains(strings.ToUpper(c.kind), "REAL"):
							values = append(values, number)
						default:
							// Distinct, for the columns that must be unique.
							values = append(values, fmt.Sprintf("v%d-%s", number, time.Now().UTC().Format(time.RFC3339)))
						}
					}

					marks := strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", ")

					if _, err := conn.ExecContext(ctx, `INSERT INTO `+table.table+` (`+strings.Join(names, ", ")+
						`) VALUES (`+marks+`)`, values...); err != nil {
						t.Fatalf("%s.%s: writing: %v", table.table, target, err)
					}
				}

				for _, c := range table.text {
					for kind, text := range texts {
						write(c, d.seal(text))

						var got string

						if err := conn.QueryRowContext(ctx, `SELECT `+c+` FROM `+table.table+
							` ORDER BY rowid DESC LIMIT 1`).Scan(&got); err != nil {
							t.Fatalf("%s.%s, %s: cannot be read back: %v", table.table, c, kind, err)
						}

						if got != text {
							t.Errorf("%s.%s, %s: %q came back as %q", table.table, c, kind, text, got)
						}

						checked++
					}
				}

				for _, c := range table.blob {
					for kind, text := range texts {
						write(c, d.sealBlob([]byte(text)))

						var got []byte

						if err := conn.QueryRowContext(ctx, `SELECT `+c+` FROM `+table.table+
							` ORDER BY rowid DESC LIMIT 1`).Scan(&got); err != nil {
							t.Fatalf("%s.%s, %s: cannot be read back: %v", table.table, c, kind, err)
						}

						if string(got) != text {
							t.Errorf("%s.%s, %s: %q came back as %q", table.table, c, kind, text, got)
						}

						checked++
					}
				}

				// And the table as a whole still reads, every column of it.
				all, err := conn.QueryContext(ctx, `SELECT * FROM `+table.table)
				if err != nil {
					t.Fatalf("%s: %v", table.table, err)
				}

				seen := 0

				for all.Next() {
					seen++
				}

				if err := all.Err(); err != nil {
					t.Errorf("%s cannot be read any more: %v", table.table, err)
				}

				all.Close()

				if seen != number {
					t.Errorf("%s: %d rows read of %d written", table.table, seen, number)
				}
			}

			listed := 0
			for _, table := range sealedColumns {
				listed += len(table.text) + len(table.blob)
			}

			if checked != listed*len(texts) || listed < 80 {
				t.Errorf("%d checks for %d columns and %d texts", checked, listed, len(texts))
			}
		})
	}
}

/*
 * The same for a column that is not sealed and for anything handed to a
 * query, whatever Go type carries it: the text is found by itself again and
 * read as written.
 */
func TestAnyColumnKeepsTextThatLooksSealed(t *testing.T) {
	for name, opened := range brains(t) {
		t.Run(name, func(t *testing.T) {
			d, key, _ := opened(t)

			if _, err := d.sql().Exec(`CREATE TABLE zz_any (id INTEGER PRIMARY KEY, kind TEXT, t TEXT, b BLOB)`); err != nil {
				t.Fatal(err)
			}

			type named string

			for kind, text := range lookalikes(t, key) {
				pointer := text

				for how, value := range map[string]any{
					"a string":          text,
					"a named string":    named(text),
					"a pointer":         &pointer,
					"a nullable string": sql.NullString{String: text, Valid: true},
				} {
					if _, err := d.sql().Exec(`INSERT INTO zz_any (kind, t) VALUES (?, ?)`, kind+" as "+how, value); err != nil {
						t.Fatalf("%s as %s: %v", kind, how, err)
					}

					var got string

					if err := d.sql().QueryRow(`SELECT t FROM zz_any WHERE t = ? AND kind = ?`, value, kind+" as "+how).Scan(&got); err != nil {
						t.Fatalf("%s as %s: not found by itself: %v", kind, how, err)
					}

					if got != text {
						t.Errorf("%s as %s: %q came back as %q", kind, how, text, got)
					}
				}

				if _, err := d.sql().Exec(`INSERT INTO zz_any (kind, b) VALUES (?, ?)`, kind+" as bytes", []byte(text)); err != nil {
					t.Fatal(err)
				}

				var got []byte

				if err := d.sql().QueryRow(`SELECT b FROM zz_any WHERE kind = ?`, kind+" as bytes").Scan(&got); err != nil {
					t.Fatalf("%s as bytes: %v", kind, err)
				}

				if string(got) != text {
					t.Errorf("%s as bytes: %q came back as %q", kind, text, got)
				}
			}

			// In a transaction and through a prepared statement as well.
			tx, err := d.sql().Begin()
			if err != nil {
				t.Fatal(err)
			}

			stmt, err := tx.Prepare(`INSERT INTO zz_any (kind, t) VALUES ('prepared', ?)`)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := stmt.Exec("pnsealed1:prepared"); err != nil {
				t.Fatal(err)
			}

			stmt.Close()

			var got string

			if err := tx.QueryRow(`SELECT t FROM zz_any WHERE kind = 'prepared'`).Scan(&got); err != nil || got != "pnsealed1:prepared" {
				t.Errorf("through a prepared statement: %q %v", got, err)
			}

			tx.Commit()

			rows, err := d.sql().Query(`SELECT kind, t, b FROM zz_any`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()

			for rows.Next() {
				var (
					kind string
					text sql.NullString
					blob []byte
				)

				if err := rows.Scan(&kind, &text, &blob); err != nil {
					t.Fatalf("the table cannot be read any more: %v", err)
				}
			}

			if err := rows.Err(); err != nil {
				t.Fatalf("the table cannot be read any more: %v", err)
			}
		})
	}
}

// rawly opens the file the way anything but this program would: no opening
// of sealed values, nothing kept apart on the way in.
func rawly(t *testing.T, path string) *sql.DB {
	t.Helper()

	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { raw.Close() })

	return raw
}

/*
 * A brain written in the clear by an older version holds such text as it
 * came. Sealing it in place seals that text too instead of taking it for
 * sealed already, and it reads back as it was.
 */
func TestSealingInPlaceSealsTextThatLooksSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	plain, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	conv := fill(t, plain, 3)
	plain.Close()

	old := rawly(t, path)

	for _, text := range []string{"pnsealed1:x", "pnsealed1:", "pnsealed1:the code is LOOKALIKE-ZZQ-4471", "pnsealed1:Здравей"} {
		if _, err := old.Exec(`INSERT INTO messages (conversation_id, role, content, created_at, updated_at) VALUES (?, 'assistant', ?, ?, ?)`,
			conv, text, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := old.Exec(`UPDATE messages SET model = 'pnsealed1:model-name' WHERE role = 'assistant'`); err != nil {
		t.Fatal(err)
	}

	old.Close()

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if _, err := d.SealInPlace(context.Background()); err != nil {
		t.Fatal(err)
	}

	history, err := d.History(conv)
	if err != nil {
		t.Fatalf("the conversation cannot be read after sealing: %v", err)
	}

	var said []string

	for _, m := range history {
		if m.Role == "assistant" {
			said = append(said, m.Content)

			if m.Model != "pnsealed1:model-name" {
				t.Errorf("a column that is not sealed came back as %q", m.Model)
			}
		}
	}

	if strings.Join(said, "|") != "pnsealed1:x|pnsealed1:|pnsealed1:the code is LOOKALIKE-ZZQ-4471|pnsealed1:Здравей" {
		t.Errorf("came back as %q", said)
	}

	checkReadsBack(t, d, conv, 3)

	d.Close()

	for _, f := range []string{path, path + "-wal"} {
		raw, _ := os.ReadFile(f)

		if bytes.Contains(raw, []byte("LOOKALIKE-ZZQ")) {
			t.Errorf("text that began with the mark was left readable in %s", filepath.Base(f))
		}
	}
}

/*
 * A sealed brain an older version wrote into: that version took text
 * beginning with the mark for sealed and wrote it as it came, and from then
 * on the table could not be read. The next start puts it right once: the
 * text is sealed and reads as it was written. A sealed value that was
 * changed is not touched by this and still fails the read.
 */
func TestWhatAnOlderVersionLeftUnreadableIsPutRightOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")
	k := sealKey(t)

	d, err := OpenSealed(path, k)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.SealInPlace(context.Background()); err != nil {
		t.Fatal(err)
	}

	conv := fill(t, d, 2)

	// Three more conversations, each with one sealed message to damage.
	damaged := map[string]int64{"one character changed": 0, "a character no sealed text has": 0, "cut short": 0}
	for kind := range damaged {
		damaged[kind] = fill(t, d, 1)
	}

	unsealed := fill(t, d, 1)
	d.Close()

	old := rawly(t, path)
	now := time.Now().UTC().Format(time.RFC3339)

	for _, text := range []string{"pnsealed1:x", "pnsealed1:the code is LOOKALIKE-ZZQ-4471"} {
		if _, err := old.Exec(`INSERT INTO messages (conversation_id, role, content, created_at, updated_at) VALUES (?, 'assistant', ?, ?, ?)`,
			conv, text, now, now); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := old.Exec(`INSERT INTO todos (list, text, created_at) VALUES ('pnsealed1:list', 'pnsealed1:milk', ?)`, now); err != nil {
		t.Fatal(err)
	}

	// The older version knew nothing of this tidy and never wrote its name.
	if _, err := old.Exec(`DELETE FROM chores WHERE name = ?`, lookalikesPutRight); err != nil {
		t.Fatal(err)
	}

	// The same kind of text in columns that are not sealed: every read of
	// their tables failed on it as well.
	if _, err := old.Exec(`UPDATE messages SET model = 'pnsealed1:model-name', provider = ? WHERE conversation_id = ?`,
		k.SealString("looks entirely sealed"), unsealed); err != nil {
		t.Fatal(err)
	}

	// And sealed values that were damaged: these are not text, and must go
	// on failing the read whatever is put right around them.
	for kind, id := range damaged {
		var stored string

		if err := old.QueryRow(`SELECT content FROM messages WHERE conversation_id = ? AND role = 'user'`, id).Scan(&stored); err != nil {
			t.Fatal(err)
		}

		changed := []byte(stored)

		switch kind {
		case "one character changed":
			at := len(changed) - 3
			if changed[at] == 'B' {
				changed[at] = 'C'
			} else {
				changed[at] = 'B'
			}
		case "a character no sealed text has":
			changed[len(changed)-3] = '!'
		case "cut short":
			changed = changed[:len(vault.Prefix)+14]
		}

		if _, err := old.Exec(`UPDATE messages SET content = ? WHERE conversation_id = ? AND role = 'user'`, string(changed), id); err != nil {
			t.Fatal(err)
		}
	}

	old.Close()

	// As the fault looks before the start that puts it right.
	broken, err := OpenSealed(path, k)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := broken.History(conv); err == nil {
		t.Fatal("this test no longer builds what an older version left behind")
	}

	broken.Close()

	for start := 1; start <= 2; start++ {
		d, err = OpenSealed(path, k)
		if err != nil {
			t.Fatal(err)
		}

		done, err := d.SealInPlace(context.Background())
		if err != nil {
			t.Fatalf("start %d: %v", start, err)
		}

		if start == 1 && done.PutRight != 4 {
			t.Errorf("the first start put %d records right, want 4", done.PutRight)
		}

		if start == 2 && done.PutRight != 0 {
			t.Errorf("the second start changed %d records", done.PutRight)
		}

		// Done again from the beginning, as after a power cut before its
		// name was written down: nothing is sealed twice.
		if start == 2 {
			if _, err := d.sql().Exec(`DELETE FROM chores WHERE name = ?`, lookalikesPutRight); err != nil {
				t.Fatal(err)
			}

			if again, err := d.SealInPlace(context.Background()); err != nil || again.PutRight != 0 {
				t.Errorf("done again it changed %d records (%v)", again.PutRight, err)
			}
		}

		history, err := d.History(conv)
		if err != nil {
			t.Fatalf("start %d: the conversation is still unreadable: %v", start, err)
		}

		var said []string

		for _, m := range history {
			if m.Role == "assistant" {
				said = append(said, m.Content)
			}
		}

		if strings.Join(said, "|") != "pnsealed1:x|pnsealed1:the code is LOOKALIKE-ZZQ-4471" {
			t.Errorf("start %d: came back as %q", start, said)
		}

		todos, err := d.Todos("pnsealed1:list", true)
		if err != nil || len(todos) != 1 || todos[0].Text != "pnsealed1:milk" {
			t.Errorf("start %d: the to-do: %+v %v", start, todos, err)
		}

		for kind, id := range damaged {
			if _, err := d.History(id); err == nil {
				t.Errorf("start %d: a sealed value with %s was read as if nothing had happened", start, kind)
			}
		}

		there, err := d.History(unsealed)
		if err != nil {
			t.Fatalf("start %d: the conversation with such text in a column that is not sealed: %v", start, err)
		}

		if m := there[0]; m.Model != "pnsealed1:model-name" || !strings.HasPrefix(m.Provider, vault.Prefix) || len(m.Provider) < 40 {
			t.Errorf("start %d: those columns came back as %q and %q", start, m.Model, m.Provider)
		}

		d.Close()
	}

	for _, f := range []string{path, path + "-wal"} {
		raw, _ := os.ReadFile(f)

		if bytes.Contains(raw, []byte("LOOKALIKE-ZZQ")) {
			t.Errorf("after being put right the text is readable in %s", filepath.Base(f))
		}
	}
}

// A value sealed by another brain's key, lying in this brain's file, still
// fails the read: only text handed over through the program is kept as text.
func TestAnotherBrainsValueInTheFileStillFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	conv := fill(t, d, 1)
	d.Close()

	old := rawly(t, path)

	if _, err := old.Exec(`UPDATE messages SET content = ? WHERE role = 'user'`, sealKey(t).SealString("moved in from elsewhere")); err != nil {
		t.Fatal(err)
	}

	old.Close()

	d, err = OpenSealed(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if _, err := d.History(conv); err == nil {
		t.Error("another brain's sealed value was read with no key")
	}
}

// Any text at all, in any column: what goes in comes out.
func TestWhateverIsWrittenIsReadBack(t *testing.T) {
	for name, opened := range brains(t) {
		t.Run(name, func(t *testing.T) {
			d, _, _ := opened(t)

			if _, err := d.sql().Exec(`CREATE TABLE zz_quick (id INTEGER PRIMARY KEY, sealed TEXT, plain TEXT, bytes BLOB)`); err != nil {
				t.Fatal(err)
			}

			marks := []string{"", vault.Prefix, vault.Kept, "\x89PNSEAL\x01", "\x89PNPLAIN\x01", "pn", "pnsealed1", " pnsealed1:"}

			check := func(a, b uint8, rest string) bool {
				// Go text only: SQLite's own handling of other bytes in a
				// TEXT column is not what is being checked.
				text := strings.ToValidUTF8(marks[int(a)%len(marks)]+marks[int(b)%len(marks)]+rest, "?")
				text = strings.ReplaceAll(text, "\x00", "0")

				res, err := d.sql().Exec(`INSERT INTO zz_quick (sealed, plain, bytes) VALUES (?, ?, ?)`,
					d.seal(text), text, []byte(text))
				if err != nil {
					t.Logf("%q: %v", text, err)

					return false
				}

				id, _ := res.LastInsertId()

				var (
					sealed, plain string
					raw           []byte
				)

				if err := d.sql().QueryRow(`SELECT sealed, plain, bytes FROM zz_quick WHERE id = ?`, id).Scan(&sealed, &plain, &raw); err != nil {
					t.Logf("%q: %v", text, err)

					return false
				}

				if sealed != text || plain != text || string(raw) != text {
					t.Logf("%q came back as %q, %q, %q", text, sealed, plain, raw)

					return false
				}

				return true
			}

			if err := quick.Check(check, &quick.Config{MaxCount: 400}); err != nil {
				t.Error(err)
			}
		})
	}
}

// Who was hired for a task, a line each: a line that begins with a mark is
// kept as written among the others.
func TestAHireLineThatLooksSealedIsKeptAsWritten(t *testing.T) {
	for name, opened := range brains(t) {
		t.Run(name, func(t *testing.T) {
			d, _, _ := opened(t)

			id, err := d.NewTask(Task{Name: "a task", Goal: "a goal"})
			if err != nil {
				t.Fatal(err)
			}

			lines := []string{"pnsealed1:Welder: needed", "pnsealed1:Title: why", "Driver: for Tuesday", "pnplain1:Cook: lunch"}

			for _, line := range lines {
				if err := d.RecordHire(id, line); err != nil {
					t.Fatal(err)
				}
			}

			task, err := d.Task(id)
			if err != nil || task == nil {
				t.Fatalf("the task cannot be read any more: %v", err)
			}

			if task.Hired != strings.Join(lines, "\n") {
				t.Errorf("the hires came back as %q", task.Hired)
			}
		})
	}
}

// What is not text passes as it always did, and a brain opened again after
// its drive came back keeps such text the same way.
func TestOtherArgumentsAndAReopenedBrain(t *testing.T) {
	for name, opened := range brains(t) {
		t.Run(name, func(t *testing.T) {
			d, _, _ := opened(t)

			if _, err := d.sql().Exec(`CREATE TABLE zz_kinds (id INTEGER PRIMARY KEY, at TIMESTAMP, yes INTEGER, none TEXT, empty BLOB, raw TEXT, n REAL)`); err != nil {
				t.Fatal(err)
			}

			at := time.Date(2026, 11, 19, 22, 0, 0, 0, time.UTC)

			if _, err := d.sql().Exec(`INSERT INTO zz_kinds (at, yes, none, empty, raw, n) VALUES (?, ?, ?, ?, ?, ?)`,
				at, true, nil, []byte(nil), json.RawMessage(`{"pnsealed1:":1}`), 2.5); err != nil {
				t.Fatal(err)
			}

			var (
				gotAt time.Time
				yes   bool
				none  sql.NullString
				empty []byte
				raw   string
				n     float64
			)

			if err := d.sql().QueryRow(`SELECT at, yes, none, empty, raw, n FROM zz_kinds`).Scan(&gotAt, &yes, &none, &empty, &raw, &n); err != nil {
				t.Fatal(err)
			}

			if !gotAt.Equal(at) || !yes || none.Valid || len(empty) != 0 || raw != `{"pnsealed1:":1}` || n != 2.5 {
				t.Errorf("came back as %v %v %v %q %q %v", gotAt, yes, none, empty, raw, n)
			}

			if err := d.Reopen(); err != nil {
				t.Fatal(err)
			}

			todo, err := d.AddTodo("list", "pnsealed1:after the drive came back", nil)
			if err != nil {
				t.Fatal(err)
			}

			todos, err := d.Todos("list", true)
			if err != nil || len(todos) != 1 || todos[0].ID != todo.ID || todos[0].Text != "pnsealed1:after the drive came back" {
				t.Errorf("after reopening: %+v %v", todos, err)
			}
		})
	}
}

// A brain written with no key by this version, such text in it kept apart
// already, and sealed in place afterwards: the text reads as it was written.
func TestTextKeptWithNoKeyReadsTheSameOnceSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	plain, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	texts := []string{"pnsealed1:x", "pnplain1:hello", "pnsealed1:the code is LOOKALIKE-ZZQ-4471", "ordinary"}

	for _, text := range texts {
		if _, err := plain.AddTodo("pnsealed1:list", text, nil); err != nil {
			t.Fatal(err)
		}
	}

	plain.Close()

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if _, err := d.SealInPlace(context.Background()); err != nil {
		t.Fatal(err)
	}

	todos, err := d.Todos("pnsealed1:list", true)
	if err != nil || len(todos) != len(texts) {
		t.Fatalf("the to-dos after sealing: %+v %v", todos, err)
	}

	for i, text := range texts {
		if todos[i].Text != text {
			t.Errorf("%q came back as %q", text, todos[i].Text)
		}
	}
}

/*
 * The one-time tidy goes through every table there is, so one it cannot
 * read must not keep the program from starting: that table is left, the
 * others are put right, and the tidy is tried again at the next start.
 */
func TestATableTheTidyCannotReadDoesNotStopTheStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")
	k := sealKey(t)

	d, err := OpenSealed(path, k)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.SealInPlace(context.Background()); err != nil {
		t.Fatal(err)
	}

	d.Close()

	old := rawly(t, path)
	now := time.Now().UTC().Format(time.RFC3339)

	// A column of its own called rowid, holding words: no row of it can be
	// named the way the tidy names rows.
	for _, q := range []string{
		`CREATE TABLE zz_odd (rowid TEXT, t TEXT)`,
		`INSERT INTO zz_odd (rowid, t) VALUES ('first', 'pnsealed1:x')`,
		`INSERT INTO todos (list, text, created_at) VALUES ('list', 'pnsealed1:milk', '` + now + `')`,
		`DELETE FROM chores`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	old.Close()

	d, err = OpenSealed(path, k)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	done, err := d.SealInPlace(context.Background())
	if err != nil {
		t.Fatalf("the start was refused over one table: %v", err)
	}

	if done.PutRight != 1 || len(done.LeftAsFound) != 1 || !strings.Contains(done.LeftAsFound[0], "zz_odd") {
		t.Errorf("put right %d, left as found %q", done.PutRight, done.LeftAsFound)
	}

	todos, err := d.Todos("list", true)
	if err != nil || len(todos) != 1 || todos[0].Text != "pnsealed1:milk" {
		t.Errorf("the to-do: %+v %v", todos, err)
	}

	var written int

	d.sql().QueryRow(`SELECT COUNT(*) FROM chores WHERE name = ?`, lookalikesPutRight).Scan(&written)

	if written != 0 {
		t.Error("the tidy was written down as done with a table left as found")
	}
}

// The tidy names rows by rowid, and no table of a person's words is without
// one: a table made WITHOUT ROWID later would be passed over in silence.
func TestEveryTableHasARowid(t *testing.T) {
	d := open(t)

	tables, err := d.names(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil || len(tables) < 30 {
		t.Fatalf("%d tables: %v", len(tables), err)
	}

	for _, table := range tables {
		var n int

		if err := d.sql().QueryRow(`SELECT COUNT(rowid) FROM "` + table + `"`).Scan(&n); err != nil {
			t.Errorf("%s: %v", table, err)
		}
	}
}
