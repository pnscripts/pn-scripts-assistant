package store

import (
	"errors"
	"testing"
	"time"
)

func TestToDoListsKeepOrderAndTicks(t *testing.T) {
	db := openDeeds(t)

	friday := time.Date(2026, 10, 30, 0, 0, 0, 0, time.Local)
	monday := time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local)

	milk, _ := db.AddTodo("Shopping", "Мляко", nil)
	_, _ = db.AddTodo("work", "Send the offer", &friday)
	tax, _ := db.AddTodo(" WORK ", "Pay the tax", &monday)
	_, _ = db.AddTodo("", "Call grandma", nil)

	if _, err := db.AddTodo("x", "  ", nil); err == nil {
		t.Error("an empty item was kept")
	}

	work, _ := db.Todos("Work", false)
	if len(work) != 2 || work[0].ID != tax.ID || work[0].List != "work" {
		t.Fatalf("%+v", work)
	}

	all, _ := db.Todos("", false)
	if len(all) != 4 || all[0].ID != tax.ID || all[2].ID != milk.ID {
		t.Fatalf("due first, then oldest: %+v", all)
	}

	if done, err := db.TickTodo(milk.ID, true); err != nil || done.DoneAt == nil {
		t.Fatal(done, err)
	}

	if open, _ := db.Todos("shopping", false); len(open) != 0 {
		t.Error("a ticked item is still open")
	}

	if withDone, _ := db.Todos("shopping", true); len(withDone) != 1 {
		t.Error("a ticked item was not kept")
	}

	lists, _ := db.TodoLists()
	if lists["work"] != 2 || lists[DefaultList] != 1 || lists["shopping"] != 0 {
		t.Errorf("%v", lists)
	}

	if err := db.RemoveTodo(tax.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.RemoveTodo(tax.ID); !errors.Is(err, ErrNoTodo) {
		t.Error("removing twice did not say so")
	}
}
