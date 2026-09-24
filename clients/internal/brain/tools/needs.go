package tools

import "sort"

/*
 * What a tool cannot work without.
 *
 * A tool is a thing this program can do; some of those things depend on
 * something else being on the machine. godot_build needs Godot. make_subtitles
 * needs whisper and ffmpeg. type_text needs xdotool. Until now that knowledge
 * lived in each tool's own error message, discovered by calling it and
 * failing — which is the most expensive way to learn anything, and on a slow
 * machine costs a minute of a model's time to find out something that could
 * have been read off a list.
 *
 * Written here as data so three different readers can use it: the code that
 * decides which tools to offer this turn, the answer to "what can you do",
 * and the sentence that says what would make a tool work. The ids are the
 * ones the environment registry uses — see internal/brain/environs.
 *
 * Deliberately not a hard gate. A tool whose requirement is missing is still
 * registered and still callable: the machine can change between the reading
 * and the call, a person may have installed the thing a moment ago, and a
 * tool that refuses on the strength of a stale list is worse than one that
 * tries and says what happened.
 */

/*
 * Needing is implemented by a tool that depends on something being installed.
 *
 * Optional, like Cued and Serves. A tool that says nothing needs nothing
 * beyond this program itself, which is true of most of them: reading a file,
 * remembering something, running a command.
 */
type Needing interface {
	// Needs is the ids of what has to be on the machine, all of them.
	Needs() []string
}

/*
 * needs is what each built-in tool cannot work without.
 *
 * A table rather than a method on each of seventy-four, for the same reason
 * the capability table is one: the question is "which tools need Godot", and
 * a question about all of them is answered badly by seventy-four separate
 * replies. Anything added at run time answers for itself through Needing.
 */
var needs = map[string][]string{
	// The engine, for everything about a Godot project.
	"godot_status": {"godot"},
	"godot_build":  {"godot"},

	// Driving the desktop: xdotool does the typing and the clicking.
	"type_text": {"xdotool"},
	"click":     {"xdotool"},
	"scroll":    {"xdotool"},

	// A page opened properly rather than merely fetched, which is a browser
	// running the page.
	"read_a_page": {"chrome"},

	// Sound and pictures.
	"make_subtitles":          {"ffmpeg", "whisper"},
	"films_without_subtitles": {"ffmpeg"},
}

/*
 * Needing is what one tool cannot work without.
 *
 * A tool added at run time is asked first, then the table — the same order as
 * Serving, and for the same reason: a skill replacing a built-in name is
 * already how a skill overrides one, and two rules disagreeing would make
 * which file you had to read depend on which tool it was.
 */
func (r *Registry) Needing(name string) []string {
	if tool, ok := r.Get(name); ok {
		if asking, ok := tool.(Needing); ok {
			if own := asking.Needs(); len(own) > 0 {
				return own
			}
		}
	}

	return needs[name]
}

// Needed is the table alone, for anything that wants to read the mapping
// without holding a registry.
func Needed(name string) []string { return needs[name] }

/*
 * WhatIsNeeded is everything the registered tools depend on, once each.
 *
 * For the reader that asks the machine about all of it in one go rather than
 * once per tool: sixty of these questions are sixty processes started, and
 * the answer is the same list either way.
 */
func (r *Registry) WhatIsNeeded() []string {
	found := map[string]bool{}

	for _, tool := range r.All() {
		for _, id := range r.Needing(tool.Name()) {
			found[id] = true
		}
	}

	out := make([]string, 0, len(found))

	for id := range found {
		out = append(out, id)
	}

	sort.Strings(out)

	return out
}
