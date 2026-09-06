package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * Only somewhere it could actually take on is offered.
 *
 * The first version offered the home folder and the drive the brain lives on,
 * which are the two obvious answers and are both refused: a whole home folder
 * is more than anybody means by "learn everything", and a brain cannot learn
 * from the drive it keeps itself on.
 *
 * So "learn everything" was approved and then did nothing, with two refusals
 * where the work should have been — which is worse than saying no, because
 * somebody had already agreed to it.
 */
func TestItOnlyOffersPlacesItCouldTakeOn(t *testing.T) {
	_, _, b := newServer(t)

	found := b.SomewhereToStart()

	if len(found) == 0 {
		t.Skip("nothing to offer on this machine")
	}

	home, _ := os.UserHomeDir()

	for _, c := range found {
		if home != "" && filepath.Clean(c.Path) == filepath.Clean(home) {
			t.Errorf("offered the whole home folder, which is always refused")
		}

		if strings.Contains(c.Path, "lost+found") {
			t.Errorf("offered %s, which belongs to the filesystem", c.Path)
		}

		// Every one has to be a real folder, or agreeing to it fails after
		// somebody has agreed.
		if info, err := os.Stat(c.Path); err != nil || !info.IsDir() {
			t.Errorf("offered %s, which is not a folder anybody can read", c.Path)
		}

		if c.Name == "" {
			t.Errorf("offered %s with no name a person would recognise", c.Path)
		}
	}
}

/*
 * And a name, not a mount point with a folder glued to the front of it.
 *
 * "DEV on /media/petar/c8fc2986-4b79-4d7b-9a8c-e6db653915ac" is a path being
 * read out. It appears in an approval prompt, which is the sentence somebody
 * has to judge before agreeing.
 */
func TestTheOfferReadsLikeSomethingSomebodyWouldSay(t *testing.T) {
	_, _, b := newServer(t)

	for _, c := range b.SomewhereToStart() {
		if strings.Contains(c.Name, "/media/") || strings.Count(c.Name, "-") > 2 {
			t.Errorf("the name is a path: %q", c.Name)
		}
	}
}
