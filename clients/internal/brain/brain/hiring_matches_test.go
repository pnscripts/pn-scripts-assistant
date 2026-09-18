package brain

import (
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/team"
)

/*
 * The near misses an audit found, each pinned: the right job, nobody when the
 * catalogue has nobody, and — for work only a qualified person may do — an
 * advisory hire that names that person.
 */
func TestHiringNearMisses(t *testing.T) {
	b := quietBrain(t)
	in := b.hiringInputs()

	cases := []struct {
		say, job     string
		professional bool
		existing     string
	}{
		{say: "hire a book writer", job: "writer"},
		{say: "hire a financial advisor", job: "financial_planner", professional: true},
		{say: "hire a tax advisor", job: "tax_specialist", professional: true},
		{say: "hire a level designer", job: "level_designer"},
		{say: "hire a game tester", job: "qa_engineer"},
		{say: "hire a pen tester", job: "penetration_tester", professional: true},
		{say: "hire a doctor", job: "physician", professional: true},
		{say: "hire a lawyer", job: "lawyer", professional: true},
		{say: "hire an accountant", job: "accountant", professional: true},
		{say: "hire a data engineer", job: "data_engineer"},
		{say: "hire a javascript developer", job: "javascript_developer"},
		{say: "Hire a game development expert", existing: "game_developer"},
	}

	for _, c := range cases {
		p, err := team.Propose(in, team.Wish{Sentence: c.say})
		if err != nil {
			t.Errorf("%s: %v", c.say, err)

			continue
		}

		if c.existing != "" {
			if p.Existing == nil || p.Existing.Name != c.existing {
				t.Errorf("%s: expected %s, who is already here; got %+v", c.say, c.existing, p.Existing)
			}

			continue
		}

		if p.Existing != nil {
			t.Errorf("%s: answered by %s, who is already here but not this", c.say, p.Existing.Name)

			continue
		}

		if !strings.HasSuffix(p.Job.ID, c.job) {
			t.Errorf("%s: job %s, not %s", c.say, p.Job.ID, c.job)
		}

		if c.professional {
			if len(p.Professional) == 0 || !strings.Contains(p.Agent.Title, "(advisory)") && len(p.Packages) == 0 {
				t.Errorf("%s: proposed as %q with no qualified person named: %v", c.say, p.Agent.Title, p.Professional)
			}

			for _, tool := range p.Agent.Tools {
				if tool == "run_command" || tool == "set_device" || tool == "send_email" {
					t.Errorf("%s: an advisory hire may use %s", c.say, tool)
				}
			}
		}
	}

	// Nobody, when there is nobody — not somebody else under the name.
	for _, say := range []string{"hire a security guard", "hire a gas fitter"} {
		if p, err := team.Propose(in, team.Wish{Sentence: say}); err == nil {
			t.Errorf("%s: proposed %s (%s)", say, p.Agent.Title, p.Job.ID)
		}
	}

	// Whoever may run commands may read and write files as well.
	p, _ := team.Propose(in, team.Wish{Sentence: "hire a javascript developer"})

	for _, want := range []string{"read_file", "write_file"} {
		if !strings.Contains(strings.Join(p.Agent.Tools, ","), want) {
			t.Errorf("a developer who may run commands cannot %s: %v", want, p.Agent.Tools)
		}
	}
}
