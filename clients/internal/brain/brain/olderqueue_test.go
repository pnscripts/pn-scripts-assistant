package brain

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * A brain 0.13 left with a full review queue reads again after the upgrade.
 *
 * 0.13 never finished its own knowledge folder: it read the same sixty guides
 * every ninety seconds and stored every reading again as a lesson waiting for
 * a person, with no check for one it already had. Past two hundred waiting,
 * reading stops on every drive (AsMuchAsAnyoneWillReview), and 1.0's fixes
 * stop new ones but leave those where they are, so the upgraded brain never
 * read anything again.
 *
 * Filled here the way 0.13 filled it, then opened by this version: the
 * repeats are gone, the built-in and imported skills are gone, one copy of
 * each reading of the brain's own guides is filed the way 1.0 files it, and
 * what really is somebody's to decide still waits for them.
 */
func TestTheQueueA013BrainFilledIsTidiedOnOpen(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "brain.sqlite")

	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		db.Close()
		learning.SetBrainRoot("")
	})

	readings := []struct{ content, source string }{
		{"Mail is read through IMAP and nothing is sent without approval.", "reading:@brain/knowledge/guides/mail.md"},
		{"A draft waits until it is approved.", "reading:@brain/knowledge/guides/mail.md"},
		{"Rules are kept as the owner says them.", "reading:@brain/knowledge/guides/rules.md"},
		{"A reading of a guide that has since gone.", "reading:@brain/knowledge/guides/gone.md"},
		{"What a built-in skill says.", "reading:@brain/knowledge/pack-skills/bookkeeping/SKILL.md"},
		{"What somebody's imported skill says.", "reading:@brain/knowledge/skills/someones-skill/SKILL.md"},
	}

	// Sixty passes of the place watcher: 360 waiting, well past the limit.
	for pass := 0; pass < 60; pass++ {
		for _, r := range readings {
			if _, err := db.AddLesson(0, r.content, learning.StatusProposed, "high", r.source); err != nil {
				t.Fatal(err)
			}
		}
	}

	// And what is genuinely somebody's to decide.
	db.AddLesson(0, "The lease ends in May.", learning.StatusProposed, "high", "document:/home/alex/lease.md")
	db.AddLesson(0, "Alex prefers tea.", learning.StatusProposed, "low", "")

	if waiting, _ := db.CountPendingLessons(); waiting < AsMuchAsAnyoneWillReview {
		t.Fatalf("only %d waiting; the test needs a queue past the limit", waiting)
	}

	cfg := config.Default()
	cfg.Owner = "Alex"

	b := New(db, cfg, root, path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.Learner.Curator.Embedder = fixedEmbedder{}

	pending, err := db.LessonsByStatus(learning.StatusProposed, 1000)
	if err != nil {
		t.Fatal(err)
	}

	if len(pending) != 2 {
		var left []string
		for _, l := range pending {
			if len(left) < 8 {
				left = append(left, l.Source)
			}
		}

		t.Fatalf("%d lessons still wait for a person, want the document and the chat lesson: %v", len(pending), left)
	}

	validated, _ := db.LessonsByStatus(learning.StatusValidated, 1000)
	if len(validated) != 3 {
		t.Errorf("%d readings of the guides filed as validated, want one of each of the 3 whose guide is there", len(validated))
	}

	rejected, _ := db.LessonsByStatus(learning.StatusRejected, 1000)
	if len(rejected) != 1 || rejected[0].Source != "reading:@brain/knowledge/guides/gone.md" {
		t.Errorf("the reading of a guide that is gone was not rejected once: %+v", rejected)
	}

	all, _ := db.CountLessonsByStatus()
	total := 0
	for _, n := range all {
		total += n
	}

	if total != 6 {
		t.Errorf("%d lessons left in all (%v), want 6: no repeats and no skills", total, all)
	}

	// Reading goes on: the queue no longer stops it.
	if !b.lookAtOnePlace(context.Background()) {
		t.Error("the upgraded brain still does not read")
	}

	// Once: lessons stored later are left alone by the next open.
	db.AddLesson(0, "A repeat stored after the tidy.", learning.StatusProposed, "high", "reading:@brain/knowledge/pack-skills/x/SKILL.md")

	New(db, cfg, root, path, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if n, _ := db.CountPendingLessons(); n != 3 {
		t.Errorf("the tidy ran again on the next open (%d waiting, want 3)", n)
	}
}
