package agent

import (
	"regexp"
	"strings"
)

/*
 * Offering a small model a short list of tools rather than every one.
 *
 * Thirty-one tools is a lot to choose between, and how well a model chooses
 * falls off sharply with size. Asked what it thought of a website, a
 * three-billion-parameter model here called list_models — not a near miss, a
 * tool with nothing to do with the question — and earlier the same question
 * produced look_at_screen, seven minutes of reading a desktop that could not
 * have held the answer.
 *
 * Sharpening the descriptions helped and did not fix it, because the problem
 * is not that any one description is unclear. It is that thirty-one plausible
 * options is a harder problem than the model can reliably solve, and every
 * irrelevant one is another chance to go wrong.
 *
 * So the obvious mismatches are dropped before the model ever sees them. Only
 * the obvious ones: a filter that guesses too confidently produces the worse
 * failure, where the right tool was never offered and the brain says it cannot
 * do something it can. When in doubt the tool stays.
 */

// mentionsSomewhere matches a domain or URL named in the message.
var mentionsSomewhere = regexp.MustCompile(
	`\b(?:[a-zA-Z0-9][a-zA-Z0-9-]*\.)+(?:com|net|org|io|dev|local|bg|co|uk|app|sh|me|ai)\b`)

/*
 * housekeeping are tools about the program itself.
 *
 * Nobody asks after these in passing; they are the subject of a request or
 * they are not wanted. Offered to every turn they are pure distraction, and
 * list_models was picked for a question about a website.
 */
var housekeeping = map[string][]string{
	"list_models":    {"model", "models", "llm", "ollama"},
	"set_appearance": {"colour", "color", "theme", "appearance", "look"},
	"set_wake_word":  {"wake", "name", "call you", "called"},
	"set_device":     {"device", "light", "lamp", "switch", "plug"},
	"list_devices":   {"device", "light", "lamp", "switch", "plug"},
	"look_at_screen": {"screen", "window", "showing", "display", "see this", "on my"},
	"list_windows":   {"screen", "window", "open", "showing"},

	/*
	 * Driving the machine — the riskiest tools here, and the ones a small
	 * model reaches for most carelessly.
	 *
	 * Asked "do you listen every time that I call you", it proposed typing
	 * "Press Return" into whatever happened to be focused. Nothing about that
	 * question implies touching the keyboard, and the approval prompt it
	 * produced is worse than useless: it trains its owner to approve things
	 * without reading them.
	 *
	 * These are offered only when the request plainly asks for them, which is
	 * how somebody actually asks for them — "type this", "click that", "open
	 * Firefox". Never as a guess.
	 */
	/*
	 * Changing things on the machine, which nothing conversational needs.
	 *
	 * "Say hello in four words" made the small model propose creating
	 * /home/Petar/greetings.txt, and the turn ended in an approval prompt
	 * instead of an answer — so the question was never answered at all. That
	 * is the worst version of this fault: not a wrong answer but no answer,
	 * reached by way of a confirmation nobody asked for.
	 *
	 * A model with thirty tools and a greeting in front of it will find
	 * something to do with them. The prompt asks it not to; a
	 * three-billion-parameter model does not reliably listen, and the list it
	 * is shown is the part that can be relied on.
	 */
	"write_file":  {"write", "save", "create", "file", "note down", "put that in"},
	"edit_file":   {"edit", "change", "replace", "fix", "update", "modify"},
	"run_command": {"run", "execute", "command", "terminal", "shell", "install"},

	"do_in_background": {"background", "meanwhile", "while you", "later", "keep going"},
	"list_background":  {"background", "running", "jobs", "still going", "progress"},
	"stop_background":  {"stop", "cancel", "abort", "kill"},

	"type_text":      {"type", "typing", "write in", "enter", "fill in", "input"},
	"click":          {"click", "press", "button", "tap", "select"},
	"scroll":         {"scroll", "page down", "page up", "scroll down", "scroll up"},
	"open_app":       {"open", "launch", "start ", "run "},
	"decide_waiting": {"waiting", "approve", "decision"},
	"list_waiting":   {"waiting", "approve", "decision"},

	/*
	 * The organisation's own: hiring, projects, engines, integrations.
	 *
	 * Each named by its own name as well, because a task's step is written
	 * as "check it with game_check" and that has to be enough to offer it.
	 */
	"hire_agent":           {"hire", "expert", "specialist", "someone who", "somebody who", "наеми", "hire_agent"},
	"confirm_hire":         {"hire", "permanent", "one task", "task only", "постоянно", "confirm_hire"},
	"plan_project":         {"make me", "build me", "create a", "make a", "build a", "game", "project", "novel", "book", " api", "website", "plan the", "plan_project"},
	"start_project":        {"start", "go ahead", "project", "start_project"},
	"inspect_project":      {"folder", "project", "directory", "inspect", "inspect_project"},
	"check_requirements":   {"need", "install", "requirement", "installed", "version", "engine", "check_requirements"},
	"install_requirement":  {"install", "инсталирай", "install_requirement"},
	"game_engines":         {"game", "engine", "godot", "unity", "unreal", "three", "game_engines"},
	"game_docs":            {"game", "godot", "three", "unity", "unreal", "documentation", "docs", "game_docs"},
	"game_check":           {"game", "godot", "three", "unity", "unreal", "game_check"},
	"game_build":           {"game", "export", "smoke", "game_build"},
	"list_integrations":    {"integration", "mcp", "list_integrations"},
	"activate_integration": {"integration", "mcp", "activate_integration"},
	"read_integration":     {"integration", "mcp", "read_integration"},
}

// mentions reports whether any of the cues appears in the message.
func mentions(text string, cues []string) bool {
	for _, cue := range cues {
		if cue != "" && strings.Contains(text, cue) {
			return true
		}
	}

	return false
}
