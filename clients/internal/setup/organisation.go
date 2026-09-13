package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/godot"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/pictures"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * The organisation, checked on the last step like everything else.
 *
 * Setup proves the model answers and the voice speaks, and until now said
 * nothing about who does the work — which on a brain that has been carried to
 * this machine on a drive is the part most likely to have gone wrong. The
 * chart and the agents are hand-editable files, and one renamed seat leaves an
 * agent sitting nowhere: it still answers, it routes badly, and nothing says
 * why. That is the same fault the rest of this step exists to catch, one
 * layer up.
 *
 * And the organisation travels while the machine does not. A game developer
 * on a machine with no Godot, or a researcher while nothing may leave this
 * one, is somebody on the chart who cannot do their job here. Said on the
 * last step, because this is the machine being set up and the only place
 * where that is a fact rather than a surprise halfway through a task.
 *
 * Read from the files and from what ships, never from the database. Setup
 * runs before the brain has opened one, and a check that needed it would
 * have nothing to say on exactly the first run it is for.
 */

// shippedJobs is the taxonomy as it comes with the program, which is all a
// brain that has never started has.
type shippedJobs map[string]*store.Occupation

func jobsThatShip() shippedJobs {
	jobs, _ := occupations.Seed()
	out := make(shippedJobs, len(jobs))

	for i := range jobs {
		out[jobs[i].ID] = &jobs[i]
	}

	return out
}

func (s shippedJobs) Occupation(id string) (*store.Occupation, error) {
	if job, ok := s[id]; ok {
		return job, nil
	}

	return nil, fmt.Errorf("no job called %s", id)
}

// organisationTicks are the two questions about the organisation: does it
// hold together, and can the people on it work on this machine.
func (s *Server) organisationTicks(cfg config.Config) []tick {
	root := s.chosenRoot()
	chart := org.Chart(root)
	roster := team.Roster(root)
	jobs := jobsThatShip()

	together := holdsTogether(chart, roster, jobs)

	// And the classifications this machine has downloaded, which the brain
	// reads in itself — said here so the count above is not taken as all
	// there will be.
	var fetched []string

	for _, c := range []struct{ source, name string }{{"esco", "ESCO"}, {"onet", "O*NET"}} {
		if found, _ := filepath.Glob(filepath.Join(preflight.CatalogueFolder(c.source), "*.zip")); len(found) > 0 {
			fetched = append(fetched, c.name)
		}
	}

	if together.State == "yes" && len(fetched) > 0 {
		together.Note += " · " + strings.Join(fetched, " and ") + " downloaded, read in when it opens"
	}

	return []tick{
		together,
		canWorkHere(chart, roster, jobs, here{
			freedom:  permits.Freedom(cfg.Asking()),
			godot:    godotHere,
			pictures: func() bool { return pictures.Reachable(cfg.PicturesURL) },
			hosted:   cfg.HasPaidProvider(),
		}),
	}
}

/*
 * holdsTogether is whether every agent sits somewhere real and every seat
 * names a job the program knows.
 *
 * Both are the shape a hand edit breaks, and both fail quietly: an agent in a
 * seat that is not there keeps working with no job behind it, so it is
 * shortlisted on its one-line description alone and loses to whoever wrote a
 * longer one.
 */
func holdsTogether(chart []org.Unit, roster []team.Agent, jobs shippedJobs) tick {
	t := tick{ID: "organisation", What: "It has people to do the work"}

	problems := []string{}
	filled := map[string]bool{}
	working := 0

	for _, agent := range roster {
		if !agent.Working() {
			continue
		}

		working++

		if agent.Position == "" {
			continue
		}

		if _, _, ok := org.Seat(chart, agent.Position); !ok {
			problems = append(problems, fmt.Sprintf("%s sits in %q, which is not on the chart",
				orTitle(agent), agent.Position))

			continue
		}

		filled[agent.Position] = true
	}

	seats := org.Seats(chart)

	for _, seat := range seats {
		if seat.Job == "" {
			continue
		}

		if _, err := jobs.Occupation(seat.Job); err != nil {
			problems = append(problems, fmt.Sprintf("the %s seat names a job it does not know, %q",
				strings.ToLower(orName(seat.Title, seat.Name)), seat.Job))
		}
	}

	if working == 0 {
		t.State, t.Note = "no", "nobody on the roster is working — every agent is suspended or retired"

		return t
	}

	if len(problems) > 0 {
		t.State, t.Note = "no", problems[0]

		if len(problems) > 1 {
			t.Note += fmt.Sprintf(" (and %d more — see Organisation)", len(problems)-1)
		}

		return t
	}

	empty := 0

	for _, seat := range seats {
		if !filled[seat.Name] {
			empty++
		}
	}

	t.State = "yes"
	t.Note = fmt.Sprintf("%d working, %d seats of which %d empty · %d jobs to hire from",
		working, len(seats), empty, len(jobs))

	return t
}

// here is what this machine has, as far as the people on the chart are
// concerned. Functions rather than answers so a test can be a machine it is
// not.
type here struct {
	freedom  permits.Freedom
	godot    func() bool
	pictures func() bool

	// hosted is whether a paid service is configured, which is the other way
	// a picture can be made.
	hosted bool
}

func godotHere() bool {
	_, found := godot.Find()

	return found
}

/*
 * A need is what a tool depends on beyond the program itself, and what to say
 * when that is missing.
 *
 * Only the dependencies that differ between machines, and only for tools some
 * agent is actually narrowed to. The generalist has every tool and so can
 * always do something; a researcher whose list is mostly the web cannot.
 */
type need struct {
	tools   []string
	missing func(here) string
}

var needs = []need{
	{
		tools: []string{"web_search", "fetch_url", "read_a_page"},
		missing: func(h here) string {
			if llm.ModeFor(string(h.freedom)).AllowsWeb() {
				return ""
			}

			return "the web is closed while it asks first"
		},
	},
	{
		tools: []string{"godot_status", "godot_docs", "godot_build"},
		missing: func(h here) string {
			if h.godot == nil || h.godot() {
				return ""
			}

			return "Godot is not installed"
		},
	},
	{
		tools: []string{"make_a_picture", "make_a_video"},
		missing: func(h here) string {
			if h.pictures != nil && h.pictures() {
				return ""
			}

			// The other way is a paid service, which only exists when one is
			// set up and something may leave this machine.
			if h.hosted && llm.ModeFor(string(h.freedom)) != llm.ModePrivate {
				return ""
			}

			return "nothing here can make a picture"
		},
	},
}

/*
 * canWorkHere names who on the chart is held back on this machine, and by
 * what.
 *
 * Never "no". An agent missing a tool is working exactly as configured on a
 * machine that lacks something, and red on the last step of setup would read
 * as setup having failed — so it is the warning mark, with the name and the
 * reason, and the fix belongs to whichever card installs that piece.
 */
func canWorkHere(chart []org.Unit, roster []team.Agent, jobs shippedJobs, machine here) tick {
	t := tick{ID: "work", What: "Everyone can do their job on this machine"}

	held := []string{}

	for _, agent := range roster {
		if !agent.Working() {
			continue
		}

		fit := team.Settle(agent, chart, jobs, nil)

		if !fit.Narrowed {
			continue
		}

		reasons := []string{}

		for _, n := range needs {
			if !usesAny(fit.Tools, n.tools) {
				continue
			}

			if why := n.missing(machine); why != "" {
				reasons = append(reasons, why)
			}
		}

		if len(reasons) > 0 {
			held = append(held, orTitle(agent)+": "+strings.Join(reasons, ", "))
		}
	}

	if len(held) == 0 {
		t.State = "yes"

		return t
	}

	t.State, t.Note = "skip", strings.Join(held, " · ")

	return t
}

func usesAny(have, wanted []string) bool {
	for _, one := range have {
		for _, w := range wanted {
			if one == w {
				return true
			}
		}
	}

	return false
}

func orTitle(agent team.Agent) string { return orName(agent.Title, agent.Name) }

func orName(title, name string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}

	return name
}

/*
 * saveFreedom is the one switch, set from setup.
 *
 * Both settings in one write, for the reason the running program gives:
 * written in two steps once, the file ended up saying private while the
 * program was open. Setup is the first place the switch can be thrown and
 * must not be the place it starts disagreeing with itself.
 */
func (s *Server) saveFreedom(level string) (permits.Freedom, error) {
	f := permits.Freedom(strings.ToLower(strings.TrimSpace(level)))

	if !permits.Known(f) {
		return "", fmt.Errorf("%q is not a choice here — it is ask, granted or everything", level)
	}

	err := s.writeSettings(0o600,
		[2]string{"BRAIN_FREEDOM", string(f)},
		[2]string{"BRAIN_PRIVACY", string(llm.ModeFor(string(f)))},
	)

	return f, err
}
