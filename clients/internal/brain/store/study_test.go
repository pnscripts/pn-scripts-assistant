package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStudyTopicsAndCards(t *testing.T) {
	db := openDeeds(t)

	fractions, err := db.SaveStudyTopic("  Fractions ", "child", "en", "# Fractions\n…")
	if err != nil {
		t.Fatal(err)
	}

	again, _ := db.SaveStudyTopic("fractions", "child", "bg", "")
	if again.ID != fractions.ID || again.Lesson == "" {
		t.Errorf("the same topic at the same level started again: %+v", again)
	}

	if other, _ := db.SaveStudyTopic("Fractions", "advanced", "en", ""); other.ID == fractions.ID {
		t.Error("another level is another topic")
	}

	if _, err := db.SaveStudyTopic(" ", "child", "en", ""); err == nil {
		t.Error("an empty topic was kept")
	}

	cards, err := db.AddStudyCards(fractions.ID, []StudyCard{
		{Kind: "choice", Question: "Which is bigger?", Options: []string{"1/2", "1/3"}, Answer: "1/2", Explain: "Halves are bigger."},
		{Kind: "number", Question: "1/2 + 1/4 as a decimal?", Answer: "0.75"},
	})
	if err != nil || len(cards) != 2 || cards[0].Box != 1 {
		t.Fatalf("%v %+v", err, cards)
	}

	if _, err := db.AddStudyCards(9999, cards); !errors.Is(err, ErrNoStudy) {
		t.Error("questions were kept for a topic that is not there")
	}

	now := time.Now()

	due, _ := db.DueStudyCards(0, now, 0)
	if len(due) != 2 || due[0].Options[1] != "1/3" {
		t.Fatalf("new questions are due at once: %+v", due)
	}

	marked, err := db.MarkStudyCard(cards[0].ID, true, 2, now.Add(24*time.Hour), now)
	if err != nil || marked.Box != 2 || marked.Right != 1 || marked.LastAt == nil {
		t.Fatalf("%v %+v", err, marked)
	}

	_, _ = db.MarkStudyCard(cards[1].ID, false, 1, now.Add(10*time.Minute), now)

	if due, _ = db.DueStudyCards(fractions.ID, now.Add(time.Hour), 0); len(due) != 1 || due[0].ID != cards[1].ID {
		t.Errorf("an hour later only the missed one is due: %+v", due)
	}

	topics, _ := db.StudyTopics(now.Add(time.Hour))
	if len(topics) != 2 || topics[0].ID != fractions.ID || topics[0].Cards != 2 || topics[0].Due != 1 ||
		topics[0].Right != 1 || topics[0].Wrong != 1 || topics[0].Lesson != "" {
		t.Errorf("the most recently studied first, counted: %+v", topics)
	}

	if found, err := db.FindStudyTopic("FRACTIONS", "child"); err != nil || found.ID != fractions.ID || found.Lesson == "" {
		t.Errorf("%v %+v", err, found)
	}

	if _, err := db.MarkStudyCard(9999, true, 2, now, now); !errors.Is(err, ErrNoStudy) {
		t.Error("a missing question was marked")
	}

	if err := db.RemoveStudyTopic(fractions.ID); err != nil {
		t.Fatal(err)
	}

	if left, _ := db.StudyCards(0); len(left) != 0 {
		t.Errorf("the questions outlived their topic: %+v", left)
	}

	if err := db.RemoveStudyTopic(fractions.ID); !errors.Is(err, ErrNoStudy) {
		t.Error("removing twice did not say so")
	}
}

// What somebody is learning is sealed like the rest of their words.
func TestStudyIsSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	topic, _ := d.SaveStudyTopic("Quadratic equations for the entrance exam", "beginner", "en", "lesson words")
	_, _ = d.AddStudyCards(topic.ID, []StudyCard{{Kind: "short", Question: "Discriminant symbol?", Answer: "D|Δ"}})

	back, _ := d.StudyCards(topic.ID)
	if len(back) != 1 || back[0].Answer != "D|Δ" {
		t.Fatalf("%+v", back)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	for _, word := range []string{"Quadratic", "Discriminant", "lesson words"} {
		if strings.Contains(string(raw)+string(wal), word) {
			t.Errorf("%q is readable on disk", word)
		}
	}
}
