package brain

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/places"
	"pn-scripts-assistant/internal/brain/progress"
)

func lessonsStored(t *testing.T, b *Brain) int {
	t.Helper()

	counts, err := b.DB.CountLessonsByStatus()
	if err != nil {
		t.Fatal(err)
	}

	n := 0
	for _, c := range counts {
		n += c
	}

	return n
}

/*
 * The knowledge folder is read to the end, once.
 *
 * Seen on a fresh brain on 08.11.2026: "read some of a place … took=60
 * learned=0 left=562", every ninety seconds, for as long as it ran. The
 * folder lives inside the brain, so what is read from it is remembered
 * relative to the brain ("@brain/knowledge/…"), and the question "what have I
 * seen under this folder" was asked with its absolute path, which matched
 * nothing. The brain read the same sixty guides again and again, stored sixty
 * more lessons each time, and competed with every question for the processor
 * while it did — the fourteen-minute answer.
 *
 * Through the real brain: the guides it ships, its own store, the place
 * watcher's own loop. Only the model is a stand-in.
 */
func TestTheKnowledgeFolderIsReadToTheEndOnce(t *testing.T) {
	b := testBrain(t)
	b.Learner.Curator.Embedder = fixedEmbedder{}

	t.Cleanup(func() { learning.SetBrainRoot("") })

	b.installSeed()

	knowledge := places.Knowledge(b.Root)

	// Written an hour ago, as a folder read on another day would be. A file
	// written in the same second as it was read could count as changed since.
	hourAgo := time.Now().Add(-time.Hour)

	err := filepath.WalkDir(knowledge, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		return os.Chtimes(path, hourAgo, hourAgo)
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := places.Status(b.Root)
	if err != nil || len(list) != 1 || list[0].Path != knowledge {
		t.Fatalf("the knowledge folder is not the one place: %+v %v", list, err)
	}

	before, err := places.Count(b.DB, b.Cfg.Owner, list[0])
	if err != nil || before == 0 {
		t.Fatalf("nothing to read in the shipped guides: %d %v", before, err)
	}

	// Enough bites for everything there, and some over: a folder that is
	// never finished is the fault.
	most := before/places.EachPass + 3
	bites := 0

	for b.lookAtOnePlace(context.Background()) {
		bites++

		if bites > most {
			t.Fatalf("still reading after %d bites of a folder of %d things", bites, before)
		}
	}

	if bites == 0 {
		t.Fatal("the knowledge folder was not read at all")
	}

	left, err := places.Count(b.DB, b.Cfg.Owner, list[0])
	if err != nil || left != 0 {
		t.Errorf("%d of %d still waiting after the folder was finished (%v)", left, before, err)
	}

	// And another round reads nothing and stores nothing.
	stored := lessonsStored(t, b)

	if b.lookAtOnePlace(context.Background()) {
		t.Error("a finished knowledge folder was read again")
	}

	if again := lessonsStored(t, b); again != stored {
		t.Errorf("another round stored %d more lessons", again-stored)
	}

	status, _ := places.Status(b.Root)
	if len(status) != 1 || status[0].Waiting != 0 {
		t.Errorf("the place still says things are waiting: %+v", status)
	}

	/*
	 * And it said so, once.
	 *
	 * The sentence follows only a bite that took something and finished the
	 * place. While what was left was found minus taken, with found counted in
	 * files and taken in the sentences read out of them, the last bite of a
	 * folder of documents never came out finished, and the empty bite after
	 * it took nothing: the sentence was never said.
	 */
	said, err := whatItSaid(b)
	if err != nil {
		t.Fatal(err)
	}

	finished := 0

	for _, m := range said {
		if strings.Contains(m.Content, "I have finished reading") {
			finished++
		}
	}

	if finished != 1 {
		t.Errorf("said it had finished reading the folder %d times, want once", finished)
	}

	// The program's own guides are not homework: nothing from them waits for
	// a person, or the queue fills on the first day and all reading stops.
	if pending, _ := b.DB.CountPendingLessons(); pending != 0 {
		t.Errorf("%d lessons from the brain's own guides wait for a person", pending)
	}

	if facts, _ := b.DB.CountFacts(); facts == 0 {
		t.Error("nothing from the guides became knowledge")
	}

	// And the built-in skills are skills, not memories.
	seen, err := b.DB.SourcesSeenAt("")
	if err != nil {
		t.Fatal(err)
	}

	for source := range seen {
		if strings.Contains(source, places.PackSkillsFolder) {
			t.Errorf("a built-in skill was read into memory: %s", source)

			break
		}
	}
}

/*
 * A bite of a place that ends while a question is being answered leaves the
 * answer's step alone. It used to clear it, so the page showed an idle brain
 * halfway through somebody's question.
 */
func TestReadingAPlaceDoesNotClearTheAnswerOnScreen(t *testing.T) {
	b := testBrain(t)
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

	// The step a question has on screen, set after the bite began: the
	// learner below begins the question in the middle of the reading.
	b.Learner.Curator.Embedder = beginsAQuestion{fixedEmbedder{}}

	// The reading gives way to an answer on screen for up to HoldOff per
	// thing it takes in; a minute each is longer than this test needs.
	was := learning.HoldOff
	learning.HoldOff = 50 * time.Millisecond

	t.Cleanup(func() { learning.HoldOff = was })

	t.Cleanup(progress.Done)

	if !b.lookAtOnePlace(context.Background()) {
		t.Fatal("the place was not read")
	}

	step := progress.Now()
	if !step.Busy || step.Kind != "thinking" || step.Background {
		t.Errorf("the answer on screen was cleared by the reading: %+v", step)
	}

	if strings.Contains(step.Note, "work") {
		t.Errorf("the reading wrote over the answer: %q", step.Note)
	}
}

// beginsAQuestion embeds, and while it does a question starts being answered
// on screen (the step only: a turn would make background reading wait).
type beginsAQuestion struct{ fixedEmbedder }

func (q beginsAQuestion) Embed(ctx context.Context, text string) ([]float32, error) {
	progress.Begin()
	progress.Set("thinking", "Thinking")

	return q.fixedEmbedder.Embed(ctx, text)
}
