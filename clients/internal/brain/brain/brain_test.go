package brain

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-brain/internal/brain/config"

	"pn-brain/internal/brain/store"
)

// Reply time on a CPU is close to linear in prompt size, and recall is the
// biggest part of that prompt.
func TestRecallIsTrimmedForThePrompt(t *testing.T) {
	long := `Petar has a PHP/Composer project called "ps_emailsubscription" at ` +
		`/media/petar/DEV/Projects/divacon.bg/modules/ps_emailsubscription, last modified 2025-08-19. ` +
		`README excerpt: # E-mail subscription form [![Build Status](https://travis-ci.com/x.svg)](https://t) ` +
		strings.Repeat("more badge noise ", 40)

	out := formatRecall([]store.Scored{{Fact: store.Fact{Content: long}}})

	if len([]rune(out)) > RecalledFactLimit+200 {
		t.Errorf("recall preamble is %d runes; trimming did not happen", len([]rune(out)))
	}

	// The part that answers questions must survive.
	for _, want := range []string{"ps_emailsubscription", "/media/petar/DEV/Projects", "2025-08-19"} {
		if !strings.Contains(out, want) {
			t.Errorf("trimming removed %q, which is the useful part", want)
		}
	}

	// The part that is noise must not.
	if strings.Contains(out, "travis-ci") {
		t.Error("a build-status badge reached the prompt")
	}
}

// A fact cut mid-word reads as though it were complete and slightly wrong.
func TestTrimPrefersASentenceBoundary(t *testing.T) {
	s := "Petar has a Go project called xplorer at /home/petar/x, last modified 2026-01-01. " +
		strings.Repeat("trailing detail that should go away ", 20)

	got := trimForPrompt(s, 120)

	if !strings.HasSuffix(got, ".") {
		t.Errorf("did not end at a sentence: %q", got)
	}

	if strings.Contains(got, "trailing detail") {
		t.Errorf("kept the tail: %q", got)
	}
}

// Short facts must pass through untouched, or every website fact would gain a
// pointless ellipsis.
func TestShortFactsAreNotTrimmed(t *testing.T) {
	s := "Petar regularly uses the website pnscripts.local (1160 visits in browser history)."

	if got := trimForPrompt(s, RecalledFactLimit); got != s {
		t.Errorf("a short fact was altered:\n  got  %q\n  want %q", got, s)
	}
}

// A greeting is composed rather than generated, so it must arrive instantly.
// Asking the model would cost most of a minute before it said hello.
func TestGreetingIsImmediate(t *testing.T) {
	b := testBrain(t)

	started := time.Now()
	greeting := b.Greet()

	if took := time.Since(started); took > 200*time.Millisecond {
		t.Errorf("greeting took %v; it must not wait on a model", took)
	}

	if greeting.Text == "" {
		t.Fatal("greeting is empty")
	}
}

// What is worth saying on opening is what is waiting, not that it exists.
func TestGreetingMentionsWhatNeedsTheOwner(t *testing.T) {
	b := testBrain(t)

	b.DB.AddLesson(0, "something the model guessed", "proposed", "low", "")
	b.DB.AddLesson(0, "another guess", "proposed", "low", "")

	text := b.Greet().Text

	if !strings.Contains(text, "2") {
		t.Errorf("greeting does not mention the two waiting lessons: %q", text)
	}
}

// With nothing waiting it should say what it has, not just announce itself.
func TestGreetingFallsBackToWhatItKnows(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < 3; i++ {
		b.DB.AddFact("project", "a thing it knows", []float32{1, 0, 0})
	}

	text := b.Greet().Text

	if !strings.Contains(text, "3") {
		t.Errorf("greeting does not mention what it knows: %q", text)
	}
}

// An empty brain should say so and point somewhere, rather than claim to know
// nothing useful and stop.
func TestEmptyBrainSaysHowToTeachIt(t *testing.T) {
	b := testBrain(t)

	text := b.Greet().Text

	if !strings.Contains(strings.ToLower(text), "folder") {
		t.Errorf("greeting gives an empty brain nothing to act on: %q", text)
	}
}

// testBrain is an otherwise empty brain on a temporary database.
func testBrain(t *testing.T) *Brain {
	t.Helper()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	cfg := config.Default()
	cfg.Owner = "Petar"

	return New(db, cfg, root, filepath.Join(root, "brain.sqlite"),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}
