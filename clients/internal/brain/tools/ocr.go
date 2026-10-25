package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/exe"
)

/*
 * Reading the text in a scan or a photo.
 *
 * Half the paper in a household arrives as a picture: a scanned contract, a
 * photo of a receipt, a PDF from a copier with no text in it at all. read_
 * document used to answer those with "it may be scanned pictures", which is
 * the problem restated.
 *
 * Two ways to read one, both on this machine. Tesseract first: it is made for
 * this, fast, and reads Bulgarian and English when its language data is
 * installed. A local model that can see second, for a machine without
 * Tesseract or a picture it cannot make out. Nothing is sent anywhere either
 * way. When neither is there, the error says what to install, by name.
 *
 * A scanned PDF is turned into page pictures with pdftoppm first, page by
 * page, at most MaxScanPages of them.
 */

// MaxScanPages is the most pages of a scanned PDF read in one go.
const MaxScanPages = 30

// pictureKinds are the picture files read by recognising their text.
var pictureKinds = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".tif": true, ".tiff": true,
	".bmp": true, ".webp": true, ".gif": true, ".pnm": true,
}

// seeable are the pictures a vision model is given; the others need
// Tesseract.
var seeable = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

// ErrNoReader is a scan with nothing on this machine to read it.
var ErrNoReader = errors.New("nothing on this machine can read text in pictures: install Tesseract " +
	"(install_requirement tesseract — on Linux the packages tesseract-ocr and tesseract-ocr-bul), " +
	"or a local model that can see (install_a_model gemma3:4b)")

// recognise reads the text in one picture: Tesseract, then a model that can
// see. It says which one read it.
func (t ReadDocument) recognise(ctx context.Context, picture string) (string, string, error) {
	var problems []string

	if tool, found := exe.Look("tesseract"); found {
		langs := tesseractLanguages(ctx, tool)

		out, err := exec.CommandContext(ctx, tool, picture, "stdout", "-l", langs, "--psm", "3").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return string(out), "Tesseract (" + langs + ")", nil
		}

		if err != nil {
			problems = append(problems, "Tesseract: "+err.Error())
		}
	}

	if t.OllamaURL != "" && seeable[strings.ToLower(filepath.Ext(picture))] {
		eyes := LookAtScreen{OllamaURL: t.OllamaURL}

		model, err := eyes.visionModel(ctx)
		if err == nil {
			image, readErr := os.ReadFile(picture)
			if readErr != nil {
				return "", "", readErr
			}

			said, askErr := eyes.ask(ctx, model, transcribe, image)
			if askErr == nil {
				said = strings.TrimSpace(said)
				if strings.EqualFold(said, "NO TEXT") {
					said = ""
				}

				return said, model, nil
			}

			problems = append(problems, model+": "+askErr.Error())
		}
	}

	if len(problems) > 0 {
		return "", "", errors.New("the text could not be read: " + strings.Join(problems, "; "))
	}

	return "", "", ErrNoReader
}

const transcribe = "Transcribe all the text in this image exactly as it is written, in its original " +
	"language and alphabet, keeping the line breaks. Do not describe the image, translate, " +
	"summarise or correct anything. If there is no text, reply NO TEXT."

/*
 * tesseractLanguages is the languages to read with: Bulgarian and English
 * where their data is installed, with whatever else is there after them.
 * "osd" is not a language and is left out.
 */
func tesseractLanguages(ctx context.Context, tool string) string {
	out, err := exec.CommandContext(ctx, tool, "--list-langs").CombinedOutput()
	if err != nil {
		return "eng"
	}

	have := map[string]bool{}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, " ") || line == "osd" {
			continue
		}

		have[line] = true
	}

	var langs []string

	for _, first := range []string{"bul", "eng"} {
		if have[first] {
			langs = append(langs, first)
			delete(have, first)
		}
	}

	if len(langs) == 0 {
		rest := make([]string, 0, len(have))
		for l := range have {
			rest = append(rest, l)
		}

		sort.Strings(rest)

		if len(rest) > 2 {
			rest = rest[:2]
		}

		langs = rest
	}

	if len(langs) == 0 {
		return "eng"
	}

	return strings.Join(langs, "+")
}

// scanned reads a picture file, with a line saying how.
func (t ReadDocument) scanned(ctx context.Context, path string) (string, error) {
	text, by, err := t.recognise(ctx, path)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no text could be found in %s", filepath.Base(path))
	}

	return scanNote(by, 1) + text, nil
}

func scanNote(by string, pages int) string {
	what := "picture"

	switch {
	case pages == 1:
	case pages > 1:
		what = fmt.Sprintf("%d scanned pages", pages)
	default:
		what = "scanned page"
	}

	return fmt.Sprintf("[Read from a %s by %s. Recognised text can be wrong — check names, numbers "+
		"and amounts against the original.]\n\n", what, by)
}

/*
 * scannedPDF reads a PDF with no text layer by turning its pages into
 * pictures and reading each one.
 */
func (t ReadDocument) scannedPDF(ctx context.Context, path string) (string, error) {
	tool, found := exe.Look("pdftoppm")
	if !found {
		return "", fmt.Errorf("%s has no text in it, only pictures of pages; reading those needs "+
			"pdftoppm (on Linux the package poppler-utils) as well as a text reader", filepath.Base(path))
	}

	dir, err := tempDir("pn-scripts-assistant-scan")
	if err != nil {
		return "", err
	}

	defer removeAll(dir)

	cmd := exec.CommandContext(ctx, tool, "-r", "200", "-png", "-l", fmt.Sprint(MaxScanPages), path, filepath.Join(dir, "page"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("could not turn the pages of %s into pictures: %v %s", filepath.Base(path), err, strings.TrimSpace(string(out)))
	}

	pages, _ := filepath.Glob(filepath.Join(dir, "page*.png"))
	sort.Strings(pages)

	if len(pages) == 0 {
		return "", fmt.Errorf("%s has no pages that could be turned into pictures", filepath.Base(path))
	}

	var (
		b        strings.Builder
		by       string
		readSome bool
	)

	for i, page := range pages {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		text, engine, err := t.recognise(ctx, page)
		if err != nil {
			return "", err
		}

		by = engine
		readSome = readSome || hasLetterOrDigit(text)

		fmt.Fprintf(&b, "--- page %d ---\n%s\n", i+1, strings.TrimSpace(text))
	}

	if !readSome {
		return "", fmt.Errorf("no text could be found on the pages of %s", filepath.Base(path))
	}

	n := len(pages)
	if n == 1 {
		n = 0 // one scanned page, not a picture
	}

	return scanNote(by, n) + b.String(), nil
}

func hasLetterOrDigit(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}

	return false
}

// letters counts the letters in s.
func letters(s string) int {
	n := 0

	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}

	return n
}
