package tools

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func zipOf(t *testing.T, path string, files map[string]string) {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	for name, body := range files {
		out, _ := w.Create(name)
		_, _ = io.WriteString(out, body)
	}

	_ = w.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func part(t *testing.T, path, name string) string {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	for _, f := range r.File {
		if f.Name == name {
			rc, _ := f.Open()
			raw, _ := io.ReadAll(rc)
			rc.Close()

			return string(raw)
		}
	}

	t.Fatalf("%s has no %s", path, name)

	return ""
}

/*
 * A table written as a spreadsheet comes back as the same rows, its numbers
 * stored as numbers (a decimal comma too), its header bold and frozen; a
 * postcode with a leading zero and an ambiguous "1,250" stay text.
 */
func TestASpreadsheetIsWrittenAndReadBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget.xlsx")

	table := "| Item | Amount | Code |\n|---|---:|---|\n| Rent | 1 200,50 | 0042 |\n| Food | 350 | 1,250 |"

	if _, err := call(t, WriteDocument{}, map[string]any{"path": path, "title": "Budget: 2026/27", "text": table}); err != nil {
		t.Fatal(err)
	}

	sheet := part(t, path, "xl/worksheets/sheet1.xml")

	for _, want := range []string{`<c r="B2"><v>1200.50</v></c>`, `<c r="B3"><v>350</v></c>`,
		`<t xml:space="preserve">0042</t>`, `<t xml:space="preserve">1,250</t>`, `s="1"`, `state="frozen"`} {
		if !strings.Contains(sheet, want) {
			t.Errorf("the sheet lacks %s:\n%s", want, sheet)
		}
	}

	if wb := part(t, path, "xl/workbook.xml"); !strings.Contains(wb, `name="Budget  2026 27"`) {
		t.Errorf("the sheet name was not made safe: %s", wb)
	}

	out, err := call(t, ReadDocument{}, map[string]any{"path": path})
	if err != nil {
		t.Fatal(err)
	}

	if want := "Item\tAmount\tCode\nRent\t1200.50\t0042\nFood\t350\t1,250"; !strings.Contains(out, want) {
		t.Errorf("read back as\n%q\nwant\n%q", out, want)
	}

	for _, text := range []string{"a\tb\n1\t2", "a;b\n1;2", "a,b\n1,2"} {
		rows := tableOf(text)
		if len(rows) != 2 || len(rows[0]) != 2 || rows[1][1] != "2" {
			t.Errorf("%q read as %q", text, rows)
		}
	}
}

// A workbook written elsewhere, with shared strings and two sheets, is read
// with its numbers where they belong.
func TestAWorkbookWithSharedStringsKeepsItsNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "two.xlsx")

	zipOf(t, path, map[string]string{
		"xl/workbook.xml":      `<workbook><sheets><sheet name="Приходи"/><sheet name="Разходи"/></sheets></workbook>`,
		"xl/sharedStrings.xml": `<sst><si><t>Месец</t></si><si><t>Сума</t></si><si><r><t>Ян</t></r><r><t>уари</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>` +
			`<row r="2"><c r="A2" t="s"><v>2</v></c><c r="C2"><v>1500</v></c></row>` +
			`</sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Наем</t></is></c><c r="B1"><v>-300.5</v></c></row></sheetData></worksheet>`,
	})

	out, err := call(t, ReadDocument{}, map[string]any{"path": path})
	if err != nil {
		t.Fatal(err)
	}

	want := "## Приходи\nМесец\tСума\nЯнуари\t\t1500\n\n## Разходи\nНаем\t-300.5"
	if out != want {
		t.Errorf("got\n%q\nwant\n%q", out, want)
	}
}

func TestOpenDocumentIsReadWithoutAnOfficeSuite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.odt")

	zipOf(t, path, map[string]string{
		"mimetype":    "application/vnd.oasis.opendocument.text",
		"content.xml": `<office:document-content><office:body><office:text><text:h>Заглавие</text:h><text:p>Две<text:s/>думи</text:p></office:text></office:body></office:document-content>`,
	})

	t.Setenv("PATH", t.TempDir()) // no LibreOffice to fall back on

	out, err := call(t, ReadDocument{}, map[string]any{"path": path})
	if err != nil || out != "Заглавие\nДве думи" {
		t.Errorf("%q %v", out, err)
	}
}

// fakeProgram puts a shell script called name on PATH.
func fakeProgram(t *testing.T, dir, name, script string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestAScanIsReadByTesseract(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin+":/usr/bin:/bin")

	fakeProgram(t, bin, "tesseract", `if [ "$1" = "--list-langs" ]; then printf 'List of available languages (3):\neng\nosd\nbul\n'; exit 0; fi
echo "Договор № 12 от 3 май — langs $4"`)

	picture := filepath.Join(t.TempDir(), "scan.png")
	_ = os.WriteFile(picture, []byte("\x89PNG"), 0o600)

	out, err := call(t, ReadDocument{}, map[string]any{"path": picture})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Договор № 12 от 3 май — langs bul+eng") || !strings.Contains(out, "Tesseract (bul+eng)") ||
		!strings.Contains(out, "check names, numbers") {
		t.Errorf("%s", out)
	}
}

func TestAScannedPDFIsReadPageByPage(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin+":/usr/bin:/bin")

	fakeProgram(t, bin, "pdftotext", "echo ' 1 '\n")
	fakeProgram(t, bin, "pdftoppm", `for last; do :; done; printf x > "$last-1.png"; printf x > "$last-2.png"`)
	fakeProgram(t, bin, "tesseract", `if [ "$1" = "--list-langs" ]; then printf 'eng\n'; exit 0; fi
echo "text of $(basename $1)"`)

	pdf := filepath.Join(t.TempDir(), "scan.pdf")
	_ = os.WriteFile(pdf, []byte("%PDF-1.4"), 0o600)

	out, err := call(t, ReadDocument{}, map[string]any{"path": pdf})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"2 scanned pages", "--- page 1 ---\ntext of page-1.png", "--- page 2 ---\ntext of page-2.png"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestWithoutTesseractALocalModelThatCanSeeReadsIt(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var asked string

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2:3b"},{"name":"gemma3:4b"}]}`))
		case "/api/generate":
			var body struct {
				Model  string   `json:"model"`
				Prompt string   `json:"prompt"`
				Images []string `json:"images"`
			}

			_ = json.NewDecoder(r.Body).Decode(&body)
			asked = body.Model + "|" + body.Prompt

			_, _ = w.Write([]byte(`{"response":"Касова бележка\nОбщо: 12,40 лв."}`))
		}
	}))
	defer ollama.Close()

	picture := filepath.Join(t.TempDir(), "receipt.jpg")
	_ = os.WriteFile(picture, []byte("\xff\xd8"), 0o600)

	out, err := call(t, ReadDocument{OllamaURL: ollama.URL}, map[string]any{"path": picture})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Общо: 12,40 лв.") || !strings.Contains(out, "by gemma3:4b") ||
		!strings.HasPrefix(asked, "gemma3:4b|Transcribe all the text") {
		t.Errorf("%s\nasked %s", out, asked)
	}

	tiff := filepath.Join(t.TempDir(), "scan.tif")
	_ = os.WriteFile(tiff, []byte("II*"), 0o600)

	if _, err := call(t, ReadDocument{OllamaURL: ollama.URL}, map[string]any{"path": tiff}); !errors.Is(err, ErrNoReader) {
		t.Errorf("a TIFF went to a model that cannot take one: %v", err)
	}
}

func TestWithNothingToReadScansItSaysWhatToInstall(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	picture := filepath.Join(t.TempDir(), "scan.png")
	_ = os.WriteFile(picture, []byte("\x89PNG"), 0o600)

	_, err := call(t, ReadDocument{}, map[string]any{"path": picture})
	if !errors.Is(err, ErrNoReader) || !strings.Contains(err.Error(), "tesseract-ocr-bul") {
		t.Errorf("%v", err)
	}
}
