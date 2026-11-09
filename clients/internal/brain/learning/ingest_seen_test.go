package learning

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

/*
 * Stopped early, Ingest says how far it got.
 *
 * Seen used to be the length of what it was given, whatever happened, so a
 * pass cut short by the program shutting down or a drive being unplugged
 * reported everything as taken in. places.Look counts what is left to read
 * from it: with the whole list counted as seen, a place could be reported
 * finished while most of the bite was never reached.
 */
func TestIngestStoppedEarlySaysHowFarItGot(t *testing.T) {
	noTurns(t)

	dir := t.TempDir()

	var obs Observations

	for _, name := range []string{"one.md", "two.md", "three.md", "four.md"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("notes"), 0o600); err != nil {
			t.Fatal(err)
		}

		obs = append(obs, Observation{
			Content: "Alex keeps notes called " + name + " in " + path,
			Source:  "document:" + path,
		})
	}

	w := testWorker(t, &slowEmbedder{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Stopped once the second observation is through.
	rep, err := w.Ingest(ctx, obs, func(r IngestReport) {
		if r.Promoted+r.Duplicates+r.Failed+r.Waiting+r.Rejected == 2 {
			cancel()
		}
	})

	if err == nil {
		t.Fatal("a run stopped halfway reported no error")
	}

	if rep.Seen != 2 {
		t.Errorf("stopped after 2 of %d, yet it says it went through %d", len(obs), rep.Seen)
	}

	if n := lessonCount(t, w.DB); n != 2 {
		t.Errorf("%d observations were recorded; the report and the store disagree", n)
	}

	// Run to the end, it has gone through all of them.
	rest, err := w.Ingest(context.Background(), obs[rep.Seen:], nil)
	if err != nil {
		t.Fatal(err)
	}

	if rest.Seen != len(obs)-2 {
		t.Errorf("a full run went through %d of %d", rest.Seen, len(obs)-2)
	}
}
