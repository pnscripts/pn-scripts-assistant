package occupations

import (
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Every capability a job asks for is one somebody has described.
 *
 * This is the test the seed file is written against, and it is the reason the
 * table can stay readable: a job names what it needs in prose, and this is
 * what stops that prose drifting into names nothing defines. Without it the
 * failure is silent — a job quietly requires "postgres", nothing matches it,
 * and routing gets slightly worse for a reason nobody can see.
 */
func TestEveryCapabilityAJobAsksForIsDescribed(t *testing.T) {
	jobs, capabilities := Seed()

	described := map[string]bool{}

	for _, c := range capabilities {
		described[c.ID] = true
	}

	for _, job := range jobs {
		for _, need := range job.Needs {
			if !described[need.ID] {
				t.Errorf("%s asks for %q, which nothing describes", job.ID, need.ID)
			}
		}
	}
}

// And nothing is described that no job asks for. A capability nobody needs is
// either a job that was never written or a name that was changed in one place.
func TestEveryCapabilityDescribedIsAskedFor(t *testing.T) {
	jobs, capabilities := Seed()

	wanted := map[string]bool{}

	for _, job := range jobs {
		for _, need := range job.Needs {
			wanted[need.ID] = true
		}
	}

	for _, c := range capabilities {
		if !wanted[c.ID] {
			t.Errorf("nothing asks for %q", c.ID)
		}
	}
}

/*
 * A job that is nothing but a title is not a job definition.
 *
 * The description is what a model is shown when it is deciding whether this
 * is the right person, and an empty one makes the choice worse than having no
 * entry at all — it looks like an answer.
 */
func TestEveryJobSaysWhatItIsAndWhatItNeeds(t *testing.T) {
	jobs, _ := Seed()

	if len(jobs) < 80 {
		t.Fatalf("only %d jobs in the seed", len(jobs))
	}

	seen := map[string]bool{}

	for _, job := range jobs {
		if seen[job.ID] {
			t.Errorf("%s is in the table twice", job.ID)
		}

		seen[job.ID] = true

		if !strings.Contains(job.ID, ".") {
			t.Errorf("%s is not filed under a category", job.ID)
		}

		if job.Title == "" || job.Description == "" {
			t.Errorf("%s has no title or no description", job.ID)
		}

		if len(job.Needs) == 0 {
			t.Errorf("%s asks for nothing", job.ID)
		}

		if job.CameFrom != store.FromSeed {
			t.Errorf("%s says it came from %q", job.ID, job.CameFrom)
		}
	}
}

/*
 * Work that can hurt somebody is marked as such.
 *
 * Not as a gate — permits is the gate — but because the interface says it and
 * the brief says it, and a seed that quietly filed "physician" as ordinary
 * work would make both of those lie. Checked here rather than trusted,
 * because it is exactly the sort of field that gets forgotten on the tenth
 * entry.
 */
func TestWorkThatNeedsAPersonSaysSo(t *testing.T) {
	jobs, _ := Seed()

	for _, job := range jobs {
		category, _, _ := strings.Cut(job.ID, ".")

		switch category {
		case Health, Legal:
			if !job.Oversight {
				t.Errorf("%s does not ask for a person to stay in the loop", job.ID)
			}

			if job.Risk != store.RiskCritical && job.Risk != store.RiskHigh {
				t.Errorf("%s is filed as %q risk", job.ID, job.Risk)
			}
		}
	}
}

/*
 * An id made from a title keeps the letters it was given.
 *
 * Including the ones that are not in the English alphabet: an id built from a
 * Bulgarian label by stripping everything it did not recognise would be a row
 * of underscores, and two different jobs would collide into the same one.
 */
func TestAnIdKeepsTheLettersItWasGiven(t *testing.T) {
	for _, c := range []struct{ from, want string }{
		{"Backend Engineer", "backend_engineer"},
		{"  C++ developer ", "c_developer"},
		{"Site Reliability Engineer (SRE)", "site_reliability_engineer_sre"},
		{"Заварчик", "заварчик"},
		{"данъчен консултант", "данъчен_консултант"},
	} {
		if got := MakeID(c.from); got != c.want {
			t.Errorf("MakeID(%q) = %q, want %q", c.from, got, c.want)
		}
	}
}

// Every category in the table is one the interface has a name for, or it
// appears in the list of shelves as a bare identifier nobody can read.
func TestEveryCategoryHasAReadableName(t *testing.T) {
	jobs, _ := Seed()

	for _, job := range jobs {
		category, _, _ := strings.Cut(job.ID, ".")

		if TitleOf(category) == category {
			t.Errorf("%q has no readable name", category)
		}
	}
}
