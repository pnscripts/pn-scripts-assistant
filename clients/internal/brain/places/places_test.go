package places

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-brain/internal/brain/learning"
)

// a brain's own folder, and somewhere with work in it.
func setUp(t *testing.T) (root, work string) {
	t.Helper()

	root = t.TempDir()
	os.WriteFile(filepath.Join(root, ".brain-root.json"), []byte(`{"id":"one"}`), 0o644)

	work = t.TempDir()

	// Something a scan will find: a project with a readme.
	project := filepath.Join(work, "a-project")
	os.MkdirAll(filepath.Join(project, ".git"), 0o755)
	os.WriteFile(filepath.Join(project, "README.md"),
		[]byte("# A project\n\nIt does a thing."), 0o644)
	os.WriteFile(filepath.Join(project, "go.mod"), []byte("module a-project\n"), 0o644)

	return root, work
}

// learner records what it was asked to take in, and takes it in.
type learner struct {
	got []learning.Observations
}

func (l *learner) Ingest(_ context.Context, obs learning.Observations,
	_ func(learning.IngestReport)) (learning.IngestReport, error) {
	l.got = append(l.got, obs)

	return learning.IngestReport{
		Seen: len(obs), Recorded: len(obs), Promoted: len(obs),
	}, nil
}

// remembered stands in for the database's account of what it has been shown.
type remembered map[string]bool

func (r remembered) KnownSources(string) (map[string]bool, error) { return r, nil }

/*
 * A place that is not attached is not a fault.
 *
 * It is the ordinary state of a drive that lives in a drawer, and the moment
 * somebody most wants to see their drives listed is the moment one of them is
 * missing. Dropping it from the list, or reporting it as an error, both make
 * the list useless exactly then.
 */
func TestADriveInADrawerIsStillOneOfThePlaces(t *testing.T) {
	root, work := setUp(t)

	if _, err := Watch(root, work, "work", Both); err != nil {
		t.Fatal(err)
	}

	away := filepath.Join(t.TempDir(), "not-plugged-in")

	if _, err := Watch(root, away, "films", Documents); err != nil {
		t.Fatalf("a drive that is not attached could not be added: %v", err)
	}

	list, err := Status(root)
	if err != nil || len(list) != 2 {
		t.Fatalf("the list holds %d places (%v)", len(list), err)
	}

	for _, p := range list {
		switch p.Path {
		case work:
			if !p.Reachable || p.Trouble != "" {
				t.Errorf("an attached folder was reported as trouble: %+v", p)
			}

		case away:
			if p.Reachable || !strings.Contains(p.Trouble, "not attached") {
				t.Errorf("a drive in a drawer was not said plainly: %+v", p)
			}
		}
	}
}

/*
 * Only what has not been seen costs anything.
 *
 * Each observation is two to three seconds of embedding on this machine, so
 * looking at a drive of two thousand documents again to find out that nothing
 * changed would be two hours of work for no answer. That is the difference
 * between a brain that watches folders and one that can only be told about
 * them once.
 */
func TestItOnlyLooksAtWhatItHasNotSeen(t *testing.T) {
	root, work := setUp(t)

	p, err := Watch(root, work, "work", Both)
	if err != nil {
		t.Fatal(err)
	}

	first := &learner{}

	pass, err := Look(context.Background(), first, remembered{}, "Petar", p)
	if err != nil {
		t.Fatal(err)
	}

	if pass.New == 0 || pass.Took == 0 {
		t.Fatalf("nothing was found in a folder with a project in it: %+v", pass)
	}

	// Everything it just saw is now known.
	known := remembered{}

	for _, batch := range first.got {
		for _, o := range batch {
			known[o.Source] = true
		}
	}

	second := &learner{}

	again, err := Look(context.Background(), second, known, "Petar", p)
	if err != nil {
		t.Fatal(err)
	}

	if again.Took != 0 || len(second.got) != 0 {
		t.Errorf("it read the same folder again: %+v", again)
	}

	if !again.Finished {
		t.Error("a place with nothing new in it was not reported as finished")
	}
}

/*
 * A big place is taken in a bite at a time.
 *
 * A drive can hold thousands of things, which at three seconds each is hours.
 * Doing that in one uninterruptible job is how a brain becomes something you
 * avoid plugging drives into — and unplugging mid-job would lose all of it
 * rather than the last few minutes.
 */
func TestABigPlaceIsTakenInBites(t *testing.T) {
	root := t.TempDir()
	work := t.TempDir()

	for i := 0; i < EachPass+20; i++ {
		dir := filepath.Join(work, "project", "p"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+string(rune('0'+i%10)))
		os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
		os.WriteFile(filepath.Join(dir, "README.md"), []byte("# thing\n\ndetails"), 0o644)
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644)
	}

	p, err := Watch(root, work, "work", Projects)
	if err != nil {
		t.Fatal(err)
	}

	l := &learner{}

	pass, err := Look(context.Background(), l, remembered{}, "Petar", p)
	if err != nil {
		t.Fatal(err)
	}

	if pass.Took > EachPass {
		t.Errorf("it tried to take %d things at once", pass.Took)
	}

	if pass.Finished {
		t.Error("a place with more still to read was reported as finished")
	}

	if pass.Place.Waiting == 0 {
		t.Error("it does not say how much of the place is left")
	}
}

// The one folder it must never watch is its own.
func TestItDoesNotLearnFromItsOwnMemory(t *testing.T) {
	root, _ := setUp(t)

	for _, where := range []string{root, filepath.Join(root, "inside"), "/", "relative/path"} {
		if _, err := Watch(root, where, "", Both); err == nil {
			t.Errorf("watching %s was allowed", where)
		}
	}
}

// Removing a place leaves what was learned from it. Knowledge is knowledge
// whatever drive it came off.
func TestForgettingAPlaceKeepsWhatItTaught(t *testing.T) {
	root, work := setUp(t)

	if _, err := Watch(root, work, "work", Both); err != nil {
		t.Fatal(err)
	}

	if err := Forget(root, work); err != nil {
		t.Fatal(err)
	}

	list, _ := List(root)

	if len(list) != 0 {
		t.Errorf("still watched: %+v", list)
	}

	if _, err := os.Stat(work); err != nil {
		t.Errorf("the folder itself was touched: %v", err)
	}
}
