package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-brain/internal/brain/learning"
)

type rememberedWhat struct {
	got learning.Observations
}

func (r *rememberedWhat) Ingest(_ context.Context, obs learning.Observations,
	_ func(learning.IngestReport)) (learning.IngestReport, error) {
	r.got = obs

	return learning.IngestReport{Seen: len(obs), Promoted: len(obs)}, nil
}

/*
 * "Update the brain from this folder" had no tool, so it was answered by
 * listing the folder and offering to list more.
 *
 * Reading files into a conversation is not learning: it lasts until the
 * context fills and is then gone. The machinery to scan, embed and store
 * existed the whole time and only the command line could reach it — a
 * capability whose owner must find a terminal to use is one the program says
 * it does not have.
 */
func TestLearningAFolderActuallyStoresWhatItFinds(t *testing.T) {
	dir := t.TempDir()
	learner := &rememberedWhat{}

	out, err := LearnFolder{Learn: func() Ingests { return learner }, Owner: "Petar"}.
		Execute(context.Background(), json.RawMessage(`{"path":`+quote(dir)+`}`))
	if err != nil {
		t.Fatalf("learning an empty folder should report, not fail: %v", err)
	}

	if !strings.Contains(out, "Nothing in") {
		t.Errorf("an empty folder was reported as %q", out)
	}
}

// A path that is not there must say so, rather than reporting a successful
// scan of nothing — the failure that made "/absolute/path/to/your/projects"
// look like a real answer.
func TestLearningRefusesWhatItCannotRead(t *testing.T) {
	_, err := LearnFolder{Learn: func() Ingests { return &rememberedWhat{} }}.
		Execute(context.Background(), json.RawMessage(`{"path":"/no/such/place/here"}`))

	if err == nil {
		t.Fatal("a folder that does not exist was accepted")
	}

	if !strings.Contains(err.Error(), "/no/such/place/here") {
		t.Errorf("the error does not name the path: %v", err)
	}
}

// And a file is not a folder, which is the other way a path can be wrong.
func TestLearningRefusesAFile(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/notafolder.txt"

	if err := writeFileForTest(file); err != nil {
		t.Fatal(err)
	}

	_, err := LearnFolder{Learn: func() Ingests { return &rememberedWhat{} }}.
		Execute(context.Background(), json.RawMessage(`{"path":`+quote(file)+`}`))

	if err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Errorf("a file was accepted as a folder: %v", err)
	}
}

func writeFileForTest(path string) error {
	return os.WriteFile(path, []byte("x"), 0o600)
}

/*
 * A nil learner stored in an interface is not a nil interface.
 *
 * The tool was built with the brain's learner forty lines before the learner
 * was created, so it held a nil *Worker wrapped in a non-nil interface. It
 * passed its own nil check, ran, was timed, was reported as having run — and
 * stored none of the sixty-two projects it had just found. Nothing anywhere
 * said so; the only symptom was that the memory count did not move.
 */
func TestLearningSaysSoWhenThereIsNothingToLearnWith(t *testing.T) {
	dir := t.TempDir()

	// The shape the bug had: a getter that resolves to nothing.
	_, err := LearnFolder{Learn: func() Ingests { return nil }}.
		Execute(context.Background(), json.RawMessage(`{"path":`+quote(dir)+`}`))

	if err == nil {
		t.Fatal("a missing learner was reported as a successful scan")
	}

	if !strings.Contains(err.Error(), "learns is not running") {
		t.Errorf("the error does not say what is wrong: %v", err)
	}

	// And no getter at all is the same answer, not a panic.
	if _, err := (LearnFolder{}).
		Execute(context.Background(), json.RawMessage(`{"path":`+quote(dir)+`}`)); err == nil {
		t.Error("a tool with no learner at all reported success")
	}
}

/*
 * A folder too big to take in says so instead of hanging.
 *
 * Every observation is embedded on the processor at about three seconds each.
 * Sixty-two projects took three minutes; the same folder holds two thousand
 * three hundred documents, which is two hours. Starting that silently is
 * indistinguishable from the brain having crashed, and a turn nobody can tell
 * from a crash is worse than one that declines and says why.
 */
func TestAFolderTooBigToLearnIsRefusedWithItsSize(t *testing.T) {
	dir := t.TempDir()

	// More projects than the cap, each the smallest thing the scanner counts.
	for i := 0; i < MostAtOnce+5; i++ {
		project := filepath.Join(dir, fmt.Sprintf("project%03d", i))

		if err := os.MkdirAll(project, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(project, "package.json"),
			[]byte(`{"name":"x"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// A learner that must never be reached: nothing may be stored, so being
	// asked to store at all is the failure.
	stored := false
	learner := &countingLearner{onIngest: func() { stored = true }}

	out, err := LearnFolder{Learn: func() Ingests { return learner }, Owner: "Petar"}.
		Execute(context.Background(), json.RawMessage(`{"path":`+quote(dir)+`}`))
	if err != nil {
		t.Fatalf("it should decline, not fail: %v", err)
	}

	if stored {
		t.Error("it stored something it had just said was too much")
	}

	for _, want := range []string{"more than can be taken in", "Nothing was stored", "minutes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal does not say %q:\n%s", want, out)
		}
	}
}

type countingLearner struct{ onIngest func() }

func (c *countingLearner) Ingest(context.Context, learning.Observations,
	func(learning.IngestReport)) (learning.IngestReport, error) {
	if c.onIngest != nil {
		c.onIngest()
	}

	return learning.IngestReport{}, nil
}
