package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/sheet"
)

/*
 * Calculating with somebody's table and checking every total (package
 * sheet). The model never calculates: asked a question, it writes a recipe
 * from the column names and a few rows, and the sheet package carries it
 * out exactly and checks it a second way. Reading a table and calculating
 * change nothing; a result is written to a new file only from the Tables
 * page, by its owner's press.
 */

// MaxTableRows is how many rows of one sheet are read.
const MaxTableRows = 100000

// ReadTables reads the sheets of a CSV, TSV, XLSX or ODS file.
func ReadTables(ctx context.Context, path string) ([]sheet.Table, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("give the full path of the table, not %q", path)
	}

	if err := GuardSensitive(ctx, path); err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("there is no file at %s", path)
	}

	if info.Size() > 50<<20 {
		return nil, errors.New("the file is larger than 50 MB")
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv", ".tsv", ".txt":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		t, err := sheet.FromRows(name, tableOf(strings.TrimPrefix(string(data), "\ufeff")))
		if err != nil {
			return nil, err
		}

		return []sheet.Table{capRows(t)}, nil
	case ".xlsx":
		text, err := spreadsheetText(path)
		if err != nil {
			return nil, err
		}

		return tablesOfText(name, text)
	case ".ods":
		return odsTables(path)
	}

	return nil, fmt.Errorf("%s is not a table this reads: give a .csv, .tsv, .xlsx or .ods file", filepath.Ext(path))
}

func capRows(t sheet.Table) sheet.Table {
	if len(t.Rows) > MaxTableRows {
		t.Rows = t.Rows[:MaxTableRows]
	}

	return t
}

// tablesOfText splits spreadsheetText's output — "## Sheet" headings over
// tab-separated rows — into tables.
func tablesOfText(name, text string) ([]sheet.Table, error) {
	var (
		out   []sheet.Table
		title = name
		rows  [][]string
	)

	flush := func() {
		if t, err := sheet.FromRows(title, rows); err == nil {
			out = append(out, capRows(t))
		}

		rows = nil
	}

	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()

			title = strings.TrimSpace(line[3:])

			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		rows = append(rows, strings.Split(line, "\t"))
	}

	flush()

	if len(out) == 0 {
		return nil, errors.New("the workbook has no rows")
	}

	return out, nil
}

/*
 * odsTables reads an OpenDocument spreadsheet's tables as rows: each cell
 * its value (the number a number cell holds, not how it is shown), a
 * repeated cell or row as many times as it says — but the run of empty ones
 * a sheet ends with is not a million rows.
 */
func odsTables(path string) ([]sheet.Table, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}

	defer archive.Close()

	var content *zip.File

	for _, f := range archive.File {
		if f.Name == "content.xml" {
			content = f
		}
	}

	if content == nil {
		return nil, errors.New("this is not an OpenDocument spreadsheet")
	}

	r, err := content.Open()
	if err != nil {
		return nil, err
	}

	defer r.Close()

	dec := xml.NewDecoder(io.LimitReader(r, 200<<20))

	var (
		out      []sheet.Table
		title    string
		rows     [][]string
		row      []string
		rowTimes int
		cell     strings.Builder
		value    string
		repeat   int
		inCell   bool
	)

	attr := func(e xml.StartElement, local string) string {
		for _, a := range e.Attr {
			if a.Name.Local == local {
				return a.Value
			}
		}

		return ""
	}

	count := func(s string, most int) int {
		n := 1
		fmt.Sscanf(s, "%d", &n)

		if n < 1 {
			n = 1
		}

		return min(n, most)
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("the spreadsheet could not be read: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "table":
				title, rows = attr(t, "name"), nil
			case "table-row":
				row = nil
				rowTimes = count(attr(t, "number-rows-repeated"), 1000)
			case "table-cell", "covered-table-cell":
				inCell = true
				cell.Reset()
				value = attr(t, "value")
				repeat = count(attr(t, "number-columns-repeated"), 1000)
			case "p":
				// A second paragraph in one cell is the same cell.
				if inCell && cell.Len() > 0 {
					cell.WriteString(" ")
				}
			}
		case xml.CharData:
			if inCell {
				cell.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "table-cell", "covered-table-cell":
				text := strings.TrimSpace(cell.String())
				if value != "" && text != "" {
					text = value
				}

				for i := 0; i < repeat; i++ {
					row = append(row, text)
				}

				inCell = false
			case "table-row":
				for len(row) > 0 && row[len(row)-1] == "" {
					row = row[:len(row)-1]
				}

				if len(row) > 0 {
					for i := 0; i < rowTimes && len(rows) < MaxTableRows; i++ {
						rows = append(rows, append([]string(nil), row...))
					}
				}
			case "table":
				if t, err := sheet.FromRows(title, rows); err == nil {
					out = append(out, t)
				}
			}
		}
	}

	if len(out) == 0 {
		return nil, errors.New("the spreadsheet has no rows")
	}

	return out, nil
}

// pickTable is the sheet asked for by name, or the first.
func pickTable(tables []sheet.Table, name string) (sheet.Table, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return tables[0], nil
	}

	var names []string

	for _, t := range tables {
		if strings.EqualFold(t.Name, name) {
			return t, nil
		}

		names = append(names, t.Name)
	}

	return sheet.Table{}, fmt.Errorf("there is no sheet %q; the sheets are: %s", name, strings.Join(names, ", "))
}

// Tables is what the table tools and the Tables page share.
type Tables struct {
	// Ask has the model write a recipe. Nil is no model: a recipe can still
	// be given whole.
	Ask func(ctx context.Context, system, user string) (string, string, error)
}

// ErrNoTableModel is a question asked with no model to turn it into a recipe.
var ErrNoTableModel = errors.New("no language model is available to turn the question into a calculation")

// Looked is a table looked over.
type Looked struct {
	Sheets  []string      `json:"sheets"`
	Summary sheet.Summary `json:"summary"`
	Header  []string      `json:"header"`
	First   [][]string    `json:"first"`
}

// Look reads a table and looks it over.
func (t Tables) Look(ctx context.Context, path, name string) (Looked, error) {
	tables, err := ReadTables(ctx, path)
	if err != nil {
		return Looked{}, err
	}

	tab, err := pickTable(tables, name)
	if err != nil {
		return Looked{}, err
	}

	out := Looked{Summary: sheet.Summarise(tab), Header: tab.Header}

	for _, x := range tables {
		out.Sheets = append(out.Sheets, x.Name)
	}

	out.First = tab.Rows[:min(len(tab.Rows), 8)]

	return out, nil
}

// Calculated is a question answered from a table, checked.
type Calculated struct {
	sheet.Result

	Sheet string `json:"sheet"`
	Model string `json:"model,omitempty"`
}

// Calculate answers a question from a table: the model writes a recipe
// (or one is given), the sheet package carries it out and checks it.
func (t Tables) Calculate(ctx context.Context, path, name, question string, given *sheet.Recipe) (Calculated, error) {
	tables, err := ReadTables(ctx, path)
	if err != nil {
		return Calculated{}, err
	}

	tab, err := pickTable(tables, name)
	if err != nil {
		return Calculated{}, err
	}

	var (
		recipe sheet.Recipe
		model  string
	)

	switch {
	case given != nil:
		recipe = *given
	case strings.TrimSpace(question) == "":
		return Calculated{}, errors.New("say what to work out")
	case t.Ask == nil:
		return Calculated{}, ErrNoTableModel
	default:
		reply, m, err := t.Ask(ctx, sheet.RecipeBrief(), sheet.RecipeInput(question, tab, sheet.Summarise(tab)))
		if err != nil {
			return Calculated{}, err
		}

		if recipe, err = sheet.ParseRecipe(reply); err != nil {
			return Calculated{}, err
		}

		model = m
	}

	res, err := sheet.Run(tab, recipe)
	if err != nil {
		return Calculated{}, err
	}

	return Calculated{Result: res, Sheet: tab.Name, Model: model}, nil
}

// Workbook is a result as a workbook: the result, the forecast, and the
// checks, each on its own sheet.
func Workbook(c Calculated) ([]byte, error) {
	sheets := []Sheet{{Title: "Result", Rows: append([][]string{c.Header}, c.Rows...)}}

	if len(c.Forecast) > 0 {
		sheets = append(sheets, Sheet{Title: "Forecast", Rows: c.Forecast})
	}

	checks := [][]string{{"Check", "Result"}}

	for _, f := range c.Checks {
		state := "OK"
		if !f.OK {
			state = "LOOK"
		}

		checks = append(checks, []string{f.What, state})
	}

	if c.Recipe.Explain != "" {
		checks = append(checks, []string{"What was calculated: " + c.Recipe.Explain, ""})
	}

	sheets = append(sheets, Sheet{Title: "Checks", Rows: checks})

	return BuildWorkbook(sheets)
}

// tableCues are the words of somebody working with a table.
var tableCues = []string{
	"spreadsheet", "table", "csv", "xlsx", "excel", "budget", "total", "sum ", "forecast", "column",
	"таблиц", "ексел", "бюджет", "сума", "общо", "прогноз", "колон",
}

// TableSummary looks a table over without a model.
type TableSummary struct{ With Tables }

func (TableSummary) Name() string   { return "table_summary" }
func (TableSummary) Risk() Risk     { return Safe }
func (TableSummary) Cues() []string { return tableCues }

func (TableSummary) Description() string {
	return "Look over a table (.csv, .tsv, .xlsx, .ods) without changing it: every column with its kind, " +
		"blanks, sum, smallest, largest and average; cells that are not numbers, by row; rows repeated in " +
		"every cell; and every row that calls itself a total, compared with what its rows really add up to. " +
		"Sums are exact, and numbers are read however they are written (1 234,56 or 1,234.56)."
}

func (TableSummary) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the table"},
			"sheet": {"type": "string", "description": "Which sheet; the first when not given"}
		},
		"required": ["path"]
	}`)
}

type tableArgs struct {
	Path     string        `json:"path"`
	Sheet    string        `json:"sheet"`
	Question string        `json:"question"`
	Recipe   *sheet.Recipe `json:"recipe"`
}

func (TableSummary) Summarize(raw json.RawMessage) string {
	var a tableArgs

	_ = json.Unmarshal(raw, &a)

	return "Look over the table " + a.Path
}

func (t TableSummary) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a tableArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	l, err := t.With.Look(ctx, a.Path, a.Sheet)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Sheet %q, %d rows", l.Summary.Name, l.Summary.Rows)

	if len(l.Sheets) > 1 {
		fmt.Fprintf(&b, " (sheets: %s)", strings.Join(l.Sheets, ", "))
	}

	b.WriteString(".\nColumns:\n")

	for _, c := range l.Summary.Columns {
		switch c.Kind {
		case "number":
			fmt.Fprintf(&b, "- %s: numbers, %d filled; sum %s, smallest %s, largest %s, average %s\n", c.Name, c.Numbers, c.Sum, c.Min, c.Max, c.Mean)
		default:
			fmt.Fprintf(&b, "- %s: %s, %d filled, %d blank\n", c.Name, c.Kind, c.Filled, c.Blank)
		}
	}

	b.WriteString("Checks:\n")

	for _, f := range l.Summary.Findings {
		mark := "LOOK"
		if f.OK {
			mark = "OK"
		}

		fmt.Fprintf(&b, "- %s: %s\n", mark, f.What)
	}

	return b.String(), nil
}

// CalculateTable answers a question from a table, checked.
type CalculateTable struct{ With Tables }

func (CalculateTable) Name() string   { return "calculate_table" }
func (CalculateTable) Risk() Risk     { return Safe }
func (CalculateTable) Cues() []string { return tableCues }

func (CalculateTable) Description() string {
	return "Answer a question from a table (.csv, .tsv, .xlsx, .ods) — totals by category, filters, a new " +
		"column from a formula, top N, averages, a forecast of the next months — without calculating anything " +
		"yourself: the question becomes a recipe the program carries out exactly, and every total is checked " +
		"a second way from the source rows. Rows that are the sheet's own totals are left out so nothing is " +
		"counted twice. Report the result with its checks; never change the numbers it gives. Changes nothing."
}

func (CalculateTable) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the table"},
			"sheet": {"type": "string", "description": "Which sheet; the first when not given"},
			"question": {"type": "string", "description": "What to work out, in the owner's words"}
		},
		"required": ["path", "question"]
	}`)
}

func (CalculateTable) Summarize(raw json.RawMessage) string {
	var a tableArgs

	_ = json.Unmarshal(raw, &a)

	return "Work out from " + filepath.Base(a.Path) + ": " + shortText(a.Question, 60)
}

func (t CalculateTable) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a tableArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	c, err := t.With.Calculate(ctx, a.Path, a.Sheet, a.Question, a.Recipe)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	if c.Recipe.Explain != "" {
		b.WriteString(c.Recipe.Explain + "\n\n")
	}

	writeMarkdown(&b, c.Header, c.Rows, 50)

	if len(c.Forecast) > 0 {
		b.WriteString("\nEstimate (from the past, not a promise):\n")
		writeMarkdown(&b, c.Forecast[0], c.Forecast[1:], 24)
	}

	b.WriteString("\nChecks:\n")

	for _, f := range c.Checks {
		mark := "LOOK"
		if f.OK {
			mark = "OK"
		}

		fmt.Fprintf(&b, "- %s: %s\n", mark, f.What)
	}

	return b.String(), nil
}

func writeMarkdown(b *strings.Builder, header []string, rows [][]string, most int) {
	cell := func(s string) string { return strings.ReplaceAll(s, "|", "/") }

	var h []string
	for _, x := range header {
		h = append(h, cell(x))
	}

	b.WriteString("| " + strings.Join(h, " | ") + " |\n|" + strings.Repeat(" --- |", len(header)) + "\n")

	for i, r := range rows {
		if i == most {
			fmt.Fprintf(b, "… and %d more rows\n", len(rows)-most)

			break
		}

		var cs []string
		for _, x := range r {
			cs = append(cs, cell(x))
		}

		b.WriteString("| " + strings.Join(cs, " | ") + " |\n")
	}
}
