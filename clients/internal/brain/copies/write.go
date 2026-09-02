package copies

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pn-brain/internal/brain/storage"
)

/*
 * Room is how much more space a copy needs than the database it holds.
 *
 * The snapshot is written beside the old one and only takes its place when it
 * is complete, so both exist at once for a moment — and a copy that fills the
 * destination on the way to replacing a good one leaves nothing behind at all.
 */
const Room = 2.2

// Source is what a copy is made from: the database to snapshot and the folder
// it lives in.
//
// An interface rather than the store type, so this package does not depend on
// the whole of the brain to copy a file.
type Source interface {
	// Snapshot writes a consistent copy of the database to path, while the
	// brain carries on using it.
	Snapshot(ctx context.Context, path string) error

	// CountFacts is how much it knows, recorded in the copy so somebody
	// choosing between copies can see which is fuller as well as which is
	// newer.
	CountFacts() (int, error)
}

/*
 * Write makes or refreshes one copy.
 *
 * The order is the whole of the safety here. A copy is written under a working
 * name, checked, and only then moved into place — so a copy that fails halfway
 * leaves the previous good one untouched, and there is no moment where the
 * only thing on the drive is half a database. Losing the new copy is a
 * nuisance; losing the one that was already there because a drive was pulled
 * out during the write is the thing this feature exists to prevent.
 */
func Write(ctx context.Context, src Source, root, dir string) (Copy, error) {
	dir = filepath.Clean(dir)

	if err := checkDestination(root, dir); err != nil {
		return Copy{}, err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Copy{}, fmt.Errorf("making %s: %w", dir, err)
	}

	live := filepath.Join(root, "brain.sqlite")

	if err := enoughRoom(live, dir); err != nil {
		return Copy{}, err
	}

	working := filepath.Join(dir, "brain.sqlite.copying")

	// Anything left from a copy that was interrupted. Left behind on purpose
	// at the time — it is evidence — but it is not evidence any more once
	// another copy is being made.
	os.Remove(working)

	if err := src.Snapshot(ctx, working); err != nil {
		os.Remove(working)

		return Copy{}, fmt.Errorf("copying the memory: %w", err)
	}

	/*
	 * The settings go too.
	 *
	 * A copy holding only the database is a brain that has forgotten its own
	 * name, its owner and which model it uses — recoverable, but the first
	 * thing somebody restoring from a backup sees would be the setup wizard,
	 * which is not what "I have a copy" is supposed to feel like.
	 */
	if err := copyFile(filepath.Join(root, "brain.conf"), filepath.Join(dir, "brain.conf"), 0o600); err != nil {
		os.Remove(working)

		return Copy{}, fmt.Errorf("copying the settings: %w", err)
	}

	// And where the other copies are kept, so a brain restored from this one
	// carries on making the backups the old one was making. Best effort: a
	// copy without the list is still the brain, and refusing to make one over
	// a missing list would be refusing over the least of it.
	copyFile(filepath.Join(root, ListFile), filepath.Join(dir, ListFile), 0o644)

	if err := os.Rename(working, filepath.Join(dir, "brain.sqlite")); err != nil {
		os.Remove(working)

		return Copy{}, fmt.Errorf("putting the copy in place: %w", err)
	}

	return note(src, root, dir)
}

// note records what this copy holds, in the copy itself.
func note(src Source, root, dir string) (Copy, error) {
	facts, _ := src.CountFacts()

	c := Copy{
		Path:      dir,
		At:        time.Now().UTC(),
		Facts:     facts,
		BrainID:   identityOf(root),
		Reachable: true,
	}

	if info, err := os.Stat(filepath.Join(dir, "brain.sqlite")); err == nil {
		c.Bytes = info.Size()
	}

	raw, err := json.MarshalIndent(struct {
		Copy

		// Said in the file itself, because the person who finds this folder in
		// two years will not have this program in front of them.
		What string `json:"what"`
	}{
		Copy: c,
		What: "A copy of a PN Brain. Not the brain itself — the program will not " +
			"start from this folder unless you tell it to.",
	}, "", "  ")
	if err != nil {
		return c, err
	}

	return c, os.WriteFile(filepath.Join(dir, Marker), raw, 0o644)
}

// identityOf reads the brain's own id, so a copy says which brain it came
// from rather than only when it was made.
func identityOf(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, ".brain-root.json"))
	if err != nil {
		return ""
	}

	var marker struct {
		ID string `json:"id"`
	}

	json.Unmarshal(raw, &marker)

	return marker.ID
}

/*
 * WriteAll refreshes every copy that can be reached.
 *
 * One unreachable copy does not stop the others. The ordinary state of this
 * feature is a list of drives of which one or two are plugged in, and treating
 * an absent drive as a failure would mean the copies that could have been made
 * were not.
 */
func WriteAll(ctx context.Context, src Source, root string) []Copy {
	places, err := Places(root)
	if err != nil {
		return nil
	}

	out := make([]Copy, 0, len(places))

	for _, place := range places {
		// Asked the same way the interface asks it, so a drive that is listed
		// as unplugged is exactly the one that gets skipped here.
		if had := describe(place); !had.Reachable {
			out = append(out, had)

			continue
		}

		copied, err := Write(ctx, src, root, place)
		if err != nil {
			/*
			 * What was already there survives a failed refresh, so report the
			 * copy that exists with the reason it is not newer — rather than
			 * an error where a perfectly good backup from yesterday is.
			 */
			had := describe(place)
			had.Trouble = err.Error()

			out = append(out, had)

			continue
		}

		out = append(out, copied)
	}

	return out
}

// enoughRoom refuses a copy that would fill the destination.
func enoughRoom(database, dir string) error {
	info, err := os.Stat(database)
	if err != nil {
		return fmt.Errorf("looking at the memory: %w", err)
	}

	need := int64(float64(info.Size()) * Room)

	free, err := storage.FreeOn(dir)
	if err != nil {
		// Not being able to ask is not the same as the answer being no.
		return nil
	}

	if int64(free) < need {
		return fmt.Errorf("there is not enough room there: the copy needs about %dMB and %dMB is free",
			need>>20, free>>20)
	}

	return nil
}

func copyFile(from, to string, mode os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		// Settings that do not exist yet are not a reason to fail a copy of
		// everything the brain knows.
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}
	defer in.Close()

	working := to + ".copying"

	out, err := os.OpenFile(working, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(working)

		return err
	}

	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(working)

		return err
	}

	if err := out.Close(); err != nil {
		os.Remove(working)

		return err
	}

	return os.Rename(working, to)
}

/*
 * Use makes a copy into the brain.
 *
 * The one thing a copy cannot do by itself, and the reason for that is the
 * point of the package: nothing here promotes anything automatically, no
 * matter how much newer or fuller it looks. A person decides, having been
 * shown what is in it.
 *
 * It writes the marker that makes a folder a brain and hands back the path;
 * pointing the program at it and restarting belongs to the caller, because
 * this package must not decide that a running brain stops being the brain.
 */
func Use(dir string) error {
	dir = filepath.Clean(dir)

	if _, err := os.Stat(filepath.Join(dir, "brain.sqlite")); err != nil {
		return fmt.Errorf("there is no memory in %s to use", dir)
	}

	raw, err := os.ReadFile(filepath.Join(dir, Marker))
	if err != nil {
		return fmt.Errorf("%s does not look like a copy of a brain", dir)
	}

	var noted struct {
		BrainID string    `json:"brain_id"`
		At      time.Time `json:"at"`
	}

	json.Unmarshal(raw, &noted)

	if noted.BrainID == "" {
		noted.BrainID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	marker, err := json.MarshalIndent(map[string]any{
		"id":      noted.BrainID,
		"schema":  1,
		"created": noted.At.Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, ".brain-root.json"), marker, 0o644); err != nil {
		return err
	}

	/*
	 * And it stops being a copy, because it is not one any more.
	 *
	 * Leaving the copy marker behind would leave a folder claiming to be both,
	 * and the next thing to read it would have to guess. It is also how a
	 * brain would end up being copied over itself: it is still on somebody's
	 * list of places to keep a copy.
	 */
	os.Remove(filepath.Join(dir, Marker))

	/*
	 * And it stops being on its own list of places to copy to.
	 *
	 * The list came with the copy, and it names this folder, because this
	 * folder was one of the places the old brain copied to. Left alone, the
	 * brain would spend the rest of its life trying to back itself up onto
	 * itself — refused every time, and reported as a copy in trouble.
	 */
	if places, err := Places(dir); err == nil {
		kept := places[:0]

		for _, p := range places {
			if p != dir {
				kept = append(kept, p)
			}
		}

		savePlaces(dir, kept)
	}

	return nil
}

// Short is a path with the home folder written the way people say it.
func Short(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(path, home) {
		return path
	}

	return "~" + strings.TrimPrefix(path, home)
}
