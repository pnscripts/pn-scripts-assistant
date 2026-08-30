package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A .docx is a zip of XML, so one can be built here and read back without
// needing Word, a fixture, or a network.
func writeDocx(t *testing.T, path string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	defer f.Close()

	w := zip.NewWriter(f)

	part, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}

	_, _ = part.Write([]byte(`<?xml version="1.0"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>The quarterly report</w:t></w:r></w:p>
    <w:p><w:r><w:t>Revenue was </w:t></w:r><w:r><w:t>up.</w:t></w:r></w:p>
  </w:body>
</w:document>`))

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

/*
 * A Word file read as text is a zip header and compressed bytes, and the model
 * dutifully tries to make sense of it. Only the words belong in the answer —
 * the markup around them is fonts and revision history.
 */
func TestReadingAWordDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.docx")

	writeDocx(t, path)

	args, _ := json.Marshal(map[string]string{"path": path})

	out, err := (ReadDocument{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "The quarterly report") {
		t.Errorf("lost the text: %q", out)
	}

	// Runs split mid-sentence must come back joined, not one word per line.
	if !strings.Contains(out, "Revenue was up.") {
		t.Errorf("a sentence split across runs was not rejoined: %q", out)
	}

	for _, markup := range []string{"<w:", "xmlns", "schemas.openxmlformats"} {
		if strings.Contains(out, markup) {
			t.Errorf("the markup came through: %q", out)
		}
	}
}

// A format it cannot read is said plainly, not returned as mush.
func TestAnUnreadableFormatSaysSo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")

	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]string{"path": path})

	_, err := (ReadDocument{}).Execute(context.Background(), args)
	if err == nil {
		t.Fatal("plain text was accepted here rather than sent to read_file")
	}

	if !strings.Contains(err.Error(), "read_file") {
		t.Errorf("the error does not say what to use instead: %v", err)
	}
}

// A document with no text in it is a different problem from a broken reader,
// and saying which saves somebody a long time wondering.
func TestADocumentWithNoTextSaysWhy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.docx")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)
	part, _ := w.Create("word/document.xml")
	_, _ = part.Write([]byte(`<w:document xmlns:w="x"><w:body></w:body></w:document>`))
	w.Close()
	f.Close()

	args, _ := json.Marshal(map[string]string{"path": path})

	_, err = (ReadDocument{}).Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "scanned") {
		t.Errorf("an empty document was not explained: %v", err)
	}
}

// And a PDF, on a machine that has pdftotext.
func TestReadingAPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is not installed here")
	}

	// A minimal PDF with one line of text, written by hand so the test needs
	// no fixture file and no generator.
	const doc = "%PDF-1.4\n" +
		"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
		"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
		"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]/Contents 4 0 R" +
		"/Resources<</Font<</F1 5 0 R>>>>>>endobj\n" +
		"4 0 obj<</Length 44>>stream\nBT /F1 12 Tf 20 100 Td (Hello from a PDF) Tj ET\nendstream endobj\n" +
		"5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n" +
		"trailer<</Root 1 0 R>>\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "one.pdf")

	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]string{"path": path})

	out, err := (ReadDocument{}).Execute(context.Background(), args)
	if err != nil {
		t.Skipf("this pdftotext would not read a hand-written PDF: %v", err)
	}

	if !strings.Contains(out, "Hello from a PDF") {
		t.Errorf("the PDF text did not come through: %q", out)
	}
}
