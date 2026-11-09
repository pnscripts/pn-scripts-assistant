package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * A tidy of the lessons runs once on a brain written before it existed, and
 * only on the lessons it was given.
 *
 * Made for the review queue 0.13 filled (see the brain's
 * tidyTheQueueOlderVersionsFilled): the same readings of the knowledge
 * folder stored again every ninety seconds. Here, through a sealed brain at
 * the schema 0.13 left (27 migrations): the decision sees the words unsealed,
 * so two sealings of one sentence are recognised as the same, and a second
 * open changes nothing.
 */
func TestALessonTidyRunsOnceOnAnOlderBrain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")
	key := sealKey(t)

	whole := migrations

	t.Cleanup(func() { migrations = whole })

	migrations = whole[:27]

	old, err := OpenSealed(path, key)
	if err != nil {
		t.Fatal(err)
	}

	guide := "reading:@brain/knowledge/guides/mail.md"

	for pass := 0; pass < 3; pass++ {
		for _, l := range []struct{ content, source string }{
			{"Mail is read through IMAP.", guide},
			{"Drafts wait for approval.", guide},
			{"A built-in skill says this.", "reading:@brain/knowledge/pack-skills/books/SKILL.md"},
		} {
			if _, err := old.AddLesson(0, l.content, "proposed", "high", l.source); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Not the tidy's business: somebody's document and a chat lesson.
	old.AddLesson(0, "The lease ends in May.", "proposed", "high", "reading:/home/alex/lease.md")
	old.AddLesson(0, "Alex likes tea.", "proposed", "low", "")

	old.Close()

	// The words are sealed on disk, each copy differently.
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "Drafts wait for approval") {
		t.Fatal("the test brain is not sealed")
	}

	migrations = whole

	db, err := OpenSealed(path, key)
	if err != nil {
		t.Fatalf("opening the 0.13 brain: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	seen := map[string]bool{}

	decide := func(l Lesson) string {
		if strings.Contains(l.Source, "/pack-skills/") {
			return TidyDelete
		}

		if seen[l.Source+"\x00"+l.Content] {
			return TidyDelete
		}

		seen[l.Source+"\x00"+l.Content] = true

		return "validated"
	}

	done, err := db.TidyLessonsOnce("test-chore", "reading:@brain/knowledge/", decide)
	if err != nil {
		t.Fatal(err)
	}

	if done[TidyDelete] != 7 || done["validated"] != 2 {
		t.Errorf("tidied %v, want 7 removed (4 repeats, 3 pack) and 2 validated", done)
	}

	pending, _ := db.LessonsByStatus("proposed", 100)
	if len(pending) != 2 {
		t.Errorf("%d lessons wait for a person, want the document and the chat lesson: %+v", len(pending), pending)
	}

	validated, _ := db.LessonsByStatus("validated", 100)
	if len(validated) != 2 || validated[0].Content != "Mail is read through IMAP." {
		t.Errorf("validated: %+v", validated)
	}

	// Once: a second open finds the chore done and touches nothing.
	again, err := db.TidyLessonsOnce("test-chore", "reading:@brain/knowledge/", func(Lesson) string {
		t.Error("the tidy ran a second time")

		return TidyDelete
	})
	if err != nil || len(again) != 0 {
		t.Errorf("second run: %v %v", again, err)
	}

	// A source with a LIKE wildcard in the prefix is matched as text.
	if _, err := db.TidyLessonsOnce("other", "reading:%", func(Lesson) string { return TidyDelete }); err != nil {
		t.Fatal(err)
	}

	if still, _ := db.LessonsByStatus("proposed", 100); len(still) != 2 {
		t.Errorf("a %% in the prefix matched every lesson: %d left", len(still))
	}
}
