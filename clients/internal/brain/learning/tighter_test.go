package learning

import (
	"fmt"
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
		said, _ := FromDocumentContents(Document{
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

	said, _ := FromDocumentContents(Document{
		Name: "handbook.pdf", Path: "/docs/handbook.pdf", Kind: "PDF",
	}, page, "Petar")

	if len(said) == 0 {
		t.Fatal("a document was silenced by its own repetition")
	}

	// Still countable as one document, so two more may carry it.
	for i := 0; i < 2; i++ {
		if got, _ := FromDocumentContents(Document{
			Name: "other.pdf", Path: filepath.Join("/docs", strings.Repeat("y", i+1)+".pdf"),
			Kind: "PDF",
		}, line+"\n", "Petar"); len(got) == 0 {
			t.Errorf("document %d was treated as boilerplate too early", i+2)
		}
	}
}

/*
 * Case and spacing are not a difference. A number is.
 *
 * Digits were stripped too, at first. Twelve different notes that differed by
 * a number came out as one line repeated, and eleven of them were thrown away
 * as a form — see TestTwelveDifferentDocumentsAreAllRead, which is what caught
 * it. A number in a sentence is usually what the sentence is about.
 */
func TestWhatCountsAsTheSameLine(t *testing.T) {
	same := [][2]string{
		{"The  deployment window is  the first Tuesday", "the deployment window is the first tuesday"},
		{"ПОЛУЧАТЕЛ ПН СКРИПТС ЕООД", "Получател ПН Скриптс ЕООД"},
	}

	for _, c := range same {
		if fingerprint(c[0]) != fingerprint(c[1]) {
			t.Errorf("%q and %q read as different lines", c[0], c[1])
		}
	}

	different := [][2]string{
		{"Report for week 3 of the project", "Report for week 7 of the project"},
		{"Предоставяне на услуги за месец 1", "Предоставяне на услуги за месец 2"},
	}

	for _, c := range different {
		if fingerprint(c[0]) == fingerprint(c[1]) {
			t.Errorf("%q and %q read as the same line", c[0], c[1])
		}
	}
}

/*
 * Undoing what an earlier, looser rule let in.
 *
 * Petar's memory held 1,220 facts and 746 of them were listings of files
 * inside a Unity package cache, a vendored PHP library and two client sites'
 * upload folders. Judged by asking the same functions the walk uses, so this
 * can never disagree with what the scanner is actually doing.
 */
func TestAFileInsideASkippedFolderIsRecognised(t *testing.T) {
	for _, path := range []string{
		"/drive/DEV/TestLLM/Library/PackageCache/com.unity.ugui/Documentation~/index.md",
		"/drive/DEV/divacon.bg/tools/htmlpurifier/vendor/x/Attr.txt",
		"/home/petar/site/public/uploads/2024/09/photo.pdf",
		"/home/petar/app/node_modules/left-pad/readme.md",
		"/home/petar/app/__pycache__/thing.txt",
	} {
		if !UnderASkippedFolder(path) {
			t.Errorf("%s would still be remembered", path)
		}
	}

	// And a file of his own is left alone.
	for _, path := range []string{
		"/home/petar/Documents/PN Scripts/Счетоводство/2025/фактура.pdf",
		"/home/petar/Desktop/notes.md",
		"/drive/DEV/Projects/pnscripts/products/pn-brain/README.md",
	} {
		if UnderASkippedFolder(path) {
			t.Errorf("%s was treated as somebody else's", path)
		}
	}

	// A relative path or none at all is not a judgement this can make.
	for _, path := range []string{"", "notes.md", "./x/y.md"} {
		if UnderASkippedFolder(path) {
			t.Errorf("%q was judged when it could not be", path)
		}
	}
}

// The two sentences the brain writes about a file, and the many it writes
// about everything else.
func TestThePathIsFoundInWhatItWrote(t *testing.T) {
	cases := []struct{ said, want string }{
		{`Petar has a text document called "Attr.txt" at /a/b/Attr.txt, last modified 2024-01-02.`,
			"/a/b/Attr.txt"},
		{`cv.pdf, a PDF document at /home/petar/cv.pdf, says: he led the migration`,
			"/home/petar/cv.pdf"},
		{`Petar has a PDF at /home/petar/scan.pdf and there is nothing in it worth remembering — it holds no readable text.`,
			"/home/petar/scan.pdf"},
		{`Petar prefers Laravel over Python.`, ""},
		{`He works at home, mostly in the evening.`, ""},
	}

	for _, c := range cases {
		if got := PathIn(c.said); got != c.want {
			t.Errorf("PathIn(%q) = %q, want %q", c.said, got, c.want)
		}
	}
}

/*
 * An invoice is a form, and the top of a form is the header block.
 *
 * Dropping repeated lines one at a time was not enough: the odd line of each
 * invoice still got through — the service description, a total in words — and
 * 2,300 invoices make a queue out of the odd line. So the question is asked of
 * the file: if most of what it offers has already been read elsewhere, this is
 * one rendering of a template and nothing in it is about itself.
 *
 * Built from what Petar's actually say, including the two lines that were
 * still arriving after the line rule was in.
 */
func TestAFormStopsBeingReadOnceItIsRecognised(t *testing.T) {
	repeated.Forget()

	header := []string{
		"Получател ПН СКРИПТС ЕООД Доставчик ПЛАНЕТ АКАУНТИНГ ЕООД",
		"Адрес кв. Бенковски ул. Тетевенска 16 Адрес ул. Н. Некрасов 32",
		"МОЛ ПЕТЪР ВЕНЦИСЛАВОВ НИКОЛОВ МОЛ Юлиана Петрова",
		"Основание за неначисляване на ДДС чл.113, ал.9 от ЗДДС - лицето не е регистрирано",
		"Код Наименование на стоката или услугата Мярка Количество Цена Сума общо",
	}

	var kept, forms int

	for month := 1; month <= 12; month++ {
		// Each invoice is the same form with one line that differs.
		text := strings.Join(header, "\n") +
			fmt.Sprintf("\nПредоставяне на софтуерни консултантски услуги за месец %d\n", month)

		said, why := FromDocumentContents(Document{
			Name: fmt.Sprintf("%d_Skillo.pdf", month),
			Path: fmt.Sprintf("/docs/%d_Skillo.pdf", month),
			Kind: "PDF",
		}, text, "Petar")

		kept += len(said)

		if len(said) == 0 {
			if why != ATemplate {
				t.Errorf("invoice %d gave nothing for the wrong reason: %q", month, why)
			}

			forms++
		}
	}

	// The first few are read, because nothing can know a template is a template
	// until it has repeated. After that the file is not read at all.
	if forms == 0 {
		t.Fatal("twelve copies of one form and none was recognised as a form")
	}

	if kept > FactsPerDocument*SeenInThisManyDocuments {
		t.Errorf("kept %d things from twelve copies of one invoice", kept)
	}

	t.Logf("kept %d from 12 invoices; %d recognised as forms", kept, forms)
}

/*
 * And a folder of somebody's own writing is not a form.
 *
 * The rule that catches an invoice must not catch twelve different notes: they
 * share nothing, so nothing about them is already known.
 */
func TestTwelveDifferentDocumentsAreAllRead(t *testing.T) {
	repeated.Forget()

	for i := 1; i <= 12; i++ {
		said, why := FromDocumentContents(Document{
			Name: fmt.Sprintf("note-%d.md", i),
			Path: fmt.Sprintf("/notes/note-%d.md", i),
			Kind: "Markdown",
		}, fmt.Sprintf(
			"The meeting on subject number %d settled that we would move the "+
				"deployment to the following week instead.\n", i), "Petar")

		if len(said) == 0 {
			t.Fatalf("note %d was thrown away as a form: %q", i, why)
		}
	}
}

/*
 * A document sharing one line with others is not a form.
 *
 * A note that quotes a line from another note is a note. More than half of
 * everything worth reading is the threshold, and it has to stay above one.
 */
func TestOneSharedLineDoesNotMakeAForm(t *testing.T) {
	repeated.Forget()

	shared := "The deployment window is the first Tuesday of the month, every month."

	// Enough copies of the shared line elsewhere that it counts as known.
	for i := 0; i < 5; i++ {
		FromDocumentContents(Document{
			Name: "other.md", Path: fmt.Sprintf("/notes/other-%d.md", i), Kind: "Markdown",
		}, shared+"\n", "Petar")
	}

	said, why := FromDocumentContents(Document{
		Name: "mine.md", Path: "/notes/mine.md", Kind: "Markdown",
	}, shared+"\n"+
		"We agreed to move the reporting database onto the new machine first.\n"+
		"Ivan is away for the whole of August so the migration waits for him.\n", "Petar")

	if len(said) == 0 {
		t.Fatalf("a note with two lines of its own was thrown away: %q", why)
	}

	if len(said) != 2 {
		t.Errorf("kept %d lines, want the two that are its own", len(said))
	}
}
