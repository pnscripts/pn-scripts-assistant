package tools

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

/*
 * Writing a spreadsheet.
 *
 * "Put that in a spreadsheet" is as ordinary as "put that in a Word file",
 * and the answer was a .docx with a table pasted in as text. An .xlsx is a zip
 * of a handful of XML files, like a .docx; written here, it opens in Excel,
 * LibreOffice and Google Sheets with nothing installed.
 *
 * The rows come as text in whatever shape a model or a person writes a table:
 * a Markdown table, tab-separated lines, or CSV with commas or semicolons.
 * Numbers become numbers (a decimal comma too, as Bulgarian writes them), so
 * a column can be summed; the first row is the header, in bold and frozen.
 */

// tableOf reads rows of cells out of text.
func tableOf(text string) [][]string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n"), "\n")

	pipes, tabs := 0, 0

	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "|") && strings.HasSuffix(t, "|") {
			pipes++
		}

		if strings.Contains(l, "\t") {
			tabs++
		}
	}

	var rows [][]string

	switch {
	case pipes*2 >= len(lines):
		for _, l := range lines {
			t := strings.TrimSpace(l)
			if !strings.HasPrefix(t, "|") {
				continue
			}

			if markdownRule.MatchString(t) {
				continue
			}

			cells := strings.Split(strings.Trim(t, "|"), "|")
			for i := range cells {
				cells[i] = strings.TrimSpace(cells[i])
			}

			rows = append(rows, cells)
		}
	case tabs*2 >= len(lines):
		for _, l := range lines {
			rows = append(rows, strings.Split(l, "\t"))
		}
	default:
		sep := ','
		if strings.Count(text, ";") > strings.Count(text, ",") {
			sep = ';'
		}

		r := csv.NewReader(strings.NewReader(strings.Join(lines, "\n")))
		r.Comma = sep
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		r.TrimLeadingSpace = true

		read, err := r.ReadAll()
		if err != nil {
			for _, l := range lines {
				rows = append(rows, []string{l})
			}
		} else {
			rows = read
		}
	}

	out := rows[:0]

	for _, row := range rows {
		empty := true

		for _, c := range row {
			if strings.TrimSpace(c) != "" {
				empty = false
			}
		}

		if !empty {
			out = append(out, row)
		}
	}

	return out
}

var (
	markdownRule   = regexp.MustCompile(`^\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?$`)
	ambiguousComma = regexp.MustCompile(`^-?\d{1,3},\d{3}$`)
	numberCell     = regexp.MustCompile(`^-?(\d{1,3}([ \x{00a0}]\d{3})+|\d+)([.,]\d+)?$`)
)

// asNumber is the cell as a number, if it is one: "1 250,50" and "1250.5"
// both are. A leading zero ("007", a postcode) stays text.
func asNumber(cell string) (string, bool) {
	c := strings.TrimSpace(cell)
	if !numberCell.MatchString(c) {
		return "", false
	}

	// "1,250" is a thousand and a quarter in English and one and a quarter
	// in Bulgarian. A wrong number is worse than a number written as text,
	// so that one is left as text.
	if ambiguousComma.MatchString(c) {
		return "", false
	}

	digits := strings.TrimPrefix(c, "-")
	if len(digits) > 1 && digits[0] == '0' && digits[1] != '.' && digits[1] != ',' {
		return "", false
	}

	c = strings.NewReplacer(" ", "", " ", "").Replace(c)
	c = strings.Replace(c, ",", ".", 1)

	if _, err := strconv.ParseFloat(c, 64); err != nil {
		return "", false
	}

	return c, true
}

func columnName(i int) string {
	name := ""

	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}

	return name
}

var badSheetName = regexp.MustCompile(`[\[\]:*?/\\]`)

func sheetName(title string) string {
	name := strings.TrimSpace(badSheetName.ReplaceAllString(title, " "))
	if name == "" {
		return "Sheet1"
	}

	if utf8.RuneCountInString(name) > 31 {
		name = string([]rune(name)[:31])
	}

	return name
}

// buildXlsx writes the rows in text as a one-sheet workbook.
func buildXlsx(title, text string) ([]byte, error) {
	rows := tableOf(text)
	if len(rows) == 0 {
		return nil, fmt.Errorf("there are no rows to put in the spreadsheet")
	}

	widest := 0
	widths := map[int]int{}

	for _, row := range rows {
		if len(row) > widest {
			widest = len(row)
		}

		for i, c := range row {
			if n := utf8.RuneCountInString(c); n > widths[i] {
				widths[i] = n
			}
		}
	}

	var sheet strings.Builder

	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)

	if len(rows) > 1 {
		sheet.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	}

	sheet.WriteString(`<cols>`)

	for i := 0; i < widest; i++ {
		w := widths[i] + 2
		if w < 8 {
			w = 8
		}

		if w > 60 {
			w = 60
		}

		fmt.Fprintf(&sheet, `<col min="%d" max="%d" width="%d" customWidth="1"/>`, i+1, i+1, w)
	}

	sheet.WriteString(`</cols><sheetData>`)

	for r, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, r+1)

		for c, cell := range row {
			ref := columnName(c) + strconv.Itoa(r+1)

			style := ""
			if r == 0 && len(rows) > 1 {
				style = ` s="1"`
			}

			if n, ok := asNumber(cell); ok && (r > 0 || len(rows) == 1) {
				fmt.Fprintf(&sheet, `<c r="%s"%s><v>%s</v></c>`, ref, style, n)

				continue
			}

			if strings.TrimSpace(cell) == "" {
				continue
			}

			fmt.Fprintf(&sheet, `<c r="%s" t="inlineStr"%s><is><t xml:space="preserve">%s</t></is></c>`,
				ref, style, escapeXML(strings.TrimSpace(cell)))
		}

		sheet.WriteString(`</row>`)
	}

	sheet.WriteString(`</sheetData></worksheet>`)

	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
			`</Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
			`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="` + escapeXML(sheetName(title)) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
			`</Relationships>`},
		{"xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
			`<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
			`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
			`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
			`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
			`<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
			`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>` +
			`</styleSheet>`},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	}

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	for _, f := range files {
		out, err := w.Create(f.name)
		if err != nil {
			return nil, err
		}

		if _, err := out.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

/*
 * spreadsheetText reads a workbook as rows: each sheet under its name, each
 * row a line, its cells separated by tabs, shared strings put back where
 * they are used and numbers kept. Reading the shared strings alone, as this
 * used to, gave every word in the workbook and none of its numbers — a
 * budget with its figures taken out.
 */
func spreadsheetText(path string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", path, err)
	}

	defer archive.Close()

	files := map[string]*zip.File{}
	for _, f := range archive.File {
		files[f.Name] = f
	}

	var shared []string

	if f, ok := files["xl/sharedStrings.xml"]; ok {
		shared, err = sharedStrings(f)
		if err != nil {
			return "", err
		}
	}

	names := sheetNames(files)

	var sheets []string

	for name := range files {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
			sheets = append(sheets, name)
		}
	}

	sort.Slice(sheets, func(i, j int) bool { return sheetNumber(sheets[i]) < sheetNumber(sheets[j]) })

	var b strings.Builder

	for i, name := range sheets {
		title := fmt.Sprintf("Sheet %d", i+1)
		if i < len(names) {
			title = names[i]
		}

		if len(sheets) > 1 {
			fmt.Fprintf(&b, "## %s\n", title)
		}

		if err := sheetRows(files[name], shared, &b); err != nil {
			return "", err
		}

		b.WriteString("\n")
	}

	return b.String(), nil
}

func sheetNumber(name string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "xl/worksheets/sheet"), ".xml"))

	return n
}

func openPart(f *zip.File) (*xml.Decoder, func(), error) {
	r, err := f.Open()
	if err != nil {
		return nil, nil, err
	}

	return xml.NewDecoder(io.LimitReader(r, 64<<20)), func() { r.Close() }, nil
}

func sharedStrings(f *zip.File) ([]string, error) {
	d, done, err := openPart(f)
	if err != nil {
		return nil, err
	}

	defer done()

	var (
		out  []string
		cur  strings.Builder
		inSI bool
		inT  bool
	)

	for {
		tok, err := d.Token()
		if err == io.EOF {
			return out, nil
		}

		if err != nil {
			return out, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSI = true
				cur.Reset()
			case "t":
				inT = inSI
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				out = append(out, cur.String())
				inSI = false
			case "t":
				inT = false
			}
		case xml.CharData:
			if inT {
				cur.Write(t)
			}
		}
	}
}

// sheetNames are the sheets' names in the workbook's order.
func sheetNames(files map[string]*zip.File) []string {
	f, ok := files["xl/workbook.xml"]
	if !ok {
		return nil
	}

	d, done, err := openPart(f)
	if err != nil {
		return nil
	}

	defer done()

	var out []string

	for {
		tok, err := d.Token()
		if err != nil {
			return out
		}

		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "sheet" {
			for _, a := range start.Attr {
				if a.Name.Local == "name" {
					out = append(out, a.Value)
				}
			}
		}
	}
}

func sheetRows(f *zip.File, shared []string, b *strings.Builder) error {
	d, done, err := openPart(f)
	if err != nil {
		return err
	}

	defer done()

	var (
		row      []string
		kind     string
		col      int
		value    strings.Builder
		inValue  bool
		inInline bool
	)

	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				row = row[:0]
			case "c":
				kind, col = "", len(row)

				for _, a := range t.Attr {
					switch a.Name.Local {
					case "t":
						kind = a.Value
					case "r":
						col = cellColumn(a.Value)
					}
				}

				value.Reset()
			case "v":
				inValue = true
			case "t":
				inInline = kind == "inlineStr"
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inValue = false
			case "t":
				inInline = false
			case "c":
				text := value.String()

				if kind == "s" {
					if n, err := strconv.Atoi(text); err == nil && n >= 0 && n < len(shared) {
						text = shared[n]
					}
				}

				for len(row) < col {
					row = append(row, "")
				}

				row = append(row, strings.ReplaceAll(strings.ReplaceAll(text, "\t", " "), "\n", " "))
			case "row":
				line := strings.TrimRight(strings.Join(row, "\t"), "\t")
				if line != "" {
					b.WriteString(line)
					b.WriteString("\n")
				}
			}
		case xml.CharData:
			if inValue || inInline {
				value.Write(t)
			}
		}
	}
}

// cellColumn is the zero-based column of a reference like "C7".
func cellColumn(ref string) int {
	n := 0

	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}

		n = n*26 + int(r-'A'+1)
	}

	return n - 1
}
