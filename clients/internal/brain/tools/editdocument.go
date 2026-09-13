package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"pn-scripts-assistant/internal/brain/undo"
)

/*
 * Changing a document that already exists, rather than writing a new one.
 *
 * The program could read eight formats and create four, and could not change
 * any of them. So "put the new client's name in that contract" meant rebuilding
 * the whole contract from what could be read out of it — losing the layout, the
 * letterhead, the table, and whatever else made it a contract rather than text.
 *
 * This replaces words and leaves everything else exactly as it was, because the
 * file is a zip and only one part of it holds the words.
 */
type EditDocument struct {
	// Root is the brain's folder, where the copy kept before a change goes.
	Root string
}

func (EditDocument) Name() string { return "edit_document" }

func (EditDocument) Description() string {
	return "Change wording inside a document that already exists — .docx, .odt, .pptx, " +
		".xlsx — keeping its layout, styling and everything else. Use when told to " +
		"correct, update or replace something in a document rather than write a new one."
}

func (EditDocument) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the document."},
			"find": {"type": "string", "description": "The exact text to replace, as it reads in the document."},
			"replace": {"type": "string", "description": "What to put there instead."},
			"all": {"type": "boolean", "description": "True to change every occurrence. Otherwise it must appear exactly once."}
		},
		"required": ["path", "find", "replace"]
	}`)
}

func (EditDocument) Risk() Risk { return Mutating }

type editDocArgs struct {
	Path    string `json:"path"`
	Find    string `json:"find"`
	Replace string `json:"replace"`
	All     bool   `json:"all"`
}

// Summarize shows both halves. Somebody approving a change to a contract needs
// to see the words going out and the words going in, not that one is happening.
func (EditDocument) Summarize(raw json.RawMessage) string {
	var a editDocArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "Change a document"
	}

	how := "the one occurrence of"
	if a.All {
		how = "every occurrence of"
	}

	return fmt.Sprintf("In %s, replace %s\n  %q\nwith\n  %q",
		a.Path, how, a.Find, a.Replace)
}

// wordsIn is the part of each format that holds the text. Everything else in
// the file is left untouched, byte for byte.
var wordsIn = map[string][]string{
	".docx": {"word/document.xml", "word/header1.xml", "word/footer1.xml"},
	".odt":  {"content.xml"},
	".ods":  {"content.xml"},
	".odp":  {"content.xml"},
	".pptx": {"ppt/slides/"},
	".xlsx": {"xl/sharedStrings.xml"},
}

func (t EditDocument) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a editDocArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read what to change: %w", err)
	}

	if !filepath.IsAbs(a.Path) {
		return "", fmt.Errorf("give the full path to the document")
	}

	if a.Find == "" {
		return "", fmt.Errorf("say what text to replace")
	}

	kind := strings.ToLower(filepath.Ext(a.Path))

	parts, known := wordsIn[kind]
	if !known {
		/*
		 * Said plainly rather than attempted.
		 *
		 * A PDF is the one that matters here: its text has no structure to
		 * edit, only positioned glyphs, and anything that claimed to change a
		 * word in one would be rewriting the file and losing everything that
		 * made it worth keeping. Better to say so and offer the thing that
		 * does work.
		 */
		if kind == ".pdf" {
			return "", fmt.Errorf(
				"a PDF cannot be edited in place — its text is positioned marks rather " +
					"than words. Read it, change what it says, and write a new one with " +
					"write_document")
		}

		return "", fmt.Errorf(
			"%s files cannot be edited this way; for a plain text file use edit_file", kind)
	}

	changed, count, err := t.rewrite(a, parts)
	if err != nil {
		return "", err
	}

	if count == 0 {
		return "", fmt.Errorf(
			"%q does not appear in that document. It may be split by formatting — "+
				"read the document first and use the wording exactly as it comes back", a.Find)
	}

	if count > 1 && !a.All {
		return "", fmt.Errorf(
			"%q appears %d times. Say which, or set all to change every one", a.Find, count)
	}

	// The copy is kept before anything is written, so put_it_back works on a
	// document exactly as it does on a file.
	undo.Keep(t.Root, a.Path, "edited")

	if err := os.WriteFile(a.Path, changed, 0o600); err != nil {
		return "", fmt.Errorf("writing the document back: %w", err)
	}

	if count == 1 {
		return "Changed one occurrence in " + filepath.Base(a.Path) + ".", nil
	}

	return fmt.Sprintf("Changed %d occurrences in %s.", count, filepath.Base(a.Path)), nil
}

/*
 * rewrite rebuilds the file with the words changed and everything else copied.
 *
 * Copied rather than regenerated: a document is mostly not its words. Styles,
 * images, the letterhead, the table that took somebody an afternoon — all of it
 * is other parts of the same zip, and the only honest way to keep them is to
 * pass them through untouched.
 */
func (t EditDocument) rewrite(a editDocArgs, parts []string) ([]byte, int, error) {
	archive, err := zip.OpenReader(a.Path)
	if err != nil {
		return nil, 0, fmt.Errorf("opening the document: %w", err)
	}

	defer archive.Close()

	var (
		out   bytes.Buffer
		count int
	)

	writer := zip.NewWriter(&out)

	for _, file := range archive.File {
		body, err := readAll(file)
		if err != nil {
			return nil, 0, err
		}

		if holdsWords(file.Name, parts) {
			replaced, n := replaceInXML(string(body), a.Find, a.Replace, a.All)
			body = []byte(replaced)
			count += n
		}

		// Kept compressed the way it was. A .docx with stored parts suddenly
		// deflated still opens, but it is a change nobody asked for.
		header := file.FileHeader

		entry, err := writer.CreateHeader(&header)
		if err != nil {
			return nil, 0, err
		}

		if _, err := entry.Write(body); err != nil {
			return nil, 0, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, 0, err
	}

	return out.Bytes(), count, nil
}

func readAll(file *zip.File) ([]byte, error) {
	handle, err := file.Open()
	if err != nil {
		return nil, err
	}

	defer handle.Close()

	// Capped, so a crafted archive cannot claim a part is a hundred gigabytes.
	return io.ReadAll(io.LimitReader(handle, 64<<20))
}

func holdsWords(name string, parts []string) bool {
	for _, part := range parts {
		if strings.HasSuffix(part, "/") {
			if strings.HasPrefix(name, part) {
				return true
			}

			continue
		}

		if name == part {
			return true
		}
	}

	return false
}

var tag = regexp.MustCompile(`<[^>]*>`)

/*
 * replaceInXML changes words that the format has cut into pieces.
 *
 * This is the whole difficulty. A word processor splits text across runs
 * wherever it likes — a spell-check mark, a tracked change, or nothing at all —
 * so "Acme Ltd" is quite often <w:t>Acme</w:t><w:t> Lt</w:t><w:t>d</w:t> in the
 * file. Searching the raw XML finds nothing, and reports that a name plainly
 * visible on the page is not in the document.
 *
 * So the text is matched with the tags taken out, and the replacement is put
 * into the first piece of the match while the rest are emptied. The tags
 * themselves are never touched, which is what keeps the document a document.
 */
func replaceInXML(body, find, replace string, all bool) (string, int) {
	pieces := split(body)

	var (
		plain strings.Builder
		where []int // where in plain each piece's text begins
	)

	for i, piece := range pieces {
		if piece.isTag {
			continue
		}

		where = append(where, plain.Len())
		plain.WriteString(piece.text)

		pieces[i].at = plain.Len() - len(piece.text)
	}

	text := plain.String()
	found := strings.Count(text, find)

	if found == 0 {
		return body, 0
	}

	/*
	 * How many were found, not how many will be changed.
	 *
	 * The caller has to be able to refuse. Reporting one when there are two
	 * meant "appears twice, say which" never fired, and the first of two
	 * occurrences was quietly changed — which is how somebody ends up with a
	 * contract naming two different companies.
	 */
	changing := found

	if !all {
		if found > 1 {
			return body, found
		}

		changing = 1
	}

	for done := 0; done < changing; done++ {
		at := strings.Index(text, find)
		if at < 0 {
			break
		}

		pieces = put(pieces, at, len(find), replace)

		// Rebuilt each time, because replacing shifts everything after it.
		text = plainOf(pieces)
	}

	return join(pieces), found
}

type piece struct {
	text  string
	isTag bool
	at    int
}

func split(body string) []piece {
	var (
		out  []piece
		last int
	)

	for _, span := range tag.FindAllStringIndex(body, -1) {
		if span[0] > last {
			out = append(out, piece{text: body[last:span[0]]})
		}

		out = append(out, piece{text: body[span[0]:span[1]], isTag: true})
		last = span[1]
	}

	if last < len(body) {
		out = append(out, piece{text: body[last:]})
	}

	return out
}

func plainOf(pieces []piece) string {
	var b strings.Builder

	for _, p := range pieces {
		if !p.isTag {
			b.WriteString(p.text)
		}
	}

	return b.String()
}

func join(pieces []piece) string {
	var b strings.Builder

	for _, p := range pieces {
		b.WriteString(p.text)
	}

	return b.String()
}

/*
 * put writes the replacement across however many pieces the match spans.
 *
 * All of it goes into the first piece touched and the rest of the match is
 * removed, so the new words inherit the formatting the old ones started with.
 * That is the right answer far more often than the alternatives: a name in bold
 * stays bold, and a name that happened to be split by a spell-check mark does
 * not come back with half of it in a different font.
 */
func put(pieces []piece, at, length int, replace string) []piece {
	seen := 0
	written := false

	for i := range pieces {
		if pieces[i].isTag {
			continue
		}

		start, end := seen, seen+len(pieces[i].text)
		seen = end

		if end <= at || start >= at+length {
			continue
		}

		from := max(at-start, 0)
		to := min(at+length-start, len(pieces[i].text))

		kept := pieces[i].text[:from] + pieces[i].text[to:]

		if !written {
			kept = pieces[i].text[:from] + replace + pieces[i].text[to:]
			written = true
		}

		pieces[i].text = kept
	}

	return pieces
}
