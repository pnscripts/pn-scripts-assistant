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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Subject is one thing its person may let other programs ask about.
type Subject string

const (
	// Programs is which programs are installed on this computer.
	Programs Subject = "programs"

	// Ask is a question put to the assistant itself, answered in words by
	// the model on this computer. It does nothing for the program that
	// asked, and knows nothing of its person unless Memory is on too.
	Ask Subject = "ask"

	// Memory is searching what the assistant remembers. The only switch
	// that hands out what is personal, so it is the last on the page.
	Memory Subject = "memory"
)

// Subjects is every switch, in the order the page shows them.
func Subjects() []Subject { return []Subject{Programs, Ask, Memory} }

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
	{"ask_assistant", Ask, "Another program asked me a question", "Друга програма ми зададе въпрос"},
	{"search_memory", Memory, "Another program searched what I remember", "Друга програма потърси в това, което помня"},
}

// said is how one switch is spoken of: what other programs "may (not)
// ask ...", in English and in Bulgarian.
var said = map[Subject][2]string{
	Programs: {"ask which programs are installed", "да питат кои програми са инсталирани"},
	Ask:      {"ask me questions", "да ми задават въпроси"},
	Memory:   {"search what I remember", "да търсят в това, което помня"},
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
	how, known := said[s]
	if !known {
		return "", ""
	}

	return "Not answered: other programs may not " + how[0] + ". " +
			"You can allow it on the Programs page, under \"What other programs may ask\".",
		"Без отговор: другите програми нямат право " + how[1] + ". " +
			"Можете да го разрешите на страницата „Програми“, в „Какво могат да питат другите програми“."
}

// Switched is how the record says a switch was turned, in English and in
// Bulgarian.
func Switched(s Subject, allowed bool) (en, bg string) {
	how, known := said[s]
	if !known {
		return "", ""
	}

	if allowed {
		return "Other programs may now " + how[0], "Другите програми вече могат " + how[1]
	}

	return "Other programs may no longer " + how[0], "Другите програми вече не могат " + how[1]
}

/*
 * How much other programs may ask, whatever is allowed.
 *
 * A program that asks in a loop would fill the record of what was done and
 * keep the computer busy, so the door answers a fixed number of questions
 * a minute, from all programs together: the name a program gives is its
 * own word, and a limit for each name would be no limit at all. The
 * questions over it are refused, and the first of them in a minute is
 * written on the record, so its person sees that it happened without the
 * record filling with it.
 */

// MostAMinute is how many questions other programs are answered in one
// minute, all of them together.
const MostAMinute = 30

// TooMany is what the record says of the questions refused for being too
// many, in English and in Bulgarian.
func TooMany() (en, bg string) {
	n := strconv.Itoa(MostAMinute)

	return "Not answered: other programs asked more than " + n + " questions in a minute. " +
			"The ones over that are refused and not listed one by one.",
		"Без отговор: другите програми зададоха повече от " + n + " въпроса за една минута. " +
			"Въпросите над тази граница получават отказ и не се изброяват един по един."
}

// Pace counts the questions of the minute. Its zero value is ready.
type Pace struct {
	mu     sync.Mutex
	since  time.Time
	asked  int
	listed bool
}

/*
 * Next counts one question asked at now. It reports whether it is within
 * the minute's number, and for one that is not, whether it is the first
 * refused in this minute (the one the record keeps).
 */
func (p *Pace) Next(now time.Time) (within, first bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// A clock set back starts a new minute too: it never locks the door.
	if p.since.IsZero() || now.Sub(p.since) >= time.Minute || now.Before(p.since) {
		p.since, p.asked, p.listed = now, 0, false
	}

	if p.asked < MostAMinute {
		p.asked++

		return true, false
	}

	first = !p.listed
	p.listed = true

	return false, first
}

// How long a question to the assistant and a search of its memory may be,
// in characters.
const (
	MostQuestion = 2000
	MostSearch   = 200
)

/*
 * Words is text another program sent to be read by the assistant (a
 * question, a search), made safe to keep and to show: nothing invisible or
 * direction-changing, no stacked marks, lines kept as lines for a question
 * and at most the given number of characters. Empty when nothing readable
 * is left.
 */
func Words(s string, most int) string {
	var lines []string

	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", "\n"), "\n") {
		if line = shown(line, most); line != "" {
			lines = append(lines, line)
		}
	}

	out := []rune(strings.Join(lines, "\n"))
	if len(out) > most {
		out = out[:most]
	}

	return strings.TrimSpace(string(out))
}

// Unnamed is recorded for a program that gave no name that can be shown.
const Unnamed = "unnamed"

// How much of a name and of a version is kept, and how many combining
// marks one letter may carry.
const (
	mostName    = 60
	mostVersion = 30
	mostMarks   = 2
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

	// marks is how many combining marks follow the letter before: a letter
	// may carry two (as real writing does), and none stands on nothing.
	marks, letter := 0, false

	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')

			marks, letter = 0, false
		case unicode.IsControl(r), unicode.In(r, unicode.Cf, unicode.Co, unicode.Cs, unicode.Zl, unicode.Zp), r == unicode.ReplacementChar:
			// Left out: it would not be seen, or would move what is.
		case unicode.In(r, unicode.Mn, unicode.Me):
			// Stacked marks draw over the lines above and below.
			if letter && marks < mostMarks {
				b.WriteRune(r)

				marks++
			}
		default:
			b.WriteRune(r)

			marks, letter = 0, !unicode.IsSpace(r)
		}
	}

	out := []rune(strings.Join(strings.Fields(b.String()), " "))
	if len(out) > most {
		out = out[:most]
	}

	return strings.TrimSpace(string(out))
}
