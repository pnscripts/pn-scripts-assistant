package translate

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Formats are the file kinds translated with their layout kept.
var Formats = []string{".txt", ".md", ".markdown", ".srt", ".vtt", ".docx", ".odt"}

// Supported reports whether a file of this name can be translated.
func Supported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	for _, f := range Formats {
		if f == ext {
			return true
		}
	}

	return false
}

// Beside is where a translation of path into to is written by default:
// report.docx → report.bg.docx.
func Beside(path, to string) string {
	ext := filepath.Ext(path)

	return strings.TrimSuffix(path, ext) + "." + to + ext
}

// MostBytes is the largest file translated: a long book in Word.
const MostBytes = 25 << 20

/*
 * File translates the file at path into a new file at out, which must not
 * exist yet: a translation never replaces anything.
 */
func (tr Translator) File(ctx context.Context, path, out, from, to string) (Result, error) {
	if !Supported(path) {
		return Result{}, fmt.Errorf("%s files cannot be translated with their layout kept; the formats are %s",
			filepath.Ext(path), strings.Join(Formats, ", "))
	}

	if out == "" {
		out = Beside(path, to)
	}

	if !strings.EqualFold(filepath.Ext(out), filepath.Ext(path)) {
		return Result{}, fmt.Errorf("the translation must be the same kind of file as the original (%s)", filepath.Ext(path))
	}

	if _, err := os.Stat(out); err == nil {
		return Result{}, fmt.Errorf("%s already exists, and a translation never replaces a file", out)
	}

	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}

	if info.Size() > MostBytes {
		return Result{}, fmt.Errorf("%s is %d MB, more than this translates in one go", filepath.Base(path), info.Size()>>20)
	}

	var (
		body []byte
		res  Result
	)

	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx":
		body, res, err = tr.office(ctx, path, from, to, wordParts, wordXML)
	case ".odt":
		body, res, err = tr.office(ctx, path, from, to, openParts, openXML)
	default:
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return Result{}, readErr
		}

		var text string

		text, res, err = tr.plain(ctx, string(raw), strings.ToLower(filepath.Ext(path)), from, to)
		body = []byte(text)
	}

	if err != nil {
		return res, err
	}

	if err := os.WriteFile(out, body, 0o644); err != nil {
		return res, err
	}

	res.Text = out

	return res, nil
}

/*
 * A plain file is a list of pieces, each either words or layout. Layout is
 * written back as it was; words are translated.
 */
type piece struct {
	text  string
	words bool
}

func (tr Translator) plain(ctx context.Context, text, ext, from, to string) (string, Result, error) {
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}

	var parts []piece

	switch ext {
	case ".srt", ".vtt":
		parts = subtitles(text)
	case ".md", ".markdown":
		parts = markdown(text)
	default:
		parts = paragraphs(text)
	}

	var words []string

	for _, p := range parts {
		if p.words {
			words = append(words, p.text)
		}
	}

	done, res, err := tr.Pieces(ctx, words, from, to)
	if err != nil {
		return "", res, err
	}

	var b strings.Builder

	k := 0

	for _, p := range parts {
		if p.words {
			b.WriteString(done[k])
			k++
		} else {
			b.WriteString(p.text)
		}
	}

	return strings.ReplaceAll(b.String(), "\n", newline), res, nil
}

// paragraphs splits at blank lines, keeping the blank lines as layout.
func paragraphs(text string) []piece {
	var out []piece

	for i, para := range strings.Split(text, "\n\n") {
		if i > 0 {
			out = append(out, piece{text: "\n\n"})
		}

		out = append(out, piece{text: para, words: true})
	}

	return out
}

// markdown is paragraphs, with fenced code and front matter left alone.
func markdown(text string) []piece {
	var (
		out   []piece
		para  []string
		fence string
	)

	flush := func() {
		if len(para) > 0 {
			out = append(out, piece{text: strings.Join(para, "\n"), words: true})
			para = nil
		}
	}

	lines := strings.SplitAfter(text, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		bare := strings.TrimRight(line, "\n")
		trimmed := strings.TrimSpace(bare)

		// Front matter at the very top is data, not prose.
		if i == 0 && trimmed == "---" {
			j := i + 1
			for j < len(lines) && strings.TrimSpace(lines[j]) != "---" {
				j++
			}

			if j < len(lines) {
				out = append(out, piece{text: strings.Join(lines[:j+1], "")})
				i = j

				continue
			}
		}

		if fence != "" {
			out = append(out, piece{text: line})

			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}

			continue
		}

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			flush()
			fence = trimmed[:3]
			out = append(out, piece{text: line})

			continue
		}

		if trimmed == "" {
			flush()
			out = append(out, piece{text: line})

			continue
		}

		// A paragraph's lines go together; the newline that ends the last
		// one is layout.
		if strings.HasSuffix(line, "\n") {
			para = append(para, bare)

			if i+1 >= len(lines) || strings.TrimSpace(lines[i+1]) == "" || isFence(lines[i+1]) {
				flush()
				out = append(out, piece{text: "\n"})
			}

			continue
		}

		para = append(para, bare)
	}

	flush()

	return out
}

func isFence(line string) bool {
	t := strings.TrimSpace(line)

	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// subtitles keeps numbers, timings and the WEBVTT header; only what is said
// is translated, each cue as one piece so its lines stay together.
func subtitles(text string) []piece {
	var out []piece

	for i, block := range strings.Split(text, "\n\n") {
		if i > 0 {
			out = append(out, piece{text: "\n\n"})
		}

		lines := strings.Split(block, "\n")

		timing := -1

		for j, l := range lines {
			if strings.Contains(l, "-->") {
				timing = j

				break
			}
		}

		if timing < 0 || timing == len(lines)-1 {
			out = append(out, piece{text: block})

			continue
		}

		out = append(out,
			piece{text: strings.Join(lines[:timing+1], "\n") + "\n"},
			piece{text: strings.Join(lines[timing+1:], "\n"), words: true})
	}

	return out
}

/*
 * Office files: a zip whose XML parts hold the words in paragraphs. Every
 * other entry is copied byte for byte, so styles, pictures, tables and
 * everything else the file is made of come out exactly as they went in.
 */
type dialect struct {
	paragraph map[string]bool
	text      string // the element words live in; empty means any
	space     string // the attribute that keeps spaces, set on a rewritten text element

	// breaks are elements that end a line or jump to a tab stop inside a
	// paragraph. The words either side are translated as separate pieces,
	// so an address written as lines stays as lines.
	breaks map[string]bool

	// spaces are elements that stand for a space; read as one, and taken
	// out once the space is inside the translation.
	spaces map[string]bool
}

var (
	wordXML = dialect{
		paragraph: map[string]bool{"w:p": true}, text: "w:t", space: `xml:space="preserve"`,
		breaks: map[string]bool{"w:br": true, "w:tab": true, "w:cr": true},
	}
	openXML = dialect{
		paragraph: map[string]bool{"text:p": true, "text:h": true},
		breaks:    map[string]bool{"text:tab": true, "text:line-break": true},
		spaces:    map[string]bool{"text:s": true},
	}
)

func wordParts(name string) bool {
	if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
		return false
	}

	base := strings.TrimSuffix(strings.TrimPrefix(name, "word/"), ".xml")

	return base == "document" || base == "footnotes" || base == "endnotes" ||
		strings.HasPrefix(base, "header") || strings.HasPrefix(base, "footer")
}

func openParts(name string) bool { return name == "content.xml" || name == "styles.xml" }

func (tr Translator) office(ctx context.Context, path, from, to string, wanted func(string) bool, d dialect) ([]byte, Result, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, Result{}, fmt.Errorf("cannot open %s: %w", filepath.Base(path), err)
	}

	defer archive.Close()

	type part struct {
		doc   *xmlDoc
		index int
	}

	parts := map[string]part{}

	var words []string

	for i, f := range archive.File {
		if !wanted(f.Name) {
			continue
		}

		raw, err := readZip(f)
		if err != nil {
			return nil, Result{}, err
		}

		doc := parseXML(string(raw), d)
		parts[f.Name] = part{doc: doc, index: i}

		words = append(words, doc.texts()...)
	}

	if len(parts) == 0 {
		return nil, Result{}, errors.New("this file has no text in the places a document keeps it")
	}

	done, res, err := tr.Pieces(ctx, words, from, to)
	if err != nil {
		return nil, res, err
	}

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)
	k := 0

	for _, f := range archive.File {
		header := f.FileHeader

		p, rewrite := parts[f.Name]
		if !rewrite {
			// Copied untouched, compressed bytes and all — the mimetype
			// entry of an OpenDocument file must stay first and stored.
			if err := w.Copy(f); err != nil {
				return nil, res, err
			}

			continue
		}

		n := len(p.doc.paras)
		body := p.doc.rebuild(done[k : k+n])
		k += n

		header.Method = zip.Deflate

		out, err := w.CreateHeader(&header)
		if err != nil {
			return nil, res, err
		}

		if _, err := io.WriteString(out, body); err != nil {
			return nil, res, err
		}
	}

	if err := w.Close(); err != nil {
		return nil, res, err
	}

	return buf.Bytes(), res, nil
}

func readZip(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}

	defer r.Close()

	return io.ReadAll(io.LimitReader(r, MostBytes*4))
}

// xmlDoc is an XML part cut into tokens, with the paragraphs found in it.
type xmlDoc struct {
	tokens []string
	isText []bool
	paras  []para
	d      dialect
}

// para is one paragraph: the text tokens that hold its words, and the start
// tag of the first one's element, which may need its spaces kept.
type para struct {
	texts []int
	open  int
}

func parseXML(raw string, d dialect) *xmlDoc {
	doc := &xmlDoc{d: d}

	var (
		stack []string
		open  []int // indexes into doc.paras of the paragraphs open now
	)

	lastStart := -1

	for i := 0; i < len(raw); {
		if raw[i] != '<' {
			j := strings.IndexByte(raw[i:], '<')
			if j < 0 {
				j = len(raw) - i
			}

			doc.tokens = append(doc.tokens, raw[i:i+j])
			doc.isText = append(doc.isText, true)
			at := len(doc.tokens) - 1

			inside := len(open) > 0
			if inside && d.text != "" {
				inside = len(stack) > 0 && stack[len(stack)-1] == d.text
			}

			if inside {
				p := &doc.paras[open[len(open)-1]]
				if len(p.texts) == 0 {
					p.open = lastStart
				}

				p.texts = append(p.texts, at)
			}

			i += j

			continue
		}

		end := tagEnd(raw, i)
		tag := raw[i:end]
		doc.tokens = append(doc.tokens, tag)
		doc.isText = append(doc.isText, false)
		at := len(doc.tokens) - 1
		i = end

		name, closing, selfClosing := tagName(tag)
		if name == "" {
			continue
		}

		if len(open) > 0 && !closing {
			switch {
			case d.breaks[name]:
				// The words after a break are a piece of their own.
				doc.paras = append(doc.paras, para{open: -1})
				open[len(open)-1] = len(doc.paras) - 1
			case d.spaces[name] && selfClosing:
				p := &doc.paras[open[len(open)-1]]
				p.texts = append(p.texts, at)
			}
		}

		switch {
		case closing:
			for k := len(stack) - 1; k >= 0; k-- {
				if stack[k] == name {
					stack = stack[:k]

					break
				}
			}

			if d.paragraph[name] && len(open) > 0 {
				open = open[:len(open)-1]
			}
		case selfClosing:
		default:
			stack = append(stack, name)
			lastStart = at

			if d.paragraph[name] {
				doc.paras = append(doc.paras, para{open: -1})
				open = append(open, len(doc.paras)-1)
			}
		}
	}

	// Paragraphs with no words in them are not translated or counted.
	kept := doc.paras[:0]

	for _, p := range doc.paras {
		if hasLetters(doc.join(p)) {
			kept = append(kept, p)
		}
	}

	doc.paras = kept

	return doc
}

// tagEnd is the index after the markup starting at i: a tag, a comment, a
// CDATA section or a declaration.
func tagEnd(raw string, i int) int {
	for _, pair := range [][2]string{{"<!--", "-->"}, {"<![CDATA[", "]]>"}, {"<?", "?>"}} {
		if strings.HasPrefix(raw[i:], pair[0]) {
			if j := strings.Index(raw[i:], pair[1]); j >= 0 {
				return i + j + len(pair[1])
			}

			return len(raw)
		}
	}

	quote := byte(0)

	for j := i + 1; j < len(raw); j++ {
		switch c := raw[j]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j + 1
		}
	}

	return len(raw)
}

func tagName(tag string) (name string, closing, selfClosing bool) {
	if strings.HasPrefix(tag, "<!") || strings.HasPrefix(tag, "<?") {
		return "", false, false
	}

	body := strings.TrimSuffix(strings.TrimPrefix(tag, "<"), ">")

	if strings.HasPrefix(body, "/") {
		closing, body = true, body[1:]
	}

	if strings.HasSuffix(body, "/") {
		selfClosing, body = true, strings.TrimSuffix(body, "/")
	}

	if i := strings.IndexAny(body, " \t\r\n"); i >= 0 {
		body = body[:i]
	}

	return body, closing, selfClosing
}

func (doc *xmlDoc) join(p para) string {
	var b strings.Builder

	for _, i := range p.texts {
		if !doc.isText[i] {
			b.WriteString(" ")

			continue
		}

		b.WriteString(html.UnescapeString(doc.tokens[i]))
	}

	return b.String()
}

func (doc *xmlDoc) texts() []string {
	out := make([]string, len(doc.paras))

	for i, p := range doc.paras {
		out[i] = doc.join(p)
	}

	return out
}

/*
 * rebuild puts each translated paragraph into the first stretch of text the
 * original paragraph had, and empties the rest. The formatting of the first
 * run is kept for the whole paragraph; tags between the runs stay where they
 * are, so nothing that is not text is lost.
 */
func (doc *xmlDoc) rebuild(translated []string) string {
	tokens := append([]string(nil), doc.tokens...)

	for k, p := range doc.paras {
		var esc bytes.Buffer

		_ = xml.EscapeText(&esc, []byte(translated[k]))

		text := esc.String()

		// EscapeText also escapes newlines and tabs, which in a paragraph's
		// text are only what the model added; spaces are what is meant.
		text = strings.NewReplacer("&#xA;", " ", "&#x9;", " ", "&#xD;", "").Replace(text)

		// The words go into the first stretch of text; everything else the
		// paragraph's words were made of — other runs, space elements — is
		// emptied, because it is all inside the translation now.
		first := -1

		for _, i := range p.texts {
			if doc.isText[i] && first < 0 {
				first = i
				tokens[i] = text

				continue
			}

			tokens[i] = ""
		}

		if doc.d.space != "" && p.open >= 0 && !strings.Contains(tokens[p.open], "xml:space") {
			start := tokens[p.open]
			tokens[p.open] = strings.TrimSuffix(start, ">") + " " + doc.d.space + ">"
		}
	}

	return strings.Join(tokens, "")
}
