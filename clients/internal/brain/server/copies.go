package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/copies"
	"pn-scripts-assistant/internal/brain/paths"
	"pn-scripts-assistant/internal/brain/storage"
)

/*
 * The brain in more than one place.
 *
 * Everything this brain knows is one file on one disk that gets carried around
 * in a bag. These endpoints are the whole answer to that, and they are
 * deliberately small: list the copies, add a place, copy now, and — separately
 * and never automatically — choose a copy to become the brain.
 *
 * Copying is exposed over HTTP where moving the brain is not, and the
 * difference is that a copy takes nothing away. A move deletes the original,
 * which does not belong behind a request that can be fired twice by an
 * impatient click.
 */
func (s *Server) handleCopies(w http.ResponseWriter, r *http.Request) {
	list, err := copies.Status(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"root":   s.brain.Root,
		"copies": shown(list),
		// Where a copy could go, so somebody does not have to know the path of
		// a drive they have just plugged in.
		"drives": placesForACopy(s.brain.Root),
	})
}

// shown adds what the page would otherwise have to work out for itself: how
// old each copy is, in words, and whether it is behind what the brain knows.
func shown(list []copies.Copy) []map[string]any {
	out := make([]map[string]any, 0, len(list))

	for _, c := range list {
		row := map[string]any{
			"path":      c.Path,
			"short":     copies.Short(c.Path),
			"facts":     c.Facts,
			"bytes":     c.Bytes,
			"reachable": c.Reachable,
			"trouble":   c.Trouble,
			"never":     c.Never(),
		}

		if !c.Never() {
			row["at"] = c.At
			row["age_seconds"] = int(time.Since(c.At).Seconds())
		}

		out = append(out, row)
	}

	return out
}

/*
 * placesForACopy is the drives a copy would fit on.
 *
 * The drive the brain is already on is left out. A copy on the same disk
 * survives nothing that matters — the disk failing, the disk being lost, the
 * drive being reformatted by a machine that did not recognise it — and
 * offering it would be offering a backup that is not one.
 */
func placesForACopy(root string) []map[string]any {
	drives, err := storage.Drives(root)
	if err != nil {
		return nil
	}

	out := make([]map[string]any, 0, len(drives))

	for _, d := range drives {
		if d.Current || !d.Writable || d.FreeBytes < storage.MinimumUsableBytes {
			continue
		}

		out = append(out, map[string]any{
			"mount_point": d.MountPoint,
			"suggested":   filepath.Join(d.MountPoint, "PN-SCRIPTS-ASSISTANT-COPY"),
			"free_bytes":  d.FreeBytes,
			"removable":   d.Removable,
		})
	}

	return append(out, homeAsAPlace(drives)...)
}

/*
 * homeAsAPlace offers the home folder, which is not a drive and so appears in
 * no list of them.
 *
 * On the machine this was written for, that list came back empty: the brain
 * lives on the external drive, so that one is excluded as the drive it is
 * already on, and the internal disk is mounted at / which an ordinary user
 * cannot write to. Every drive was therefore either the brain's own or
 * unwritable, and the panel offered nowhere at all — on a machine with 200GB
 * free in the owner's own home folder.
 *
 * Left out when home is on the same disk as the brain. A copy there would
 * survive a deleted folder and nothing else: not the disk failing, not the
 * drive being lost, which are the things it is for.
 */
func homeAsAPlace(drives []storage.Drive) []map[string]any {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}

	// Which drive home sits on, by longest matching mount point — every path
	// starts with "/", so a plain prefix test matches the root filesystem for
	// everything and would answer the same wherever home actually is.
	var on storage.Drive

	for _, d := range drives {
		if strings.HasPrefix(home, d.MountPoint) && len(d.MountPoint) > len(on.MountPoint) {
			on = d
		}
	}

	if on.Current || !canWriteHome(home) {
		return nil
	}

	return []map[string]any{{
		"mount_point": home,
		"suggested":   filepath.Join(home, "PN-SCRIPTS-ASSISTANT-COPY"),
		"free_bytes":  on.FreeBytes,
		"removable":   false,
		"home":        true,
	}}
}

// canWriteHome asks the only question that matters about a folder offered as a
// destination, by trying it rather than reasoning about permissions.
func canWriteHome(home string) bool {
	f, err := os.CreateTemp(home, ".pn-scripts-assistant-check-*")
	if err != nil {
		return false
	}

	name := f.Name()
	f.Close()
	os.Remove(name)

	return true
}

func (s *Server) handleKeepCopy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "could not read that request")

		return
	}

	if err := copies.Keep(s.brain.Root, body.Path); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	/*
	 * And the first copy is made now, not at the next tick.
	 *
	 * Adding a backup drive and being told it will be used within ten minutes
	 * is the kind of promise nobody believes until they have seen it happen.
	 * The database is small enough that this is a second or two.
	 */
	made, err := copies.Write(r.Context(), s.brain.DB, s.brain.Root, body.Path)
	if err != nil {
		ok(w, map[string]any{"kept": body.Path, "copied": false, "trouble": err.Error()})

		return
	}

	ok(w, map[string]any{"kept": body.Path, "copied": true, "facts": made.Facts, "bytes": made.Bytes})
}

func (s *Server) handleStopCopy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "could not read that request")

		return
	}

	if err := copies.Stop(s.brain.Root, body.Path); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// Said back plainly, because "stop" is the word somebody clicked and they
	// are entitled to know it did not mean "delete".
	ok(w, map[string]any{
		"stopped": body.Path,
		"note":    "The copy already on that drive is untouched.",
	})
}

func (s *Server) handleCopyNow(w http.ResponseWriter, r *http.Request) {
	made := copies.WriteAll(r.Context(), s.brain.DB, s.brain.Root)

	ok(w, map[string]any{"copies": shown(made)})
}

/*
 * handleUseCopy makes a copy into the brain.
 *
 * The one operation here that changes which memory is the real one, so it is
 * the one that does the least: it marks the folder as a brain and records it
 * as the place to open next time. It does not touch the brain that is running
 * now, and it does not delete anything anywhere.
 *
 * Taking effect needs a restart, and saying so is part of the answer — a
 * button that appears to have done nothing is worse than one that explains
 * what has to happen next.
 */
func (s *Server) handleUseCopy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "could not read that request")

		return
	}

	if err := copies.Use(body.Path); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	if _, err := paths.Choose(body.Path); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"using": body.Path,
		"note":  "Restart PN Scripts Assistant to open this one. The brain it was copied from is still where it was.",
	})
}
