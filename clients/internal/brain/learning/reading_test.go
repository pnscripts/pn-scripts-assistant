package learning

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * A pass now opens the documents it is about to learn from.
 *
 * The order is the point and it is why this is affordable: scan names, throw
 * away everything already known, cut to one bite, and only then open anything.
 * Reading every document on a drive to discover they were all read last week
 * would be an afternoon of processor for nothing.
 */
func TestAPassReadsTheDocumentsItIsAboutToLearn(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "plan.md"),
		[]byte("The aim this quarter is to move billing onto the new server entirely.\n"),
		0o600); err != nil {
		t.Fatal(err)
	}

	listing := Observations{
		{
			Content: "Petar has a Markdown document called plan.md",
			Source:  "document:" + filepath.Join(dir, "plan.md"),
		},
		// What the scan leaves behind for each document it has not read.
		{Source: ReadingSource(filepath.Join(dir, "plan.md"))},
	}

	got := ReadContents(context.Background(), listing, "Petar")

	if len(got) < 2 {
		t.Fatalf("the document was listed and not read: %+v", got)
	}

	var said bool

	for _, o := range got {
		if strings.Contains(o.Content, "move billing onto the new server") {
			said = true
		}
	}

	if !said {
		t.Fatalf("what the document says was not learned:\n%+v", got)
	}
}

// The listing survives: it is the only thing available for a format nothing
// here can open, and worth knowing on its own.
func TestTheListingIsKeptAlongsideTheContents(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "plan.md"),
		[]byte("A line long enough to be worth keeping in the brain's memory.\n"), 0o600)

	listing := Observations{
		{
			Content: "Petar has a Markdown document called plan.md",
			Source:  "document:" + filepath.Join(dir, "plan.md"),
		},
		{Source: ReadingSource(filepath.Join(dir, "plan.md"))},
	}

	got := ReadContents(context.Background(), listing, "Petar")

	if got[0].Content != listing[0].Content {
		t.Fatalf("the listing was replaced rather than added to: %q", got[0].Content)
	}
}

// A project observation is not a document and must not be opened as one.
func TestOnlyDocumentsAreOpened(t *testing.T) {
	only := Observations{{
		Content: "Petar has a Go project called pn-scripts-assistant",
		Source:  "project:/home/petar/pn-scripts-assistant",
	}}

	if got := ReadContents(context.Background(), only, "Petar"); len(got) != 1 {
		t.Fatalf("something other than a document was opened: %+v", got)
	}
}

// A document that has gone, or that nothing here can read, costs the pass
// nothing and stops nothing: a folder of photographs is not a fault.
func TestAnUnreadableDocumentDoesNotStopThePass(t *testing.T) {
	listing := Observations{
		{Content: "a", Source: "document:/no/such/file.md"},
		{Content: "b", Source: "document:/home/petar/holiday.heic"},
		// A file that has gone since the scan: the placeholder is dropped so
		// it comes round again rather than being marked read on one failure.
		{Source: ReadingSource("/no/such/file.md")},
	}

	if got := ReadContents(context.Background(), listing, "Petar"); len(got) != 2 {
		t.Fatalf("a missing document was not left to come round again: %+v", got)
	}
}

/*
 * A document that opens and says nothing is recorded as saying nothing.
 *
 * Otherwise it is opened again on every pass for the rest of the machine's
 * life — and "your CV is a photograph of a page" is the answer to a real
 * question about why the brain knows nothing about it.
 */
func TestADocumentWithNothingInItIsRecordedRatherThanRetried(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scan.md")

	os.WriteFile(path, []byte("Page 1\n\n7\n"), 0o600)

	got := ReadContents(context.Background(),
		Observations{{Source: ReadingSource(path)}}, "Petar")

	if len(got) != 1 {
		t.Fatalf("expected one honest note, got %+v", got)
	}

	if got[0].Source != ReadingSource(path) {
		t.Fatalf("filed under %q, so it will be opened again", got[0].Source)
	}

	if !strings.Contains(got[0].Content, "nothing in it worth remembering") {
		t.Fatalf("did not say why it learned nothing: %q", got[0].Content)
	}
}
