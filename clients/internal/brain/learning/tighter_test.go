package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * The measurement this whole change came out of.
 *
 * Petar's review queue held 9,199 things. Grouped by the folder they came
 * from, the top ten were all somebody else's — a Unity project's package cache
 * and build artifacts, a vendored copy of HTMLPurifier, two client websites'
 * upload folders. The document walk had a skip list; it had never met a game
 * engine or a Python project.
 */
func TestSomebodyElsesFoldersAreNotOpened(t *testing.T) {
	for _, dir := range []string{
		"PackageCache", "packagecache", "Bee", "bower_components", "Pods",
		"site-packages", "venv", ".venv", "__pycache__", "obj", "coverage",
		"logs", "cache", "tmp", "node_modules", "vendor",
	} {
		if worthDescending(filepath.Join("/somewhere", dir), dir) {
			t.Errorf("%s would still be read", dir)
		}
	}

	// And the ordinary ones are still opened.
	for _, dir := range []string{
		"Documents", "Счетоводство", "Projects", "src", "app", "notes",
	} {
		if !worthDescending(filepath.Join("/home/petar", dir), dir) {
			t.Errorf("%s stopped being read", dir)
		}
	}
}

/*
 * "Library" is Unity's build cache and is also a perfectly ordinary name for a
 * folder of somebody's own documents. What stands beside it decides.
 */
func TestLibraryIsJudgedByWhatIsBesideIt(t *testing.T) {
	root := t.TempDir()

	unity := filepath.Join(root, "TestLLM")
	os.MkdirAll(filepath.Join(unity, "Assets"), 0o755)
	os.MkdirAll(filepath.Join(unity, "Library"), 0o755)

	if worthDescending(filepath.Join(unity, "Library"), "Library") {
		t.Error("a Unity build cache would still be read")
	}

	mine := filepath.Join(root, "Documents")
	os.MkdirAll(filepath.Join(mine, "Library"), 0o755)

	if !worthDescending(filepath.Join(mine, "Library"), "Library") {
		t.Error("somebody's own Library folder stopped being read")
	}
}

// Uploads under a website's public directory are other people's files.
// Anywhere else they are likely to be your own.
func TestUploadsAreJudgedByWhatIsAboveThem(t *testing.T) {
	if worthDescending("/var/www/site/public/uploads", "uploads") {
		t.Error("a website's uploads folder would still be read")
	}

	if !worthDescending("/home/petar/uploads", "uploads") {
		t.Error("somebody's own uploads folder stopped being read")
	}
}

/*
 * len() is bytes and Cyrillic is two bytes a letter, so for everything Petar
 * writes in Bulgarian the floor of 45 was really 22.
 *
 * "Петър Венциславов Николов" is 25 letters and reached his review queue as a
 * thing worth remembering.
 */
func TestLengthIsCountedInLettersNotBytes(t *testing.T) {
	repeated.Forget()

	if kept := worthKeeping("Петър Венциславов Николов\n"); len(kept) != 0 {
		t.Errorf("a three-word name was kept: %q", kept)
	}

	// And a real Bulgarian sentence of a reasonable length still is.
	long := "Основание за неначисляване на данък добавена стойност по член сто и тринадесет от закона"

	repeated.Forget()

	if kept := worthKeeping(long + "\n"); len(kept) != 1 {
		t.Errorf("a whole sentence was dropped: %q", kept)
	}
}

// A form's header line is prose by every other test and says nothing once it
// is out of the form.
func TestAFieldIsNotAStatement(t *testing.T) {
	repeated.Forget()

	// Long enough in letters, few enough in words.
	if kept := worthKeeping("Приходни фактури Счетоводство две хиляди\n"); len(kept) != 0 {
		t.Errorf("a five-word heading was kept: %q", kept)
	}
}

/*
 * The same sentence in a fourth document is a template.
 *
 * Judged on the line, because the stored form wraps it in a path that is
 * different for every file — so nothing downstream comparing whole
 * observations can see that the interesting half is identical.
 */
func TestTheSameLineInManyDocumentsStopsBeingAFact(t *testing.T) {
	repeated.Forget()

	line := "This package is part of the Unity engine and is documented here for reference"

	var kept int

	for i := 0; i < 10; i++ {
		said := FromDocumentContents(Document{
			Name: "index.md",
			Path: filepath.Join("/pkg", strings.Repeat("x", i+1), "index.md"),
			Kind: "Markdown",
		}, line+"\n", "Petar")

		kept += len(said)
	}

	if kept != SeenInThisManyDocuments {
		t.Errorf("the same line was kept from %d documents, want %d",
			kept, SeenInThisManyDocuments)
	}
}

// A line repeated inside one document — a page banner — counts once for it,
// so a long manual does not use up the allowance on its own.
func TestARefrainWithinOneDocumentCountsOnce(t *testing.T) {
	repeated.Forget()

	line := "This page is part of the handbook and may be reproduced freely"
	page := strings.Repeat(line+"\n", 20)

	said := FromDocumentContents(Document{
		Name: "handbook.pdf", Path: "/docs/handbook.pdf", Kind: "PDF",
	}, page, "Petar")

	if len(said) == 0 {
		t.Fatal("a document was silenced by its own repetition")
	}

	// Still countable as one document, so two more may carry it.
	for i := 0; i < 2; i++ {
		if got := FromDocumentContents(Document{
			Name: "other.pdf", Path: filepath.Join("/docs", strings.Repeat("y", i+1)+".pdf"),
			Kind: "PDF",
		}, line+"\n", "Petar"); len(got) == 0 {
			t.Errorf("document %d was treated as boilerplate too early", i+2)
		}
	}
}

// Numbers are what change between copies of a template.
func TestNumbersDoNotMakeTwoCopiesDifferent(t *testing.T) {
	if fingerprint("Page 3 of 12") != fingerprint("Page 7 of 12") {
		t.Error("two pages of one document read as two different lines")
	}

	if fingerprint("Фактура № 2 / 2025") != fingerprint("Фактура № 31 / 2025") {
		t.Error("two invoices read as two different lines")
	}
}
