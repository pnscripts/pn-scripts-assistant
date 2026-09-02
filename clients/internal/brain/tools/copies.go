package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pn-brain/internal/brain/copies"
)

/*
 * Being able to answer "is my brain backed up?" out loud.
 *
 * The panel shows this, and the panel is not where the question gets asked.
 * Somebody about to unplug a drive asks the assistant, in whatever language
 * they happen to be speaking, and a brain that has to say "open the storage
 * card and look" for a fact it holds is not answering.
 *
 * It can also make the copies now, which is the other half of the same moment
 * — "back yourself up, I am taking the drive out" — and that is a safe thing
 * to do on request: it writes to places already chosen, takes nothing away
 * from anywhere, and cannot overwrite another brain.
 *
 * What it cannot do is add a new place. Choosing where a complete copy of
 * everything personal gets written is a decision about where private data
 * lives, and a misheard word is a plausible path. That one stays a thing
 * somebody types.
 */
type Copies struct {
	// Root is the brain's own folder, which is what the list belongs to.
	Root string

	// Source is the database, for making a copy on the spot. Held as the
	// interface the copying needs rather than the whole brain.
	Source copies.Source
}

func (Copies) Name() string { return "brain_copies" }

func (Copies) Description() string {
	return "Report where copies of this brain's own memory are kept, how old each one " +
		"is and how much is in it, and — when asked — refresh them now. Use this for " +
		"any question about backups of the brain itself: whether it is backed up, when " +
		"it last was, whether it is safe to unplug a drive, or a request to back up now."
}

func (Copies) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"copy_now":{
				"type":"boolean",
				"description":"Refresh every reachable copy before answering. Only when asked to back up, not for a question about the state of the copies."
			}
		},
		"additionalProperties":false
	}`)
}

func (Copies) Risk() Risk { return Safe }

func (Copies) Summarize(raw json.RawMessage) string {
	var args struct {
		CopyNow bool `json:"copy_now"`
	}

	json.Unmarshal(raw, &args)

	if args.CopyNow {
		return "Copy the brain to the places it is kept"
	}

	return "Check where the brain is copied"
}

func (t Copies) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		CopyNow bool `json:"copy_now"`
	}

	json.Unmarshal(raw, &args)

	var made []copies.Copy

	if args.CopyNow && t.Source != nil {
		made = copies.WriteAll(ctx, t.Source, t.Root)
	} else {
		var err error

		if made, err = copies.Status(t.Root); err != nil {
			return "", fmt.Errorf("could not read where the copies are kept: %w", err)
		}
	}

	if len(made) == 0 {
		return "There are no copies of this brain. Everything it knows is in one place: " +
			t.Root + ". A copy can be added in the storage panel, on any other drive or folder.", nil
	}

	var b strings.Builder

	if args.CopyNow {
		b.WriteString("Copied just now where possible.\n")
	}

	for _, c := range made {
		fmt.Fprintf(&b, "%s — ", copies.Short(c.Path))

		switch {
		case c.Never() && c.Trouble != "":
			fmt.Fprintf(&b, "nothing copied yet (%s)", c.Trouble)

		case c.Never():
			b.WriteString("nothing copied yet")

		default:
			fmt.Fprintf(&b, "%d things, copied %s", c.Facts, howLongAgo(c.At))

			if c.Trouble != "" {
				fmt.Fprintf(&b, " (not refreshed now: %s)", c.Trouble)
			}
		}

		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// howLongAgo is a gap in the words somebody would use, since a backup being
// "two hours old" is the useful fact and the exact minute is not.
func howLongAgo(at time.Time) string {
	since := time.Since(at)

	switch {
	case since < 2*time.Minute:
		return "just now"

	case since < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(since.Minutes()))

	case since < 48*time.Hour:
		return fmt.Sprintf("%d hours ago", int(since.Hours()))
	}

	return fmt.Sprintf("%d days ago", int(since.Hours()/24))
}
