package places

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/learning"
)

/*
 * notes writes documents that each say several things.
 *
 * Every line is its own sentence, long enough and wordy enough to be kept,
 * and different from every other line in every other test: the same sentence
 * in three documents counts as boilerplate, and that count lives for the
 * whole test binary.
 */
func notes(t *testing.T, dir, tag string, documents, lines int) {
	t.Helper()

	for d := 0; d < documents; d++ {
		var b strings.Builder

		for l := 0; l < lines; l++ {
			fmt.Fprintf(&b, "In the %s notebook %d, entry %d explains how the garden beds by the northern fence were dug.\n",
				tag, d, l)
		}

		name := filepath.Join(dir, fmt.Sprintf("%s-%02d.md", tag, d))
		if err := os.WriteFile(name, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

/*
 * What is left is counted in the same things that were found.
 *
 * A pass finds things (a document's listing, the mark that its insides are
 * still to be read) and the reading turns each mark into the several
 * sentences the document says. "Left" was found minus taken, with found
 * counted in things and taken counted in sentences: four documents of three
 * sentences each came out as eight found, sixteen taken, and a negative
 * remainder that was thrown away. So the place was never finished on the pass
 * that finished it, the next pass found nothing and took nothing, and "I have
 * finished reading" (said only after a pass that took something) was never
 * said.
 */
func TestWhatIsLeftIsCountedInTheThingsThatWereFound(t *testing.T) {
	root := t.TempDir()
	work := t.TempDir()

	notes(t, work, "left", 4, 3)

	p, err := Watch(root, work, "notes", Documents)
	if err != nil {
		t.Fatal(err)
	}

	l := &learner{}

	pass, err := Look(context.Background(), l, remembered{}, "Alex", p)
	if err != nil {
		t.Fatal(err)
	}

	if pass.New != 8 {
		t.Fatalf("four documents should be eight things to look at (a listing and a reading each), found %d", pass.New)
	}

	if pass.Took != pass.New {
		t.Errorf("took %d of the %d things found, in a folder small enough for one pass", pass.Took, pass.New)
	}

	if pass.Place.Waiting != 0 || !pass.Finished {
		t.Errorf("everything was read in one pass, yet %d are said to be left (finished: %v)",
			pass.Place.Waiting, pass.Finished)
	}

	var sentences int
	for _, batch := range l.got {
		sentences += len(batch)
	}

	if sentences != 4+4*3 {
		t.Errorf("handed over %d observations, want the 4 listings and the 12 sentences", sentences)
	}
}

/*
 * A document is never half taken in.
 *
 * A pass is capped, and the cap was applied to the sentences after the
 * documents had been read: the document straddling it had its first sentences
 * remembered and the rest dropped. Its reading was then on record, so it
 * counted as read and the rest of it was never seen again. Read pass after
 * pass to the end, every document here must arrive whole, exactly once, and
 * what each pass says is left must be what the next one finds.
 */
func TestADocumentIsNeverHalfTakenIn(t *testing.T) {
	root := t.TempDir()
	work := t.TempDir()

	const documents = 30

	notes(t, work, "whole", documents, learning.FactsPerDocument)

	p, err := Watch(root, work, "notes", Documents)
	if err != nil {
		t.Fatal(err)
	}

	known := remembered{}
	statements := map[string]int{}
	waitingBefore := -1
	passes := 0

	for ; passes < 10; passes++ {
		l := &learner{}

		pass, err := Look(context.Background(), l, known, "Alex", p)
		if err != nil {
			t.Fatal(err)
		}

		if waitingBefore >= 0 && pass.New != waitingBefore {
			t.Errorf("pass %d: the last pass said %d were left, this one found %d",
				passes, waitingBefore, pass.New)
		}

		handed := 0

		for _, batch := range l.got {
			handed += len(batch)

			for _, o := range batch {
				if strings.HasPrefix(o.Source, "reading:") {
					statements[o.Source]++
				}

				known[o.Source] = true
			}
		}

		if handed > EachPass {
			t.Errorf("pass %d handed over %d observations at once; a pass is %d", passes, handed, EachPass)
		}

		if pass.Place.Waiting != pass.New-pass.Took {
			t.Errorf("pass %d: %d left, but %d found and %d taken", passes, pass.Place.Waiting, pass.New, pass.Took)
		}

		waitingBefore = pass.Place.Waiting
		p = pass.Place

		if pass.Finished {
			break
		}
	}

	if passes == 10 {
		t.Fatal("the folder was never finished")
	}

	if len(statements) != documents {
		t.Errorf("%d documents were read, want all %d", len(statements), documents)
	}

	for source, n := range statements {
		if n != learning.FactsPerDocument {
			t.Errorf("%s arrived with %d of its %d sentences", source, n, learning.FactsPerDocument)
		}
	}
}
