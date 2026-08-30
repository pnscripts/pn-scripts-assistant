package server

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

/*
 * The plain scripts share one global scope.
 *
 * index.html loads six files with plain <script> tags, so every top-level
 * const, let, class and function in them lands on the same object. Two files
 * declaring the same name is a SyntaxError, and a SyntaxError does not fail
 * the line — it fails the whole file, before any of it runs.
 *
 * That failure is silent in the worst possible way: the panels the file was
 * meant to fill simply stay empty, which looks exactly like having no data
 * yet. It happened twice. Once over `el`, which emptied the settings panel,
 * and once over `list`, `note` and `render`, which had left the whole "what
 * it runs on" view dead without anybody noticing, because it was checked
 * through the API rather than on screen.
 *
 * Each of those files is wrapped in an IIFE now, so nothing is exposed by
 * accident. This test is here because the next unwrapped script would be
 * just as quiet, and because a name like `render` is the natural thing to
 * reach for in a file that draws a panel — the collision is likely, not
 * exotic.
 */
func TestPlainScriptsShareNoGlobals(t *testing.T) {
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}

	files := plainScripts(string(page))
	if len(files) == 0 {
		t.Fatal("no plain script tags found in index.html; has the loading changed?")
	}

	owners := map[string][]string{}

	for _, name := range files {
		body, err := assets.ReadFile("assets/js/" + name)
		if err != nil {
			t.Fatalf("%s is loaded by index.html but missing: %v", name, err)
		}

		for _, declared := range topLevelNames(string(body)) {
			owners[declared] = append(owners[declared], name)
		}
	}

	for declared, in := range owners {
		if len(in) > 1 {
			sort.Strings(in)
			t.Errorf("%q is declared at the top level of %s. These share one "+
				"global scope, so the second one to load throws a SyntaxError "+
				"and none of it runs — its panels stay empty with nothing on "+
				"screen to say why. Wrap the file in an IIFE.",
				declared, strings.Join(in, " and "))
		}
	}
}

// Which scripts are loaded into the shared scope. Modules have their own, so
// they are not at risk and are not counted.
func plainScripts(page string) []string {
	var found []string

	tag := regexp.MustCompile(`<script([^>]*)>`)

	for _, m := range tag.FindAllStringSubmatch(page, -1) {
		attrs := m[1]
		if strings.Contains(attrs, `type="module"`) {
			continue
		}

		src := regexp.MustCompile(`src="/js/([^"]+)"`).FindStringSubmatch(attrs)
		if src != nil {
			found = append(found, src[1])
		}
	}

	return found
}

var declaration = regexp.MustCompile(`^(?:const|let|var|function|class)\s+([A-Za-z_$][\w$]*)`)

// Names a file puts into the scope it is loaded in.
//
// Depth is counted after comments and string bodies are removed, so that a
// brace inside a message or a regular expression cannot make the count drift
// and hide a real declaration below it.
func topLevelNames(body string) []string {
	var found []string

	depth := 0

	for _, line := range strings.Split(stripLiterals(body), "\n") {
		if depth == 0 {
			if m := declaration.FindStringSubmatch(strings.TrimRight(line, " \t")); m != nil {
				found = append(found, m[1])
			}
		}

		depth += strings.Count(line, "{") - strings.Count(line, "}")
	}

	return found
}

// Comments and the insides of strings, blanked so only real code is counted.
func stripLiterals(body string) string {
	var out strings.Builder

	const (
		code = iota
		lineComment
		blockComment
		str
	)

	mode, quote := code, byte(0)

	for i := 0; i < len(body); i++ {
		c := body[i]
		next := byte(0)

		if i+1 < len(body) {
			next = body[i+1]
		}

		switch mode {
		case code:
			switch {
			case c == '/' && next == '/':
				mode = lineComment
			case c == '/' && next == '*':
				mode = blockComment
			case c == '\'' || c == '"' || c == '`':
				mode, quote = str, c
			}

			if mode == code {
				out.WriteByte(c)

				continue
			}
		case lineComment:
			if c == '\n' {
				mode = code
			}
		case blockComment:
			if c == '*' && next == '/' {
				mode = code

				i++
			}
		case str:
			if c == '\\' {
				i++

				continue
			}

			if c == quote {
				mode = code
			}
		}

		// Newlines are kept so that line numbering and the per-line scan below
		// still line up with the file.
		if c == '\n' {
			out.WriteByte(c)
		}
	}

	return out.String()
}

/*
 * No two elements may share an id.
 *
 * getElementById returns the first one, so a duplicate means script writes into
 * whichever happens to come first in the document while the panel somebody is
 * looking at keeps whatever it was born with. It is the same shape of failure
 * as two scripts sharing a global name: the code runs, nothing appears, and
 * there is no error anywhere to explain it.
 *
 * It happened with models-note — reused for a new card while the Models page
 * already had one — and the card read "—" through two rebuilds while the value
 * it wanted was being written into a hidden page three views away.
 */
func TestNoTwoElementsShareAnID(t *testing.T) {
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]int{}

	for _, m := range regexp.MustCompile(`\sid="([^"]+)"`).FindAllStringSubmatch(string(page), -1) {
		seen[m[1]]++
	}

	var repeated []string

	for id, n := range seen {
		if n > 1 {
			repeated = append(repeated, fmt.Sprintf("%s (%d times)", id, n))
		}
	}

	if len(repeated) > 0 {
		sort.Strings(repeated)
		t.Errorf("these ids appear more than once, so script will fill the wrong "+
			"one and the visible panel will stay as it was: %s",
			strings.Join(repeated, ", "))
	}
}
