package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openDeeds(t *testing.T) *DB {
	t.Helper()

	db, err := Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return db
}

// What was done is kept with its reason, and the filters find it.
func TestDeedsAreKeptAndFiltered(t *testing.T) {
	db := openDeeds(t)

	conv, _ := db.NewConversation("t")

	for _, d := range []Deed{
		{ConversationID: conv, Tool: "read_file", Summary: "Read notes", Request: "when is it?", Outcome: "done"},
		{ConversationID: conv, Tool: "write_file", Summary: "Write a file", Outcome: "waiting", Changes: true},
		{ConversationID: conv, Tool: "web_search", Summary: "Search", Outcome: "done", Leaves: "web search",
			Asked: "query: token sk-ant-abcdefghijklmnopqrstuvwxyz0123456789"},
	} {
		if _, err := db.AddDeed(d); err != nil {
			t.Fatal(err)
		}
	}

	all, err := db.Deeds(DeedFilter{})
	if err != nil {
		t.Fatal(err)
	}

	if len(all) != 3 || all[0].Tool != "web_search" {
		t.Fatalf("expected three, newest first: %+v", all)
	}

	if strings.Contains(all[0].Asked, "sk-ant-abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("a secret was kept in the record: %q", all[0].Asked)
	}

	if all[2].Request != "when is it?" || all[2].ConversationID != conv {
		t.Errorf("the reason was lost: %+v", all[2])
	}

	changes, _ := db.Deeds(DeedFilter{Changes: true})
	if len(changes) != 1 || changes[0].Tool != "write_file" {
		t.Errorf("changes: %+v", changes)
	}

	leaves, _ := db.Deeds(DeedFilter{Leaves: true})
	if len(leaves) != 1 || leaves[0].Leaves != "web search" {
		t.Errorf("leaves: %+v", leaves)
	}

	if later, _ := db.Deeds(DeedFilter{Since: time.Now().Add(time.Hour)}); len(later) != 0 {
		t.Errorf("the future has %d deeds", len(later))
	}
}

// A call made inside a task is shown with the task, found through its thread.
func TestADeedInATaskNamesTheTask(t *testing.T) {
	db := openDeeds(t)

	conv, _ := db.NewConversation("t")
	work, _ := db.NewTaskThread("work")

	if _, err := db.NewTask(Task{Name: "Tidy the photos", Goal: "g", ConversationID: conv,
		WorkConversationID: work, Provider: "ollama"}); err != nil {
		t.Fatal(err)
	}

	db.AddDeed(Deed{ConversationID: work, Tool: "list_directory", Outcome: "done"})

	got, _ := db.Deeds(DeedFilter{})
	if len(got) != 1 || got[0].TaskName != "Tidy the photos" || got[0].TaskID == 0 {
		t.Errorf("the task was not named: %+v", got)
	}
}

// Deleting a conversation takes its words out of the record and leaves the
// fact that something was done.
func TestForgettingAConversationKeepsTheDeedsButNotTheWords(t *testing.T) {
	db := openDeeds(t)

	conv, _ := db.NewConversation("t")

	db.AddDeed(Deed{ConversationID: conv, Tool: "read_file", Summary: "Read notes", SummaryBG: "Прочитане на notes",
		Request: "my private question", Asked: "path: /home/you/diary.txt", Result: "secret", Outcome: "done"})

	if err := db.DeleteConversation(conv); err != nil {
		t.Fatal(err)
	}

	got, _ := db.Deeds(DeedFilter{})
	if len(got) != 1 {
		t.Fatalf("the deed went: %+v", got)
	}

	if got[0].Request != "" || got[0].Asked != "" || got[0].Result != "" || got[0].ConversationID != 0 {
		t.Errorf("the conversation's words stayed: %+v", got[0])
	}

	if got[0].Summary != "Read notes" || got[0].SummaryBG != "Прочитане на notes" {
		t.Errorf("what was done was lost: %+v", got[0])
	}
}

// An answer's sources come back with the conversation, and go with it.
func TestSourcesTravelWithTheirMessage(t *testing.T) {
	db := openDeeds(t)

	conv, _ := db.NewConversation("t")

	db.AddMessage(conv, "user", "", "", "when?")
	id, _ := db.AddMessage(conv, "assistant", "", "", "Tuesday [1].")

	if err := db.SetSources(id, []map[string]any{{"n": 1, "kind": "document", "title": "notes.txt", "cited": true}}); err != nil {
		t.Fatal(err)
	}

	history, err := db.History(conv)
	if err != nil {
		t.Fatal(err)
	}

	last := history[len(history)-1]

	var sources []map[string]any
	if err := json.Unmarshal(last.Sources, &sources); err != nil || len(sources) != 1 || sources[0]["title"] != "notes.txt" {
		t.Fatalf("sources %s (%v)", last.Sources, err)
	}

	if history[0].Sources != nil {
		t.Errorf("a question has sources: %s", history[0].Sources)
	}

	db.DeleteConversation(conv)

	if raw, _ := db.Sources(id); raw != nil {
		t.Errorf("the sources outlived their message: %s", raw)
	}
}

// The record is kept for half a year, not for ever.
func TestOldDeedsArePruned(t *testing.T) {
	db := openDeeds(t)

	db.AddDeed(Deed{Tool: "read_file", Outcome: "done", CreatedAt: time.Now().Add(-DeedsKept - time.Hour)})
	db.AddDeed(Deed{Tool: "list_directory", Outcome: "done"})

	if n, err := db.PruneDeeds(time.Now().Add(-DeedsKept)); err != nil || n != 1 {
		t.Fatalf("pruned %d (%v)", n, err)
	}

	if left, _ := db.Deeds(DeedFilter{}); len(left) != 1 || left[0].Tool != "list_directory" {
		t.Errorf("left %+v", left)
	}
}

/*
 * What was done is kept in Bulgarian beside the English, like the question
 * it may have started as: the record on a Bulgarian page says the same card
 * the approval showed. Bounded and scrubbed like the English line, and
 * empty for a record written before, which the page shows as it was.
 */
func TestADeedIsKeptInBothLanguages(t *testing.T) {
	db := openDeeds(t)

	db.AddDeed(Deed{Tool: "write_file", Summary: "Create notes.txt", SummaryBG: "Създаване на notes.txt", Outcome: "waiting"})
	db.AddDeed(Deed{Tool: "read_file", Summary: "Read notes.txt", Outcome: "done"})
	db.AddDeed(Deed{Tool: "run_command", Summary: "Run: curl -H 'Authorization: Bearer sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789'",
		SummaryBG: "Изпълнение: curl -H 'Authorization: Bearer sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789' " + strings.Repeat("я", 2000),
		Outcome:   "done"})

	got, err := db.Deeds(DeedFilter{})
	if err != nil || len(got) != 3 {
		t.Fatalf("%+v (%v)", got, err)
	}

	if got[2].SummaryBG != "Създаване на notes.txt" || got[2].Summary != "Create notes.txt" {
		t.Errorf("the card: %+v", got[2])
	}

	if got[1].SummaryBG != "" {
		t.Errorf("a record with no Bulgarian card has one: %+v", got[1])
	}

	if strings.Contains(got[0].SummaryBG, "abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("a key is kept in the Bulgarian card: %q", got[0].SummaryBG)
	}

	if n := len([]rune(got[0].SummaryBG)); n > deedAskedLimit+40 {
		t.Errorf("the Bulgarian card is kept whole, %d letters", n)
	}

	out, _ := json.Marshal(got[2])
	if !strings.Contains(string(out), `"summary_bg":"Създаване на notes.txt"`) {
		t.Errorf("written as %s", out)
	}

	if out, _ := json.Marshal(got[1]); strings.Contains(string(out), "summary_bg") {
		t.Errorf("an empty card is written: %s", out)
	}
}
