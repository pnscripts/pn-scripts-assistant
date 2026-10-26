package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheAnswerBook(t *testing.T) {
	db := openDeeds(t)

	if _, err := db.AddCareAnswer(" ", "x"); err == nil {
		t.Error("an answer with no question was kept")
	}

	a, err := db.AddCareAnswer(" Do you deliver? ", "Yes, in 2 days.")
	if err != nil || a.Question != "Do you deliver?" {
		t.Fatalf("%v %+v", err, a)
	}

	b, _ := db.AddCareAnswer("Opening hours?", "9 to 6.")

	if _, err := db.ChangeCareAnswer(a.ID, "Do you deliver to Plovdiv?", "Yes, in 2 working days."); err != nil {
		t.Fatal(err)
	}

	list, _ := db.CareAnswers()
	if len(list) != 2 || list[0].ID != a.ID || list[0].Answer != "Yes, in 2 working days." {
		t.Errorf("%+v", list)
	}

	if err := db.RemoveCareAnswer(b.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.RemoveCareAnswer(b.ID); !errors.Is(err, ErrNoCareAnswer) {
		t.Errorf("%v", err)
	}

	if _, err := db.ChangeCareAnswer(b.ID, "q", "a"); !errors.Is(err, ErrNoCareAnswer) {
		t.Errorf("%v", err)
	}

	if err := db.SetCareProfile(map[string]string{"name": "Green Shop", "tone": "formal"}); err != nil {
		t.Fatal(err)
	}

	_ = db.SetCareProfile(map[string]string{"tone": "", "signature": "Maria"})

	p, _ := db.CareProfile()
	if p["name"] != "Green Shop" || p["tone"] != "" || p["signature"] != "Maria" || len(p) != 2 {
		t.Errorf("%v", p)
	}
}

func TestTheAnswerBookIsSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	_, _ = d.AddCareAnswer("Wholesale price for Petrov Ltd?", "Ask Maria directly.")
	_ = d.SetCareProfile(map[string]string{"signature": "Maria Ivanova, Green Shop"})

	if p, _ := d.CareProfile(); p["signature"] != "Maria Ivanova, Green Shop" {
		t.Fatalf("%v", p)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	for _, word := range []string{"Petrov Ltd", "Ask Maria", "Maria Ivanova"} {
		if strings.Contains(string(raw)+string(wal), word) {
			t.Errorf("%q is readable on disk", word)
		}
	}
}
