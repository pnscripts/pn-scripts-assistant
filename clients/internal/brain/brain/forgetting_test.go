package brain

import (
	"os"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/places"
)

// fact writes a fact that came from somewhere, the way promotion does.
func fact(t *testing.T, b *Brain, source, content string) int64 {
	t.Helper()

	id, err := b.DB.AddFactFrom("document", content, source, []float32{0.1, 0.2, 0.3})
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func living(t *testing.T, b *Brain, id int64) bool {
	t.Helper()

	believed, err := b.DB.Believed(id)
	if err != nil {
		t.Fatal(err)
	}

	return believed
}

/*
 * A drive in a drawer is not a drive that is gone.
 *
 * The dangerous version of this feature is obvious and must never happen: the
 * brain lives on a removable disk and learns from folders on others, so a
 * sweep that retired everything it could not stat would empty somebody's
 * memory the first time they unplugged one — silently, with no symptom but an
 * assistant that had forgotten their work.
 */
func TestAnUnpluggedDriveIsNotAForgottenOne(t *testing.T) {
	b := testBrain(t)

	// A place that is not attached, holding a file that therefore cannot be
	// checked.
	if _, err := places.Watch(b.Root, "/nowhere/that/exists", "the other drive", "everything"); err != nil {
		t.Skip("this build will not watch a place that is not there: " + err.Error())
	}

	id := fact(t, b, "document:/nowhere/that/exists/notes.md", "something from the other drive")

	b.forgetWhatIsGone()

	if !living(t, b, id) {
		t.Error("unplugging a drive wiped what had been learned from it")
	}
}

/*
 * A file deleted from a drive that is plugged in is forgotten.
 *
 * Half of getting sharper is stopping being wrong: a document deleted last
 * year is otherwise still quoted with complete confidence, because the
 * sentence about it is still the best match for a question about it.
 */
func TestAFileDeletedFromAnAttachedPlaceIsForgotten(t *testing.T) {
	b := testBrain(t)

	folder := t.TempDir()

	if _, err := places.Watch(b.Root, folder, "work", "everything"); err != nil {
		t.Fatal(err)
	}

	kept := filepath.Join(folder, "still-here.md")

	if err := os.WriteFile(kept, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	survives := fact(t, b, "document:"+kept, "something still true")
	gone := fact(t, b, "document:"+filepath.Join(folder, "deleted.md"), "something no longer true")

	// And one from a conversation, which has no file and must never be swept.
	said, err := b.DB.AddFact("conversation", "Petar bills in advance", []float32{0.4, 0.5, 0.6})
	if err != nil {
		t.Fatal(err)
	}

	b.forgetWhatIsGone()

	if living(t, b, gone) {
		t.Error("a memory of a file that no longer exists is still believed")
	}

	if !living(t, b, survives) {
		t.Error("a memory of a file that is still there was retired")
	}

	if !living(t, b, said) {
		t.Error("something said in conversation was swept as though it were a file")
	}
}

// And a retired memory is not recalled, which is the point of retiring it.
func TestARetiredMemoryIsNotRecalled(t *testing.T) {
	b := testBrain(t)

	id, err := b.DB.AddFact("document", "the old version", []float32{1, 0, 0})
	if err != nil {
		t.Fatal(err)
	}

	before, err := b.DB.Search([]float32{1, 0, 0}, 10, 0.1)
	if err != nil {
		t.Fatal(err)
	}

	if len(before) != 1 {
		t.Fatalf("found %d before retiring", len(before))
	}

	if err := b.DB.Retire(id, "its source is gone"); err != nil {
		t.Fatal(err)
	}

	after, err := b.DB.Search([]float32{1, 0, 0}, 10, 0.1)
	if err != nil {
		t.Fatal(err)
	}

	if len(after) != 0 {
		t.Errorf("a retired memory was still recalled: %+v", after)
	}
}
