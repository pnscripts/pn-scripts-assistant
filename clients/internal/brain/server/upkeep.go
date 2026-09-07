package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"pn-scripts-assistant/internal/brain/godot"
	"pn-scripts-assistant/internal/brain/selfupdate"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * What is out of date, everywhere, in one answer.
 *
 * The pieces were each checked somewhere: the parts list knew whether Ollama
 * had a newer release, the Godot tool asked its own project, the program knew
 * its own commit and told nobody. So "is anything out of date" was a question
 * with four answers in four places and no way to ask it once.
 *
 * One check, and it says how many rather than that there are some — a number
 * is something a person can decide about and "updates available" is a badge
 * they learn to ignore.
 */

// Upkeep is one thing that could be brought up to date.
type Upkeep struct {
	Name string `json:"name"`

	// Have and Latest, both, because "out of date" without the two versions is
	// an assertion rather than a fact.
	Have   string `json:"have,omitempty"`
	Latest string `json:"latest,omitempty"`

	Newer bool `json:"newer"`

	// How it is brought up to date, so the interface can offer the right
	// button rather than guessing from the name.
	How string `json:"how"`

	// Why anything could not be determined. A check that quietly reports "up
	// to date" because it never looked is worse than one that says so.
	Why string `json:"why,omitempty"`
}

// handleUpkeep checks everything that can be out of date.
func (s *Server) handleUpkeep(w http.ResponseWriter, r *http.Request) {
	/*
	 * Every one of these asks somebody else's server, so privacy decides.
	 *
	 * An update check is a small thing to leak and it is still a request that
	 * says this machine exists and runs this software at this version.
	 */
	if !s.brain.Router.Mode().AllowsWeb() {
		ok(w, map[string]any{
			"items": []Upkeep{},
			"why": "Checking for updates means asking other people's servers, " +
				"and your privacy setting keeps this machine to itself.",
		})

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	items := []Upkeep{}

	// The program itself.
	self := selfupdate.Check(ctx, nil)

	program := Upkeep{
		Name:   "The assistant",
		Have:   self.Running,
		Latest: self.Latest,
		Newer:  self.Newer,
		How:    "self",
		Why:    self.Why,
	}

	if self.Dirty {
		program.Have += " + your own changes"
	}

	if self.Behind > 0 {
		program.Latest = fmt.Sprintf("%s (%d commits ahead)", self.Latest, self.Behind)
	}

	items = append(items, program)

	// The game engine, which publishes releases and says so plainly.
	if engine, installed := godot.Find(); installed {
		item := Upkeep{Name: "Godot", Have: engine.Version, How: "part:Godot (making games)"}

		if latest, err := godot.Latest(ctx, nil); err == nil {
			item.Latest = latest.Version
			item.Newer = godot.Newer(engine.Version, latest.Version)
		} else {
			item.Why = err.Error()
		}

		items = append(items, item)
	}

	/*
	 * And everything the requirements already know about.
	 *
	 * They carry their own version check — the one that put "0.33.3 available"
	 * beside Ollama — so this reads what is already there rather than asking
	 * again in a second way that could disagree with the first.
	 */
	for _, req := range preflight.Requirements() {
		state, detail := req.Check()

		if state != preflight.Outdated {
			continue
		}

		items = append(items, Upkeep{
			Name:   req.Name,
			Have:   detail,
			Newer:  true,
			How:    "part:" + req.Name,
			Latest: "",
		})
	}

	var waiting int

	for _, i := range items {
		if i.Newer {
			waiting++
		}
	}

	ok(w, map[string]any{"items": items, "waiting": waiting})
}

/*
 * handleSelfUpdate pulls and rebuilds, behind the conversation.
 *
 * A background job because it is a network fetch and a compile, and because
 * the thing being replaced is the program serving this request — a handler
 * that waited for its own replacement would be holding a connection open
 * across the moment its binary changed.
 */
func (s *Server) handleSelfUpdate(w http.ResponseWriter, r *http.Request) {
	if s.brain.Jobs == nil {
		fail(w, http.StatusInternalServerError, "There is nowhere to run that.")

		return
	}

	_, err := s.brain.Jobs.Start("Updating the assistant", func(ctx context.Context) (string, error) {
		return selfupdate.Update(ctx, func(note string) {
			s.brain.Log.Info("updating the assistant", "step", note)
		})
	})
	if err != nil {
		fail(w, http.StatusConflict, err.Error())

		return
	}

	ok(w, map[string]any{"started": "Updating in the background. " +
		"It appears under Activity, and you will need to reopen the program at the end."})
}

// so encoding/json stays used if the shapes above change.
var _ = json.Marshal
