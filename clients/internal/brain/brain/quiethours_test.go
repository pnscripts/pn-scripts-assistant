package brain

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/places"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Work nobody asked for at that moment waits for the quiet hours.
 *
 * A goal that starts itself and the reading of the places the assistant looks
 * after are the heaviest work it does, and nobody is there to say yes to
 * them. With a window set they wait for it and start in it by themselves;
 * with none, or inside it, nothing changes. Real store, real places, real
 * goal loop; the model is a server that refuses, so nothing leaves the
 * machine.
 */

// windowAround is a quiet hours setting relative to the real clock: from
// start to end after now (negative is before), wrapping midnight the way
// people write it. DueGoals reads the real clock, so the window must too.
func windowAround(start, end time.Duration) string {
	now := time.Now()

	return now.Add(start).Format("15:04") + "-" + now.Add(end).Format("15:04")
}

// outside is a window that opens in two hours; inside, one open now.
var (
	outside = func() string { return windowAround(2*time.Hour, 4*time.Hour) }
	inside  = func() string { return windowAround(-time.Hour, time.Hour) }
)

// refusingBrain is a test brain whose model refuses every call at once.
func refusingBrain(t *testing.T) *Brain {
	t.Helper()

	refuses := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no model here", http.StatusServiceUnavailable)
	}))
	t.Cleanup(refuses.Close)

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	cfg := config.Default()
	cfg.Owner = "Alex"
	cfg.OllamaURL = refuses.URL
	cfg.Privacy = "private"

	return New(db, cfg, root, filepath.Join(root, "brain.sqlite"),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func aDueGoal(t *testing.T, b *Brain, startsItself bool) store.Goal {
	t.Helper()

	id, err := b.DB.NewGoal(store.Goal{Name: "Tidy the garden notes", EveryDays: 7, StartsItself: startsItself})
	if err != nil {
		t.Fatal(err)
	}

	// Due a minute ago, so the store counts it due by the real clock.
	if _, err := b.DB.SQL().Exec(`UPDATE goals SET next_due = ? WHERE id = ?`,
		time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), id); err != nil {
		t.Fatal(err)
	}

	g, err := b.DB.Goal(id)
	if err != nil || g == nil || !g.Due {
		t.Fatalf("the goal is not due: %+v %v", g, err)
	}

	return *g
}

func TestASelfStartingGoalWaitsForTheQuietHours(t *testing.T) {
	b := refusingBrain(t)
	goal := aDueGoal(t, b, true)

	b.SetQuietHours(outside())
	b.startDueGoals(context.Background())

	after, _ := b.DB.Goal(goal.ID)
	if !after.LastWorked.IsZero() || !after.Due {
		t.Fatalf("a goal that starts itself was started outside the quiet hours: %+v", after)
	}

	if live, _ := b.DB.LiveTasks(); len(live) > 0 {
		t.Fatalf("work began outside the quiet hours: %+v", live)
	}

	// The window opens: the same loop starts it by itself.
	b.SetQuietHours(inside())
	b.startDueGoals(context.Background())

	after, _ = b.DB.Goal(goal.ID)
	if after.LastWorked.IsZero() {
		t.Fatal("the goal was not taken up inside the quiet hours")
	}
}

func TestWithNoQuietHoursASelfStartingGoalStartsWhenDue(t *testing.T) {
	b := refusingBrain(t)
	goal := aDueGoal(t, b, true)

	b.startDueGoals(context.Background())

	if after, _ := b.DB.Goal(goal.ID); after.LastWorked.IsZero() {
		t.Fatal("with no quiet hours a due goal that starts itself was not taken up")
	}
}

// Asked work is never held for the window: "Work on it now" is the one press
// that starts a waiting goal anyway.
func TestWorkingOnAGoalNowIgnoresTheQuietHours(t *testing.T) {
	b := refusingBrain(t)
	goal := aDueGoal(t, b, true)

	b.SetQuietHours(outside())

	_, _ = b.WorkOnGoal(context.Background(), goal.ID)

	if after, _ := b.DB.Goal(goal.ID); after.LastWorked.IsZero() {
		t.Fatal("asking for work on a goal now was held for the quiet hours")
	}
}

// A goal that never starts itself is untouched either way.
func TestAGoalThatDoesNotStartItselfStaysForSomebodyToStart(t *testing.T) {
	b := refusingBrain(t)
	goal := aDueGoal(t, b, false)

	b.SetQuietHours(inside())
	b.startDueGoals(context.Background())

	if after, _ := b.DB.Goal(goal.ID); !after.LastWorked.IsZero() {
		t.Fatal("a goal that does not start itself was started")
	}
}

// The goal loop wakes at the opening rather than up to a round later.
func TestTheGoalLoopWakesAtTheOpening(t *testing.T) {
	b := refusingBrain(t)

	b.SetQuietHours(windowAround(3*time.Minute, 3*time.Hour))

	if got := b.untilNextGoalCheck(); got > 3*time.Minute || got < 90*time.Second {
		t.Errorf("three minutes before the opening the loop sleeps %v", got)
	}

	b.SetQuietHours("")

	if got := b.untilNextGoalCheck(); got != howOftenToCheckGoals {
		t.Errorf("with no window the loop sleeps %v, want %v", got, howOftenToCheckGoals)
	}
}

func aPlaceToRead(t *testing.T, b *Brain) {
	t.Helper()

	b.Learner.Curator.Embedder = fixedEmbedder{}

	work := t.TempDir()
	note := filepath.Join(work, "plans.md")

	if err := os.WriteFile(note, []byte("# Plans\n\nThe garden shed gets a new roof in spring, after the frost.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	hourAgo := time.Now().Add(-time.Hour)
	os.Chtimes(note, hourAgo, hourAgo)

	if _, err := places.Watch(b.Root, work, "work", places.Documents); err != nil {
		t.Fatal(err)
	}
}

func TestReadingThePlacesWaitsForTheQuietHours(t *testing.T) {
	b := testBrain(t)
	aPlaceToRead(t, b)

	b.SetQuietHours(outside())

	if b.lookAtOnePlace(context.Background()) {
		t.Fatal("a place was read outside the quiet hours")
	}

	if n := lessonsStored(t, b); n != 0 {
		t.Fatalf("%d lessons stored outside the quiet hours", n)
	}

	b.SetQuietHours(inside())

	if !b.lookAtOnePlace(context.Background()) {
		t.Fatal("the place was not read inside the quiet hours")
	}
}

func TestWithNoQuietHoursThePlacesAreReadAsBefore(t *testing.T) {
	b := testBrain(t)
	aPlaceToRead(t, b)

	if !b.lookAtOnePlace(context.Background()) {
		t.Fatal("with no quiet hours the place was not read")
	}
}

// What the pages show: a sentence while unasked work waits, nothing otherwise.
func TestTheWaitIsSaidOnlyWhileItLasts(t *testing.T) {
	b := testBrain(t)

	b.SetQuietHours(outside())

	if s := b.UnaskedWorkWaits(); !strings.Contains(s, "starts by itself at") {
		t.Errorf("outside the window: %q", s)
	}

	b.SetQuietHours(inside())

	if s := b.UnaskedWorkWaits(); s != "" {
		t.Errorf("inside the window: %q", s)
	}

	b.SetQuietHours("")

	if s := b.UnaskedWorkWaits(); s != "" {
		t.Errorf("no window: %q", s)
	}
}
