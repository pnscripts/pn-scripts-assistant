package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aDocx writes a minimal .docx with the given document body, plus a part that
// is nothing to do with the text and must survive untouched.
func aDocx(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "contract.docx")

	var out bytes.Buffer

	w := zip.NewWriter(&out)

	for _, part := range []struct{ name, content string }{
		{"[Content_Types].xml", `<?xml version="1.0"?><Types/>`},
		{"word/document.xml", body},
		{"word/media/logo.png", "\x89PNG\r\n\x1a\n-not-really-a-png-"},
		{"word/styles.xml", `<?xml version="1.0"?><styles>the letterhead</styles>`},
	} {
		entry, err := w.Create(part.name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := entry.Write([]byte(part.content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func partOf(t *testing.T, path, name string) string {
	t.Helper()

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}

	defer archive.Close()

	for _, file := range archive.File {
		if file.Name != name {
			continue
		}

		body, err := readAll(file)
		if err != nil {
			t.Fatal(err)
		}

		return string(body)
	}

	t.Fatalf("%s is gone from the document", name)

	return ""
}

func edit(t *testing.T, path, find, replace string, all bool) (string, error) {
	t.Helper()

	raw, _ := json.Marshal(editDocArgs{Path: path, Find: find, Replace: replace, All: all})

	return EditDocument{Root: t.TempDir()}.Execute(context.Background(), raw)
}

/*
 * A name the word processor cut into pieces is still one name.
 *
 * This is the whole difficulty and the reason this could not be done by
 * searching the file. A word processor splits text across runs wherever it
 * likes — a spell-check mark, a tracked change, or nothing at all — so "Acme
 * Ltd" is quite often three separate runs. Searching the raw XML finds
 * nothing, and reports that a name plainly visible on the page is not in the
 * document.
 */
func TestANameSplitAcrossRunsIsStillFound(t *testing.T) {
	path := aDocx(t, `<?xml version="1.0"?><w:document><w:body><w:p>`+
		`<w:r><w:t>This agreement is with </w:t></w:r>`+
		`<w:r><w:t>Acme</w:t></w:r>`+
		`<w:r><w:rPr><w:b/></w:rPr><w:t> Lt</w:t></w:r>`+
		`<w:r><w:t>d</w:t></w:r>`+
		`<w:t> of Sofia.</w:t>`+
		`</w:p></w:body></w:document>`)

	said, err := edit(t, path, "Acme Ltd", "Beta Studio", false)
	if err != nil {
		t.Fatalf("a name split across runs was not found: %v", err)
	}

	if !strings.Contains(said, "one occurrence") {
		t.Errorf("it said %q", said)
	}

	document := partOf(t, path, "word/document.xml")

	if strings.Contains(document, "Acme") {
		t.Errorf("the old name is still there: %s", document)
	}

	if !strings.Contains(document, "Beta Studio") {
		t.Errorf("the new name is not there: %s", document)
	}

	// The tags themselves are untouched, which is what keeps it a document.
	if !strings.Contains(document, "<w:b/>") {
		t.Error("the formatting was lost")
	}

	if !strings.Contains(document, "This agreement is with ") ||
		!strings.Contains(document, " of Sofia.") {
		t.Errorf("the text around it was damaged: %s", document)
	}
}

/*
 * Everything that is not the words comes through byte for byte.
 *
 * A document is mostly not its words. The letterhead, the images, the styles —
 * rebuilding rather than copying is exactly what made "change the client's
 * name" mean losing the thing that made it a contract.
 */
func TestEverythingThatIsNotTheWordsSurvives(t *testing.T) {
	path := aDocx(t, `<?xml version="1.0"?><w:document><w:t>Acme Ltd</w:t></w:document>`)

	if _, err := edit(t, path, "Acme Ltd", "Beta Studio", false); err != nil {
		t.Fatal(err)
	}

	if got := partOf(t, path, "word/styles.xml"); !strings.Contains(got, "the letterhead") {
		t.Errorf("the styles were rewritten: %q", got)
	}

	if got := partOf(t, path, "word/media/logo.png"); !strings.Contains(got, "not-really-a-png") {
		t.Errorf("an image was damaged: %q", got)
	}
}

/*
 * Something appearing twice is not changed on a guess.
 *
 * Changing the first of two and saying nothing is how somebody ends up with a
 * contract that names two different companies.
 */
func TestSomethingAppearingTwiceIsNotGuessedAt(t *testing.T) {
	path := aDocx(t, `<?xml version="1.0"?><w:document>`+
		`<w:t>Acme Ltd agrees. Acme Ltd will pay.</w:t></w:document>`)

	_, err := edit(t, path, "Acme Ltd", "Beta Studio", false)
	if err == nil {
		t.Fatal("it changed one of two occurrences without asking")
	}

	if !strings.Contains(err.Error(), "appears 2 times") {
		t.Errorf("it said %q", err)
	}

	// Nothing was written while it was refusing.
	if !strings.Contains(partOf(t, path, "word/document.xml"), "Acme Ltd agrees") {
		t.Error("the document was changed anyway")
	}

	// Told to change them all, it does.
	said, err := edit(t, path, "Acme Ltd", "Beta Studio", true)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "2 occurrences") {
		t.Errorf("it said %q", said)
	}

	document := partOf(t, path, "word/document.xml")

	if strings.Contains(document, "Acme") || strings.Count(document, "Beta Studio") != 2 {
		t.Errorf("document is now: %s", document)
	}
}

// Text that is not there says so, with the reason it is most often not found.
func TestTextThatIsNotThereSaysSo(t *testing.T) {
	path := aDocx(t, `<?xml version="1.0"?><w:document><w:t>Nothing of the sort</w:t></w:document>`)

	_, err := edit(t, path, "Acme Ltd", "Beta Studio", false)
	if err == nil {
		t.Fatal("it claimed to change text that was not there")
	}

	if !strings.Contains(err.Error(), "read the document first") {
		t.Errorf("it does not say what to do about it: %q", err)
	}
}

/*
 * A PDF is refused, and the refusal explains itself.
 *
 * Its text is positioned marks rather than words, so anything claiming to
 * change one would be rewriting the file and losing everything that made it
 * worth keeping. Saying so beats doing that quietly.
 */
func TestAPDFIsRefusedWithTheReason(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.pdf")

	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := edit(t, path, "Acme", "Beta", false)
	if err == nil {
		t.Fatal("a PDF was edited in place")
	}

	if !strings.Contains(err.Error(), "write_document") {
		t.Errorf("it does not offer the thing that works: %q", err)
	}
}

// Changing a document waits for approval, and the summary shows both halves —
// the words going out and the words going in.
func TestChangingADocumentWaitsAndShowsBothHalves(t *testing.T) {
	var tool EditDocument

	if tool.Risk() != Mutating {
		t.Fatal("a document can be changed without anybody approving it")
	}

	raw, _ := json.Marshal(editDocArgs{
		Path: "/home/petar/contract.docx", Find: "Acme Ltd", Replace: "Beta Studio",
	})

	summary := tool.Summarize(raw)

	for _, want := range []string{"contract.docx", "Acme Ltd", "Beta Studio"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the summary does not mention %q: %q", want, summary)
		}
	}
}
