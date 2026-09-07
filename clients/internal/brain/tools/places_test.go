package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/places"
)

/*
 * Answering out loud for the drives it looks after.
 *
 * "Have you read the film drive yet?" is asked in a room, not in a settings
 * panel, and a brain that has to say "open the storage card and look" for a
 * fact it holds is not answering. The two things the answer has to carry are
 * which drives are attached and how much of each is still to read.
 */
func TestItCanSayWhichDrivesItLooksAfter(t *testing.T) {
	root := t.TempDir()
	work := t.TempDir()
	away := filepath.Join(t.TempDir(), "unplugged")

	if _, err := places.Watch(root, work, "work", places.Projects); err != nil {
		t.Fatal(err)
	}

	if _, err := places.Watch(root, away, "films", places.Documents); err != nil {
		t.Fatal(err)
	}

	// As if a pass had run over the attached one and left some to do.
	places.Note(root, places.Place{Path: work, Learned: 40, Waiting: 120})

	tool := Places{Root: root, Owner: "Petar"}

	said, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"work", "films", "not attached", "40", "120"} {
		if !strings.Contains(said, want) {
			t.Errorf("the answer does not mention %q:\n%s", want, said)
		}
	}

	// And how long that is, because "120 things" means nothing to somebody
	// deciding whether to leave the drive plugged in.
	if !strings.Contains(said, "minutes") {
		t.Errorf("it does not say how long the rest would take:\n%s", said)
	}
}

// A brain with no places says so, and says where to add one — rather than
// answering an empty list with silence.
func TestWithNoPlacesItSaysSo(t *testing.T) {
	root := t.TempDir()

	said, err := Places{Root: root}.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "no drives or folders") {
		t.Errorf("an empty list produced: %s", said)
	}
}

/*
 * And reading from one that is not attached is refused in words, not as an
 * error.
 *
 * "Read the film drive" when the film drive is in a drawer is a reasonable
 * thing to have asked, and the answer is a fact about the room rather than a
 * failure — an error here would come back as the brain apologising for a bug.
 */
func TestReadingFromADriveInADrawer(t *testing.T) {
	root := t.TempDir()
	away := filepath.Join(t.TempDir(), "unplugged")

	if _, err := places.Watch(root, away, "films", places.Documents); err != nil {
		t.Fatal(err)
	}

	said, err := Places{Root: root}.Execute(context.Background(),
		json.RawMessage(`{"read_now":"films"}`))
	if err != nil {
		t.Fatalf("it treated an unplugged drive as a failure: %v", err)
	}

	if !strings.Contains(said, "not attached") {
		t.Errorf("it does not say why nothing was read:\n%s", said)
	}
}

// The list on disk is the list it answers from, so a place added while it was
// running is one it knows about without a restart.
func TestItReadsTheListEachTime(t *testing.T) {
	root := t.TempDir()
	tool := Places{Root: root}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}

	work := t.TempDir()
	os.WriteFile(filepath.Join(work, "note.txt"), []byte("something"), 0o644)

	if _, err := places.Watch(root, work, "work", places.Both); err != nil {
		t.Fatal(err)
	}

	said, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "work") {
		t.Errorf("a place added while it was running is invisible to it:\n%s", said)
	}
}
