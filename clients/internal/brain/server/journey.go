package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"pn-brain/internal/brain/copies"
	"pn-brain/internal/brain/paths"
	"pn-brain/internal/brain/storage"
)

/*
 * Two things that happen to a brain kept on a drive: it runs out of room, and
 * it gets carried somewhere else.
 *
 * Both had answers already and both answers were a terminal command, which for
 * this program is the same as not having one. Nobody moves their assistant to
 * a bigger disk by remembering a flag.
 */

// handleJourney says whether the brain has been opened somewhere new, and what
// that costs.
func (s *Server) handleJourney(w http.ResponseWriter, r *http.Request) {
	j := s.brain.Travelled()

	if j == nil {
		ok(w, map[string]any{"travelled": false})

		return
	}

	ok(w, map[string]any{
		"travelled":  true,
		"from":       j.From,
		"to":         j.To,
		"old_prefix": j.OldPrefix,
		"new_prefix": j.NewPrefix,
		"affected":   j.Affected,
	})
}

/*
 * handleRepairJourney rewrites the paths in what it knows.
 *
 * Long: every repaired memory is re-embedded, which is seconds each. Run to
 * completion rather than in the background, because it is the one operation
 * where a half-finished state is worse than either end of it — half the
 * memories naming the old drive and half the new, with no record of which.
 */
func (s *Server) handleRepairJourney(w http.ResponseWriter, r *http.Request) {
	j := s.brain.Travelled()

	if j == nil {
		fail(w, http.StatusBadRequest, "nothing has moved; there is nothing to repair")

		return
	}

	changed, embedded, err := s.brain.RepairJourney(r.Context(), *j)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"repaired":    changed,
		"re_embedded": embedded,
		"from":        j.OldPrefix,
		"to":          j.NewPrefix,
	})
}

// handleForgetJourney puts the offer away for somebody who does not want it.
//
// Without this the banner is a question with no "no" — asked again at every
// start, forever, about paths its owner may have stopped caring about.
func (s *Server) handleForgetJourney(w http.ResponseWriter, r *http.Request) {
	s.brain.ForgetJourney()

	ok(w, map[string]any{"forgotten": true})
}

/*
 * handleMoveHome moves the brain to another drive.
 *
 * Done as a copy that is then chosen, rather than as a move: the new one is
 * written and verified before anything about the old one changes, so there is
 * no moment where the only copy is half-written. What was the brain is left on
 * the old drive, complete, with its marker renamed so that nothing finds two
 * brains and has to guess between them.
 *
 * Nothing is deleted. Somebody who has just moved their entire memory to a new
 * disk wants to see it working there before the old one goes, and that is
 * their decision to make rather than a step in this one.
 */
func (s *Server) handleMoveHome(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "could not read that request")

		return
	}

	destination := filepath.Clean(body.Path)

	if !filepath.IsAbs(destination) {
		fail(w, http.StatusBadRequest, "that has to be a full path, starting from the top of the disk")

		return
	}

	// Everything a copy refuses, a move refuses for the same reasons: the
	// brain's own folder, somewhere inside it, and another brain.
	if _, err := copies.Write(r.Context(), s.brain.DB, s.brain.Root, destination); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	if err := copies.Use(destination); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if _, err := paths.Choose(destination); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	/*
	 * And the old folder stops claiming to be the brain.
	 *
	 * Renamed rather than deleted: everything is still there and still
	 * readable, but the search that looks for a brain on every attached drive
	 * will not find two of them. Two folders both saying "I am the brain" is
	 * how somebody ends up talking to the old one for a week.
	 */
	stepped := filepath.Join(s.brain.Root, "brain-was-here.json")

	if err := os.Rename(filepath.Join(s.brain.Root, paths.Marker), stepped); err != nil {
		s.log.Warn("the brain was moved but the old folder still claims to be one",
			"where", s.brain.Root, "error", err)
	}

	ok(w, map[string]any{
		"moved": destination,
		"from":  s.brain.Root,
		"note": "Restart PN Brain to use it there. Everything is still in the old " +
			"folder as well — delete it yourself once you are happy.",
	})
}

/*
 * handleWhereItCouldLive is the drives a whole brain would fit on.
 *
 * Different from the list for copies in one way that matters: the drive the
 * brain is already on is excluded, because moving something to where it
 * already is is not a move — and free space is measured against what the
 * brain actually takes rather than against a fixed floor.
 */
func (s *Server) handleWhereItCouldLive(w http.ResponseWriter, r *http.Request) {
	drives, err := storage.Drives(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	report := storage.Check(s.brain.Root, filepath.Join(s.brain.Root, "brain.sqlite"))

	out := make([]map[string]any, 0, len(drives))

	for _, d := range drives {
		if d.Current || !d.Writable {
			continue
		}

		out = append(out, map[string]any{
			"mount_point": d.MountPoint,
			"suggested":   filepath.Join(d.MountPoint, "PN-BRAIN-DATA"),
			"free_bytes":  d.FreeBytes,
			"removable":   d.Removable,
			"fits":        int64(d.FreeBytes) > report.DatabaseBytes*int64(copiesRoom),
		})
	}

	/*
	 * And the home folder, which is not a drive and appears in no list of
	 * them.
	 *
	 * Without it this list comes back empty on the machine it was written for:
	 * the brain is on the external drive, so that is excluded as where it
	 * already lives, and the internal disk is mounted at / which an ordinary
	 * user cannot write to. A move offered nowhere at all, on a machine with
	 * room to spare in the owner's own folder.
	 */
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		var on storage.Drive

		for _, d := range drives {
			if strings.HasPrefix(home, d.MountPoint) && len(d.MountPoint) > len(on.MountPoint) {
				on = d
			}
		}

		if !on.Current && canWriteHome(home) {
			out = append(out, map[string]any{
				"mount_point": home,
				"suggested":   filepath.Join(home, "PN-BRAIN-DATA"),
				"free_bytes":  on.FreeBytes,
				"removable":   false,
				"home":        true,
				"fits":        int64(on.FreeBytes) > report.DatabaseBytes*int64(copiesRoom),
			})
		}
	}

	facts, _ := s.brain.DB.CountFacts()

	ok(w, map[string]any{
		"here":           s.brain.Root,
		"database_bytes": report.DatabaseBytes,
		"free_bytes":     report.FreeBytes,
		"level":          report.Level,
		"drives":         out,
		// What the numbers mean for somebody worrying about room, said rather
		// than left to be worked out from two byte counts.
		"outlook": outlook(report, facts),
	})
}

// copiesRoom mirrors the headroom a copy needs, so "fits" here and a refusal
// there cannot disagree.
const copiesRoom = 3

/*
 * outlook turns the free space into the question people actually have.
 *
 * Nobody wants to know how many bytes a database is. They want to know whether
 * running out of room is a problem they have to plan for, and for a brain of a
 * few megabytes on a disk with hundreds of gigabytes free the honest answer is
 * that it is not — which is worth saying plainly rather than leaving somebody
 * to infer it from two numbers.
 */
func outlook(r storage.Report, facts int) string {
	if r.DatabaseBytes <= 0 || r.FreeBytes == 0 || facts <= 0 {
		return ""
	}

	each := r.DatabaseBytes / int64(facts)

	if each < 1 {
		each = 1
	}

	room := int64(r.FreeBytes) / each

	switch {
	case room >= 1_000_000:
		return "There is room here for millions more things than it knows now. " +
			"Running out of space is not something this brain is going to do."

	case room >= 1_000:
		return fmt.Sprintf("There is room here for roughly %d thousand more things.", room/1000)
	}

	return fmt.Sprintf("There is room here for roughly %d more things.", room)
}
