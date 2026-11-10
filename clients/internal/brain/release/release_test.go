package release

import (
	"strings"
	"testing"
)

/*
 * A copy that was not released says so, rather than claiming a number.
 *
 * "0.1.0" printed by a build from somebody's working tree is worse than no
 * answer at all: it is a version that cannot be looked up, attached to a
 * program that is not the one that was released under it.
 */
func TestABuildFromSourceDoesNotClaimAVersion(t *testing.T) {
	was, wasCommit := Version, Commit
	t.Cleanup(func() { Version, Commit = was, wasCommit })

	Version, Commit = "", "e9b80e0dd7a1c2b3"

	if got := Is(); !strings.Contains(got, "built from source") {
		t.Errorf("an unreleased build calls itself %q", got)
	}

	if got := Revision(); got != "e9b80e0" {
		t.Errorf("the commit is %q, not the short form", got)
	}
}

// And a released copy says what it is, what it was built from and when — the
// line somebody quotes back when something goes wrong.
func TestAReleasedCopySaysEverythingItKnows(t *testing.T) {
	was, wasCommit, wasBuilt := Version, Commit, Built
	t.Cleanup(func() { Version, Commit, Built = was, wasCommit, wasBuilt })

	Version, Commit, Built = "0.1.0", "e9b80e0dd7a1c2b3", "2026-09-23"

	line := Full("PN Scripts Assistant")

	for _, want := range []string{"PN Scripts Assistant", "0.1.0", "e9b80e0", "2026-09-23"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q is missing from %q", want, line)
		}
	}

	// Nothing invented: with no build date there is no build date in it.
	Built = ""

	if strings.Contains(Full("PN Scripts Assistant"), "built ") {
		t.Errorf("it made up a build date: %q", Full("PN Scripts Assistant"))
	}
}

// The second stamp, the one a release check searches the file for, agrees
// with the version or the line says it does not. A copy put together from
// two builds must not quote either number as if it were sure.
func TestTheSearchableStampAgreesWithTheVersion(t *testing.T) {
	was, wasMark, wasCommit, wasBuilt := Version, Mark, Commit, Built
	t.Cleanup(func() { Version, Mark, Commit, Built = was, wasMark, wasCommit, wasBuilt })

	Version, Mark, Commit, Built = "1.0.1", "PNSA-RELEASE[1.0.1]", "", ""

	if got := Full("PN Scripts Assistant"); got != "PN Scripts Assistant 1.0.1" {
		t.Errorf("a consistent copy says %q", got)
	}

	Mark = "PNSA-RELEASE[1.0.0]"

	if got := Full("PN Scripts Assistant"); !strings.Contains(got, "stamped inconsistently") {
		t.Errorf("two stamps that disagree went unmentioned: %q", got)
	}

	// 1.0.1 is a prefix of 1.0.10; the brackets keep them apart.
	Version, Mark = "1.0.1", "PNSA-RELEASE[1.0.10]"

	if got := Full("PN Scripts Assistant"); !strings.Contains(got, "stamped inconsistently") {
		t.Errorf("1.0.10 passed for 1.0.1: %q", got)
	}
}
