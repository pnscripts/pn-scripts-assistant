package setup

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/desktop"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * Closing setup half way leaves the machine as it was found.
 *
 * Setup is the one screen that writes gigabytes to somebody's computer before
 * they have decided to keep the program. Abandoning it used to leave all of
 * it: an Ollama in ~/.local, a service pointed at it, four gigabytes of model,
 * a compiled recogniser and an entry in the applications menu — none of which
 * the program would ever mention again, because the next run offered to
 * install them rather than to remove them.
 *
 * So what this run installed, this run takes back. Only what it installed:
 * a piece that was already on the machine when setup opened is somebody
 * else's, and removing it because it appeared in a list here would be the
 * worst possible reading of "undo".
 */

// added is one thing this run put on the machine, and how to take it off.
type added struct {
	kind string // "part", "model" or "menu"
	name string
}

/*
 * note records something as newly installed, if it really was new.
 *
 * Checked before the install rather than after, because "was it here before"
 * is unanswerable once it is here. An update to something already present
 * records nothing: undoing an update by deleting the thing would leave
 * somebody worse off than not having run setup at all.
 */
func (s *Server) note(a added) {
	s.mu.Lock()
	s.added = append(s.added, a)
	s.mu.Unlock()
}

/*
 * RollBack undoes this run, newest first.
 *
 * Newest first because the order things were installed in is a dependency
 * order — the models need Ollama — and taking Ollama away first would leave
 * the model removal with no ollama to run.
 *
 * Every failure is reported and none of them stop the rest: a rollback that
 * gives up half way is the state it exists to prevent.
 */
func (s *Server) RollBack(w io.Writer) {
	/*
	 * Nothing is removed while something is still being installed.
	 *
	 * Closing the window during an install leaves applyAll running in its own
	 * goroutine, and rolling back over the top of it would race an installer
	 * unpacking an archive against a remover deleting the directory it is
	 * unpacking into. Waiting is bounded: a step that has wedged must not hold
	 * the machine in a half-installed state forever, so after a minute the
	 * rollback proceeds and reports what it can.
	 */
	for i := 0; i < 600; i++ {
		s.mu.Lock()
		busy := s.busy
		s.mu.Unlock()

		if !busy {
			break
		}

		if i == 0 {
			fmt.Fprint(w, "\n  Waiting for the step in progress to stop.\n")
		}

		time.Sleep(100 * time.Millisecond)
	}

	s.mu.Lock()
	items := make([]added, len(s.added))
	copy(items, s.added)
	s.added = nil
	s.mu.Unlock()

	if len(items) == 0 {
		return
	}

	fmt.Fprintf(w, "\n  Setup was closed before it finished, so the %s it installed "+
		"%s being removed.\n", count(len(items), "thing", "things"),
		map[bool]string{true: "is", false: "are"}[len(items) == 1])

	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]

		switch item.kind {
		case "menu":
			if err := desktop.Remove(); err != nil {
				fmt.Fprintf(w, "  could not take it out of the menu: %v\n", err)

				continue
			}

			fmt.Fprint(w, "  taken out of the applications menu\n")

		case "model":
			// ollama's own command, because the model store is its business
			// and the files inside it are content-addressed and shared.
			out, err := exec.Command("ollama", "rm", item.name).CombinedOutput()
			if err != nil {
				fmt.Fprintf(w, "  could not remove %s: %v %s\n",
					item.name, err, strings.TrimSpace(string(out)))

				continue
			}

			fmt.Fprintf(w, "  removed the model %s\n", item.name)

		case "part":
			removed := false

			for _, r := range preflight.Requirements() {
				if r.Name != item.name || !r.Removable() {
					continue
				}

				if _, err := preflight.RemoveAndVerify(r); err != nil {
					fmt.Fprintf(w, "  could not remove %s: %v\n", item.name, err)
				} else {
					fmt.Fprintf(w, "  removed %s\n", item.name)
				}

				removed = true

				break
			}

			if !removed {
				fmt.Fprintf(w, "  %s was left in place — it is not mine to remove\n",
					item.name)
			}
		}
	}

	fmt.Fprint(w, "\n  The machine is as it was. Start the program again to set it up.\n\n")
}

// count says "one thing" or "three things".
func count(n int, one, many string) string {
	if n == 1 {
		return "one " + one
	}

	return fmt.Sprintf("%d %s", n, many)
}

// modelPresent reports whether ollama already holds a model, so pulling one
// somebody already had is not recorded as this run's to delete.
func modelPresent(name string) bool {
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(out), "\n") {
		field, _, _ := strings.Cut(strings.TrimSpace(line), " ")

		// ollama list prints "name:tag"; a bare name means the latest tag.
		if field == name || field == name+":latest" ||
			strings.TrimSuffix(field, ":latest") == name {
			return true
		}
	}

	return false
}
