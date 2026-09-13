package brain

import (
	"path/filepath"
	"testing"
)

/*
 * The drive lands somewhere different on every machine.
 *
 * /media/you/LABEL here, /media/someone/LABEL there, /Volumes/LABEL on a Mac,
 * E:\ on Windows. What the brain knows about files is written down with the
 * path those files had, so after the journey a large part of what it knows is
 * accurate and unusable at once — it names real files by a path that does not
 * exist on this machine.
 *
 * The substitution that repairs that is on the mount point, not on the brain's
 * own folder: the folder is where the brain keeps itself, and the memories are
 * about everything else on the disk beside it.
 */
func TestWhatToRewriteAfterAJourney(t *testing.T) {
	from := "/media/petar/WORKDRIVE/PN-SCRIPTS-ASSISTANT-DATA"
	to := "/Volumes/WORKDRIVE/PN-SCRIPTS-ASSISTANT-DATA"

	old, now := prefixes(from, to)

	if old != "/media/petar/WORKDRIVE" || now != "/Volumes/WORKDRIVE" {
		t.Errorf("the substitution is %q → %q; it should be on the drive, not the folder", old, now)
	}

	/*
	 * And when the folder itself was renamed, there is no wider claim that is
	 * safe to make. Rewriting the parent then would rewrite paths that have
	 * nothing to do with the move — the rest of somebody's home folder, for
	 * one — so only the root's own path is offered.
	 */
	old, now = prefixes("/home/petar/brain", "/home/petar/somewhere-else")

	if old != "/home/petar/brain" || now != "/home/petar/somewhere-else" {
		t.Errorf("a renamed folder produced %q → %q, which claims too much", old, now)
	}
}

/*
 * A journey nobody needs to hear about is not mentioned.
 *
 * A brain that has only ever been talked to, and never pointed at a folder,
 * has no memory that names a path — so the drive moving costs it nothing and
 * offering to repair a thousand memories would be inventing work.
 */
func TestAJourneyThatBrokeNothingIsNotReported(t *testing.T) {
	b, _ := withACopy(t)

	b.NoteJourney(filepath.Join(t.TempDir(), "somewhere-it-used-to-be"))

	if j := b.Travelled(); j != nil {
		t.Errorf("it offered to repair %d memories that do not mention the old place", j.Affected)
	}
}

// And one that did break something is reported, with the count.
func TestAJourneyThatBrokeSomethingSaysHowMuch(t *testing.T) {
	b, _ := withACopy(t)

	was := "/media/someone/OLDDRIVE/PN-SCRIPTS-ASSISTANT-DATA"
	old, _ := prefixes(was, b.Root)

	for i := 0; i < 3; i++ {
		if _, err := b.DB.AddFact("project",
			"Petar has a Go project at "+old+"/DEV/thing", nil); err != nil {
			t.Fatal(err)
		}
	}

	b.NoteJourney(was)

	j := b.Travelled()

	if j == nil {
		t.Fatal("memories naming a drive that has moved were not noticed")
	}

	if j.Affected != 3 {
		t.Errorf("it says %d memories are affected, not 3", j.Affected)
	}

	if j.OldPrefix != old {
		t.Errorf("it would rewrite %q rather than %q", j.OldPrefix, old)
	}
}
