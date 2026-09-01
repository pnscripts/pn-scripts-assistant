package learning

import (
	"errors"
	"os"
	"strings"
	"testing"
)

/*
 * Every observation reports, whatever became of it.
 *
 * The call used to sit after Promoted++, so only an observation that became a
 * new fact said anything — rejected, waiting, failed and duplicate all reach a
 * continue first. Learning a folder a second time is mostly duplicates, so the
 * progress line stopped exactly when it was needed most: three hundred
 * already-known documents in a row, each costing its two seconds, none of them
 * reporting, and ten minutes of real work behind a line that had not moved.
 *
 * Checked by reading the code rather than running a Worker, which needs a
 * database, an embedder and a curator: what matters is that no path out of the
 * loop body skips the report.
 */
func TestEveryOutcomeReportsProgress(t *testing.T) {
	body, err := sourceOfIngest()
	if err != nil {
		t.Skip(err)
	}

	// Each of these is a way an observation is finished with.
	for _, outcome := range []string{
		"rep.Rejected++", "rep.Waiting++", "rep.Duplicates++", "rep.Promoted++",
	} {
		at := indexAfter(body, outcome)
		if at < 0 {
			t.Errorf("%s is no longer in the loop; this test is out of date", outcome)

			continue
		}

		// The report has to come before the next way out of the iteration.
		next := earliest(body[at:], "continue", "}\n\t}")

		if !containsBefore(body[at:], "report()", next) {
			t.Errorf("%s leaves the loop without reporting progress", outcome)
		}
	}
}

func sourceOfIngest() (string, error) {
	raw, err := os.ReadFile("ingest.go")
	if err != nil {
		return "", err
	}

	s := string(raw)
	start := strings.Index(s, "for _, o := range observations {")

	if start < 0 {
		return "", errors.New("the ingest loop has moved")
	}

	end := strings.Index(s[start:], "\n\treturn rep, nil")
	if end < 0 {
		return "", errors.New("the end of the ingest loop has moved")
	}

	return s[start : start+end], nil
}

func indexAfter(body, needle string) int {
	i := strings.Index(body, needle)
	if i < 0 {
		return -1
	}

	return i + len(needle)
}

func earliest(s string, options ...string) int {
	best := len(s)

	for _, o := range options {
		if i := strings.Index(s, o); i >= 0 && i < best {
			best = i
		}
	}

	return best
}

func containsBefore(s, needle string, limit int) bool {
	i := strings.Index(s, needle)

	return i >= 0 && i < limit
}
