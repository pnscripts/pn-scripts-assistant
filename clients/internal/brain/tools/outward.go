package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/rules"
)

/*
 * Outward is a tool that says itself which kinds of outward or lasting
 * action a call is (see package rules): sending, posting, committing,
 * changing files, paying, deleting.
 *
 * Most tools do not need to: ActionsOf reads the common ones from what the
 * tool is and what it was handed. A tool whose effect cannot be read from its
 * name or its arguments says so here.
 */
type Outward interface {
	Outward(args json.RawMessage) []string
}

/*
 * ActionsOf is which kinds of outward or lasting action a call is, for the
 * user's rules to be checked against. Empty for a call that only reads.
 *
 * Read from the call, never from what the model says it is doing: a rule
 * about deleting holds for "rm" whatever the summary calls it.
 */
func ActionsOf(t Tool, args json.RawMessage) []string {
	if o, ok := t.(Outward); ok {
		return o.Outward(args)
	}

	if t.Risk() != Mutating {
		return nil
	}

	found := map[string]bool{}

	if s, ok := t.(Spending); ok && s.Spends(args) != "" {
		found[rules.Purchase] = true
	}

	switch t.Name() {
	case "send_email", "send_draft":
		found[rules.Send] = true
	case "remove_a_part":
		found[rules.Delete] = true
	case "run_command", "run_as_admin":
		for _, a := range commandActions(args) {
			found[a] = true
		}
	case "drive_browser", "click", "type_text":
		// What a browser or a click does is read from the words it was
		// given: "submit", "post", "send", "pay", "delete".
		for _, a := range wordActions(string(args)) {
			found[a] = true
		}
	}

	if writes, _, said := TouchesOf(t, args); said && len(nonEmpty(writes)) > 0 {
		found[rules.Files] = true
	}

	// A tool nobody here wrote — an MCP server's, a skill's — is read from
	// its name: "send_message", "create_post", "delete_file", "git_commit".
	if len(found) == 0 {
		for _, a := range nameActions(t.Name()) {
			found[a] = true
		}
	}

	var out []string

	for _, a := range rules.Actions() {
		if found[a] {
			out = append(out, a)
		}
	}

	return out
}

func nonEmpty(list []string) []string {
	var out []string

	for _, s := range list {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}

	return out
}

// commandActions reads a command — and every command a shell is handed —
// for committing, pushing and deleting.
func commandActions(raw json.RawMessage) []string {
	var a commandArgs

	_ = json.Unmarshal(raw, &a)

	commands := [][]string{a.Argv}
	if inner, _, ok := Inline(a.Argv); ok {
		commands = inner
	}

	found := map[string]bool{}

	for _, argv := range commands {
		if len(argv) == 0 {
			continue
		}

		program := strings.ToLower(filepath.Base(argv[0]))

		if program == "git" && len(argv) > 1 {
			for _, word := range argv[1:] {
				switch word {
				case "commit", "push", "merge", "rebase", "cherry-pick", "tag":
					found[rules.Commit] = true
				}
			}
		}

		if Destructive(argv) != "" {
			found[rules.Delete] = true
		}
	}

	var out []string

	for _, x := range rules.Actions() {
		if found[x] {
			out = append(out, x)
		}
	}

	return out
}

var wordsForActions = map[string][]string{
	rules.Send:     {"send", "reply", "message", "изпрати", "отговори"},
	rules.Post:     {"post", "publish", "submit", "comment", "tweet", "share", "публикувай", "коментирай", "сподели"},
	rules.Purchase: {"pay", "buy", "checkout", "purchase", "order", "subscribe", "плати", "купи", "поръчай"},
	rules.Delete:   {"delete", "remove", "trash", "изтрий", "премахни"},
}

// wordsInNames adds what a tool's name can say that a browser's words
// cannot be trusted to: "git_commit", "write_file".
var wordsInNames = map[string][]string{
	rules.Commit: {"commit", "push", "merge"},
	rules.Files:  {"write", "edit", "save", "overwrite", "move", "rename"},
}

func wordActions(text string) []string { return actionsOfWords(text, wordsForActions) }

func nameActions(name string) []string {
	all := map[string][]string{}

	for a, list := range wordsForActions {
		all[a] = append(all[a], list...)
	}

	for a, list := range wordsInNames {
		all[a] = append(all[a], list...)
	}

	return actionsOfWords(name, all)
}

func actionsOfWords(text string, vocabulary map[string][]string) []string {
	words := map[string]bool{}

	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r)
	}) {
		words[w] = true
	}

	var out []string

	for _, a := range rules.Actions() {
		for _, w := range vocabulary[a] {
			if words[w] {
				out = append(out, a)

				break
			}
		}
	}

	return out
}
