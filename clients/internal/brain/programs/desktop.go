package programs

import (
	"bufio"
	"io"
	"strings"
)

// Entry is a menu entry as read, with what decides whether it is listed.
type Entry struct {
	Program Program

	Type      string
	Hidden    bool // masks the same desktop id further down the search path
	NoDisplay bool
	DBus      bool
	TryExec   string
}

// Listed is whether the entry is a program in the menu.
func (e Entry) Listed() bool {
	return e.Type == "Application" && !e.Hidden && !e.NoDisplay &&
		e.Program.Name != "" && (e.Program.Exec != "" || e.DBus)
}

// longestLine bounds a line of a menu entry; anything longer is skipped.
const longestLine = 64 << 10

/*
 * ParseDesktop reads a menu entry (the freedesktop Desktop Entry
 * Specification): only the [Desktop Entry] group, only the untranslated
 * keys this inventory uses. id is its desktop id. Never fails: what cannot
 * be read is simply not listed.
 */
func ParseDesktop(id string, r io.Reader) Entry {
	e := Entry{Program: Program{Kind: App, ID: id}}

	in := bufio.NewReaderSize(r, 4096)
	inGroup := false

	for {
		line, keep, err := readLine(in)
		if keep {
			line = strings.TrimSpace(line)

			switch {
			case line == "" || strings.HasPrefix(line, "#"):
			case strings.HasPrefix(line, "["):
				inGroup = line == "[Desktop Entry]"
			case inGroup:
				if key, value, ok := strings.Cut(line, "="); ok {
					e.set(strings.TrimSpace(key), strings.TrimSpace(value))
				}
			}
		}

		if err != nil {
			break
		}
	}

	return e
}

// readLine is the next line; keep is false for one longer than longestLine,
// which is read to its end and dropped.
func readLine(in *bufio.Reader) (line string, keep bool, err error) {
	var b []byte

	keep = true

	for {
		part, isPrefix, rerr := in.ReadLine()
		if keep && len(b)+len(part) > longestLine {
			keep, b = false, nil
		}

		if keep {
			b = append(b, part...)
		}

		if rerr != nil || !isPrefix {
			return string(b), keep, rerr
		}
	}
}

func (e *Entry) set(key, value string) {
	value = unescape(value)

	switch key {
	case "Type":
		e.Type = value
	case "Name":
		e.Program.Name = value
	case "Exec":
		e.Program.Exec = programOf(value)
	case "TryExec":
		e.TryExec = value
	case "Hidden":
		e.Hidden = value == "true"
	case "NoDisplay":
		e.NoDisplay = value == "true"
	case "DBusActivatable":
		e.DBus = value == "true"
	}
}

/*
 * programOf is the program an Exec line starts: its first word, unquoted,
 * after `env` and its VAR=value settings. Field codes (%f, %U...) are
 * arguments and never the first word of a valid entry.
 */
func programOf(exec string) string {
	words := splitExec(exec)

	if len(words) > 0 && (words[0] == "env" || strings.HasSuffix(words[0], "/env")) {
		words = words[1:]

		for len(words) > 0 && strings.Contains(words[0], "=") && !strings.HasPrefix(words[0], "-") {
			words = words[1:]
		}
	}

	for _, w := range words {
		if !strings.HasPrefix(w, "%") {
			return w
		}
	}

	return ""
}

// splitExec splits an Exec value into words: double quotes group, and inside
// them a backslash escapes ", `, $ and \.
func splitExec(s string) []string {
	var (
		words  []string
		cur    strings.Builder
		quoted bool
		any    bool
	)

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch {
		case quoted && c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
		case c == '"':
			quoted, any = !quoted, true
		case !quoted && (c == ' ' || c == '\t'):
			if cur.Len() > 0 || any {
				words = append(words, cur.String())
				cur.Reset()
				any = false
			}
		default:
			cur.WriteByte(c)
		}
	}

	if cur.Len() > 0 || any {
		words = append(words, cur.String())
	}

	return words
}

// unescape undoes the escapes every string value may hold (\s, \n, \t,
// \r, \\); Exec's own quoting is read after it.
func unescape(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}

	var b strings.Builder

	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i+1 == len(v) {
			b.WriteByte(v[i])

			continue
		}

		i++

		switch v[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(v[i])
		}
	}

	return b.String()
}
