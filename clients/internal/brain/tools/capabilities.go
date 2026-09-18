package tools

import (
	"sort"
	"strings"
)

/*
 * Which tools somebody good at something actually needs.
 *
 * A capability and a tool are not the same thing and the difference is easy
 * to lose. A tool is something this program can do — read a file, send an
 * email. A capability is something a person is good at — debugging, tax,
 * welding. Most capabilities map to no tool at all, and that is the normal
 * case rather than a gap: reading a description of welding to a model does
 * not make it a welder, and there is no tool that would.
 *
 * This is what turns a roster of people into a roster of people who have been
 * handed the right things. An agent that is good at research gets the three
 * tools that reach the web; one that is good at writing does not, and so
 * cannot be talked into fetching a page whatever a page tells it to do.
 *
 * The built-ins are a table here rather than a method on each of the sixty-six
 * of them, and that is the same shape the cue list already has, for the same
 * reason: the answer is "which tools serve this capability", and a question
 * about all of them at once is answered badly by sixty-six separate replies.
 * Anything added at run time says so itself, through Serves, because it is not
 * in this file and cannot be.
 */

/*
 * Serves is implemented by a tool added at run time that wants to say which
 * capabilities it is for.
 *
 * Optional, like Cued: a tool that does not implement it falls back to the
 * table, and one in neither belongs to no profession — which is a real answer
 * and not a missing one.
 */
type Serves interface {
	// Serves is the capability ids this tool helps with. Lower case.
	Serves() []string
}

/*
 * serves is what each built-in tool is for.
 *
 * A tool listed against nothing is deliberate, not unfinished. Three kinds
 * end up there and each for its own reason:
 *
 *   - the harness itself — ask_first, do_in_background, what_can_you_do.
 *     These are how an agent behaves rather than what it is good at, and
 *     every agent has them or none does.
 *   - the owner's own decisions — list_waiting, decide_waiting. Approving
 *     what the gate is holding is not a profession, and an agent working
 *     unattended has no business with it.
 *   - teaching — remember_how_to_do_this and its pair. Changing what the
 *     assistant knows is its owner's act, not a skill somebody is hired for.
 */
var serves = map[string][]string{
	// Reading and writing what is on the disk.
	"read_file":      {"file_management", "programming", "debugging", "code_review", "research"},
	"write_file":     {"programming", "writing", "note_taking", "file_management", "documentation"},
	"edit_file":      {"programming", "refactoring", "debugging"},
	"search_files":   {"file_management", "programming", "code_review", "research", "refactoring"},
	"list_directory": {"file_management", "programming", "research"},
	"list_drives":    {"file_management", "machine_upkeep"},
	"read_document":  {"research", "editing", "writing", "literature_review", "documentation", "legal_research", "accounting"},
	"write_document": {"writing", "copywriting", "technical_writing", "documentation", "reporting", "editing"},
	"edit_document":  {"writing", "editing", "documentation", "technical_writing"},

	/*
	 * Running things, which is most of what building software is.
	 *
	 * The longest list here, and it earns it: version control, tests, builds
	 * and deployment are all one tool on this machine. An agent good at any
	 * of them needs it, and an agent good at none of them should not have it.
	 */
	"run_command": {
		"programming", "debugging", "testing", "deployment", "automation",
		"linux", "shell", "version_control", "build_systems", "release_management",
		"troubleshooting",
	},

	// Reaching the web. The three that leave this machine, so the list of who
	// gets them is worth reading twice.
	"web_search":  {"web_research", "research", "seo", "market_research", "competitive_analysis", "journalism", "literature_review"},
	"fetch_url":   {"web_research", "research", "seo", "market_research", "competitive_analysis", "journalism"},
	"read_a_page": {"web_research", "research", "seo", "market_research", "competitive_analysis", "journalism"},

	// Games.
	"godot_status": {"game_development", "godot"},
	"godot_docs":   {"game_development", "godot", "gdscript"},
	"godot_build":  {"game_development", "godot"},

	// Things to be looked at.
	"make_a_picture":          {"visual_design", "illustration", "branding", "three_d_modelling"},
	"make_a_video":            {"video_editing", "animation", "storytelling"},
	"make_subtitles":          {"video_editing", "translation", "accessibility"},
	"films_without_subtitles": {"video_editing", "translation"},
	"set_appearance":          {"visual_design", "machine_upkeep"},

	// Time, and what is coming.
	"put_in_the_diary": {"scheduling", "event_planning", "organisation"},
	"change_the_diary": {"scheduling", "event_planning"},
	"what_is_on":       {"scheduling", "organisation"},
	"remind_me":        {"scheduling", "organisation"},
	"list_reminders":   {"scheduling", "organisation"},
	"forget_reminder":  {"scheduling", "organisation"},

	// Speaking to people in the owner's name, which is why both of these need
	// approval whoever holds them.
	"read_email": {"correspondence", "customer_support", "communication", "account_management"},
	"send_email": {"correspondence", "communication", "customer_support", "public_relations", "account_management", "selling"},

	// What the brain itself knows.
	"what_you_know":         {"knowledge_base", "research", "note_taking"},
	"learn_from_folder":     {"knowledge_base", "note_taking", "research"},
	"places_it_learns_from": {"knowledge_base", "machine_upkeep"},

	// Driving this computer as a person would.
	"look_at_screen": {"desktop_control"},
	"list_windows":   {"desktop_control"},
	"click":          {"desktop_control"},
	"type_text":      {"desktop_control"},
	"scroll":         {"desktop_control"},
	"open_app":       {"desktop_control"},

	// Looking after the machine and the program on it.
	"what_this_machine_needs":   {"machine_upkeep", "troubleshooting"},
	"install_a_part":            {"machine_upkeep"},
	"remove_a_part":             {"machine_upkeep"},
	"install_a_model":           {"machine_upkeep"},
	"list_models":               {"machine_upkeep"},
	"check_for_updates":         {"machine_upkeep"},
	"brain_copies":              {"machine_upkeep"},
	"put_it_back":               {"machine_upkeep"},
	"change_a_setting":          {"machine_upkeep"},
	"set_wake_word":             {"machine_upkeep"},
	"how_fast_can_you_answer":   {"machine_upkeep"},
	"what_am_i_hearing":         {"machine_upkeep"},
	"stop_hearing_this_machine": {"machine_upkeep"},

	// The house.
	"list_devices": {"smart_home"},
	"set_device":   {"smart_home"},
}

/*
 * ForCapabilities is the tools somebody good at these things needs.
 *
 * A narrowing of what the assistant may already do, never a widening: this
 * only ever names tools that are registered, and what comes back is still put
 * through the privacy filter and the approval gate like anything else. An
 * agent built from capabilities alone can therefore be given to a model
 * without giving the model anything new.
 */
func (r *Registry) ForCapabilities(capabilities []string) []string {
	if len(capabilities) == 0 {
		return nil
	}

	wanted := map[string]bool{}

	for _, c := range capabilities {
		wanted[c] = true
	}

	found := map[string]bool{}

	for _, tool := range r.All() {
		for _, c := range r.Serving(tool.Name()) {
			if wanted[c] {
				found[tool.Name()] = true

				break
			}
		}
	}

	out := make([]string, 0, len(found))

	for name := range found {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

/*
 * Serving is what one tool is for.
 *
 * A tool added at run time is asked; everything else is looked up. That order
 * rather than the other way round, because a skill replacing a built-in name
 * is already how a skill overrides one, and the two rules disagreeing would
 * make which file you had to read depend on which tool it was.
 */
func (r *Registry) Serving(name string) []string {
	if tool, ok := r.Get(name); ok {
		if serving, ok := tool.(Serves); ok {
			if own := serving.Serves(); len(own) > 0 {
				return own
			}
		}
	}

	return serves[name]
}

// ServedBy is the table alone, for anything that wants to read the mapping
// without holding a registry.
func ServedBy(name string) []string { return serves[name] }

/*
 * Allowed is the narrowing rule, in one place.
 *
 * An empty list means everything, which is what the generalist has; anything
 * else is exactly what it names. Written here rather than in the loop or on
 * the agent because both of those ask the same question, and two copies of an
 * authority check is one too many — the copy that gets fixed is never the one
 * being called.
 */
func Allowed(only []string, name string) bool {
	return len(only) == 0 || Listed(only, name)
}

// Listed is the plain membership test, for the lists where empty means empty
// rather than everything.
func Listed(names []string, name string) bool {
	for _, one := range names {
		if one == name {
			return true
		}

		/*
		 * mcp_files_* is every tool the integration called files has, which
		 * cannot be named in advance: they are whatever its server lists.
		 * Only for integrations — a star anywhere else is a tool named with a
		 * star, which there are none of.
		 */
		if strings.HasPrefix(one, "mcp_") && strings.HasSuffix(one, "_*") &&
			strings.HasPrefix(name, strings.TrimSuffix(one, "*")) {
			return true
		}
	}

	return false
}

// EveryCapabilityServed is every capability the built-in tools claim to help
// with, so a test can check that each one is a capability somebody describes.
func EveryCapabilityServed() []string {
	seen := map[string]bool{}

	for _, list := range serves {
		for _, c := range list {
			seen[c] = true
		}
	}

	out := make([]string, 0, len(seen))

	for c := range seen {
		out = append(out, c)
	}

	sort.Strings(out)

	return out
}
