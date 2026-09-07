package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"pn-brain/internal/brain/progress"
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

/*
 * Every tool the interface offers to announce is a tool that exists.
 *
 * The spoken line covering a slow tool never played, for as long as it had
 * existed. The interface guessed which tool was running by taking the first
 * word of the summary written for a person — "Read /etc/hosts" gives "read" —
 * and looked that up in a list keyed by "read_file". Nothing ever matched.
 *
 * Nothing failed either, which is why it went unnoticed: an announcement that
 * does not play is indistinguishable from a tool that finished quickly, and
 * the only symptom was a program that went quiet for twenty seconds while
 * somebody who had asked by voice wondered whether it was still alive.
 *
 * So the two lists are checked against each other rather than trusted to
 * match. A name here that no tool answers to is dead weight; the check is that
 * every key is spelled the way the tool spells itself.
 */
func TestAnnouncedToolsExist(t *testing.T) {
	body, err := assets.ReadFile("assets/js/working.js")
	if err != nil {
		t.Fatalf("could not read working.js: %v", err)
	}

	// Matches the map whether it holds anything or not — an empty one is now
	// the ordinary state, not a fault. See below.
	block := regexp.MustCompile(`(?s)const SPOKEN = \{(.*?)\};`).FindSubmatch(body)
	if block == nil {
		t.Fatal("could not find the SPOKEN list in working.js")
	}

	var announced []string

	for _, m := range regexp.MustCompile(`(?m)^\s{4}(\w+):`).FindAllSubmatch(block[1], -1) {
		announced = append(announced, string(m[1]))
	}

	/*
	 * An empty list is the design, not a failure.
	 *
	 * This used to insist on at least one, from when every tool announced
	 * itself. Twenty lines like "I am looking at the folder" described things
	 * already on the screen, most covered tools that return in under a second,
	 * and a turn using three tools spoke three of them before the answer. They
	 * are gone; what a long wait needed is one line at forty seconds, and what
	 * a person cannot miss is a decision waiting on them.
	 *
	 * The check that remains is the one that was always the point: if a tool
	 * is named here it has to exist, or the line silently never plays.
	 */

	real := make(map[string]bool)
	for _, tool := range toolNames(t) {
		real[tool] = true
	}

	for _, name := range announced {
		if !real[name] {
			t.Errorf("working.js announces %q, which is not a tool this program has;"+
				" it will never play", name)
		}
	}
}

// toolNames reads what the tools call themselves, from the tools package.
//
// Read from the source rather than by building a registry, because several
// tools need a database, a model or a desktop to construct, and none of that
// has any bearing on what they are called.
func toolNames(t *testing.T) []string {
	t.Helper()

	dir := filepath.Join("..", "tools")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("could not read the tools package: %v", err)
	}

	pattern := regexp.MustCompile(`func \([^)]*\) Name\(\) string \{\s*return "(\w+)"`)

	var names []string

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		for _, m := range pattern.FindAllSubmatch(body, -1) {
			names = append(names, string(m[1]))
		}
	}

	if len(names) == 0 {
		t.Fatal("found no tools at all, so the check would pass vacuously")
	}

	return names
}

/*
 * Everything the step carries reaches the page.
 *
 * The progress endpoint used to rebuild the step field by field into a map,
 * which is two lists that have to agree with nothing checking that they do. A
 * field added to the step for the interface to read was not copied here, so it
 * existed in the program, in the tests, and everywhere except the one boundary
 * that mattered — and the spoken line that depended on it never played.
 *
 * Checked against the struct's own tags rather than a list written out here,
 * because a list written out here is the same mistake one level up.
 */
func TestTheProgressEndpointDropsNothing(t *testing.T) {
	shape := reflect.TypeOf(progress.Step{})

	step := progress.Step{}

	body, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("could not encode a step: %v", err)
	}

	var onTheWire map[string]any
	if err := json.Unmarshal(body, &onTheWire); err != nil {
		t.Fatalf("could not read the encoded step: %v", err)
	}

	for i := range shape.NumField() {
		tag := shape.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")

		if name == "" || name == "-" {
			continue
		}

		if _, found := onTheWire[name]; !found {
			t.Errorf("the step carries %q but it does not reach the page", name)
		}
	}
}

/*
 * Every status the brain can report has a colour, everywhere.
 *
 * There were three separate answers to "what is the brain doing, and what
 * colour is that". The core read four states from the server's palette, the
 * feed knew nine and hard-coded five tones of its own, and the talk button had
 * a third set written into CSS. So one moment was amber in the middle of the
 * screen, cyan in the panel beside it and violet on the button below, and
 * nothing agreed with anything else.
 *
 * Three lists now have to line up: the progress kinds the Go code sets, the
 * mapping in status.js, and the CSS variables. Nothing checks that by looking
 * at it, so it is checked here — a kind added next month without a colour is
 * the same silent failure all over again, and it shows up as one panel going
 * grey while the rest are lit.
 */
func TestEveryStatusIsColouredTheSameEverywhere(t *testing.T) {
	statuses, err := assets.ReadFile("assets/js/status.js")
	if err != nil {
		t.Fatal(err)
	}

	styles, err := assets.ReadFile("assets/css/console.css")
	if err != nil {
		t.Fatal(err)
	}

	// Every kind the Go code actually reports.
	kinds := progressKinds(t)

	if len(kinds) == 0 {
		t.Fatal("found no progress kinds, so this would pass vacuously")
	}

	mapping := regexp.MustCompile(`(?s)const STATUS_OF = \{(.*?)\n\};`).FindSubmatch(statuses)
	if mapping == nil {
		t.Fatal("could not find STATUS_OF in status.js")
	}

	for _, kind := range kinds {
		if !regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(kind) + `:`).Match(mapping[1]) {
			t.Errorf("the brain reports %q but status.js gives it no status, so "+
				"whichever panel shows it will be the wrong colour", kind)
		}
	}

	// And every status named there has a colour to be.
	named := regexp.MustCompile(`(?s)const STATUSES = \[(.*?)\n\];`).FindSubmatch(statuses)
	if named == nil {
		t.Fatal("could not find STATUSES in status.js")
	}

	for _, m := range regexp.MustCompile(`'(\w+)'`).FindAllSubmatch(named[1], -1) {
		status := string(m[1])

		if !strings.Contains(string(styles), "--status-"+status+":") {
			t.Errorf("status %q has no --status-%s colour in the stylesheet, so "+
				"anything showing it falls back to grey", status, status)
		}

		if !strings.Contains(string(styles), `[data-status="`+status+`"]`) {
			t.Errorf("status %q has no [data-status] rule, so it is never "+
				"actually applied", status)
		}
	}
}

// progressKinds reads the kinds the Go code sets, from the calls themselves.
func progressKinds(t *testing.T) []string {
	t.Helper()

	pattern := regexp.MustCompile(`progress\.Set(?:Background)?\("(\w+)"`)

	seen := map[string]bool{}

	var out []string

	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		for _, m := range pattern.FindAllSubmatch(body, -1) {
			if kind := string(m[1]); !seen[kind] {
				seen[kind] = true
				out = append(out, kind)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("could not read the source: %v", err)
	}

	// SetTool always reports "tool", which no Set call spells out.
	if !seen["tool"] {
		out = append(out, "tool")
	}

	sort.Strings(out)

	return out
}

/*
 * A line that says what the brain is doing must take its colour from that.
 *
 * The live line under the conversation had amber baked into its border, its
 * fill and its text, from when it only ever said "Working". It says every
 * state now, so it rendered "Listening" as a blue dot inside an amber box in
 * amber text: three colours for one fact, two of them wrong, in the single
 * element on screen whose whole job is to report what is happening.
 *
 * The dot and the clock were already wired to --status. Only the words and the
 * frame were left behind, which is exactly the kind of gap that survives
 * review — nothing looks wrong until a state that is not amber comes along.
 */
func TestTheLiveStatusLinesTakeTheirColourFromTheStatus(t *testing.T) {
	styles, err := assets.ReadFile("assets/css/console.css")
	if err != nil {
		t.Fatal(err)
	}

	for _, block := range []string{".working", ".thinking-now"} {
		rule := regexp.MustCompile(`(?s)\n\` + block + ` \{(.*?)\n\}`).FindSubmatch(styles)
		if rule == nil {
			t.Errorf("could not find the %s rule", block)

			continue
		}

		body := string(rule[1])

		/*
		 * A literal colour here is the bug: it cannot vary with the state, so
		 * whatever it names is right for one status and wrong for the other
		 * seven.
		 */
		for _, literal := range []string{"rgba(240, 178, 107", "#f0b26b", "var(--warn)"} {
			if strings.Contains(body, literal) && !strings.Contains(body, "var(--status") {
				t.Errorf("%s fixes its colour with %s instead of following --status",
					block, literal)
			}
		}

		/*
		 * A neutral base is fine and is not what this is looking for.
		 *
		 * .thinking-now rests in the furniture greys and takes its colour from
		 * the state only where the state is what it is reporting. Quiet is a
		 * legitimate choice; claiming the wrong state is not.
		 */
	}

	// And the words between the dot and the clock, which were the part missed.
	for _, label := range []string{"#working-note", "#thinking-now-text"} {
		want := regexp.MustCompile(regexp.QuoteMeta(label) + `\s*\{\s*color: var\(--status`)

		if !want.Match(styles) {
			t.Errorf("%s is not coloured by the status, so the label can disagree "+
				"with the dot beside it", label)
		}
	}
}
