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
	"bytes"
	"encoding/json"
	"sort"
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

	/*
	 * Permit is another program putting its own permission question to
	 * the assistant's person: "may I run this command, may I write this
	 * file". The assistant passes the question on and the answer back; it
	 * never answers one itself. The last on the page: with it on, a yes
	 * given here lets that other program act on the computer.
	 */
	Permit Subject = "permissions"
)

// Subjects is every switch, in the order the page shows them.
func Subjects() []Subject { return []Subject{Programs, Ask, Memory, Permit} }

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
	{PermissionTool, Permit, "Another program asked whether it may do something", "Друга програма попита дали може да направи нещо"},
}

// PermissionTool is the tool another program puts its permission
// questions through (Claude Code: --permission-prompt-tool).
const PermissionTool = "permission_prompt"

// said is how one switch is spoken of: what other programs "may (not)
// ask ...", in English and in Bulgarian.
var said = map[Subject][2]string{
	Programs: {"ask which programs are installed", "да питат кои програми са инсталирани"},
	Ask:      {"ask me questions", "да ми задават въпроси"},
	Memory:   {"search what I remember", "да търсят в това, което помня"},
	Permit:   {"put their permission questions to you", "да ви задават своите въпроси за разрешение"},
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

/*
 * MostPermissionsAMinute is how many permission questions of other
 * programs (PermissionTool) are taken in one minute, all programs together.
 *
 * Counted by themselves. They shared the number above, and a program that
 * asked "is it running" in a loop used it up: every permission question of
 * Claude Code was then answered no without its person being asked (the
 * review of 1.2.0). Their own number keeps the two apart both ways: a flood
 * of other questions costs a permission question nothing, and a flood of
 * permission questions leaves the others their minute.
 *
 * Small, because each one is a card a person reads: one every five seconds
 * is more than anybody answers, and well over the few that wait at once
 * (MostPermissionsWaiting). It is still one number for all programs, for
 * the reason above: a program that floods permission questions makes the
 * next program's question a no for that minute, with the flood on the
 * record. Only its person can tell the two programs apart.
 */
const MostPermissionsAMinute = 12

// TooManyPermissions is what the record says of the permission questions
// refused for being too many, in English and in Bulgarian. The page reads
// the number out of the English (programs.js), as it does from TooMany.
func TooManyPermissions() (en, bg string) {
	n := strconv.Itoa(MostPermissionsAMinute)

	return "Not answered: other programs asked for your permission with more than " + n + " questions in a minute. " +
			"The ones over that are refused and not listed one by one.",
		"Без отговор: другите програми поискаха вашето разрешение с повече от " + n + " въпроса за една минута. " +
			"Въпросите над тази граница получават отказ и не се изброяват един по един."
}

/*
 * Pace counts the questions of the minute. Its zero value is ready, and
 * counts to MostAMinute; Most is another number to count to (anything
 * under one is MostAMinute, never "no limit").
 */
type Pace struct {
	// Most is how many are within the minute. Set before the first Next.
	Most int

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

	most := p.Most
	if most < 1 {
		most = MostAMinute
	}

	if p.asked < most {
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

/*
 * A permission question of another program.
 *
 * Claude Code, run with --permission-prompt-tool, hands its tool the name
 * of what it wants to use and what it would give it. The assistant puts
 * that to its person in one line, and the line is all the person reads
 * before saying yes or no, so it is built here, from what was asked alone:
 * the thing that matters first (the command, the file, the address), said
 * the same way every time, and when something was too long to show the
 * line says so and how long it was. Nothing in it is the other program's
 * own description of what it is doing: a model wrote that.
 */

// How much of a permission question is read and shown.
const (
	// MostPermissionTool is the longest name of what is asked about.
	MostPermissionTool = 120

	// MostPermissionShown is how many characters of a command, a path or
	// an address the question shows; what is longer is said to be cut.
	MostPermissionShown = 1500

	// MostPermissionsWaiting is how many permission questions wait for an
	// answer at once. One over is refused at once: a program that asks in
	// a loop must not bury the desk in cards.
	MostPermissionsWaiting = 3
)

// PermissionAsks is what the question is about: the name another program
// gave of the thing it wants to use, safe to show, or empty when nothing
// readable is left of it.
func PermissionAsks(tool string) string { return shown(tool, MostPermissionTool) }

// what one known tool of Claude Code wants, and the field that says it.
type permissionOf struct {
	field  string
	en, bg string
}

var permissionsOf = map[string]permissionOf{
	"Bash":         {"command", "Wants to run this command: ", "Иска да изпълни тази команда: "},
	"PowerShell":   {"command", "Wants to run this command: ", "Иска да изпълни тази команда: "},
	"Write":        {"file_path", "Wants to write this file: ", "Иска да запише този файл: "},
	"Edit":         {"file_path", "Wants to change this file: ", "Иска да промени този файл: "},
	"MultiEdit":    {"file_path", "Wants to change this file: ", "Иска да промени този файл: "},
	"NotebookEdit": {"notebook_path", "Wants to change this file: ", "Иска да промени този файл: "},
	"Read":         {"file_path", "Wants to read this file: ", "Иска да прочете този файл: "},
	"WebFetch":     {"url", "Wants to open this address: ", "Иска да отвори този адрес: "},
	"WebSearch":    {"query", "Wants to search the web for: ", "Иска да потърси в интернет: "},
}

/*
 * PermissionLine is one permission question as its person reads it, in
 * English and in Bulgarian. tool is what PermissionAsks gave; input is
 * what the program would give that tool, as it sent it.
 *
 * The line is what a yes is given to, so it hides nothing it knows of: a
 * line break inside a command is shown as a mark (a second line of a
 * command is a second command), characters that cannot be shown are
 * counted and said, what was too long is said to be cut, and whatever
 * else the program would give the tool besides the one thing shown is
 * named after it (a switch such as "run without the sandbox" with its
 * value).
 *
 * cut is how long each text of the input really is, in characters, where
 * what was sent here is shorter (the MCP server sends a long text cut):
 * the line then says the real length, not the length of what arrived.
 */
func PermissionLine(tool string, input json.RawMessage, cut map[string]int) (en, bg string) {
	var fields map[string]json.RawMessage

	// Not an object: said as it came, below.
	_ = json.Unmarshal(input, &fields)

	if of, known := permissionsOf[tool]; known {
		var value string

		if json.Unmarshal(fields[of.field], &value) == nil {
			if value, note, noteBG := cutShown(value, cut[of.field]); value != "" {
				also, alsoBG := alsoGiven(fields, of.field, cut)

				return of.en + value + note + also, of.bg + value + noteBG + alsoBG
			}
		}
	}

	en, bg = "Wants to use "+tool, "Иска да използва "+tool

	// A tool of an MCP server is named mcp__<server>__<tool>.
	if rest, is := strings.CutPrefix(tool, "mcp__"); is {
		if server, name, both := strings.Cut(rest, "__"); both && server != "" && name != "" {
			en = "Wants to use the tool " + name + " of " + server
			bg = "Иска да използва инструмента " + name + " на " + server
		}
	}

	var compact bytes.Buffer

	if len(bytes.TrimSpace(input)) == 0 || json.Compact(&compact, input) != nil {
		return en + ".", bg + "."
	}

	given, note, noteBG := cutShown(compact.String(), 0)
	if given == "" || given == "{}" || given == "null" {
		return en + ".", bg + "."
	}

	// What was sent here cut is not seen in the middle of what is shown:
	// each such text is named with its real length.
	sentCut, sentCutBG := cutNamed(cut)

	return en + ", with: " + given + note + sentCut, bg + ", със следното: " + given + noteBG + sentCutBG
}

// cutNamed says which texts of an input arrived cut, each with its real
// length, for a tool whose whole input is shown.
func cutNamed(cut map[string]int) (en, bg string) {
	names := make([]string, 0, len(cut))

	for name := range cut {
		if shown(name, 40) != "" {
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		return "", ""
	}

	sort.Strings(names)

	var partsEN, partsBG []string

	for i, name := range names {
		if i == mostAlso {
			more := strconv.Itoa(len(names) - mostAlso)

			partsEN = append(partsEN, "and "+more+" more")
			partsBG = append(partsBG, "и още "+more)

			break
		}

		n := strconv.Itoa(cut[name])

		partsEN = append(partsEN, shown(name, 40)+" (length "+n+")")
		partsBG = append(partsBG, shown(name, 40)+" (дължина "+n+")")
	}

	return " (texts shown cut, read them in the program that asks: " + strings.Join(partsEN, ", ") + ")",
		" (текстове, показани съкратено, прочетете ги в програмата, която пита: " + strings.Join(partsBG, ", ") + ")"
}

// LineBreak stands where a command, a path or an address had a line
// break: on the card everything is one line, and a break must be seen.
const LineBreak = "↵"

/*
 * cutShown is a value on one line, safe to show, at most
 * MostPermissionShown characters, and the words that say what the line
 * does not show as it came: that it was cut (really is its true length
 * when what arrived was cut already), and that characters which cannot be
 * shown were left out.
 */
func cutShown(value string, really int) (out, note, noteBG string) {
	// A break is kept as its mark, with a space either side: every
	// character that starts a new line or page, not the two usual ones
	// alone.
	const mark = " " + LineBreak + " "

	marked := strings.NewReplacer("\r\n", mark, "\n", mark, "\r", mark, "\v", mark, "\f", mark,
		"\u0085", mark, "\u2028", mark, "\u2029", mark).Replace(value)

	whole := shown(marked, len(marked))

	// What shown leaves out and does not replace: anything but spaces,
	// which it only gathers.
	left := 0

	for _, r := range marked {
		if !unicode.IsSpace(r) {
			left++
		}
	}

	for _, r := range whole {
		if !unicode.IsSpace(r) {
			left--
		}
	}

	out = shown(marked, MostPermissionShown)

	// As long as it really is: what arrived, or more when it was sent cut.
	n := len([]rune(whole))
	if really > n {
		n = really
	}

	if n > len([]rune(out)) {
		count := strconv.Itoa(n)

		note = " (cut here: " + count + " characters in all, read the rest in the program that asks)"
		noteBG = " (съкратено тук: общо " + count + " знака, останалото прочетете в програмата, която пита)"
	}

	if left > 0 {
		count := strconv.Itoa(left)

		note += " (characters that cannot be shown were left out: " + count + ")"
		noteBG += " (пропуснати са знаци, които не могат да се покажат: " + count + ")"
	}

	return out, note, noteBG
}

// mostAlso is how many other fields of an input are named on the line.
const mostAlso = 8

/*
 * alsoGiven names what else a known tool would be given besides the one
 * thing the line shows: each by its name, a switch with its value, a text
 * with its length in characters. Never the text itself: a model wrote it (a
 * "description" of the command is not the command).
 */
func alsoGiven(fields map[string]json.RawMessage, shownField string, cut map[string]int) (en, bg string) {
	names := make([]string, 0, len(fields))

	for name := range fields {
		if name != shownField && shown(name, 40) != "" {
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		return "", ""
	}

	// Switches that are on come first, so that many other names cannot
	// push one out of the few that are listed; then by name.
	on := func(name string) bool {
		var flag bool

		return json.Unmarshal(fields[name], &flag) == nil && flag
	}

	sort.Slice(names, func(i, j int) bool {
		if a, b := on(names[i]), on(names[j]); a != b {
			return a
		}

		return names[i] < names[j]
	})

	var partsEN, partsBG []string

	for i, name := range names {
		if i == mostAlso {
			more := strconv.Itoa(len(names) - mostAlso)

			partsEN = append(partsEN, "and "+more+" more")
			partsBG = append(partsBG, "и още "+more)

			break
		}

		var (
			flag bool
			text string
		)

		label := shown(name, 40)

		switch raw := fields[name]; {
		case json.Unmarshal(raw, &flag) == nil:
			partsEN = append(partsEN, label+": "+strconv.FormatBool(flag))
			partsBG = append(partsBG, label+": "+map[bool]string{true: "да", false: "не"}[flag])
		case json.Unmarshal(raw, &text) == nil:
			length := len([]rune(text))
			if cut[name] > length {
				length = cut[name]
			}

			n := strconv.Itoa(length)

			partsEN = append(partsEN, label+" (length "+n+")")
			partsBG = append(partsBG, label+" (дължина "+n+")")
		default:
			partsEN = append(partsEN, label)
			partsBG = append(partsBG, label)
		}
	}

	return " (also given, not shown here: " + strings.Join(partsEN, ", ") + ")",
		" (подадено е и следното, което не се показва тук: " + strings.Join(partsBG, ", ") + ")"
}
