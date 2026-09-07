package learning

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * Reading what is inside a document.
 *
 * The brain reported 966 documents learned and could not answer a question
 * about one of them, because what it had learned was that they exist. These
 * tests are about the difference between those two things.
 */

func write(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func read(t *testing.T, path string) string {
	t.Helper()

	text, err := TextOf(context.Background(), path)
	if err != nil {
		t.Fatalf("reading %s: %v", filepath.Base(path), err)
	}

	return text
}

func TestPlainTextIsRead(t *testing.T) {
	path := write(t, "notes.md", "# The plan\n\nShip the thing on Tuesday.\n")

	if got := read(t, path); !strings.Contains(got, "Ship the thing on Tuesday") {
		t.Fatalf("got %q", got)
	}
}

/*
 * A .docx is a zip of XML, so it is opened with the standard library.
 *
 * Built here rather than checked in as a fixture: a binary in the repository
 * that nobody can read or amend is a worse test than one that says exactly
 * what shape it is testing.
 */
func docx(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "report.docx")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	defer f.Close()

	archive := zip.NewWriter(f)

	entry, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := entry.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestAWordDocumentIsRead(t *testing.T) {
	path := docx(t, `<?xml version="1.0"?>
		<w:document xmlns:w="x"><w:body>
		<w:p><w:r><w:t>The quarterly review</w:t></w:r></w:p>
		<w:p><w:r><w:t>Revenue was up.</w:t></w:r></w:p>
		</w:body></w:document>`)

	got := read(t, path)

	for _, want := range []string{"The quarterly review", "Revenue was up"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q from:\n%s", want, got)
		}
	}
}

/*
 * Words must not run together.
 *
 * Without a break where a paragraph ends, a document comes back as
 * "ThePlanRevisedTuesday" and nothing downstream can make a sentence of it —
 * which would leave the memory full of unusable strings that took an
 * afternoon of processor to produce.
 */
func TestParagraphsDoNotRunTogether(t *testing.T) {
	path := docx(t, `<w:document><w:body>
		<w:p><w:r><w:t>The plan</w:t></w:r></w:p>
		<w:p><w:r><w:t>Revised Tuesday</w:t></w:r></w:p>
		</w:body></w:document>`)

	if got := read(t, path); strings.Contains(got, "planRevised") {
		t.Fatalf("the paragraphs ran together:\n%s", got)
	}
}

// Malformed halfway through is normal for a file somebody has been editing;
// what was readable up to that point is still worth having.
func TestAHalfBrokenDocumentGivesUpWhatItCan(t *testing.T) {
	path := docx(t, `<w:document><w:body>
		<w:p><w:r><w:t>The part that survived</w:t></w:r></w:p>
		<w:p><w:r><w:t>and then <<<`)

	if got := read(t, path); !strings.Contains(got, "The part that survived") {
		t.Fatalf("threw away the readable part: %q", got)
	}
}

/*
 * A binary that happens to end in .txt is not learned from.
 *
 * Worse than learning nothing: fragments of a binary look like sentences, and
 * they go into memory looking like facts about somebody's life.
 */
func TestABinaryIsNotReadAsProse(t *testing.T) {
	path := write(t, "export.txt", "PK\x03\x04\x00\x00rubbish\x00\x00\xff\xfe binary")

	if got := read(t, path); got != "" {
		t.Fatalf("read a binary as prose: %q", got)
	}
}

// A file too large to be prose is skipped rather than read: a 200MB .txt is a
// log or an export, and reading it costs a machine that is already busy.
func TestSomethingTooBigIsLeftAlone(t *testing.T) {
	path := write(t, "huge.txt", strings.Repeat("x", 1024))

	if err := os.Truncate(path, BiggestWorthOpening+1); err != nil {
		t.Skip("cannot make a sparse file here")
	}

	if got := read(t, path); got != "" {
		t.Fatalf("read %d bytes of something oversized", len(got))
	}
}

// And a very long document is cut rather than swallowed whole.
func TestALongDocumentIsCutToWhatIsWorthKeeping(t *testing.T) {
	path := write(t, "book.txt", strings.Repeat("a sentence about the work. ", 20_000))

	if got := read(t, path); len(got) > MostOfADocument+200 {
		t.Fatalf("kept %d characters, cap is %d", len(got), MostOfADocument)
	}
}

// A format nothing here can open is not an error — it is a document to leave
// alone, and the difference matters to everything that calls this.
func TestAnUnknownFormatIsQuietlySkipped(t *testing.T) {
	path := write(t, "photo.heic", "not really an image")

	text, err := TextOf(context.Background(), path)

	if err != nil || text != "" {
		t.Fatalf("got %q, %v — want nothing and no error", text, err)
	}
}

func TestAMissingFileIsAnError(t *testing.T) {
	if _, err := TextOf(context.Background(), "/no/such/document.txt"); err == nil {
		t.Fatal("a missing file should be reported")
	}
}

/*
 * What can be read is answerable before anything is promised.
 *
 * The interface says "PDFs need poppler-utils" rather than silently learning
 * nothing from every PDF on a machine that has hundreds.
 */
func TestItKnowsWhatItCanOpen(t *testing.T) {
	for _, name := range []string{"a.txt", "b.md", "c.docx", "d.odt", "e.xlsx", "f.pptx"} {
		if !CanRead(name) {
			t.Errorf("%s should be readable with nothing installed", name)
		}
	}

	for _, name := range []string{"g.heic", "h.zip", "i.mp4", "j.exe"} {
		if CanRead(name) {
			t.Errorf("%s should not be claimed as readable", name)
		}
	}

	// PDF depends on the machine, and the answer must match reality rather
	// than an assumption either way.
	if CanRead("k.pdf") != HavePDFReader() {
		t.Error("PDF readability does not match whether pdftotext is installed")
	}
}

// Blank lines from the formatting elements are collapsed, or every prompt
// built from a document carries a page of nothing.
func TestWhitespaceIsTidied(t *testing.T) {
	got := tidy("First\n\n\n\n\nSecond   \n\t\n\nThird\n\n\n")

	if got != "First\n\nSecond\n\nThird" {
		t.Fatalf("got %q", got)
	}
}

/*
 * What a document says becomes something the brain can be asked about.
 *
 * This is the whole point of the reader. Before it, "966 documents" meant 966
 * file names, and the honest answer to "what does my CV say" was that it did
 * not know — while the interface reported the documents as learned.
 */
func TestWhatADocumentSaysIsRemembered(t *testing.T) {
	doc := Document{Name: "plan.md", Path: "/home/petar/plan.md", Kind: "Markdown"}

	text := "The aim this quarter is to move the whole billing system off the old " +
		"server and onto the new one.\n" +
		"Anna is leading the migration and expects it to take about six weeks.\n"

	out, _ := FromDocumentContents(doc, text, "Petar")

	if len(out) != 2 {
		t.Fatalf("kept %d lines, want 2:\n%+v", len(out), out)
	}

	if !strings.Contains(out[0].Content, "billing system") {
		t.Fatalf("did not keep what it said: %q", out[0].Content)
	}
}

/*
 * Attributed, never asserted.
 *
 * "The document says Anna is leading the migration" is true whether or not she
 * is. "Anna is leading the migration" is a claim the brain has no business
 * making because it read a file somebody left on a disk — and the documents
 * least likely to be the owner's own are exactly the ones a listing cannot
 * tell apart from theirs.
 */
func TestWhatADocumentSaysIsAttributedToTheDocument(t *testing.T) {
	doc := Document{Name: "contract.pdf", Path: "/home/petar/contract.pdf", Kind: "PDF"}

	out, _ := FromDocumentContents(doc,
		"The client agrees to pay the sum of forty thousand on completion of the work.",
		"Petar")

	if len(out) == 0 {
		t.Fatal("nothing was kept")
	}

	if !strings.Contains(out[0].Content, "contract.pdf") ||
		!strings.Contains(out[0].Content, "says:") {
		t.Fatalf("stated as a fact about its owner rather than as what the document says:\n%s",
			out[0].Content)
	}
}

// The source stays the document, so a second pass knows it has been read and
// does not open every file on the drive again.
func TestContentsAreFiledUnderTheDocument(t *testing.T) {
	doc := Document{Name: "plan.md", Path: "/home/petar/plan.md", Kind: "Markdown"}

	said, _ := FromDocumentContents(doc,
		"A long enough line about the work to be worth keeping in memory here.", "Petar")

	for _, o := range said {
		if o.Source != doc.Source() {
			t.Fatalf("filed under %q rather than the document", o.Source)
		}
	}
}

/*
 * A few lines, not the whole document.
 *
 * A brain that has swallowed every sentence of nine hundred documents has a
 * worse memory than one that knows what four of them said, because everything
 * it recalls is buried under everything else it recalls.
 */
func TestOnlyAFewLinesAreTakenFromEachDocument(t *testing.T) {
	var long strings.Builder

	for i := 0; i < 200; i++ {
		long.WriteString("This is a sentence with quite enough words in it to be kept.\n")
	}

	kept, _ := FromDocumentContents(Document{Name: "x", Path: "/x"}, long.String(), "Petar")

	if got := len(kept); got != FactsPerDocument {
		t.Fatalf("kept %d lines, want %d", got, FactsPerDocument)
	}
}

// Page numbers, headings, dates on their own: all true, none worth having.
func TestBoilerplateIsNotRemembered(t *testing.T) {
	text := "Page 3 of 12\nCONFIDENTIAL\n2026-01-04\n7\n· · ·\n" +
		"The report finds that the second option is cheaper over five years.\n"

	out, _ := FromDocumentContents(Document{Name: "r.pdf", Path: "/r.pdf"}, text, "Petar")

	if len(out) != 1 {
		t.Fatalf("kept %d lines, want only the sentence:\n%+v", len(out), out)
	}
}

// A table of figures is true and unreadable out of context.
func TestATableRowIsNotRemembered(t *testing.T) {
	text := "2024 118,402 91,220 27,182 30.1% 12,004 2025 133,900 99,100 34,800 35.1%\n"

	if out, _ := FromDocumentContents(Document{Name: "n.xlsx", Path: "/n"}, text, "Petar"); len(out) != 0 {
		t.Fatalf("remembered a row of figures: %q", out[0].Content)
	}
}

// A document with nothing readable in it contributes nothing, which is not a
// failure: a folder of photographs is not a fault.
func TestADocumentWithNothingInItContributesNothing(t *testing.T) {
	if out, _ := FromDocumentContents(Document{Name: "e.txt", Path: "/e"}, "", "Petar"); len(out) != 0 {
		t.Fatalf("invented %d things from an empty document", len(out))
	}
}

func TestTheKindMatchesWhatTheScannerCallsIt(t *testing.T) {
	for path, want := range map[string]string{
		"/a/b.pdf": "PDF", "/a/b.docx": "Word", "/a/b.md": "Markdown",
		"/a/b.heic": "document",
	} {
		if got := KindOf(path); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
}
