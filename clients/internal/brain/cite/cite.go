/*
 * Package cite is where an answer's claims came from, said with the answer.
 *
 * Asked what they want from an assistant before ease of use, people rank
 * accuracy and trust first, and most of those who do not use one say they do
 * not trust what it tells them. An answer that says where it came from can be
 * checked; one that does not has to be believed. So every turn keeps a ledger:
 * what it remembered and what it read while answering, each with a number, and
 * the model is asked to put the number after a sentence that relies on it.
 *
 * The ledger is this program's, never the model's. The model is told the
 * numbers and asked to use them; what it writes back is checked against the
 * ledger, and a number it invented is taken out of the answer rather than
 * shown as though it pointed somewhere. A source it used without saying so is
 * still listed — as read, not as cited — because the honest account of an
 * answer includes everything it was built from.
 */
package cite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/store"
)

// Kind is what sort of place a source is, which decides how it is shown.
type Kind string

const (
	// Memory is something it was told or taught, with no file behind it.
	Memory Kind = "memory"

	// Document is a file it learned from or read while answering.
	Document Kind = "document"

	// Project is a project folder it learned about.
	Project Kind = "project"

	// Page is a web page it read.
	Page Kind = "page"

	// Result is a web search result it was shown.
	Result Kind = "result"

	// Mail is a message in the mailbox.
	Mail Kind = "mail"

	// Conversation is an earlier conversation a lesson came from.
	Conversation Kind = "conversation"

	// Site is a site its owner visits, learned from the browser.
	Site Kind = "site"
)

// Source is one numbered place an answer was built from.
type Source struct {
	N     int    `json:"n"`
	Kind  Kind   `json:"kind"`
	Title string `json:"title"`

	// Where is a path on this machine, an address, or empty for a memory.
	Where string `json:"where,omitempty"`

	// Excerpt is a few words of what was taken from it.
	Excerpt string `json:"excerpt,omitempty"`

	// Cited is whether the answer names it. A source that is not cited was
	// still read, and is listed as such.
	Cited bool `json:"cited"`

	// FactID is the remembered fact, for a source that is one, so the
	// interface can open it in the memory view.
	FactID int64 `json:"fact_id,omitempty"`

	key string
}

// Ledger is one turn's sources, numbered in the order they were met.
type Ledger struct {
	// Quiet records what was read without numbering it for the model: for
	// an answer that will be heard, or when its owner switched citing off.
	// The list is still kept and shown.
	Quiet bool

	mu    sync.Mutex
	list  []Source
	byKey map[string]int
}

// NewLedger is an empty ledger for one turn.
func NewLedger() *Ledger { return &Ledger{byKey: map[string]int{}} }

type ledgerKey struct{}

// With carries a ledger through the turn, so the tool loop can note what it
// read without knowing anything about answers.
func With(ctx context.Context, l *Ledger) context.Context {
	return context.WithValue(ctx, ledgerKey{}, l)
}

// From is the turn's ledger, or nil — and a nil ledger notes nothing.
func From(ctx context.Context) *Ledger {
	l, _ := ctx.Value(ledgerKey{}).(*Ledger)

	return l
}

// add numbers a source, or returns the number it already has.
func (l *Ledger) add(s Source) int {
	if n, ok := l.byKey[s.key]; ok {
		return n
	}

	s.N = len(l.list) + 1
	l.list = append(l.list, s)
	l.byKey[s.key] = s.N

	return s.N
}

// Sources is everything in the ledger, in number order.
func (l *Ledger) Sources() []Source {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]Source, len(l.list))
	copy(out, l.list)

	return out
}

/*
 * Facts numbers what was recalled, one number per place.
 *
 * Two facts learned from the same report are one source, not two: somebody
 * checking the answer opens the report once. A fact with no place behind it
 * — something its owner said — is its own source.
 */
func (l *Ledger) Facts(facts []store.Scored) []int {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	numbers := make([]int, len(facts))

	for i, f := range facts {
		numbers[i] = l.add(fromFact(f))
	}

	return numbers
}

func fromFact(f store.Scored) Source {
	source := strings.TrimSpace(f.Source)

	kind, rest, _ := strings.Cut(source, ":")

	switch kind {
	case "document", "reading":
		path := learning.Resolve(rest)

		return Source{Kind: Document, Title: filepath.Base(path), Where: path,
			Excerpt: excerpt(f.Content), FactID: f.ID, key: "file:" + path}

	case "project":
		path := learning.Resolve(rest)

		return Source{Kind: Project, Title: filepath.Base(path), Where: path,
			Excerpt: excerpt(f.Content), FactID: f.ID, key: "file:" + path}

	case "browser":
		title := rest
		if rest == "__ranking__" {
			title = ""
		}

		return Source{Kind: Site, Title: title, Where: title,
			Excerpt: excerpt(f.Content), FactID: f.ID, key: "site:" + rest}

	case "conversation":
		return Source{Kind: Conversation, Title: rest, Where: source,
			Excerpt: excerpt(f.Content), FactID: f.ID, key: source}
	}

	return Source{Kind: Memory, Title: excerpt(f.Content), Excerpt: excerpt(f.Content),
		FactID: f.ID, key: fmt.Sprintf("fact:%d", f.ID)}
}

/*
 * Note is a tool's answer as the model should see it, with its sources
 * numbered.
 *
 * Only the tools that read something from a place: a page, a search, a file,
 * a message. Anything else comes back unchanged and numbers nothing. A failed
 * call is not a source — nothing was read.
 *
 * The number is written into what the model reads, beside the words it came
 * with, because a model asked to cite a list it saw three steps ago gets the
 * numbers wrong; one that sees "[3]" on the result in front of it does not.
 */
func (l *Ledger) Note(tool string, args json.RawMessage, output string) string {
	if l == nil || strings.HasPrefix(output, "Error:") || strings.TrimSpace(output) == "" {
		return output
	}

	var a readArgs

	_ = json.Unmarshal(args, &a)

	l.mu.Lock()
	defer l.mu.Unlock()

	labelled := l.noting(tool, a, output)

	if l.Quiet {
		return output
	}

	return labelled
}

// readArgs is what the reading tools are handed that says where they read.
type readArgs struct {
	URL    string `json:"url"`
	Path   string `json:"path"`
	Number int    `json:"number"`
	Folder string `json:"folder"`
}

// noting numbers what one call read and returns its output labelled.
func (l *Ledger) noting(tool string, a readArgs, output string) string {
	switch tool {
	case "fetch_url", "read_a_page":
		address := strings.TrimSpace(a.URL)
		if address == "" {
			return output
		}

		n := l.add(Source{Kind: Page, Title: pageTitle(address), Where: address,
			Excerpt: excerpt(output), key: "url:" + address})

		return label(n) + output

	case "web_search":
		return l.results(output)

	case "research":
		return l.report(output)

	case "read_document", "read_file":
		path := strings.TrimSpace(a.Path)
		if path == "" {
			return output
		}

		n := l.add(Source{Kind: Document, Title: filepath.Base(path), Where: path,
			Excerpt: excerpt(output), key: "file:" + filepath.Clean(path)})

		return label(n) + output

	case "read_email", "read_mailbox":
		if a.Number <= 0 {
			return output
		}

		folder := a.Folder
		if folder == "" {
			folder = "INBOX"
		}

		key := fmt.Sprintf("mail:%s:%d", folder, a.Number)

		n := l.add(Source{Kind: Mail, Title: mailTitle(output, a.Number), Where: key,
			Excerpt: excerpt(output), key: key})

		return label(n) + output
	}

	return output
}

func label(n int) string {
	return fmt.Sprintf("Source [%d]. If your answer uses what follows, end that sentence with [%d].\n", n, n)
}

// searchResult is one result as the search tool writes it: a numbered title,
// then its address indented beneath.
var searchResult = regexp.MustCompile(`(?m)^(\d+)\. (.*)\n\s+(https?://\S+)`)

/*
 * results renumbers a search's results as sources.
 *
 * The search tool numbers its results 1, 2, 3 — which, beside sources that
 * are also numbered, would let the model cite "[1]" meaning the first result
 * when the ledger's 1 is a remembered fact. So each result is given its
 * ledger number in place of its own.
 */
func (l *Ledger) results(output string) string {
	return searchResult.ReplaceAllStringFunc(output, func(block string) string {
		m := searchResult.FindStringSubmatch(block)
		if m == nil {
			return block
		}

		address := m[3]

		n := l.add(Source{Kind: Result, Title: strings.TrimSpace(m[2]), Where: address,
			key: "url:" + address})

		return strings.Replace(block, m[1]+". ", "["+strconv.Itoa(n)+"] ", 1)
	}) + "\nIf your answer uses a result, end that sentence with its number in square brackets."
}

// reportSource is one line of a research report's list of sources:
// "3. [Title](https://…) — …".
var reportSource = regexp.MustCompile(`(?m)^(\d{1,3})\. \[(.*?)\]\((https?://[^)\s]+)\)`)

var reportCite = regexp.MustCompile(`\[(\d{1,3})\]`)

/*
 * report renumbers a research report's sources as the turn's own.
 *
 * The report numbers its sources from 1, as a document must; beside a
 * remembered fact that is already [1] in this turn, the model would cite the
 * wrong one. So every source in its list is put in the ledger, and every
 * [n] in the report — in the text and in the list — is rewritten to the
 * ledger's number.
 */
func (l *Ledger) report(output string) string {
	to := map[string]string{}

	for _, m := range reportSource.FindAllStringSubmatch(output, -1) {
		n := l.add(Source{Kind: Page, Title: strings.TrimSpace(m[2]), Where: m[3], key: "url:" + m[3]})
		to[m[1]] = strconv.Itoa(n)
	}

	if len(to) == 0 {
		return output
	}

	// The list's own numbers, then the citations; a citation followed by
	// "(" is a link's text, not a number, and is left alone.
	output = reportSource.ReplaceAllStringFunc(output, func(line string) string {
		m := reportSource.FindStringSubmatch(line)

		return strings.Replace(line, m[1]+". ", to[m[1]]+". ", 1)
	})

	var b strings.Builder

	last := 0

	for _, at := range reportCite.FindAllStringSubmatchIndex(output, -1) {
		if at[1] < len(output) && output[at[1]] == '(' {
			continue
		}

		if n, ok := to[output[at[2]:at[3]]]; ok {
			b.WriteString(output[last:at[2]])
			b.WriteString(n)
			last = at[3]
		}
	}

	b.WriteString(output[last:])

	return b.String()
}

func pageTitle(address string) string {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return address
	}

	host := strings.TrimPrefix(u.Host, "www.")

	path := strings.Trim(u.Path, "/")
	if path == "" {
		return host
	}

	if utf8.RuneCountInString(path) > 40 {
		path = string([]rune(path)[:40]) + "…"
	}

	return host + "/" + path
}

func mailTitle(output string, number int) string {
	for _, line := range strings.Split(output, "\n") {
		if subject, ok := strings.CutPrefix(strings.TrimSpace(line), "Subject:"); ok {
			if s := strings.TrimSpace(subject); s != "" {
				return s
			}
		}
	}

	return fmt.Sprintf("#%d", number)
}

// excerpt is the first few words, enough to recognise what was taken.
func excerpt(s string) string {
	s = strings.Join(strings.Fields(s), " ")

	const most = 140

	if utf8.RuneCountInString(s) <= most {
		return s
	}

	cut := string([]rune(s)[:most])

	if i := strings.LastIndex(cut, " "); i > most/2 {
		cut = cut[:i]
	}

	return cut + "…"
}

// marker is a citation as models write it: [2], [2, 5], [2][5].
var marker = regexp.MustCompile(`\s?\[(\d{1,3}(?:\s*,\s*\d{1,3})*)\]`)

// code is a fenced block or an inline span, where brackets are code.
var code = regexp.MustCompile("(?s)```.*?(```|$)|`[^`\n]*`")

/*
 * Check holds an answer to its ledger.
 *
 * Every number the answer cites is looked up. A number the ledger has marks
 * that source cited; one it does not have — invented, or a search's own
 * numbering repeated — is taken out of the text, because a citation that
 * points nowhere is worse than none: it looks checked and is not.
 *
 * Only prose is read. "list[0]" and anything inside code is code, and an
 * answer with no sources at all is left exactly as it was written.
 */
func Check(reply string, sources []Source) (string, []Source) {
	out := make([]Source, len(sources))
	copy(out, sources)

	if len(sources) == 0 {
		return reply, out
	}

	have := map[int]int{}

	for i, s := range sources {
		have[s.N] = i
	}

	return outsideCode(reply, func(prose string) string {
		return citations(prose, have, out)
	}), out
}

func citations(prose string, have map[int]int, out []Source) string {
	var b strings.Builder

	last, index := 0, -1

	for _, at := range marker.FindAllStringSubmatchIndex(prose, -1) {
		start, end := at[0], at[1]

		bracket := strings.IndexByte(prose[start:end], '[') + start

		// Written straight after a word, or straight after an index, it is
		// an index too: list[0], matrix[1][2].
		if bracket == start && start > 0 {
			before, _ := utf8.DecodeLastRuneInString(prose[:start])

			if before == '_' || unicode.IsLetter(before) || unicode.IsDigit(before) || start == index {
				index = end

				continue
			}
		}

		var kept []string

		for _, part := range strings.Split(prose[at[2]:at[3]], ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				continue
			}

			if i, ok := have[n]; ok {
				out[i].Cited = true
				kept = append(kept, strconv.Itoa(n))
			}
		}

		b.WriteString(prose[last:start])

		if len(kept) > 0 {
			b.WriteString(prose[start:bracket] + "[" + strings.Join(kept, ", ") + "]")
		}

		last = end
	}

	b.WriteString(prose[last:])

	return b.String()
}

// outsideCode applies change to the prose of text and leaves code alone.
func outsideCode(text string, change func(string) string) string {
	var b strings.Builder

	last := 0

	for _, at := range code.FindAllStringIndex(text, -1) {
		b.WriteString(change(text[last:at[0]]))
		b.WriteString(text[at[0]:at[1]])

		last = at[1]
	}

	b.WriteString(change(text[last:]))

	return b.String()
}

// Strip removes every citation from the prose, for an answer that is going
// to be heard: a number read out in brackets is noise.
func Strip(reply string) string {
	return outsideCode(reply, func(prose string) string {
		return citations(prose, map[int]int{}, nil)
	})
}

/*
 * Preamble is the recalled facts as the model reads them, numbered.
 *
 * Each line carries its source's number and, when it has one, the place's
 * name — "from: report.pdf" — so the model can say where something came from
 * in words as well as by number.
 */
func Preamble(facts []store.Scored, numbers []int, limit int, trim func(string, int) string) string {
	if len(facts) == 0 {
		return ""
	}

	sources := map[int]Source{}

	var b strings.Builder

	b.WriteString("Relevant things you already know, recalled from your own memory, each with its source number:\n")

	for i, f := range facts {
		n := 0
		if i < len(numbers) {
			n = numbers[i]
		}

		line := trim(f.Content, limit)

		if n == 0 {
			b.WriteString("- " + line + "\n")

			continue
		}

		if _, seen := sources[n]; !seen {
			sources[n] = fromFact(f)
		}

		from := ""
		if s := sources[n]; s.Kind != Memory && s.Title != "" {
			from = " (from: " + s.Title + ")"
		}

		fmt.Fprintf(&b, "[%d] %s%s\n", n, line, from)
	}

	b.WriteString("\nUse these if they help. When a sentence of your answer relies on one, " +
		"end that sentence with its number in square brackets, like [1]. " +
		"Use only these numbers and never invent one. Do not mention this list itself.")

	return b.String()
}
