package translate

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"pn-scripts-assistant/internal/brain/glossary"
)

// upper is a model that "translates" by writing every piece in capitals,
// keeping the markers, and remembers what it was told.
type upper struct {
	mu      sync.Mutex
	calls   int
	systems []string
	drop    bool // loses the markers when given several pieces
}

func (u *upper) engine(_ context.Context, system, text string) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.calls++
	u.systems = append(u.systems, system)

	if u.drop && strings.Contains(text, "⟦2⟧") {
		return strings.ToUpper(marker.ReplaceAllString(text, "")), nil
	}

	return strings.ToUpper(text), nil
}

func TestATextIsTranslatedInPartsAndPutBackTogether(t *testing.T) {
	u := &upper{}
	tr := Translator{Engine: u.engine, Batch: 20}

	res, err := tr.Text(context.Background(), "First paragraph here.\n\n12345\n\nSecond one.\n\nThird.", "en", "bg")
	if err != nil {
		t.Fatal(err)
	}

	if res.Text != "FIRST PARAGRAPH HERE.\n\n12345\n\nSECOND ONE.\n\nTHIRD." || res.Segments != 3 {
		t.Fatalf("%q %d", res.Text, res.Segments)
	}

	if u.calls != 2 {
		t.Errorf("three paragraphs in batches of 20 characters took %d calls, want 2", u.calls)
	}
}

func TestLostMarkersFallBackToOnePieceAtATime(t *testing.T) {
	u := &upper{drop: true}
	tr := Translator{Engine: u.engine}

	res, err := tr.Text(context.Background(), "One.\n\nTwo.\n\nThree.", "en", "bg")
	if err != nil {
		t.Fatal(err)
	}

	if res.Text != "ONE.\n\nTWO.\n\nTHREE." || u.calls != 4 {
		t.Errorf("%q after %d calls", res.Text, u.calls)
	}
}

func TestTheGlossaryIsGivenAndChecked(t *testing.T) {
	u := &upper{}
	tr := Translator{Engine: u.engine, Glossary: []glossary.Term{
		{Source: "invoice", Target: "фактура", To: "bg"},
		{Source: "unused", Target: "неизползван", To: "bg"},
		{Source: "invoice", Target: "Rechnung", To: "de"},
	}}

	res, err := tr.Text(context.Background(), "Send the invoice to me@example.com.", "en", "bg")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(u.systems[0], "invoice → фактура") || strings.Contains(u.systems[0], "unused") ||
		strings.Contains(u.systems[0], "Rechnung") {
		t.Errorf("the model was told:\n%s", u.systems[0])
	}

	if len(res.Terms) != 1 || len(res.Missed) != 2 {
		t.Fatalf("terms %+v missed %+v", res.Terms, res.Missed)
	}

	if !strings.Contains(u.systems[0], "Bulgarian") {
		t.Error("the target language was not named")
	}
}

func TestNothingIsAskedWithoutAModelOrForTheSameLanguage(t *testing.T) {
	if _, err := (Translator{}).Text(context.Background(), "x", "en", "bg"); err != ErrNoEngine {
		t.Errorf("no engine: %v", err)
	}

	u := &upper{}
	if _, err := (Translator{Engine: u.engine}).Text(context.Background(), "x", "bg", "bg"); err == nil {
		t.Error("bg into bg was translated")
	}

	if _, err := (Translator{Engine: u.engine}).Text(context.Background(), "x", "en", "Bulgarian"); err == nil {
		t.Error("a language name was taken as a code")
	}
}

func TestSubtitlesKeepTheirNumbersAndTimings(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "film.srt")

	srt := "1\r\n00:00:01,000 --> 00:00:02,500\r\nHello there.\r\nHow are you?\r\n\r\n2\r\n00:00:03,000 --> 00:00:04,000\r\nFine.\r\n"
	_ = os.WriteFile(in, []byte(srt), 0o600)

	u := &upper{}

	res, err := (Translator{Engine: u.engine}).File(context.Background(), in, "", "en", "bg")
	if err != nil {
		t.Fatal(err)
	}

	if res.Text != filepath.Join(dir, "film.bg.srt") {
		t.Errorf("written to %s", res.Text)
	}

	got, _ := os.ReadFile(res.Text)
	want := "1\r\n00:00:01,000 --> 00:00:02,500\r\nHELLO THERE.\r\nHOW ARE YOU?\r\n\r\n2\r\n00:00:03,000 --> 00:00:04,000\r\nFINE.\r\n"

	if string(got) != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}

	if _, err := (Translator{Engine: u.engine}).File(context.Background(), in, "", "en", "bg"); err == nil {
		t.Error("a second translation replaced the first")
	}

	if _, err := (Translator{Engine: u.engine}).File(context.Background(), filepath.Join(dir, "a.pdf"), "", "en", "bg"); err == nil {
		t.Error("a PDF was accepted")
	}
}

func TestMarkdownCodeAndFrontMatterAreLeftAlone(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")

	md := "---\ntitle: notes\n---\n# Title\n\nSome text\nover two lines.\n\n```go\nfmt.Println(\"keep me\")\n```\n\n- a list item\n"
	_ = os.WriteFile(in, []byte(md), 0o600)

	u := &upper{}

	res, err := (Translator{Engine: u.engine}).File(context.Background(), in, filepath.Join(dir, "out.md"), "en", "bg")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(res.Text)
	want := "---\ntitle: notes\n---\n# TITLE\n\nSOME TEXT\nOVER TWO LINES.\n\n```go\nfmt.Println(\"keep me\")\n```\n\n- A LIST ITEM\n"

	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func writeZip(t *testing.T, path string, files [][2]string) {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	for _, f := range files {
		method := zip.Deflate
		if f[0] == "mimetype" {
			method = zip.Store
		}

		out, _ := w.CreateHeader(&zip.FileHeader{Name: f[0], Method: method})
		_, _ = io.WriteString(out, f[1])
	}

	_ = w.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readPart(t *testing.T, path, name string) (string, []string) {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	var (
		body  string
		names []string
	)

	for _, f := range r.File {
		names = append(names, f.Name+":"+map[uint16]string{zip.Store: "stored", zip.Deflate: "deflated"}[f.Method])

		if f.Name == name {
			raw, _ := readZip(f)
			body = string(raw)
		}
	}

	return body, names
}

/*
 * A Word document comes back with every run's formatting where it was: the
 * words of a paragraph go into its first run, the other runs are emptied,
 * and everything that is not text is untouched — including a paragraph
 * with no words, and a picture.
 */
func TestAWordDocumentKeepsItsLayout(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "letter.docx")

	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="x"><w:body>` +
		`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>Tom &amp; </w:t></w:r><w:r><w:t xml:space="preserve">Jerry</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>2026</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>In a table</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`

	writeZip(t, in, [][2]string{
		{"[Content_Types].xml", "<Types/>"},
		{"word/document.xml", doc},
		{"word/header1.xml", `<w:hdr xmlns:w="x"><w:p><w:r><w:t>Header text</w:t></w:r></w:p></w:hdr>`},
		{"word/media/image1.png", "\x89PNG not really"},
	})

	u := &upper{}

	res, err := (Translator{Engine: u.engine}).File(context.Background(), in, "", "en", "bg")
	if err != nil {
		t.Fatal(err)
	}

	got, names := readPart(t, res.Text, "word/document.xml")

	want := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="x"><w:body>` +
		`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">TOM &amp; JERRY</w:t></w:r><w:r><w:t xml:space="preserve"></w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>2026</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t xml:space="preserve">IN A TABLE</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`

	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}

	header, _ := readPart(t, res.Text, "word/header1.xml")
	if !strings.Contains(header, "HEADER TEXT") {
		t.Errorf("the header was not translated: %s", header)
	}

	picture, _ := readPart(t, res.Text, "word/media/image1.png")
	if picture != "\x89PNG not really" || len(names) != 4 {
		t.Errorf("the picture or the entries changed: %q %v", picture, names)
	}
}

func TestAnOpenDocumentStaysValid(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "note.odt")

	content := `<office:document-content><office:body><office:text>` +
		`<text:h text:outline-level="1">A <text:span text:style-name="T1">bold</text:span> title</text:h>` +
		`<text:p>Line<text:s/>two</text:p>` +
		`<text:p>Street 1<text:line-break/>Sofia</text:p>` +
		`</office:text></office:body></office:document-content>`

	writeZip(t, in, [][2]string{
		{"mimetype", "application/vnd.oasis.opendocument.text"},
		{"content.xml", content},
		{"META-INF/manifest.xml", "<manifest/>"},
	})

	u := &upper{}

	res, err := (Translator{Engine: u.engine}).File(context.Background(), in, "", "en", "bg")
	if err != nil {
		t.Fatal(err)
	}

	got, names := readPart(t, res.Text, "content.xml")

	want := `<office:document-content><office:body><office:text>` +
		`<text:h text:outline-level="1">A BOLD TITLE<text:span text:style-name="T1"></text:span></text:h>` +
		`<text:p>LINE TWO</text:p>` +
		`<text:p>STREET 1<text:line-break/>SOFIA</text:p>` +
		`</office:text></office:body></office:document-content>`

	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}

	if names[0] != "mimetype:stored" {
		t.Errorf("the mimetype entry is not first and stored: %v", names)
	}
}
