package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeDoc(t *testing.T, path, title, text string) {
	t.Helper()

	args, _ := json.Marshal(writeDocArgs{Path: path, Title: title, Text: text})

	if _, err := (WriteDocument{}).Execute(context.Background(), args); err != nil {
		t.Fatal(err)
	}
}

/*
 * A PDF this writes has to be one a PDF reader will open.
 *
 * Written by hand, so the only test worth having is the round trip: produce
 * one and read it back with the same tool used to read anybody else's. A PDF
 * is a list of objects followed by a table of where each begins, and a reader
 * trusts those offsets absolutely — one byte wrong and the file opens blank,
 * with nothing to say why.
 */
func TestAPDFThatCanBeReadBack(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is not installed here")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "letter.pdf")

	const body = "The first paragraph says something worth keeping.\n\n" +
		"The second one carries on, and is long enough that it has to be broken " +
		"across more than one line by the layout rather than running off the " +
		"edge of the page where nobody would ever see it."

	writeDoc(t, path, "A Report", body)

	out, err := exec.Command("pdftotext", "-layout", path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext could not read the PDF this wrote: %v", err)
	}

	read := string(out)

	for _, want := range []string{"A Report", "first paragraph", "edge of the page"} {
		if !strings.Contains(read, want) {
			t.Errorf("the PDF lost %q; it read back as %q", want, first(read, 300))
		}
	}
}

// And the brain's own reader agrees, which is the path an owner will take when
// they ask it to read back what it just wrote.
func TestTheBrainCanReadWhatItWrote(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"note.docx", "note.pdf"} {
		path := filepath.Join(dir, name)

		if strings.HasSuffix(name, ".pdf") {
			if _, err := exec.LookPath("pdftotext"); err != nil {
				continue
			}
		}

		writeDoc(t, path, "Quarterly", "Revenue was up. Costs were flat.")

		args, _ := json.Marshal(map[string]string{"path": path})

		read, err := (ReadDocument{}).Execute(context.Background(), args)
		if err != nil {
			t.Errorf("could not read back the %s it wrote: %v", name, err)

			continue
		}

		for _, want := range []string{"Quarterly", "Revenue was up"} {
			if !strings.Contains(read, want) {
				t.Errorf("%s lost %q, read back as %q", name, want, first(read, 200))
			}
		}
	}
}

/*
 * Characters that would break the file if written straight through.
 *
 * A bracket ends a string in a PDF and an ampersand starts an entity in XML.
 * Both appear in ordinary prose, and both produce a file that opens as empty
 * rather than one that reports a problem.
 */
func TestCharactersThatWouldBreakTheFile(t *testing.T) {
	dir := t.TempDir()

	const awkward = "Costs (before tax) rose & margins fell; see \"the note\"."

	docx := filepath.Join(dir, "awkward.docx")
	writeDoc(t, docx, "R & D (2026)", awkward)

	args, _ := json.Marshal(map[string]string{"path": docx})

	read, err := (ReadDocument{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("the Word file could not be reopened: %v", err)
	}

	if !strings.Contains(read, "margins fell") || !strings.Contains(read, "R & D") {
		t.Errorf("the awkward characters broke it: %q", read)
	}

	if _, err := exec.LookPath("pdftotext"); err == nil {
		pdf := filepath.Join(dir, "awkward.pdf")
		writeDoc(t, pdf, "R & D (2026)", awkward)

		out, err := exec.Command("pdftotext", pdf, "-").Output()
		if err != nil {
			t.Fatalf("the PDF with brackets in it could not be read: %v", err)
		}

		if !strings.Contains(string(out), "before tax") {
			t.Errorf("the brackets broke the PDF: %q", out)
		}
	}
}

// A long document runs onto more pages rather than off the bottom of one.
func TestALongDocumentGetsMorePages(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is not installed here")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "long.pdf")

	var text strings.Builder

	for i := 0; i < 120; i++ {
		text.WriteString("Paragraph number ")
		text.WriteString(strings.Repeat("x", 3))
		text.WriteString(" carries some words along with it.\n\n")
	}

	writeDoc(t, path, "Long", text.String())

	out, err := exec.Command("pdftotext", path, "-").Output()
	if err != nil {
		t.Fatal(err)
	}

	// Form feeds are how pdftotext separates pages.
	if pages := strings.Count(string(out), "\f"); pages < 2 {
		t.Errorf("a very long document came out as %d page(s)", pages+1)
	}
}

// A format it cannot write is refused rather than written as something else.
func TestAnUnknownFormatIsRefused(t *testing.T) {
	dir := t.TempDir()

	args, _ := json.Marshal(writeDocArgs{
		Path: filepath.Join(dir, "sheet.xlsx"), Text: "a,b,c",
	})

	if _, err := (WriteDocument{}).Execute(context.Background(), args); err == nil {
		t.Error("a spreadsheet was written as something else without saying so")
	}
}

// The approval shows what will actually be written, not a description of it.
func TestTheApprovalShowsTheDocument(t *testing.T) {
	args, _ := json.Marshal(writeDocArgs{
		Path: "/home/somebody/report.pdf", Title: "Q3", Text: "Revenue was up.",
	})

	summary := (WriteDocument{}).Summarize(args)

	for _, want := range []string{"report.pdf", "pdf", "Revenue was up."} {
		if !strings.Contains(summary, want) {
			t.Errorf("the approval does not mention %q: %q", want, summary)
		}
	}
}

var _ = os.WriteFile
