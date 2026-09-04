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

	/*
	 * Candidates are the drives and folders the brain has itself found —
	 * the home directory, whatever is mounted.
	 *
	 * A closed list, and that is the whole reason adding one is allowed here
	 * at all. The rule was that choosing what gets read is a thing somebody
	 * types, because a misheard word is a plausible path. A misheard word is
	 * not a plausible entry in a list the brain assembled by looking at the
	 * machine, so offering those costs nothing and closes the gap that made
	 * "learn everything" unanswerable: it could see two drives and still had
	 * to ask somebody to type one.
	 */
	Candidates func() []Candidate
}

// Candidate is somewhere the brain could start learning from.
type Candidate struct {
	Path string
	Name string
	Home bool
}

func (Places) Name() string { return "places_it_learns_from" }

func (Places) Description() string {
	return "Report the drives and folders this brain looks after: which are attached, " +
		"how much it has learned from each and how much it still has to read; read " +
		"some more from one of them now; or start looking after a drive it has found " +
		"but is not yet reading. Use this for any question about what it is learning " +
		"from, and for any instruction to learn everything, learn this machine, or " +
		"go through a drive or folder."
}

func (Places) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"read_now":{
				"type":"string",
				"description":"The name or path of a place to read some of now. Leave it out to only report."
			},
			"start_learning":{
				"type":"string",
				"description":"Somewhere the brain has found but is not yet reading, to start looking after. A mount point or folder from the list it reports, or \"everything\" for all of them. Use this when told to learn everything or to learn this machine."
			}
		},
		"additionalProperties": false
	}`)
}

/*
 * Mutating, because one of the three things it does changes something.
 *
 * Reading and reporting are safe. Taking on a whole drive is not: everything
 * in it becomes something the brain knows, which is a decision about somebody
 * else's data made on the strength of a sentence heard across a room. It is
 * offered, named in full, and waits.
 */
func (Places) Risk() Risk { return Mutating }

func (Places) Summarize(raw json.RawMessage) string {
	var args placesArgs

	json.Unmarshal(raw, &args)

	if start := strings.TrimSpace(args.StartLearning); start != "" {
		if strings.EqualFold(start, "everything") {
			return "Start learning from every drive and folder it has found"
		}

		return "Start learning from " + start
	}

	if strings.TrimSpace(args.ReadNow) != "" {
		return "Read more of " + args.ReadNow
	}

	return "Check the drives and folders it looks after"
}

type placesArgs struct {
	ReadNow       string `json:"read_now"`
	StartLearning string `json:"start_learning"`
}

func (t Places) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var args placesArgs

	json.Unmarshal(raw, &args)

	if start := strings.TrimSpace(args.StartLearning); start != "" {
		line, err := t.startLearning(start)
		if err != nil {
			return "", err
		}

		return line, nil
	}

	list, err := places.Status(t.Root)
	if err != nil {
		return "", fmt.Errorf("could not read the list of places: %w", err)
	}

	if len(list) == 0 {
		/*
		 * Not a dead end any more.
		 *
		 * This used to say "one can be added in the storage panel", which is
		 * an assistant telling somebody to go and do it themselves — and it
		 * was said in answer to "learn everything", with two drives sitting in
		 * front of it that it had found by itself.
		 */
		return t.nothingWatchedYet(), nil
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

/*
 * nothingWatchedYet names what it could start on, rather than sending somebody
 * to a panel.
 */
func (t Places) nothingWatchedYet() string {
	found := t.candidates()

	if len(found) == 0 {
		return "There are no drives or folders on the list yet, so nothing is being " +
			"read on its own, and I cannot see anywhere obvious to start. One can be " +
			"added in the storage panel — any folder, on any drive."
	}

	var b strings.Builder

	b.WriteString("Nothing is on the list yet, so nothing is being read on its own. " +
		"What I can see to start from:\n")

	for _, c := range found {
		fmt.Fprintf(&b, "  · %s (%s)\n", c.Name, c.Path)
	}

	b.WriteString("\nSay which, or say everything, and I will start — it reads a " +
		"little at a time in the background rather than all at once.")

	return b.String()
}

/*
 * startLearning takes on one of the places the brain found, or all of them.
 *
 * Only from that list. A path somebody typed goes through the panel, where it
 * can be read before it is agreed to; a path the recogniser invented out of a
 * sentence must never become a folder the brain reads, and the closed list is
 * what makes the difference.
 */
func (t Places) startLearning(want string) (string, error) {
	found := t.candidates()

	if len(found) == 0 {
		return "I cannot see any drive or folder to start from.", nil
	}

	var wanted []Candidate

	if strings.EqualFold(strings.TrimSpace(want), "everything") ||
		strings.EqualFold(strings.TrimSpace(want), "all") {
		wanted = found
	} else {
		for _, c := range found {
			if strings.EqualFold(c.Path, want) || strings.EqualFold(c.Name, want) ||
				strings.Contains(strings.ToLower(c.Path), strings.ToLower(want)) {
				wanted = append(wanted, c)
			}
		}
	}

	if len(wanted) == 0 {
		var names []string

		for _, c := range found {
			names = append(names, c.Path)
		}

		return fmt.Sprintf("I have not found anywhere called %q. What I can see is: %s.",
			want, strings.Join(names, ", ")), nil
	}

	watching, _ := places.List(t.Root)

	onList := map[string]bool{}

	for _, p := range watching {
		onList[p.Path] = true
	}

	var (
		added   []string
		already []string
		refused []string
	)

	for _, c := range wanted {
		if onList[c.Path] {
			already = append(already, c.Name)

			continue
		}

		if _, err := places.Watch(t.Root, c.Path, c.Name, places.Both); err != nil {
			/*
			 * Said rather than swallowed.
			 *
			 * A place can be refused for good reasons — it is inside the
			 * brain's own folder, it is not there any more — and reporting
			 * that as "already on the list" would be a brain claiming to be
			 * doing something it is not, which is the exact failure this whole
			 * tool exists to stop.
			 */
			refused = append(refused, fmt.Sprintf("%s (%v)", c.Name, err))

			continue
		}

		added = append(added, fmt.Sprintf("%s (%s)", c.Name, c.Path))
	}

	if len(added) == 0 {
		var b strings.Builder

		if len(already) > 0 {
			fmt.Fprintf(&b, "%s was already on the list — it is being read a little at "+
				"a time whenever it is attached.", strings.Join(already, " and "))
		}

		if len(refused) > 0 {
			if b.Len() > 0 {
				b.WriteString(" ")
			}

			fmt.Fprintf(&b, "I could not take on %s.", strings.Join(refused, "; "))
		}

		return b.String(), nil
	}

	said := fmt.Sprintf("Started on %s. It reads a little at a time in the background, "+
		"projects and documents both, and I will say what it finds as it goes rather "+
		"than at the end.", strings.Join(added, " and "))

	if len(already) > 0 {
		said += fmt.Sprintf(" %s was already on the list.", strings.Join(already, " and "))
	}

	if len(refused) > 0 {
		said += fmt.Sprintf(" I could not take on %s.", strings.Join(refused, "; "))
	}

	return said, nil
}

func (t Places) candidates() []Candidate {
	if t.Candidates == nil {
		return nil
	}

	return t.Candidates()
}
