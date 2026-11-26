package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/exe"
	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * Tools that use another program say which one, and how (package programs,
 * permits): the loop then weighs what the person decided on that program's
 * card before it asks or acts. A removed program is never used, a "never"
 * is refused, an "always" runs without asking.
 *
 * Saying which program is reading only: a menu entry, a program file on
 * PATH, the process behind a window. Nothing is started to find out.
 */
type ProgramBound interface {
	ProgramUse(ctx context.Context, args json.RawMessage) programs.Use
}

// ProgramUseOf is what a call does with another program, and whether it
// uses one at all.
func ProgramUseOf(ctx context.Context, t Tool, args json.RawMessage) (programs.Use, bool) {
	b, ok := t.(ProgramBound)
	if !ok {
		return programs.Use{}, false
	}

	return b.ProgramUse(ctx, args), true
}

/*
 * ActsInOtherPrograms is a tool that acts in other programs' windows
 * without saying which one (scroll): nothing to weigh against a card, but
 * stopped like the rest by "Stop using other programs".
 */
type ActsInOtherPrograms interface {
	ActsInOtherPrograms()
}

// UsesOtherPrograms is whether a tool opens, types or clicks in, scrolls,
// or runs another program: every one of them is stopped by "Stop using
// other programs". Tools that only read (list_windows, list_programs)
// are not.
func UsesOtherPrograms(t Tool) bool {
	if _, ok := t.(ProgramBound); ok {
		return true
	}

	_, ok := t.(ActsInOtherPrograms)

	return ok
}

// Scrolling moves another program's window.
func (Scroll) ActsInOtherPrograms() {}

// The places a tool finds a program; tests replace them.
var (
	findDesktopEntry = desktopEntryFor
	lookProgram      = exe.Look

	// windowProgram is the program file (and Flatpak id) of the window
	// in front, or of the window at a point when atPoint is true.
	windowProgram = windowProgramOf
)

// Opening an application starts it.
func (OpenApp) ProgramUse(_ context.Context, raw json.RawMessage) programs.Use {
	var a struct {
		Name string `json:"name"`
	}

	_ = json.Unmarshal(raw, &a)

	name := strings.TrimSpace(a.Name)
	u := programs.Use{Verb: programs.VerbStart, Name: name}

	if name == "" {
		return u
	}

	/*
	 * A place ("/tmp/gimp", "./gimp") is a file, said as a command says
	 * one: elsewhere unless it is the very file its name runs here, so a
	 * card that knows the program by its file name lends it nothing.
	 * Execute does not open it either.
	 */
	if givenByPlace(name) {
		c := commandUse(strings.Fields(name)[:1], "")
		c.Verb = programs.VerbStart

		return c
	}

	// The same order Execute tries: the menu entry, then a command.
	if entry, ok := findDesktopEntry(name); ok {
		u.Desktop = entry
		return u
	}

	if p, ok := lookProgram(strings.Fields(strings.ToLower(name))[0]); ok {
		u.Path = p
	}

	return u
}

// A command runs its program with arguments.
func (RunCommand) ProgramUse(_ context.Context, raw json.RawMessage) programs.Use {
	var a commandArgs

	_ = json.Unmarshal(raw, &a)

	return commandUse(a.Argv, a.Dir)
}

// As administrator is a command like any other: its program's card and
// "Stop using other programs" decide it before the system is asked.
func (RunAsAdmin) ProgramUse(_ context.Context, raw json.RawMessage) programs.Use {
	var a adminArgs

	_ = json.Unmarshal(raw, &a)

	u := commandUse(a.Argv, "")

	// As administrator is itself a wrapper: a program's "always" was not
	// given for running it as root. Its "never" still refuses.
	u.Wrapped = !u.Empty()

	return u
}

/*
 * AlsoRunning is a tool whose call can start more programs than the one
 * ProgramUse names: the other commands of a shell line, the programs a
 * one-line script names. Each is said as a use that can only be refused
 * (Wrapped): a "never" on any of their cards, or a removal, refuses the
 * whole call; no card's "always" answers for them.
 */
type AlsoRunning interface {
	AlsoRuns(ctx context.Context, args json.RawMessage) []programs.Use
}

// AlsoRunsOf is the other programs a call would start, nil for a tool
// that says none.
func AlsoRunsOf(ctx context.Context, t Tool, args json.RawMessage) []programs.Use {
	a, ok := t.(AlsoRunning)
	if !ok {
		return nil
	}

	return a.AlsoRuns(ctx, args)
}

func (RunCommand) AlsoRuns(_ context.Context, raw json.RawMessage) []programs.Use {
	var a commandArgs

	_ = json.Unmarshal(raw, &a)

	return alsoRun(a.Argv, a.Dir)
}

func (RunAsAdmin) AlsoRuns(_ context.Context, raw json.RawMessage) []programs.Use {
	var a adminArgs

	_ = json.Unmarshal(raw, &a)

	return alsoRun(a.Argv, "")
}

/*
 * alsoRun is every program a command line names beyond its first: each
 * command of a shell line (`sh -c "a; b && c"`, `script -c`, `eval`, and
 * the lines inside those), seen through what wraps it, and every word of a
 * one-line script (`python3 -c`, `perl -e`, `node -e`) that is a program
 * here. Each one once. A plain command says none.
 */
func alsoRun(argv []string, dir string) []programs.Use {
	var out []programs.Use

	seen := map[programs.Use]bool{}
	tried := map[string]bool{}

	add := func(cmd []string) bool {
		key := strings.Join(cmd, "\x00")
		if tried[key] {
			return false
		}

		tried[key] = true

		u := commandUse(cmd, dir)
		u.Wrapped = true

		if !u.Empty() && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}

		return true
	}

	/*
	 * wrapper is one of the wrappers read since the read after 1.2.0 (su,
	 * find, a terminal, cmd, powershell...; wrappers.go) that the walk
	 * passes through on its way in: named too, so a "never" on its own
	 * card still refuses the call, as when it was read as the program
	 * itself; like the rest, it allows nothing.
	 */
	wrapper := func(argv []string) {
		if !namedWrapper(argv) {
			return
		}

		u := useOf(argv[:1], dir, true)

		if !u.Empty() && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}

	var walk func(argv []string, depth int)

	walk = func(argv []string, depth int) {
		for step := 0; step < 12 && len(argv) > 0 && depth < 8; step++ {
			if line, ok := lineOf(argv); ok {
				wrapper(argv)

				for _, cmd := range commandsOf(line, 0) {
					if add(cmd) {
						walk(cmd, depth+1)
					}
				}

				return
			}

			if code, ok := codeOf(argv); ok {
				for _, word := range programWords(code) {
					add([]string{word})
				}

				return
			}

			rest, was := insideOf(argv)
			if !was {
				return
			}

			wrapper(argv)

			argv = rest
		}
	}

	walk(argv, 0)

	return out
}

// The shells that run a line given after -c.
var shellNames = map[string]bool{"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "fish": true, "ash": true}

/*
 * lineOf is the shell line a command hands over to be run: a shell's
 * `-c "..."` (also among other letters, "-lc"), `script -c "..."`, and
 * whether there is one.
 */
func lineOf(argv []string) (string, bool) {
	if len(argv) == 0 {
		return "", false
	}

	rest := argv[1:]

	if line, ok, known := handedLine(argv); known {
		return line, ok
	}

	switch base := filepath.Base(argv[0]); {
	case shellNames[base]:
		for i := 0; i < len(rest); i++ {
			word := rest[i]

			if !strings.HasPrefix(word, "-") || word == "--" {
				break
			}

			if strings.HasPrefix(word, "--") {
				continue
			}

			// "-c", or letters with it among them ("-lc", "-ce").
			if strings.Contains(word[1:], "c") {
				if i+1 >= len(rest) {
					return "", false
				}

				return rest[i+1], true
			}

			// "-o errexit": the option's name is the next word.
			if strings.HasSuffix(word, "o") {
				i++
			}
		}
	case base == "script":
		for i := 0; i < len(rest); i++ {
			word := rest[i]

			switch {
			case word == "--":
				return "", false
			case strings.HasPrefix(word, "--command="):
				return strings.TrimPrefix(word, "--command="), true
			case word == "--command":
				if i+1 < len(rest) {
					return rest[i+1], true
				}

				return "", false
			case strings.HasPrefix(word, "--"):
				if !strings.Contains(word, "=") && scriptValued[word] {
					i++
				}
			case strings.HasPrefix(word, "-") && len(word) > 1:
				// Letters together ("-qc"): the last one may take the
				// next word, and "c" takes the line.
				last := word[len(word)-1]

				if i := strings.IndexByte(word[1:], 'c'); i >= 0 && i+2 < len(word) {
					// "-cfirefox": the line is the rest of the word.
					return word[i+2:], true
				}

				if last == 'c' {
					if i+1 < len(rest) {
						return rest[i+1], true
					}

					return "", false
				}

				if strings.IndexByte("IOBTmEo", last) >= 0 {
					i++
				}
			}
		}
	}

	return "", false
}

var scriptValued = map[string]bool{"--log-in": true, "--log-out": true, "--log-io": true, "--log-timing": true,
	"--logging-format": true, "--echo": true, "--output-limit": true}

// Words a shell command may open with that are not its program.
var shellKeywords = map[string]bool{"if": true, "then": true, "else": true, "elif": true, "fi": true, "do": true, "done": true,
	"while": true, "until": true, "for": true, "case": true, "esac": true, "in": true, "!": true, "{": true, "}": true,
	"time": true, "exec": true, "command": true, "builtin": true, "function": true}

/*
 * commandsOf is every command of a shell line, as words: split where a
 * shell ends a command (; & | newline, brackets), quotes honoured, what is
 * substituted ($(...), backquotes, inside double quotes too) read as
 * commands of its own, redirections and their files left out, settings
 * and keywords in front dropped, and a line given to eval read again.
 * Quotes that never close are read as if they were not there, so nothing
 * hides behind a broken line.
 */
func commandsOf(line string, depth int) [][]string {
	if depth > 8 {
		return nil
	}

	var (
		out    [][]string
		words  []string
		cur    strings.Builder
		any    bool // a word is open (an empty quoted one too)
		skip   bool // the next word is a redirection's file
		nested []string
	)

	endWord := func() {
		if !any {
			return
		}

		if skip {
			skip = false
		} else {
			words = append(words, cur.String())
		}

		cur.Reset()

		any = false
	}

	endCommand := func() {
		endWord()

		skip = false

		for len(words) > 0 {
			w := words[0]

			if w == "eval" {
				nested = append(nested, strings.Join(words[1:], " "))
				words = nil

				break
			}

			if !shellKeywords[w] && !(strings.Contains(w, "=") && !strings.ContainsAny(w, `/\`) && !strings.HasPrefix(w, "=")) {
				break
			}

			words = words[1:]
		}

		if len(words) > 0 {
			out = append(out, words)
		}

		words = nil
	}

	// closing is where the substitution opened before i ends: the
	// matching bracket (nested ones counted) or the next backquote.
	closing := func(i int, backquote bool) int {
		open := 1

		for j := i; j < len(line); j++ {
			switch c := line[j]; {
			case c == '\\':
				j++
			case backquote && c == '`':
				return j
			case !backquote && c == '(':
				open++
			case !backquote && c == ')':
				if open--; open == 0 {
					return j
				}
			}
		}

		return -1
	}

	quote := byte(0)

	for i := 0; i < len(line); i++ {
		c := line[i]

		// What is substituted is a line of its own, in or out of
		// double quotes.
		if quote != '\'' && (c == '`' || (c == '$' && i+1 < len(line) && line[i+1] == '(')) {
			from := i + 1
			if c == '$' {
				from = i + 2
			}

			if end := closing(from, c == '`'); end >= 0 {
				nested = append(nested, line[from:end])
				any = true
				i = end

				continue
			}
		}

		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case quote == '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < len(line):
				i++
				cur.WriteByte(line[i])
			default:
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote, any = c, true
		case c == '\\' && i+1 < len(line):
			i++

			if line[i] != '\n' {
				cur.WriteByte(line[i])

				any = true
			}
		case c == ' ' || c == '\t':
			endWord()
		case c == ';' || c == '&' || c == '|' || c == '\n' || c == '(' || c == ')' || c == '`':
			endCommand()
		case c == '<' || c == '>':
			// "2>file", ">>file", ">&2": the number before it and the
			// file after it are no words of the command.
			if any && strings.Trim(cur.String(), "0123456789") == "" {
				cur.Reset()

				any = false
			}

			endWord()

			for i+1 < len(line) && (line[i+1] == '>' || line[i+1] == '<' || line[i+1] == '&') {
				i++
			}

			skip = true
		default:
			cur.WriteByte(c)

			any = true
		}
	}

	// A quote that never closed: nothing may hide behind it.
	if quote != 0 {
		return commandsOf(strings.NewReplacer(`'`, " ", `"`, " ").Replace(line), depth+1)
	}

	endCommand()

	for _, n := range nested {
		out = append(out, commandsOf(n, depth+1)...)
	}

	return out
}

// The option after which a language's program takes its code on the
// command line, by the program's name without its version.
var codeOptions = map[string]string{"python": "c", "pypy": "c", "perl": "eE", "ruby": "e", "node": "ep", "nodejs": "ep",
	"php": "r", "lua": "e", "bun": "e", "deno": ""}

/*
 * codeOf is the code a language's program is given on its command line
 * (`python3 -c "..."`, `perl -e`, `node -e`, `php -r`, `deno eval`), and
 * whether there is any before the first word that is not an option.
 */
func codeOf(argv []string) (string, bool) {
	if len(argv) < 2 {
		return "", false
	}

	base := strings.ToLower(strings.TrimSuffix(filepath.Base(argv[0]), ".exe"))
	family := strings.TrimRight(base, "0123456789.-")

	letters, known := codeOptions[family]
	if !known {
		return "", false
	}

	rest := argv[1:]

	if family == "deno" {
		if rest[0] == "eval" && len(rest) > 1 {
			return strings.Join(rest[1:], " "), true
		}

		return "", false
	}

	var code []string

	for i := 0; i < len(rest); i++ {
		word := rest[i]

		if !strings.HasPrefix(word, "-") || word == "-" || word == "--" {
			break
		}

		if word == "--eval" || word == "--print" || word == "--execute" || word == "--run" {
			if i+1 < len(rest) {
				code = append(code, rest[i+1])
				i++
			}

			continue
		}

		if strings.HasPrefix(word, "--") {
			if _, v, ok := strings.Cut(word, "="); ok && (strings.HasPrefix(word, "--eval=") || strings.HasPrefix(word, "--print=")) {
				code = append(code, v)
			}

			continue
		}

		// "-c code", "-ne code": the code letter last, the code next.
		if strings.IndexByte(letters, word[len(word)-1]) >= 0 && i+1 < len(rest) {
			code = append(code, rest[i+1])
			i++

			// Python runs the code and takes the rest as its arguments.
			if family == "python" || family == "pypy" {
				break
			}
		}
	}

	if len(code) == 0 {
		return "", false
	}

	return strings.Join(code, "\n"), true
}

/*
 * programWords is every word of a piece of code that is a program here:
 * a file given by its whole place, or a name found where programs are
 * looked for. What the code does with it is not read; naming it is
 * enough for its card's "never" to refuse.
 */
func programWords(code string) []string {
	var out []string

	seen := map[string]bool{}

	for _, word := range strings.FieldsFunc(code, func(r rune) bool {
		return !(r == '_' || r == '.' || r == '/' || r == '+' || r == '-' || r == '~' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
	}) {
		if seen[word] || strings.HasPrefix(word, "-") {
			continue
		}

		seen[word] = true

		if filepath.IsAbs(word) {
			if info, err := statProgram(word); err == nil && !info.IsDir() {
				out = append(out, word)
			}

			continue
		}

		if strings.Contains(word, "/") {
			continue
		}

		if _, ok := lookProgram(word); ok {
			out = append(out, word)
		}
	}

	return out
}

// statProgram looks at a file named in code; tests replace it.
var statProgram = os.Stat

/*
 * commandUse is the program a command line runs, seen through what wraps
 * it (unwrapped): `flatpak run <id>`, `snap run <name>` and `gtk-launch
 * <entry>` name the program they start.
 *
 * A program given by its place rather than its name is said to be
 * Elsewhere unless it is the file that name runs here: a file called gimp
 * in a downloaded folder shares a name with GIMP and nothing else. A place
 * given from the folder the command runs in ("./gimp") is always
 * elsewhere.
 */
func commandUse(argv []string, dir string) programs.Use {
	inside, wrapped := unwrapped(argv)

	return useOf(inside, dir, wrapped)
}

/*
 * useOf is the program argv names by its first word, not unwrapped.
 * Read through a wrapper (wrapped), the program is named so that its card
 * can refuse it, and no card allows it: env can change which file runs
 * and what it loads, and a shell line can run anything after it.
 */
func useOf(argv []string, dir string, wrapped bool) programs.Use {
	u := programs.Use{Verb: programs.VerbChange}

	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return u
	}

	u.Wrapped = wrapped

	first := filepath.Base(argv[0])

	switch {
	case filepath.IsAbs(argv[0]):
		u.Path = filepath.Clean(argv[0])
		u.Elsewhere = elsewhere(u.Path)
	case strings.ContainsAny(argv[0], `/\`):
		u.Path = filepath.Clean(argv[0])
		if filepath.IsAbs(dir) {
			u.Path = filepath.Join(dir, argv[0])
		}

		u.Elsewhere = true
	default:
		if p, ok := lookProgram(argv[0]); ok {
			u.Path = p
		}
	}

	u.Name = first

	// A starter that is itself some file elsewhere names nothing.
	if u.Elsewhere {
		return u
	}

	switch first {
	case "flatpak":
		u.Flatpak = ranBy(argv, flatpakValued)
	case "snap":
		u.Snap = ranBy(argv, snapValued)
		if s, _, ok := strings.Cut(u.Snap, "."); ok {
			u.Snap = s
		}
	case "gtk-launch":
		if len(argv) > 1 {
			u.Desktop = argv[1]
		}
	}

	return u
}

// Options of `flatpak run` and `snap run` (and of flatpak itself) whose
// value is the next word: the program's name is not one of those.
var (
	flatpakValued = map[string]bool{
		"--installation": true, "--arch": true, "--command": true, "--cwd": true, "--branch": true,
		"--runtime": true, "--runtime-version": true, "--env": true, "--env-fd": true, "--filesystem": true,
		"--nofilesystem": true, "--share": true, "--unshare": true, "--socket": true, "--nosocket": true,
		"--device": true, "--nodevice": true, "--allow": true, "--disallow": true, "--persist": true,
		"--own-name": true, "--talk-name": true, "--no-talk-name": true, "--system-own-name": true,
		"--system-talk-name": true, "--system-no-talk-name": true, "--add-policy": true, "--remove-policy": true,
		"--instance-id-fd": true, "--app-path": true, "--usr-path": true, "--parent-pid": true,
		"--a11y-own-name": true, "--unset-env": true,
	}
	snapValued = map[string]bool{"--hook": true, "-r": true, "--command": true, "--experimental-gdbserver": true}
)

// ranBy is the program `<tool> ... run [options] <name>` runs: the first
// word after "run" that is neither an option nor an option's value.
func ranBy(argv []string, valued map[string]bool) string {
	ran := false

	for i := 1; i < len(argv); i++ {
		a := argv[i]

		switch {
		case !ran && a == "run":
			ran = true
		case valued[a]:
			i++
		case strings.HasPrefix(a, "-"):
		case ran:
			return a
		}
	}

	return ""
}

/*
 * unwrapped is a command line without what only wraps its program: env
 * and its settings and options, nohup, setsid, nice, ionice, timeout,
 * stdbuf, time, and a shell given one command to run (`sh -c "..."`: the
 * first command of the line), and whether anything wrapped it at all:
 * said by the wrappers found, never told from how many words are left.
 * What a script or a longer shell line starts further on is not followed;
 * the guide's known limits say so.
 */
func unwrapped(argv []string) (inside []string, wrapped bool) {
	for depth := 0; depth < 12 && len(argv) > 0; depth++ {
		rest, was := insideOf(argv)
		if !was {
			return argv, wrapped
		}

		argv, wrapped = rest, true
	}

	return argv, wrapped
}

// insideOf is what one wrapper runs, and whether argv starts with one.
func insideOf(argv []string) ([]string, bool) {
	rest := argv[1:]

	// skip drops the options in front: every word starting with "-",
	// and the next word too for the ones in valued.
	skip := func(valued string) []string {
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
			word := rest[0]
			rest = rest[1:]

			if word == "--" {
				break
			}

			if len(word) == 2 && strings.Contains(valued, word[1:]) && len(rest) > 0 {
				rest = rest[1:]
			}
		}

		return rest
	}

	// again is env's own words once more, with a line it was told to
	// split put in front: the line may open with settings too.
	again := func(split, after []string) []string {
		return append(append([]string{"env"}, split...), after...)
	}

	switch filepath.Base(argv[0]) {
	case "env":
		for len(rest) > 0 {
			word := rest[0]

			switch {
			case word == "-S" || word == "--split-string":
				if len(rest) < 2 {
					return nil, true
				}

				return again(strings.Fields(rest[1]), rest[2:]), true
			case strings.HasPrefix(word, "--split-string="):
				return again(strings.Fields(strings.TrimPrefix(word, "--split-string=")), rest[1:]), true
			case strings.HasPrefix(word, "-S") && !strings.HasPrefix(word, "--"):
				return again(strings.Fields(word[2:]), rest[1:]), true
			case word == "--":
				return rest[1:], true
			case word == "-u" || word == "-C" || word == "--unset" || word == "--chdir":
				rest = rest[1:]
				if len(rest) > 0 {
					rest = rest[1:]
				}
			case strings.HasPrefix(word, "-"), strings.Contains(word, "="):
				rest = rest[1:]
			default:
				return rest, true
			}
		}

		return nil, true
	case "nohup", "setsid", "time":
		return skip(""), true
	case "nice":
		return skip("n"), true
	case "ionice":
		return skip("cn"), true
	case "stdbuf":
		return skip("ioe"), true
	case "timeout":
		rest = skip("sk")

		// The time allowed stands before the command.
		if len(rest) > 0 {
			rest = rest[1:]
		}

		return rest, true
	case "sudo", "doas", "pkexec":
		rest = pastOptions(rest, "CDghpRrTtUu", "--user", "--group", "--host", "--prompt", "--chdir", "--chroot",
			"--role", "--type", "--other-user", "--close-from", "--command-timeout")

		// Settings may stand before the command (sudo A=1 tool).
		for len(rest) > 0 && strings.Contains(rest[0], "=") && !strings.ContainsAny(rest[0], `/\`) {
			rest = rest[1:]
		}

		return rest, true
	case "xargs":
		return pastOptions(rest, "adEILnPs", "--max-args", "--max-procs", "--delimiter", "--arg-file", "--max-chars",
			"--max-lines", "--process-slot-var"), true
	case "systemd-run":
		return pastOptions(rest, "upEMH", "--unit", "--property", "--slice", "--setenv", "--uid", "--gid", "--nice",
			"--working-directory", "--description", "--machine", "--host", "--service-type", "--on-active", "--on-boot",
			"--on-startup", "--on-unit-active", "--on-unit-inactive", "--on-calendar", "--on-clock-change",
			"--on-timezone-change", "--timer-property", "--path-property", "--socket-property", "--background",
			"--json", "--slice-inherit", "--expand-environment"), true
	case "busybox":
		// "busybox sh -c ...": the applet is the command.
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			return rest, true
		}
	}

	if inside, was, known := moreWrapped(argv); known {
		return inside, was
	}

	// A shell given one line, and script -c: the line's first command.
	if line, ok := lineOf(argv); ok {
		if inside := firstCommand(line); len(inside) > 0 {
			return inside, true
		}
	}

	return argv, false
}

/*
 * pastOptions is a wrapper's words after its own options: every word
 * starting with "-" up to "--" or the first that does not, and the next
 * word too after an option that takes a value: a letter of short (alone
 * or last among letters together) or one of long written without "=".
 */
func pastOptions(rest []string, short string, long ...string) []string {
	valued := map[string]bool{}
	for _, l := range long {
		valued[l] = true
	}

	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
		word := rest[0]
		rest = rest[1:]

		if word == "--" {
			break
		}

		takes := false

		if strings.HasPrefix(word, "--") {
			takes = valued[word]
		} else if i := strings.IndexAny(word[1:], short); i >= 0 {
			// The value is the rest of the word when letters follow.
			takes = i+2 == len(word)
		}

		if takes && len(rest) > 0 {
			rest = rest[1:]
		}
	}

	return rest
}

// firstCommand is the first command of a shell line as words: up to the
// first sign that ends it, without settings in front, "exec" and quotes
// around a word.
func firstCommand(line string) []string {
	if i := strings.IndexAny(line, ";&|\n()<>`"); i >= 0 {
		line = line[:i]
	}

	words := strings.Fields(line)

	for len(words) > 0 {
		word := strings.Trim(words[0], `'"`)

		if word != "exec" && word != "command" && word != "eval" && word != "builtin" &&
			!(strings.Contains(word, "=") && !strings.ContainsAny(word, `/\`)) {
			break
		}

		words = words[1:]
	}

	for i := range words {
		words[i] = strings.Trim(words[i], `'"`)
	}

	return words
}

// sameFile is whether two paths are one file, links followed. Tests
// replace it.
var sameFile = func(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}

	rb, err := filepath.EvalSymlinks(b)

	return err == nil && ra == rb
}

/*
 * elsewhere is whether a program file, given by its place, is another
 * file than the one its name runs here (links followed): it then only
 * shares that program's name. See programs.Use.Elsewhere.
 */
func elsewhere(path string) bool {
	if path == "" {
		return false
	}

	if p, ok := lookProgram(filepath.Base(path)); ok && (p == path || sameFile(p, path)) {
		return false
	}

	return true
}

// Typing goes to the window in front.
func (TypeText) ProgramUse(ctx context.Context, _ json.RawMessage) programs.Use {
	path, flatpak := windowProgram(ctx, false, 0, 0)

	return windowUse(path, flatpak)
}

// A click goes to the window under the point.
func (Click) ProgramUse(ctx context.Context, raw json.RawMessage) programs.Use {
	var a clickArgs

	_ = json.Unmarshal(raw, &a)

	path, flatpak := windowProgram(ctx, true, a.X, a.Y)

	return windowUse(path, flatpak)
}

// windowUse is typing or clicking in a window of the program at path. A
// Flatpak's window is known by the application its sandbox names, whose
// program file is a place inside the sandbox.
func windowUse(path, flatpak string) programs.Use {
	return programs.Use{Verb: programs.VerbInput, Path: path, Flatpak: flatpak, Elsewhere: flatpak == "" && elsewhere(path)}
}

/*
 * windowProgramOf asks X11 (the computer tools' only display today) which
 * process owns the window, then reads that process's program file and,
 * for a Flatpak, its application id. Empty when it cannot tell: the loop
 * then asks, since typing into an unknown program is never allowed by a
 * general setting.
 */
func windowProgramOf(ctx context.Context, atPoint bool, x, y int) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	id := ""

	if atPoint {
		id = windowAt(ctx, x, y)
	} else if out, err := pointer(ctx, "getactivewindow"); err == nil {
		id = strings.TrimSpace(out)
	} else if out, err := pointer(ctx, "getwindowfocus", "-f"); err == nil {
		// A window manager that does not say which window is active still
		// knows which has the keyboard.
		id = strings.TrimSpace(out)
	}

	if id == "" {
		return "", ""
	}

	out, err := pointer(ctx, "getwindowpid", id)
	if err != nil {
		return "", ""
	}

	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || pid <= 0 {
		return "", ""
	}

	return processProgram("/proc", pid)
}

// processProgram is a process's program file and Flatpak id, read from
// proc (a folder in tests).
func processProgram(proc string, pid int) (string, string) {
	dir := filepath.Join(proc, strconv.Itoa(pid))

	path, _ := os.Readlink(filepath.Join(dir, "exe"))
	path = strings.TrimSuffix(path, " (deleted)")

	return path, flatpakInfo(filepath.Join(dir, "root", ".flatpak-info"))
}

// flatpakInfo is the application id in a sandbox's .flatpak-info.
func flatpakInfo(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}

	defer f.Close()

	in := bufio.NewScanner(f)
	group := ""

	for in.Scan() {
		line := strings.TrimSpace(in.Text())

		if strings.HasPrefix(line, "[") {
			group = line
			continue
		}

		if group == "[Application]" {
			if v, ok := strings.CutPrefix(line, "name="); ok {
				return strings.TrimSpace(v)
			}
		}
	}

	return ""
}

/*
 * windowAt is the topmost visible window containing a point: the window
 * manager's stacking list (bottom to top), the visible windows, and each
 * one's place, from the top down.
 */
func windowAt(ctx context.Context, x, y int) string {
	stack, err := xprop(ctx, "-root", "_NET_CLIENT_LIST_STACKING")
	if err != nil {
		return ""
	}

	visible := map[int64]bool{}

	if out, err := pointer(ctx, "search", "--onlyvisible", "--name", "."); err == nil {
		for _, w := range strings.Fields(out) {
			if n, err := strconv.ParseInt(w, 10, 64); err == nil {
				visible[n] = true
			}
		}
	}

	ids := stackingIDs(stack)

	for i := len(ids) - 1; i >= 0; i-- {
		if !visible[ids[i]] {
			continue
		}

		id := strconv.FormatInt(ids[i], 10)

		geo, err := pointer(ctx, "getwindowgeometry", "--shell", id)
		if err != nil {
			continue
		}

		if geometryHolds(geo, x, y) {
			return id
		}
	}

	return ""
}

// stackingIDs reads `xprop -root _NET_CLIENT_LIST_STACKING`:
// "_NET_CLIENT_LIST_STACKING(WINDOW): window id # 0x1e00003, 0x2a00007".
func stackingIDs(out string) []int64 {
	_, list, ok := strings.Cut(out, "#")
	if !ok {
		return nil
	}

	var ids []int64

	for _, w := range strings.Split(list, ",") {
		w = strings.TrimSpace(w)

		if n, err := strconv.ParseInt(strings.TrimPrefix(w, "0x"), 16, 64); err == nil && strings.HasPrefix(w, "0x") {
			ids = append(ids, n)
		}
	}

	return ids
}

// geometryHolds is whether xdotool's --shell geometry holds the point.
func geometryHolds(shell string, x, y int) bool {
	v := map[string]int{}

	for _, line := range strings.Split(shell, "\n") {
		k, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}

		if n, err := strconv.Atoi(val); err == nil {
			v[k] = n
		}
	}

	w, h := v["WIDTH"], v["HEIGHT"]

	return w > 0 && h > 0 && x >= v["X"] && x < v["X"]+w && y >= v["Y"] && y < v["Y"]+h
}

func xprop(ctx context.Context, args ...string) (string, error) {
	tool, found := exe.Look("xprop")
	if !found {
		return "", os.ErrNotExist
	}

	out, err := display(exec.CommandContext(ctx, tool, args...)).Output()

	return string(out), err
}
