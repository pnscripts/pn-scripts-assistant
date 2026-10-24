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

	db.AddDeed(Deed{ConversationID: conv, Tool: "read_file", Summary: "Read notes",
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

	if got[0].Summary != "Read notes" {
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
