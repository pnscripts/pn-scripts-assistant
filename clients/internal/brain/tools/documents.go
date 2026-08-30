package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"pn-brain/internal/brain/exe"
	"strings"
)

/*
 * Reading the documents people actually have.
 *
 * read_file handles text and gives back mush for anything else: a .docx opened
 * as text is a zip header followed by compressed bytes, and the model dutifully
 * tries to make sense of it. Most of what somebody wants read to them is a PDF,
 * a Word file or a spreadsheet.
 *
 * Word and spreadsheet files are zip archives full of XML, so they are read
 * here directly with the standard library and no dependency at all. PDF is a
 * real format with a real parser behind it, so that one is handed to pdftotext
 * — and when it is not installed, the tool says exactly that instead of
 * returning something that looks like a failure of comprehension.
 */

// ReadDocument extracts the text of a document.
type ReadDocument struct{}

func (ReadDocument) Name() string { return "read_document" }

func (ReadDocument) Description() string {
	return "Read the text of a PDF, Word document (.docx), spreadsheet (.xlsx) " +
		"or presentation. Use this rather than read_file for anything that is " +
		"not plain text."
}

func (ReadDocument) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the document"}
		},
		"required": ["path"]
	}`)
}

func (ReadDocument) Risk() Risk { return Safe }

func (ReadDocument) Summarize(raw json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
	}

	_ = json.Unmarshal(raw, &a)

	return "Read the document " + a.Path
}

func (ReadDocument) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := GuardSensitive(a.Path); err != nil {
		return "", err
	}

	if !filepath.IsAbs(a.Path) {
		return "", fmt.Errorf("give an absolute path, not %q", a.Path)
	}

	var (
		text string
		err  error
	)

	switch strings.ToLower(filepath.Ext(a.Path)) {
	case ".pdf":
		text, err = pdfText(ctx, a.Path)
	case ".docx":
		text, err = officeText(a.Path, "word/document.xml")
	case ".xlsx":
		text, err = officeText(a.Path, "xl/sharedStrings.xml")
	case ".pptx":
		text, err = officeText(a.Path, "ppt/slides/")
	case ".odt", ".ods", ".odp", ".doc", ".xls", ".ppt", ".rtf":
		text, err = convertedText(ctx, a.Path)
	default:
		return "", fmt.Errorf(
			"%s is not a document format this reads; for plain text use read_file",
			filepath.Ext(a.Path))
	}

	if err != nil {
		return "", err
	}

	text = strings.TrimSpace(collapse(text))

	if text == "" {
		return "", fmt.Errorf("%s has no text in it that could be read; it may be "+
			"scanned pictures rather than text", a.Path)
	}

	if len(text) > MaxReadBytes {
		text = text[:MaxReadBytes] + "\n\n[truncated]"
	}

	return text, nil
}

// pdfText hands the file to pdftotext.
func pdfText(ctx context.Context, path string) (string, error) {
	tool, found := exe.Look("pdftotext")
	if !found {
		return convertedText(ctx, path)
	}

	// -layout keeps columns and tables roughly where they were, which is the
	// difference between a readable table and its cells in one long line.
	out, err := exec.CommandContext(ctx, tool, "-layout", path, "-").Output()
	if err != nil {
		return "", fmt.Errorf("could not read the PDF %s: %w", path, err)
	}

	return string(out), nil
}

/*
 * convertedText is the fallback for the older formats.
 *
 * LibreOffice can read all of them, and is on most desktops already. It is
 * slow — seconds, and it wants a writable profile directory — so it is only
 * reached for the formats nothing else here handles.
 */
func convertedText(ctx context.Context, path string) (string, error) {
	tool, found := exe.Look("soffice", "libreoffice")
	if !found {
		return "", fmt.Errorf(
			"reading %s needs pdftotext or libreoffice, and neither is installed",
			filepath.Base(path))
	}

	dir, err := tempDir("pn-brain-doc")
	if err != nil {
		return "", err
	}

	defer removeAll(dir)

	cmd := exec.CommandContext(ctx, tool,
		"--headless", "-env:UserInstallation=file://"+dir,
		"--convert-to", "txt:Text", "--outdir", dir, path)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not convert %s: %w", filepath.Base(path), err)
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".txt"

	body, err := readFileAt(filepath.Join(dir, name))
	if err != nil {
		return "", fmt.Errorf("converted %s but could not read the result: %w",
			filepath.Base(path), err)
	}

	return body, nil
}

/*
 * officeText reads the XML inside a Word, Excel or PowerPoint file.
 *
 * These are zip archives, so the standard library is the whole implementation.
 * Only the character data is kept: the markup around it describes fonts and
 * revision history, which is not what anybody asked to have read to them.
 */
func officeText(path, want string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", path, err)
	}

	defer archive.Close()

	var b strings.Builder

	for _, f := range archive.File {
		if !strings.HasPrefix(f.Name, want) {
			continue
		}

		body, err := f.Open()
		if err != nil {
			continue
		}

		text, err := charData(body)

		body.Close()

		if err != nil {
			continue
		}

		b.WriteString(text)
		b.WriteString("\n")
	}

	// A spreadsheet with no shared strings is one whose cells are all numbers
	// or all inline; the sheets themselves still have them.
	if strings.TrimSpace(b.String()) == "" && strings.HasPrefix(want, "xl/") {
		return officeText(path, "xl/worksheets/")
	}

	return b.String(), nil
}

// charData pulls the text out of an XML document, ignoring every tag.
func charData(r io.Reader) (string, error) {
	decoder := xml.NewDecoder(r)

	var b strings.Builder

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return b.String(), nil
		}

		if err != nil {
			return b.String(), err
		}

		switch t := token.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			// A paragraph or row break is the only structure worth keeping;
			// without it every sentence in the file runs into the next.
			switch t.Name.Local {
			case "p", "br", "tr", "row":
				b.WriteString("\n")
			case "tab", "tc", "c":
				b.WriteString("\t")
			}
		}
	}
}

// collapse tidies the blank lines these formats leave behind.
func collapse(text string) string {
	lines := strings.Split(text, "\n")

	out := make([]string, 0, len(lines))

	blank := 0

	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")

		if strings.TrimSpace(line) == "" {
			blank++

			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}
