package brain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"pn-scripts-assistant/internal/brain/tools"
	"sort"
	"strings"
)

/*
 * Telling somebody what it can now do, once.
 *
 * Everything this program can be asked for is reachable by saying a sentence,
 * and none of it is discoverable by saying a sentence. Its owner was finding
 * out what to say by asking the person who wrote it, which is not a program
 * that introduces itself — and asking a program what it can do only works if
 * you already suspect it can do anything.
 *
 * So it says. Once, on first meeting, and once more whenever it gains
 * something it could not do the last time it was opened. Never again after
 * that: the greeting recited every launch is the exact fault this has to avoid
 * repeating, and an introduction that keeps introducing is worse than none,
 * because it teaches somebody to stop reading the first line.
 */

// IntroducedFile is where it keeps what it has already mentioned.
const IntroducedFile = "introduced.json"

// NewAbilities is what is loaded now and has never been mentioned.
//
// Sorted, so that a run which gained three things says them in the same order
// every time and the record does not churn on map ordering.
func NewAbilities(root string, loaded []string) []string {
	known := introduced(root)

	var fresh []string

	for _, name := range loaded {
		if !known[name] {
			fresh = append(fresh, name)
		}
	}

	sort.Strings(fresh)

	return fresh
}

// MarkIntroduced records that these have been mentioned, so they are not
// mentioned again.
func MarkIntroduced(root string, loaded []string) error {
	all := introduced(root)

	for _, name := range loaded {
		all[name] = true
	}

	var names []string

	for name := range all {
		names = append(names, name)
	}

	sort.Strings(names)

	raw, err := json.MarshalIndent(names, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(root, IntroducedFile), raw, 0o600)
}

// EverIntroduced reports whether it has ever said what it can do.
//
// A brain that has never introduced itself is meeting somebody; one that has
// gained a tool is telling somebody about a change. Those are different
// sentences, and saying the wrong one is how a program sounds like it has
// forgotten who it is talking to.
func EverIntroduced(root string) bool { return len(introduced(root)) > 0 }

func introduced(root string) map[string]bool {
	out := map[string]bool{}

	raw, err := os.ReadFile(filepath.Join(root, IntroducedFile))
	if err != nil {
		return out
	}

	var names []string

	if err := json.Unmarshal(raw, &names); err != nil {
		return out
	}

	for _, name := range names {
		out[name] = true
	}

	return out
}

/*
 * introductionLine is the one line the greeting carries.
 *
 * One line, and only when there is something to say. The greeting is already
 * the thing most at risk of becoming a recital, and a paragraph about
 * capabilities on top of it would be a program talking about itself to
 * somebody who came to talk about their work.
 */
func introductionLine(root string, loaded []string) string {
	if !EverIntroduced(root) {
		return "We have not met properly: ask me what I can do, and I will tell you " +
			"what to say for each of it."
	}

	fresh := NewAbilities(root, loaded)

	if len(fresh) == 0 {
		return ""
	}

	return "I can do something I could not last time — ask me what I can do, or " +
		"just say " + firstExample(fresh) + "."
}

/*
 * firstExample turns a tool name into words somebody would actually say.
 *
 * Named here rather than derived from the tool, because a tool's name is
 * written for a model and "say places_it_learns_from" is not an instruction
 * anybody can follow.
 */
func firstExample(fresh []string) string {
	examples := map[string]string{
		"change_a_setting":          `"answer only me"`,
		"what_can_you_do":           `"what can you do"`,
		"places_it_learns_from":     `"learn everything"`,
		"what_am_i_hearing":         `"why did you ignore me"`,
		"how_fast_can_you_answer":   `"why are you so slow"`,
		"stop_hearing_this_machine": `"stop hearing this machine"`,
		"put_it_back":               `"put that file back"`,
	}

	for _, name := range fresh {
		if say, known := examples[name]; known {
			return say
		}
	}

	return `"what can you do"`
}

// SpokenIntroduction trims the line for saying out loud, where a list is worse
// than a sentence.
func SpokenIntroduction(line string) string {
	if at := strings.Index(line, " — "); at > 0 {
		return line[:at] + "."
	}

	return line
}

// loadedTools is the name of every tool this brain actually has.
//
// Read from the registry rather than written down, so that what it says it can
// do and what it can do are the same list by construction.
func (b *Brain) loadedTools() []string {
	if b.Agent == nil || b.Agent.Registry == nil {
		return nil
	}

	var out []string

	for _, tool := range b.Agent.Registry.All() {
		out = append(out, tool.Name())
	}

	return out
}

// describesTool is what one tool says about itself, for an answer that
// describes the program rather than listing its function names.
func (b *Brain) describesTool(name string) string {
	if b.Agent == nil || b.Agent.Registry == nil {
		return ""
	}

	if tool, ok := b.Agent.Registry.Get(name); ok {
		return tool.Description()
	}

	return ""
}

/*
 * machineInWords is what is on this machine, for the introduction.
 *
 * The same block the prompt carries, from the same reading — so that asking
 * "what can you do" and being told at the start of a conversation cannot
 * disagree about whether Godot is installed.
 */
func (b *Brain) machineInWords() string { return strings.TrimSpace(b.whatIsHere()) }

/*
 * WhatItCanDo is the introduction in full, for the conversation.
 *
 * The same words the what_can_you_do tool produces, from the same registry, so
 * that being told and asking give the same answer. Two descriptions of one
 * program is how a manual starts disagreeing with the program.
 */
func (b *Brain) WhatItCanDo() string {
	said, err := tools.Introduce{
		Loaded:    b.loadedTools,
		Describes: b.describesTool,
		Here:      b.machineInWords,
		Owner:     b.Cfg.Owner,
	}.Facts()
	if err != nil {
		return ""
	}

	return said
}

/*
 * WhatItCanDoInShort is the same thing in a few lines, for writing an
 * introduction from.
 *
 * The full list is four thousand characters, and a model on this machine
 * reads that at about eight tokens a second before it writes anything. The
 * introduction needs the shape; somebody who actually asks gets the list.
 */
func (b *Brain) WhatItCanDoInShort() string {
	said, err := tools.Introduce{
		Loaded: b.loadedTools,
		Here:   b.machineInWords,
		Owner:  b.Cfg.Owner,
	}.Shape()
	if err != nil {
		return ""
	}

	return said
}

/*
 * Delivered records that a greeting actually reached somebody.
 *
 * Separate from building it, because those are not the same event and
 * conflating them cost the introduction entirely: Greet marked every tool as
 * mentioned, then the greeting was consumed by something that never showed it,
 * and the one moment the program had to say what it could do went by with
 * nobody told. It had no way to notice, because as far as it was concerned the
 * job was done.
 *
 * Called by whatever put it on somebody's screen.
 */
func (b *Brain) Delivered(g Greeting) {
	if g.Shown == "" {
		return
	}

	MarkIntroduced(b.Root, b.loadedTools())
}
