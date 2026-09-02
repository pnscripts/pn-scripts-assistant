package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-brain/internal/brain/places"
)

/*
 * Answering for the drives and folders it looks after.
 *
 * The panel shows this, and the panel is not where the question gets asked.
 * "Have you read the film drive yet?" is asked out loud, in whatever language
 * somebody happens to be speaking, and a brain that has to say "open the
 * storage card and look" for a fact it holds is not answering.
 *
 * It can also take a bite on request — "read my work drive now" — which is
 * safe: it reads and remembers, changes nothing outside the brain's own
 * memory, and is bounded to a few minutes rather than the hours a whole drive
 * would take.
 *
 * What it cannot do is add a place. Everything in a folder becomes something
 * the brain knows, and a misheard word is a plausible path — so choosing what
 * gets read is a thing somebody types.
 */
type Places struct {
	// Root is the brain's own folder, which is where the list lives.
	Root string

	// Owner is whose work this is, for the sentences a scan produces.
	Owner string

	// Learn and Seen are the two halves of taking a bite, asked for when the
	// tool runs rather than when it is built — the tools are registered before
	// the learner exists, and a nil worker in an interface is not nil.
	Learn func() places.Reads
	Seen  places.Seen
}

func (Places) Name() string { return "places_it_learns_from" }

func (Places) Description() string {
	return "Report the drives and folders this brain looks after: which are attached, " +
		"how much it has learned from each and how much it still has to read — and, " +
		"when asked, read some more from one of them now. Use this for any question " +
		"about what it is learning from, whether a drive has been read, or a request " +
		"to go through a folder it already watches."
}

func (Places) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"read_now":{
				"type":"string",
				"description":"The name or path of a place to read some of now. Leave it out to only report."
			}
		},
		"additionalProperties": false
	}`)
}

func (Places) Risk() Risk { return Safe }

func (Places) Summarize(raw json.RawMessage) string {
	var args struct {
		ReadNow string `json:"read_now"`
	}

	json.Unmarshal(raw, &args)

	if strings.TrimSpace(args.ReadNow) != "" {
		return "Read more of " + args.ReadNow
	}

	return "Check the drives and folders it looks after"
}

func (t Places) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		ReadNow string `json:"read_now"`
	}

	json.Unmarshal(raw, &args)

	list, err := places.Status(t.Root)
	if err != nil {
		return "", fmt.Errorf("could not read the list of places: %w", err)
	}

	if len(list) == 0 {
		return "There are no drives or folders on the list yet, so nothing is being read " +
			"on its own. One can be added in the storage panel — any folder, on any drive.", nil
	}

	var b strings.Builder

	if want := strings.TrimSpace(args.ReadNow); want != "" {
		line, err := t.readSome(ctx, list, want)
		if err != nil {
			return "", err
		}

		b.WriteString(line + "\n\n")

		// And the list again, because it has just changed.
		list, _ = places.Status(t.Root)
	}

	for _, p := range list {
		fmt.Fprintf(&b, "%s (%s) — ", p.Name, places.Short(p.Path))

		switch {
		case !p.Reachable:
			fmt.Fprintf(&b, "%s; %d things learned from it so far", p.Trouble, p.Learned)

		case p.Never() && p.Learned == 0:
			fmt.Fprintf(&b, "attached, nothing read from it yet")

		default:
			fmt.Fprintf(&b, "attached, %d things learned", p.Learned)
		}

		if p.Waiting > 0 {
			fmt.Fprintf(&b, ", %d still to read (about %d minutes of work)",
				p.Waiting, p.Waiting*places.SecondsEach/60)
		} else if p.Reachable && !p.Never() {
			b.WriteString(", all of it read")
		}

		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// readSome takes one bite out of the place somebody named, by name or by path.
func (t Places) readSome(ctx context.Context, list []places.Place, want string) (string, error) {
	var found *places.Place

	for i := range list {
		if strings.EqualFold(list[i].Name, want) || list[i].Path == want {
			found = &list[i]

			break
		}
	}

	// And by part of the name, because a spoken name arrives approximately.
	if found == nil {
		for i := range list {
			if strings.Contains(strings.ToLower(list[i].Name), strings.ToLower(want)) {
				found = &list[i]

				break
			}
		}
	}

	if found == nil {
		return "", fmt.Errorf("%q is not one of the places I look after", want)
	}

	if !found.Reachable {
		return fmt.Sprintf("%s is %s, so there is nothing to read from it right now.",
			found.Name, found.Trouble), nil
	}

	if t.Learn == nil || t.Seen == nil {
		return "", fmt.Errorf("the part of me that learns is not running")
	}

	learner := t.Learn()
	if learner == nil {
		return "", fmt.Errorf("the part of me that learns is not running")
	}

	pass, err := places.Look(ctx, learner, t.Seen, t.Owner, *found)

	places.Note(t.Root, pass.Place)

	if err != nil {
		return "", fmt.Errorf("reading %s: %w", found.Name, err)
	}

	if pass.Took == 0 {
		return fmt.Sprintf("Nothing new in %s — everything in it has been read.", found.Name), nil
	}

	line := fmt.Sprintf("Read %d more things from %s: %d newly remembered, %d already known.",
		pass.Took, found.Name, pass.Learned, pass.Known)

	if pass.Place.Waiting > 0 {
		line += fmt.Sprintf(" %d still to read there; I carry on with it in the background.",
			pass.Place.Waiting)
	} else {
		line += " That is all of it."
	}

	return line, nil
}
