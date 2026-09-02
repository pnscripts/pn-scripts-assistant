package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"pn-brain/internal/brain/places"
	"pn-brain/internal/brain/storage"
)

/*
 * The drives and folders the brain looks after.
 *
 * Adding one is a small action with a large consequence — everything in that
 * folder becomes something the brain knows — so it is a thing somebody does
 * here, deliberately, rather than something the program decides by finding a
 * drive attached.
 *
 * Removing one is smaller than it looks and says so: what was already learned
 * stays. Knowledge is knowledge whatever drive it came off, and a button that
 * silently unlearned a thousand things would be the worst button in the
 * program.
 */
func (s *Server) handlePlaces(w http.ResponseWriter, r *http.Request) {
	list, err := places.Status(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	out := make([]map[string]any, 0, len(list))

	for _, p := range list {
		row := map[string]any{
			"path":      p.Path,
			"short":     places.Short(p.Path),
			"name":      p.Name,
			"kind":      p.Kind,
			"learned":   p.Learned,
			"waiting":   p.Waiting,
			"reachable": p.Reachable,
			"trouble":   p.Trouble,
			"never":     p.Never(),
		}

		if !p.Never() {
			row["last_learned"] = p.LastLearned
			row["age_seconds"] = int(time.Since(p.LastLearned).Seconds())
		}

		if !p.LastSeen.IsZero() {
			row["last_seen"] = p.LastSeen
		}

		out = append(out, row)
	}

	ok(w, map[string]any{
		"places": out,
		// Somewhere to start from, so nobody has to know what the system
		// called the drive they just plugged in.
		"drives": placesToLearnFrom(s.brain.Root),
	})
}

/*
 * placesToLearnFrom is the attached drives, as somewhere to point at.
 *
 * The brain's own drive is included here, unlike for copies: a copy on the
 * drive the brain lives on protects nothing, but the work somebody wants
 * learned is very often on exactly that drive.
 */
func placesToLearnFrom(root string) []map[string]any {
	drives, err := storage.Drives(root)
	if err != nil {
		return nil
	}

	out := make([]map[string]any, 0, len(drives))

	for _, d := range drives {
		if !d.Removable && !d.Current {
			// Everything else on the machine is a system mount, and offering
			// twenty of them is not offering a choice.
			continue
		}

		out = append(out, map[string]any{
			"mount_point": d.MountPoint,
			"free_bytes":  d.FreeBytes,
			"removable":   d.Removable,
		})
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, map[string]any{
			"mount_point": home,
			"home":        true,
		})
	}

	return out
}

func (s *Server) handleWatchPlace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
		Kind string `json:"kind"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "could not read that request")

		return
	}

	place, err := places.Watch(s.brain.Root, body.Path, body.Name, body.Kind)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	/*
	 * And how much is in there, counted now.
	 *
	 * Scanning is seconds where learning is hours, so the one thing somebody
	 * gets immediately is the size of what they have just asked for — which is
	 * the difference between a button that appears to do nothing and one that
	 * says "2386 things, I will work through them".
	 *
	 * Unless the drive is not plugged in, which is a perfectly ordinary way to
	 * add one. Counting an absent folder finds nothing in it, and reporting
	 * that as "nothing to read" about a drive nobody has looked at yet would
	 * be a plain falsehood — it is counted when it next appears.
	 */
	if _, err := os.Stat(place.Path); err != nil {
		ok(w, map[string]any{
			"watching": place.Path,
			"name":     place.Name,
			"counted":  false,
			"note":     "Not attached right now — I will read it when it is.",
		})

		return
	}

	waiting, err := places.Count(s.brain.DB, s.brain.Cfg.Owner, place)
	if err != nil {
		ok(w, map[string]any{"watching": place.Path, "name": place.Name, "counted": false})

		return
	}

	place.Waiting = waiting
	places.Note(s.brain.Root, place)

	ok(w, map[string]any{
		"watching": place.Path,
		"name":     place.Name,
		"counted":  true,
		"waiting":  waiting,
		// Said in minutes because that is the unit of the wait, and it is a
		// long one on this hardware.
		"minutes": waiting * places.SecondsEach / 60,
	})
}

func (s *Server) handleForgetPlace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "could not read that request")

		return
	}

	if err := places.Forget(s.brain.Root, body.Path); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"forgotten": body.Path,
		"note":      "It will not be read again. What it already taught me is kept.",
	})
}

func (s *Server) handleRenamePlace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "could not read that request")

		return
	}

	if err := places.Rename(s.brain.Root, body.Path, body.Name); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"renamed": body.Path, "name": body.Name})
}

/*
 * handleLookNow takes one bite out of a place on request.
 *
 * The same bite the background pass takes, not a special larger one: the cap
 * is what keeps this from being an hours-long request that a closed window
 * would abandon halfway.
 */
func (s *Server) handleLookNow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}

	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)

	list, err := places.Status(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	want := filepath.Clean(body.Path)

	for _, p := range list {
		if p.Path != want {
			continue
		}

		if !p.Reachable {
			fail(w, http.StatusBadRequest, p.Trouble)

			return
		}

		pass, err := places.Look(r.Context(), s.brain.Learner, s.brain.DB, s.brain.Cfg.Owner, p)

		places.Note(s.brain.Root, pass.Place)

		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())

			return
		}

		ok(w, map[string]any{
			"read":     pass.Took,
			"learned":  pass.Learned,
			"known":    pass.Known,
			"waiting":  pass.Place.Waiting,
			"finished": pass.Finished,
		})

		return
	}

	fail(w, http.StatusNotFound, "that is not one of the places I look after")
}
