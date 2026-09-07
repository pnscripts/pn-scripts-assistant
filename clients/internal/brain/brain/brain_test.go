package brain

import (
	"context"
	"crypto/sha256"
	"fmt"
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

// The greeting says four things are waiting, so "remember them" is the obvious
// next sentence. Before this the model answered "I will remember these
// instructions" and did nothing at all.
func TestRememberThemActuallyRemembers(t *testing.T) {
	b := testBrain(t)
	b.Learner.Curator.Embedder = fixedEmbedder{}

	b.DB.AddLesson(0, "Petar prefers Laravel over Python", "proposed", "high", "")
	b.DB.AddLesson(0, "Petar works from Sofia", "proposed", "high", "")

	answer, handled := b.handleLessonInstruction(context.Background(), "Okay, remember them.")

	if !handled {
		t.Fatal("the instruction was not recognised")
	}

	if !strings.Contains(answer, "2") {
		t.Errorf("answer does not say what happened: %q", answer)
	}

	pending, _ := b.DB.CountPendingLessons()
	if pending != 0 {
		t.Errorf("%d lessons still waiting after being told to remember them", pending)
	}

	facts, _ := b.DB.CountFacts()
	if facts != 2 {
		t.Errorf("%d facts stored, want 2", facts)
	}
}

func TestForgetThemDiscards(t *testing.T) {
	b := testBrain(t)

	b.DB.AddLesson(0, "a guess", "proposed", "low", "")

	answer, handled := b.handleLessonInstruction(context.Background(), "forget them")

	if !handled {
		t.Fatal("not recognised")
	}

	if !strings.Contains(strings.ToLower(answer), "discard") {
		t.Errorf("answer is %q", answer)
	}

	if pending, _ := b.DB.CountPendingLessons(); pending != 0 {
		t.Errorf("%d still pending", pending)
	}

	if facts, _ := b.DB.CountFacts(); facts != 0 {
		t.Errorf("%d facts stored despite being told to forget", facts)
	}
}

// A false positive silently rewrites what the brain believes about its owner,
// so the match has to be narrow.
func TestOrdinarySentencesAreNotInstructions(t *testing.T) {
	b := testBrain(t)
	b.DB.AddLesson(0, "a guess", "proposed", "low", "")

	notInstructions := []string{
		"Remind me to forget about the meeting tomorrow morning",
		"What do you remember about my Go projects?",
		"I keep them in the drawer next to my desk downstairs",
		"Do you remember when we discussed the Laravel upgrade last week?",
		"Can you help me remember my password",
		"Tell me what you would like to remember and why",
	}

	for _, message := range notInstructions {
		if _, handled := b.handleLessonInstruction(context.Background(), message); handled {
			t.Errorf("treated as an instruction: %q", message)
		}
	}

	if pending, _ := b.DB.CountPendingLessons(); pending != 1 {
		t.Error("the queue was changed by a sentence that was not an instruction")
	}
}

// With nothing waiting, "remember that" is somebody talking about something
// else entirely.
func TestInstructionIsIgnoredWithAnEmptyQueue(t *testing.T) {
	b := testBrain(t)

	if _, handled := b.handleLessonInstruction(context.Background(), "remember them"); handled {
		t.Error("handled an instruction with an empty queue")
	}
}

// fixedEmbedder gives every distinct text its own direction.
//
// Summing character codes was not enough: two sentences that both begin
// "Petar " came out similar enough to be treated as the same fact. A hash
// spreads them properly, which is what a real embedder does for genuinely
// different statements.
type fixedEmbedder struct{}

func (fixedEmbedder) EmbedModel() string { return "fixed" }

func (fixedEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	sum := sha256.Sum256([]byte(text))

	vec := make([]float32, 8)

	for i := range vec {
		vec[i] = float32(sum[i]) - 128
	}

	return vec, nil
}

/*
 * A queue worked a hundred at a time must not report the hundred as the whole.
 *
 * Petar's queue held 9,197 lessons. Every count in the interface and every
 * answer in the conversation was a report on one page of it: the badge read
 * 100 because that is what the fetch returns, and the reply read "all 100"
 * because that is what the batch contained. Both were true of the page and
 * false of the queue, which is the only reading anybody cares about.
 */
func TestALargeQueueSaysWhatIsLeft(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < queuePage+5; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	answer, handled := b.handleLessonInstruction(context.Background(), "forget them")

	if !handled {
		t.Fatal("not recognised")
	}

	left, _ := b.DB.CountPendingLessons()
	if left != 5 {
		t.Fatalf("%d left after one page, want 5", left)
	}

	if !strings.Contains(answer, "5 more are still waiting") {
		t.Errorf("answer does not say what is left: %q", answer)
	}

	// "Discarded all 100" over five that are still there is the same lie in
	// friendlier words.
	if strings.Contains(answer, "all") {
		t.Errorf("called one page of the queue all of it: %q", answer)
	}
}

// The ordinary case must not grow a footnote: four lessons, four gone, nothing
// behind them, so the answer ends where it always did.
func TestAQueueThatIsFinishedSaysNothingMore(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < 4; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	answer, _ := b.handleLessonInstruction(context.Background(), "forget them")

	if strings.Contains(answer, "still waiting") {
		t.Errorf("mentioned a remainder that does not exist: %q", answer)
	}

	if !strings.Contains(answer, "all 4") {
		t.Errorf("would not say all when it really was all: %q", answer)
	}
}
