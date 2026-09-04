package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"pn-brain/internal/brain/undo"
)

/*
 * Writing the documents people actually send.
 *
 * Reading a PDF and being unable to produce one is half a job. "Write that up
 * and put it in a Word file" is an ordinary sentence in an ordinary week, and
 * an assistant that answers it with a paragraph of text on screen has not done
 * the thing that was asked.
 *
 * Both formats are built here rather than by driving an office suite. A .docx
 * is a zip of XML and a PDF is a text format with an index at the end; both are
 * a few hundred lines to write correctly and neither needs anything installed.
 * Handing it to LibreOffice would mean the feature works on machines that have
 * it and quietly does not on machines that do not.
 */

// WriteDocument writes a PDF, Word document or plain text file.
type WriteDocument struct{}

func (WriteDocument) Name() string { return "write_document" }

func (WriteDocument) Description() string {
	return "Write a document to a file: a PDF (.pdf), a Word document (.docx), " +
		"or plain text (.txt, .md). Give the text to put in it, and a title for " +
		"the first page. Use this rather than write_file when the owner asks for " +
		"a document rather than a file of code or notes."
}

func (WriteDocument) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to write, ending .pdf, .docx, .txt or .md"},
			"title": {"type": "string", "description": "A heading for the top of the document"},
			"text": {"type": "string", "description": "The body. Blank lines separate paragraphs."}
		},
		"required": ["path", "text"]
	}`)
}

func (WriteDocument) Risk() Risk { return Mutating }

func (WriteDocument) Summarize(raw json.RawMessage) string {
	var a writeDocArgs

	_ = json.Unmarshal(raw, &a)

	kind := strings.TrimPrefix(strings.ToLower(filepath.Ext(a.Path)), ".")
	if kind == "" {
		kind = "document"
	}

	return fmt.Sprintf("Write a %s to %s (%d words)\n\n%s",
		kind, a.Path, len(strings.Fields(a.Text)), first(a.Text, 400))
}

type writeDocArgs struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

func (WriteDocument) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a writeDocArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := GuardSensitive(a.Path); err != nil {
		return "", err
	}

	if !filepath.IsAbs(a.Path) {
		return "", fmt.Errorf("give an absolute path, not %q", a.Path)
	}

	if strings.TrimSpace(a.Text) == "" {
		return "", fmt.Errorf("give the text to put in the document")
	}

	var (
		body []byte
		err  error
	)

	switch strings.ToLower(filepath.Ext(a.Path)) {
	case ".pdf":
		body, err = buildPDF(a.Title, a.Text)
	case ".docx":
		body, err = buildDocx(a.Title, a.Text)
	case ".txt", ".md", "":
		body = []byte(plainText(a.Title, a.Text))
	default:
		return "", fmt.Errorf("%s is not a document this writes; use .pdf, .docx, .txt or .md",
			filepath.Ext(a.Path))
	}

	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(a.Path), 0o755); err != nil {
		return "", err
	}

	// What was there before, so "put it back" has something to put back.
	undo.Keep(Root, a.Path, "written")

	if err := os.WriteFile(a.Path, body, 0o644); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", a.Path, err)
	}

	return fmt.Sprintf("Wrote %s (%d bytes)", a.Path, len(body)), nil
}

func plainText(title, text string) string {
	if strings.TrimSpace(title) == "" {
		return text
	}

	return title + "\n" + strings.Repeat("=", utf8.RuneCountInString(title)) + "\n\n" + text
}

/* ---------- Word ---------- */

/*
 * buildDocx writes the smallest valid Word document.
 *
 * Four parts: the content types, the package relationship pointing at the
 * document, the document relationships, and the document itself. Word and
 * LibreOffice both refuse the file if any of them is missing, and the error
 * they give says nothing useful about which — so all four are written even
 * though only the last has anything in it.
 */
func buildDocx(title, text string) ([]byte, error) {
	var out bytes.Buffer

	w := zip.NewWriter(&out)

	parts := []struct{ name, body string }{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", packageRels},
		{"word/_rels/document.xml.rels", documentRels},
		{"word/document.xml", documentXML(title, text)},
	}

	for _, part := range parts {
		f, err := w.Create(part.name)
		if err != nil {
			return nil, err
		}

		if _, err := f.Write([]byte(part.body)); err != nil {
			return nil, err
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

const contentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const packageRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const documentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`

func documentXML(title, text string) string {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)

	if strings.TrimSpace(title) != "" {
		// Bold and larger, which is what a heading is without a style sheet.
		fmt.Fprintf(&b, `<w:p><w:pPr><w:spacing w:after="240"/></w:pPr><w:r>`+
			`<w:rPr><w:b/><w:sz w:val="36"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`,
			escapeXML(title))
	}

	for _, para := range paragraphs(text) {
		fmt.Fprintf(&b, `<w:p><w:pPr><w:spacing w:after="160"/></w:pPr><w:r>`+
			`<w:t xml:space="preserve">%s</w:t></w:r></w:p>`, escapeXML(para))
	}

	b.WriteString(`</w:body></w:document>`)

	return b.String()
}

// escapeXML is written out rather than using the standard library's, which
// escapes newlines and tabs into entities that Word renders literally.
func escapeXML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
	).Replace(s)
}

/* ---------- PDF ---------- */

const (
	pageWidth   = 595.0 // A4 at 72 points to the inch
	pageHeight  = 842.0
	pageMargin  = 64.0
	bodySize    = 11.0
	titleSize   = 17.0
	lineSpacing = 1.45
)

/*
 * buildPDF writes a PDF by hand.
 *
 * A PDF is a list of numbered objects followed by a table saying where each one
 * starts. The table is the part that has to be right: a reader trusts those
 * offsets absolutely, and a file whose offsets are wrong by one byte opens as
 * blank or not at all, with no clue which. So the offsets are recorded as the
 * objects are written rather than calculated afterwards.
 *
 * One font, Helvetica, which every reader has built in — embedding one would
 * multiply the size of a two-page letter by fifty.
 */
func buildPDF(title, text string) ([]byte, error) {
	lines := layout(title, text)

	var pages [][]pdfLine

	// Through a variable, because the compiler will not narrow an exact
	// constant expression to int on its own.
	usableHeight := pageHeight - 2*pageMargin
	perPage := int(usableHeight / (bodySize * lineSpacing))

	for len(lines) > 0 {
		n := perPage
		if n > len(lines) {
			n = len(lines)
		}

		pages = append(pages, lines[:n])
		lines = lines[n:]
	}

	if len(pages) == 0 {
		pages = [][]pdfLine{{}}
	}

	var (
		out     bytes.Buffer
		offsets []int
	)

	object := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}

	out.WriteString("%PDF-1.4\n")

	// 1 catalog, 2 pages, 3 font, then one content stream and one page each.
	kids := make([]string, 0, len(pages))

	for i := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 4+len(pages)+i))
	}

	object("<</Type/Catalog/Pages 2 0 R>>")
	object(fmt.Sprintf("<</Type/Pages/Kids[%s]/Count %d>>",
		strings.Join(kids, " "), len(pages)))
	object("<</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>>")

	for _, page := range pages {
		stream := content(page)
		object(fmt.Sprintf("<</Length %d>>\nstream\n%s\nendstream", len(stream), stream))
	}

	for i := range pages {
		object(fmt.Sprintf(
			"<</Type/Page/Parent 2 0 R/MediaBox[0 0 %.0f %.0f]"+
				"/Resources<</Font<</F1 3 0 R>>>>/Contents %d 0 R>>",
			pageWidth, pageHeight, 4+i))
	}

	// The table of where every object started, which is the part a reader
	// trusts absolutely.
	start := out.Len()

	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)

	for _, at := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", at)
	}

	fmt.Fprintf(&out, "trailer\n<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets)+1, start)

	return out.Bytes(), nil
}

type pdfLine struct {
	text string
	size float64
	bold bool
}

// content is the drawing instructions for one page.
func content(lines []pdfLine) string {
	var b strings.Builder

	y := pageHeight - pageMargin

	b.WriteString("BT\n")

	for _, line := range lines {
		fmt.Fprintf(&b, "/F1 %.1f Tf\n1 0 0 1 %.1f %.1f Tm\n(%s) Tj\n",
			line.size, pageMargin, y, escapePDF(line.text))

		y -= line.size * lineSpacing
	}

	b.WriteString("ET")

	return b.String()
}

/*
 * layout breaks the text into lines that fit the page.
 *
 * Width is estimated from the character count rather than measured from the
 * font's metrics table. Helvetica is proportional, so this is approximate — but
 * the alternative is embedding a metrics table for one font to get the last few
 * percent of the margin, and a line that stops slightly early is invisible
 * while a line that runs off the page is not.
 */
func layout(title, text string) []pdfLine {
	var out []pdfLine

	usable := pageWidth - 2*pageMargin

	if strings.TrimSpace(title) != "" {
		for _, line := range wrap(title, int(usable/(titleSize*0.5))) {
			out = append(out, pdfLine{text: line, size: titleSize, bold: true})
		}

		out = append(out, pdfLine{text: "", size: bodySize})
	}

	for _, para := range paragraphs(text) {
		for _, line := range wrap(para, int(usable/(bodySize*0.5))) {
			out = append(out, pdfLine{text: line, size: bodySize})
		}

		out = append(out, pdfLine{text: "", size: bodySize})
	}

	return out
}

// wrap breaks one paragraph into lines of at most n characters.
func wrap(text string, n int) []string {
	if n < 8 {
		n = 8
	}

	words := strings.Fields(text)

	if len(words) == 0 {
		return []string{""}
	}

	var (
		lines []string
		line  string
	)

	for _, word := range words {
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= n:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}

	return append(lines, line)
}

// escapePDF protects the characters that end a string inside a PDF.
func escapePDF(s string) string {
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s)
}

/* ---------- shared ---------- */

// paragraphs splits on blank lines, which is how people separate them.
func paragraphs(text string) []string {
	var out []string

	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		block = strings.TrimSpace(strings.ReplaceAll(block, "\n", " "))

		if block != "" {
			out = append(out, block)
		}
	}

	if len(out) == 0 {
		return []string{strings.TrimSpace(text)}
	}

	return out
}

func first(text string, n int) string {
	r := []rune(strings.TrimSpace(text))

	if len(r) <= n {
		return string(r)
	}

	return string(r[:n]) + "…"
}
