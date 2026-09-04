package learning

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

/*
 * Reading what is inside a document, rather than noting that it exists.
 *
 * The scanner lists documents: name, kind, when they changed. That was the
 * whole of it, and it is why a brain reporting 966 documents learned could not
 * answer a question about a single one of them. Knowing that a file called
 * cv.pdf exists is not knowing anything about somebody's work; it is knowing
 * the name of a thing that does.
 *
 * So the formats people actually keep things in are opened and read. Three
 * routes, in order of how much they can be trusted:
 *
 *   - Plain text and Markdown are read directly.
 *   - The office formats are zip archives of XML, so they are opened with the
 *     standard library and the tags stripped. No dependency, no parser to keep
 *     up to date, and it works on a machine with nothing else installed.
 *   - PDF needs a real parser, so it asks pdftotext and does without when that
 *     is missing rather than pretending.
 *
 * Everything here is read-only and stays on this machine. It reads somebody's
 * entire working life, and the one guarantee worth making about that is that
 * looking changes nothing.
 */

// MostOfADocument is how much of one file is read.
//
// Enough for a report, a note, a chapter; short of a novel or a database dump
// that happens to end in .txt. What is wanted from a document is what it is
// about, and the first pages of anything written by a person say that.
const MostOfADocument = 40_000

// BiggestWorthOpening skips files too large to be prose.
//
// A 200MB .txt is a log or an export, and the cost of reading it is real on a
// machine where the model is already waiting for the processor.
const BiggestWorthOpening = 25 << 20

// ReadingTimeout bounds one file, because an outside program is involved and a
// PDF can be pathological.
const ReadingTimeout = 30 * time.Second

/*
 * TextOf reads a document and returns what it says.
 *
 * An empty string with no error means "nothing readable here" — an image-only
 * PDF, an empty file, a format this cannot open — and callers treat that as a
 * document to leave alone rather than as a failure.
 */
func TextOf(ctx context.Context, path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if info.Size() > BiggestWorthOpening {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(ctx, ReadingTimeout)
	defer cancel()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".markdown", ".rst", ".csv", ".log":
		return plainText(path)

	case ".docx", ".dotx":
		return insideOffice(path, "word/document.xml")

	case ".odt", ".ods", ".odp":
		return insideOffice(path, "content.xml")

	case ".xlsx":
		return insideOffice(path, "xl/sharedStrings.xml")

	case ".pptx":
		return insidePresentation(path)

	case ".pdf":
		return fromPDF(ctx, path)
	}

	return "", nil
}

// CanRead reports whether a document's contents can be got at on this machine.
//
// Asked before promising anything, so the interface can say "PDFs need
// poppler-utils" rather than quietly learning nothing from half of them.
func CanRead(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".markdown", ".rst", ".csv", ".log",
		".docx", ".dotx", ".odt", ".ods", ".odp", ".xlsx", ".pptx":
		return true

	case ".pdf":
		return HavePDFReader()
	}

	return false
}

// HavePDFReader reports whether pdftotext is on this machine.
func HavePDFReader() bool {
	_, err := exec.LookPath("pdftotext")

	return err == nil
}

// plainText reads a text file, up to the cap.
func plainText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(bufio.NewReader(f), MostOfADocument))
	if err != nil {
		return "", err
	}

	/*
	 * Only if it is actually text.
	 *
	 * A .log or .csv can be anything, and a binary read as prose produces
	 * fragments that look like sentences and are not — which is worse than
	 * learning nothing, because it goes into memory looking like a fact.
	 */
	if !looksLikeText(raw) {
		return "", nil
	}

	return tidy(string(raw)), nil
}

/*
 * insideOffice pulls the text out of one entry of a zipped office document.
 *
 * docx, odt and their relatives are a zip archive containing XML. The text is
 * spread across a tree of formatting elements, so rather than model any of
 * that, the character data is taken and the tags dropped — which is exactly
 * what is wanted here and would be quite wrong for a converter.
 */
func insideOffice(path, entry string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}

	defer archive.Close()

	for _, file := range archive.File {
		if file.Name != entry {
			continue
		}

		inside, err := file.Open()
		if err != nil {
			return "", err
		}

		defer inside.Close()

		return charactersIn(inside)
	}

	// A document with no such entry is not an error: an empty spreadsheet has
	// no shared strings, and a .odp may be laid out differently.
	return "", nil
}

/*
 * insidePresentation reads every slide of a pptx.
 *
 * A presentation keeps each slide in its own part, so unlike the others there
 * is no single entry holding the text.
 */
func insidePresentation(path string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}

	defer archive.Close()

	var said strings.Builder

	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "ppt/slides/slide") ||
			!strings.HasSuffix(file.Name, ".xml") {
			continue
		}

		inside, err := file.Open()
		if err != nil {
			continue
		}

		text, err := charactersIn(inside)

		inside.Close()

		if err != nil || text == "" {
			continue
		}

		said.WriteString(text)
		said.WriteString("\n\n")

		if said.Len() > MostOfADocument {
			break
		}
	}

	return tidy(said.String()), nil
}

/*
 * charactersIn takes the character data out of an XML stream.
 *
 * Streamed rather than read whole, because the cap has to apply to the text
 * and not to the markup: a page of prose in a .docx can be a megabyte of XML,
 * and reading a fixed number of bytes of that would stop mid-document with
 * nothing to show for it.
 */
func charactersIn(from io.Reader) (string, error) {
	decoder := xml.NewDecoder(from)
	decoder.Strict = false

	var (
		out    strings.Builder
		spaced bool
	)

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}

			// What has been read so far is still worth having: a document that
			// is malformed halfway through is usually readable up to there.
			break
		}

		switch t := token.(type) {
		case xml.CharData:
			if len(t) == 0 {
				continue
			}

			out.Write(t)

			spaced = false

		case xml.EndElement:
			/*
			 * A break between blocks, so words do not run together.
			 *
			 * Without this a document comes back as
			 * "TheProjectPlanRevisedTuesday" and nothing downstream can make
			 * sentences out of it.
			 */
			if !spaced && isBlockEnd(t.Name.Local) {
				out.WriteString("\n")

				spaced = true
			}
		}

		if out.Len() > MostOfADocument {
			break
		}
	}

	return tidy(out.String()), nil
}

// isBlockEnd names the elements that end a line in the office formats.
func isBlockEnd(name string) bool {
	switch name {
	case "p", "tab", "br", "tr", "h", "title", "text-box", "si":
		return true
	}

	return false
}

/*
 * fromPDF asks pdftotext, which is the only thing here that needs a program.
 *
 * A PDF is a page description rather than a document — the words are placed
 * rather than written in order — and a parser for it is a project of its own.
 * Missing, this returns nothing rather than a guess, and the interface says so
 * once instead of silently learning nothing from every PDF on the machine.
 */
func fromPDF(ctx context.Context, path string) (string, error) {
	if !HavePDFReader() {
		return "", nil
	}

	// -layout keeps columns apart; a two-column paper read without it
	// interleaves the columns line by line into nonsense.
	out, err := exec.CommandContext(ctx, "pdftotext", "-layout", "-q",
		"-enc", "UTF-8", path, "-").Output()
	if err != nil {
		return "", fmt.Errorf("could not read %s: %w", filepath.Base(path), err)
	}

	if len(out) > MostOfADocument {
		out = out[:MostOfADocument]
	}

	return tidy(string(out)), nil
}

/*
 * looksLikeText decides whether a run of bytes is prose.
 *
 * Judged on what proportion is printable, and on there being no null bytes,
 * which no text file has and almost every binary does within its first page.
 */
func looksLikeText(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}

	if !utf8.Valid(raw) {
		return false
	}

	var printable int

	for _, r := range string(raw) {
		if r == 0 {
			return false
		}

		if unicode.IsPrint(r) || unicode.IsSpace(r) {
			printable++
		}
	}

	return float64(printable)/float64(utf8.RuneCount(raw)) > 0.9
}

/*
 * tidy collapses the whitespace a document extraction leaves behind.
 *
 * Office XML in particular produces runs of blank lines wherever formatting
 * elements ended, and everything downstream — the sentence splitter, the
 * prompts, the token count — is better off without them.
 */
func tidy(text string) string {
	var (
		out   strings.Builder
		blank int
	)

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t\r")

		if strings.TrimSpace(line) == "" {
			blank++

			// One blank line between paragraphs, never four.
			if blank > 1 {
				continue
			}

			out.WriteString("\n")

			continue
		}

		blank = 0

		out.WriteString(line)
		out.WriteString("\n")
	}

	return strings.TrimSpace(out.String())
}

/*
 * FactsPerDocument is how many lines are taken out of one document.
 *
 * Four. The point is to know what a document is about, not to hold a copy of
 * it: a brain that has swallowed every sentence of nine hundred documents has
 * a worse memory than one that knows what four of them said, because
 * everything it recalls is buried under everything else it recalls.
 */
const FactsPerDocument = 4

// LeastWorthKeeping is how short a line can be and still say something.
//
// Headings, page numbers, "Page 3 of 12", a date on its own: all true, none
// worth remembering, and all of them shorter than this.
const LeastWorthKeeping = 45

// MostWorthKeeping cuts a line that is really a paragraph. What comes back
// from a PDF is often a whole column on one line.
const MostWorthKeeping = 320

/*
 * FromDocumentContents turns what a document says into things to remember.
 *
 * Attributed rather than asserted. "The document cv.pdf says Petar led the
 * migration" is true whether or not he did; "Petar led the migration" is a
 * claim the brain has no business making on the strength of having read a file
 * somebody left on a disk. The distinction matters most for the documents
 * least likely to be the owner's own — a contract, a paper, somebody else's
 * report — and those are exactly the ones a listing cannot tell apart.
 */
func FromDocumentContents(d Document, text, owner string) Observations {
	var out Observations

	for _, line := range worthKeeping(text) {
		out = append(out, Observation{
			Content: fmt.Sprintf("%s, a %s document at %s, says: %s",
				d.Name, d.Kind, d.Path, line),
			Source: d.Source(),
		})

		if len(out) == FactsPerDocument {
			break
		}
	}

	return out
}

/*
 * worthKeeping picks the lines of a document that carry something.
 *
 * From the opening, because that is where anything written by a person says
 * what it is: the first paragraph of a report, the summary on a CV, the
 * problem statement in a proposal. Deeper in is detail that means nothing
 * without the rest of the document around it.
 */
func worthKeeping(text string) []string {
	var out []string

	for _, line := range strings.Split(text, "\n") {
		line = strings.Join(strings.Fields(line), " ")

		if len(line) < LeastWorthKeeping || len(line) > MostWorthKeeping {
			continue
		}

		// Mostly numbers and punctuation is a table row, a reference list, or
		// a page of figures — true, and unreadable out of context.
		if !mostlyWords(line) {
			continue
		}

		out = append(out, line)

		// A little beyond what is wanted, so the caller has something to
		// choose from if it ever wants to.
		if len(out) >= FactsPerDocument*2 {
			break
		}
	}

	return out
}

// mostlyWords reports whether a line is prose rather than a table row.
func mostlyWords(line string) bool {
	var letters int

	for _, r := range line {
		if unicode.IsLetter(r) {
			letters++
		}
	}

	return float64(letters)/float64(utf8.RuneCountInString(line)) > 0.6
}

// KindOf names a document's format the way the scanner does, so a document
// built from a path alone describes itself the same way.
func KindOf(path string) string {
	if kind, known := documentKinds[strings.ToLower(filepath.Ext(path))]; known {
		return kind
	}

	return "document"
}

// ReadingSource marks the contents of a document, as distinct from the fact
// that it exists.
//
// Two sources for one file, on purpose. A document listed last week is known
// under "document:" and would never be looked at again — so the nine hundred
// already on this machine would stay unread forever, and the reading would
// only ever apply to files that arrived after it was written. Under its own
// source, each of them comes up exactly once more.
func ReadingSource(path string) string { return "reading:" + path }

/*
 * ContentsWanted marks the documents whose insides are still to be read.
 *
 * Placeholders rather than the contents themselves: this runs during the scan,
 * which is meant to be names and nothing else, and opening nine hundred files
 * to find out they were all read last week is the cost the whole shape of this
 * exists to avoid. Only the extension is looked at here.
 */
func ContentsWanted(docs []Document) Observations {
	var out Observations

	for _, d := range docs {
		if !CanRead(d.Path) {
			continue
		}

		out = append(out, Observation{Source: ReadingSource(d.Path)})
	}

	return out
}

// WantsReading reports whether an observation is one of those placeholders,
// and the file it stands for.
func WantsReading(o Observation) (string, bool) {
	if o.Content != "" {
		return "", false
	}

	return strings.CutPrefix(o.Source, "reading:")
}

/*
 * NothingInside records that a document was opened and had no text in it.
 *
 * Worth storing rather than silently skipping, for two reasons. It stops the
 * file being opened again on every pass forever, which is what dropping it
 * would mean. And it is the answer to a real question — "why do you not know
 * what is in my CV" — where the answer is that the CV is a photograph of a
 * page and nothing here can read it.
 */
func NothingInside(path, owner, why string) Observation {
	if owner == "" {
		owner = "The owner"
	}

	return Observation{
		Content: fmt.Sprintf("%s has a %s at %s and there is nothing in it worth "+
			"remembering — %s.", owner, KindOf(path), path, why),
		Source: ReadingSource(path),
	}
}

// NoText and NoProse are the two reasons a document that opened fine still
// taught the brain nothing.
const (
	NoText  = "it holds no readable text, most likely a scan or an image"
	NoProse = "what it holds is headings, figures or fragments rather than sentences"
)

/*
 * withContents opens each document and adds what it says to what is known
 * about it.
 *
 * The listing stays: "there is a PDF called this, here, changed then" is worth
 * knowing on its own and is the only thing available for a format nothing can
 * open. What is added is a few lines of what it actually says.
 *
 * A document that cannot be read, or that turns out to be a binary with a
 * misleading name, simply contributes nothing further — never an error, since
 * a folder of photographs is not a fault.
 */
func ReadContents(ctx context.Context, observations Observations, owner string) Observations {
	out := make(Observations, 0, len(observations))

	for _, o := range observations {
		path, wants := WantsReading(o)

		if !wants {
			out = append(out, o)

			continue
		}

		if err := ctx.Err(); err != nil {
			return out
		}

		/*
		 * The placeholder is replaced by what the document says, or dropped.
		 *
		 * Dropped rather than recorded empty, so a file that could not be
		 * opened this time — a drive being unplugged mid-pass, a document
		 * somebody has open and locked — comes round again rather than being
		 * marked read forever on the strength of one failure.
		 */
		text, err := TextOf(ctx, path)

		/*
		 * An error means try again; empty means there was nothing in it.
		 *
		 * The two look the same and are not. A file that would not open — a
		 * drive unplugged mid-pass, a document somebody has locked — comes
		 * round again. A file that opened and held no text is a scan, or a
		 * spreadsheet of numbers, and asking again next week would mean
		 * opening it forever.
		 */
		if err != nil {
			continue
		}

		if text == "" {
			out = append(out, NothingInside(path, owner, NoText))

			continue
		}

		said := FromDocumentContents(Document{
			Name: filepath.Base(path),
			Path: path,
			Kind: KindOf(path),
		}, text, owner)

		/*
		 * Opened, read, and nothing in it worth keeping.
		 *
		 * A page of headings, a spreadsheet of figures, a two-line note. That
		 * has to be recorded as well, or the file is opened again on every
		 * pass for the rest of the machine's life.
		 */
		if len(said) == 0 {
			out = append(out, NothingInside(path, owner, NoProse))

			continue
		}

		for i := range said {
			// Filed under the reading rather than the listing, so this
			// document is not opened again next week.
			said[i].Source = ReadingSource(path)
		}

		out = append(out, said...)
	}

	return out
}
