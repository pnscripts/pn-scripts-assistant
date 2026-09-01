package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"pn-brain/internal/brain/storage"
)

/*
 * Somewhere to look, when somebody says "my external drive".
 *
 * Asked to find the projects on an external drive, the assistant answered
 * "could you please specify which external drive you're referring to?" — three
 * times, over several minutes, while the drive sat mounted the whole way
 * through. That is not a model being unhelpful: there was no tool that could
 * answer the question, so the only move available to it was to ask.
 *
 * A mount point is not something anybody knows or should have to. "The
 * external one" is how people refer to a drive, and turning that into
 * /media/name/c8fc2986-4b79-4d7b-9a8c-e6db653915ac is exactly the sort of
 * translation a program should do rather than demand.
 */
type ListDrives struct {
	// Root is where the brain keeps itself, so it can say which drive that is.
	Root string
}

func (ListDrives) Name() string { return "list_drives" }

func (ListDrives) Description() string {
	return "List the disks and drives on this machine with their paths, size and free " +
		"space, marking which are removable. Use this whenever somebody refers to a " +
		"drive by description rather than by path — \"my external drive\", \"the USB " +
		"stick\", \"the big disk\" — instead of asking them for the path."
}

func (ListDrives) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (ListDrives) Risk() Risk { return Safe }

func (ListDrives) Summarize(json.RawMessage) string { return "List the drives on this machine" }

func (t ListDrives) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	found, err := storage.Drives(t.Root)
	if err != nil {
		return "", fmt.Errorf("could not read the drives: %w", err)
	}

	if len(found) == 0 {
		return "No drives could be read on this machine.", nil
	}

	const gb = 1 << 30

	/*
	 * Which drive actually holds the brain, by longest match.
	 *
	 * Every absolute path begins with "/", so a plain prefix test marks the
	 * root filesystem as holding the brain no matter where the brain is — and
	 * the answer then names two drives as its home, one of them wrongly.
	 */
	var holdsTheBrain string

	for _, d := range found {
		if t.Root == "" || d.MountPoint == "" {
			continue
		}

		if strings.HasPrefix(t.Root, d.MountPoint) && len(d.MountPoint) > len(holdsTheBrain) {
			holdsTheBrain = d.MountPoint
		}
	}

	var b strings.Builder

	for _, d := range found {
		kind := "built in"
		if d.Removable {
			kind = "removable — plugged in"
		}

		fmt.Fprintf(&b, "%s — %s, %.0fGB free of %.0fGB",
			d.MountPoint, kind,
			float64(d.FreeBytes)/gb, float64(d.TotalBytes)/gb)

		if d.MountPoint == holdsTheBrain {
			b.WriteString(" — the brain's own memory is kept here")
		}

		b.WriteString("\n")
	}

	/*
	 * And the home folder, because it is where somebody's own work usually is
	 * and it is not a drive, so nothing above would have mentioned it.
	 */
	if home, err := os.UserHomeDir(); err == nil {
		fmt.Fprintf(&b, "%s — the home folder\n", home)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}
