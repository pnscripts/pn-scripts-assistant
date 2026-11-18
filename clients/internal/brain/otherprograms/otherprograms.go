/*
 * Package otherprograms is what other programs may ask the assistant, and what
 * its person decides about that.
 *
 * Another program (Claude Code, Claude Desktop, Cursor, anything that
 * speaks the Model Context Protocol) starts "pn-scripts-assistant mcp" and
 * asks through it. Each question arrives at the running assistant through
 * one door, with the name the asking program gave. This package holds the
 * rules of that door and nothing else: which questions exist, which switch
 * each is behind, and how the name is made safe to show.
 *
 * Two things hold for every question:
 *
 *   - Only whether the assistant is running is answered with no switch.
 *     Everything else is off until its person turns it on at the desk.
 *   - The name is the program's own word for itself. Nothing checks it, so
 *     it is shown and recorded, and never decides anything.
 */
package otherprograms

import (
	"strings"
	"unicode"
)

// Subject is one thing its person may let other programs ask about.
type Subject string

// Programs is which programs are installed on this computer.
const Programs Subject = "programs"

// Subjects is every switch, in the order the page shows them.
func Subjects() []Subject { return []Subject{Programs} }

// Known reports whether s is a switch.
func Known(s Subject) bool {
	for _, each := range Subjects() {
		if s == each {
			return true
		}
	}

	return false
}

// question is one tool of the MCP server: the switch it is behind (none
// for the status) and how the record says it.
type question struct {
	tool    string
	subject Subject
	en, bg  string
}

var questions = []question{
	{"assistant_status", "", "Another program asked whether I am running", "Друга програма попита дали работя"},
	{"list_programs", Programs, "Another program asked which programs are installed", "Друга програма попита кои програми са инсталирани"},
}

// Tools is every question other programs may ask, by the name of its tool.
func Tools() []string {
	out := make([]string, 0, len(questions))
	for _, q := range questions {
		out = append(out, q.tool)
	}

	return out
}

// SubjectOf is the switch a question is behind (empty: answered always),
// and whether there is such a question at all.
func SubjectOf(tool string) (Subject, bool) {
	for _, q := range questions {
		if q.tool == tool {
			return q.subject, true
		}
	}

	return "", false
}

// Asked is how the record says a question, in English and in Bulgarian.
func Asked(tool string) (en, bg string) {
	for _, q := range questions {
		if q.tool == tool {
			return q.en, q.bg
		}
	}

	return "", ""
}

// NotAllowed is what the record says of a question refused because its
// switch is off, in English and in Bulgarian.
func NotAllowed(s Subject) (en, bg string) {
	if s == Programs {
		return "Not answered: other programs may not ask which programs are installed. " +
				"You can allow it on the Programs page, under \"What other programs may ask\".",
			"Без отговор: другите програми нямат право да питат кои програми са инсталирани. " +
				"Можете да го разрешите на страницата „Програми“, в „Какво могат да питат другите програми“."
	}

	return "", ""
}

// Unnamed is recorded for a program that gave no name that can be shown.
const Unnamed = "unnamed"

// How much of a name and of a version is kept.
const (
	mostName    = 60
	mostVersion = 30
)

/*
 * Caller is the asking program as it is shown and recorded: the name it
 * gave and its version, on one line, with nothing that could draw outside
 * its place (control characters, direction marks, invisible letters), and
 * cut to a length a card can hold.
 */
func Caller(name, version string) string {
	name = shown(name, mostName)
	if name == "" {
		return Unnamed
	}

	if version = shown(version, mostVersion); version != "" {
		return name + " " + version
	}

	return name
}

func shown(s string, most int) string {
	var b strings.Builder

	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r), unicode.In(r, unicode.Cf, unicode.Co, unicode.Cs, unicode.Zl, unicode.Zp), r == unicode.ReplacementChar:
			// Left out: it would not be seen, or would move what is.
		default:
			b.WriteRune(r)
		}
	}

	out := []rune(strings.Join(strings.Fields(b.String()), " "))
	if len(out) > most {
		out = out[:most]
	}

	return strings.TrimSpace(string(out))
}
