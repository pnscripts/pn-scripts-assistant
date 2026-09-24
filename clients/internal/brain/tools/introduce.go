package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

/*
 * Saying what it can do — from what it has, not from a list somebody typed.
 *
 * This used to be eighteen hand-written lines: a capability, a sentence about
 * it, and an example of what to say. They were checked against the registry,
 * so nothing was promised that was not loaded, and they were still wrong in
 * the way that matters — they described eighteen of seventy-four tools, said
 * nothing whatever about the machine, and had to be edited by hand every time
 * the program learned something. An assistant whose account of itself is a
 * string constant is an assistant that will eventually be lying.
 *
 * So this reports facts and lets the model do the talking: every tool that is
 * loaded, grouped by what it is for, plus what is on the machine and what is
 * waiting to be set up. The model turns that into an answer for whoever
 * asked, in their words and about their question — which is the whole point,
 * because "what can you do" from somebody who has just installed Godot and
 * "what can you do" from somebody with an empty machine are different
 * questions with the same words.
 */
type Introduce struct {
	// Loaded is which tools exist, so nothing is claimed that is not there.
	Loaded func() []string

	// Describes is what one tool is for, in its own words. Nil falls back to
	// the tool's name, which is worse and still true.
	Describes func(name string) string

	/*
	 * Here is what is on this machine — engines, languages, editors, coding
	 * agents — and what is waiting for something. Written by environs.Words
	 * and passed in rather than imported, so that this package keeps knowing
	 * nothing about how the machine is read.
	 */
	Here func() string

	// Owner is who is being spoken to.
	Owner string
}

func (Introduce) Name() string { return "what_can_you_do" }

func (Introduce) Description() string {
	return "What this assistant can do on this machine, right now: every tool it has, " +
		"and what is installed for it to work with. Use this for \"what can you do\", " +
		"\"can you use Godot\", \"what can I ask for\", \"how does this work\", or any " +
		"question about its own capabilities. Answer from what this returns rather " +
		"than from general knowledge about assistants."
}

func (Introduce) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

// Safe: it describes itself and changes nothing.
func (Introduce) Risk() Risk { return Safe }

func (Introduce) Summarize(json.RawMessage) string { return "Say what it can do" }

/*
 * groups are the headings the tools are sorted under.
 *
 * By what somebody wants, not by what the code calls things: "remembering"
 * rather than "the learning subsystem". The mapping is from the capability
 * table that already exists — see capabilities.go — so a tool added with its
 * capabilities named lands in the right group without anybody touching this
 * file.
 */
var groups = []struct {
	title string
	of    []string
}{
	{"Remembering and learning", []string{"knowledge_base", "note_taking"}},
	{"Files and folders", []string{"file_management"}},
	{"Writing documents", []string{"writing", "documentation", "editing", "technical_writing", "copywriting", "reporting"}},
	{"Code and projects", []string{"programming", "debugging", "code_review", "refactoring", "testing", "deployment", "automation", "shell", "linux", "version_control", "build_systems", "release_management"}},
	{"Making games", []string{"game_development", "godot", "gdscript"}},
	{"The web", []string{"web_research", "seo", "market_research", "competitive_analysis", "journalism"}},
	{"Pictures, video and sound", []string{"illustration", "animation", "video_editing", "visual_design", "three_d_modelling", "accessibility", "translation", "storytelling", "branding"}},
	{"The desktop", []string{"desktop_control"}},
	{"Mail and the house", []string{"correspondence", "communication", "customer_support", "account_management", "smart_home", "public_relations", "selling"}},
	{"The diary and reminders", []string{"scheduling", "organisation", "event_planning"}},
	{"This machine, and itself", []string{"machine_upkeep", "troubleshooting"}},
	{"Reading and research", []string{"research", "literature_review", "legal_research", "accounting"}},
}

/*
 * Execute is the answer for a model, which is told what to do with it.
 *
 * The facts are the same either way; only this reader needs the instruction.
 * Facts() is the same material for a reader who is a person, and the two are
 * one function apart so they cannot drift.
 */
func (t Introduce) Execute(context.Context, json.RawMessage) (string, error) {
	facts, err := t.Facts()
	if err != nil {
		return "", err
	}

	return "What I can do here, from what is actually loaded. " +
		"Say it in your own words to whoever asked, about what they asked — " +
		"do not read this list out.\n\n" + facts, nil
}

/*
 * Facts is what this assistant can do and what is on the machine, with no
 * instructions to anybody.
 *
 * For the introduction, which is phrased by the model from these and shown to
 * a person — and shown as it is when there is no model to phrase it, which is
 * why it must read as facts rather than as a note addressed to one.
 */
func (t Introduce) Facts() (string, error) {
	if t.Loaded == nil {
		return "", fmt.Errorf("nothing here can say what is loaded")
	}

	loaded := t.Loaded()

	if len(loaded) == 0 {
		return "None of my tools are loaded, which means I can talk and nothing else.", nil
	}

	var b strings.Builder

	b.WriteString("TOOLS I HAVE (" + fmt.Sprint(len(loaded)) + "):\n")

	for _, line := range grouped(loaded, t.Describes) {
		b.WriteString(line + "\n")
	}

	if t.Here != nil {
		if here := strings.TrimSpace(t.Here()); here != "" {
			b.WriteString("\n" + here + "\n")
		}
	}

	/*
	 * And the two things worth knowing before asking for any of it.
	 *
	 * Both are the kind of thing somebody finds out by being surprised, and
	 * being surprised by an assistant is how people stop trusting one.
	 */
	b.WriteString("\nHOW IT BEHAVES: anything that changes something — a file, a " +
		"setting, a whole drive — stops and asks first, and says exactly what it is " +
		"about to do. Privacy is the one thing it will not change from a " +
		"conversation: that is set in the privacy panel, so that nothing said to it " +
		"can talk it into sharing more.")

	return b.String(), nil
}

/*
 * Shape is what this assistant can do, in a few lines rather than seventy.
 *
 * For the introduction, which is written by the model and therefore paid for
 * by the model: the full list is four thousand characters, which on a machine
 * with no graphics card is two minutes of reading before a word is written —
 * and none of it is needed. To introduce itself the assistant needs to know
 * the shape of what it has and what is on the machine, not the exact wording
 * of every tool's description. The full list is what the what_can_you_do tool
 * returns, when somebody has actually asked.
 */
func (t Introduce) Shape() (string, error) {
	if t.Loaded == nil {
		return "", fmt.Errorf("nothing here can say what is loaded")
	}

	loaded := t.Loaded()

	if len(loaded) == 0 {
		return "None of my tools are loaded, which means I can talk and nothing else.", nil
	}

	counted := map[string]int{}
	var order []string

	for _, line := range grouped(loaded, nil) {
		title, rest, found := strings.Cut(strings.TrimPrefix(line, "· "), ": ")
		if !found {
			continue
		}

		if _, seen := counted[title]; !seen {
			order = append(order, title)
		}

		counted[title] += len(strings.Split(rest, "; "))
	}

	var b strings.Builder

	fmt.Fprintf(&b, "I have %d tools:\n", len(loaded))

	for _, title := range order {
		fmt.Fprintf(&b, "· %s (%d)\n", title, counted[title])
	}

	if t.Here != nil {
		if here := strings.TrimSpace(t.Here()); here != "" {
			b.WriteString("\n" + here + "\n")
		}
	}

	b.WriteString("\nAnything that changes something stops and asks first.")

	return b.String(), nil
}

/*
 * grouped puts the loaded tools under headings a person would recognise.
 *
 * A tool belonging to no group is still listed, under the last heading,
 * because a tool left out of the answer is a tool nobody will ever ask for.
 */
func grouped(loaded []string, describes func(string) string) []string {
	left := map[string]bool{}

	for _, name := range loaded {
		left[name] = true
	}

	var out []string

	for _, group := range groups {
		var mine []string

		for _, name := range loaded {
			if !left[name] {
				continue
			}

			for _, capability := range ServedBy(name) {
				if contains(group.of, capability) {
					mine = append(mine, describe(name, describes))
					delete(left, name)

					break
				}
			}
		}

		if len(mine) == 0 {
			continue
		}

		sort.Strings(mine)

		out = append(out, "· "+group.title+": "+strings.Join(mine, "; "))
	}

	var rest []string

	for _, name := range loaded {
		if left[name] {
			rest = append(rest, describe(name, describes))
		}
	}

	if len(rest) > 0 {
		sort.Strings(rest)

		out = append(out, "· Everything else: "+strings.Join(rest, "; "))
	}

	return out
}

// describe is a tool in a few words: its name, and the first clause of what
// it says about itself.
func describe(name string, describes func(string) string) string {
	if describes == nil {
		return name
	}

	said := strings.TrimSpace(describes(name))
	if said == "" {
		return name
	}

	// The first sentence, and not all of one: this is a list of seventy-odd
	// and every extra clause is paid for on the turn that reads it.
	if cut := strings.IndexAny(said, ".;\n"); cut > 0 {
		said = said[:cut]
	}

	// Cut at a word, not mid-word, and never leave the remains of a clause
	// hanging: "change wording inside a document that already exists — " is
	// not a description, it is the start of one.
	if len(said) > 60 {
		said = said[:60]

		if space := strings.LastIndex(said, " "); space > 20 {
			said = said[:space]
		}
	}

	said = strings.TrimRight(strings.TrimSpace(said), " ,—-:(")

	if said == "" {
		return name
	}

	return name + " (" + strings.ToLower(said[:1]) + said[1:] + ")"
}

func contains(list []string, want string) bool {
	for _, each := range list {
		if each == want {
			return true
		}
	}

	return false
}
