// Package sheet is calculating with somebody's table — a budget, sales, a
// list of expenses — and checking every total before anybody relies on it.
//
// The model does not calculate. Asked a question about a table, it writes a
// recipe: which rows, which columns to add, how to group, what to sort by,
// what to forecast. The recipe is data, not code, and this package carries
// it out with exact arithmetic. Then it checks its own work a second way:
// every total of the result is recomputed straight from the source rows and
// must match to the cent; the rows counted into groups must be the rows
// that passed the filter; cells that are not numbers are listed by row, not
// silently taken as nothing; and a row in the sheet that is itself a total
// ("Total", "Общо") is left out of the sums, so it is not counted twice, and
// compared with what the rows really add up to.
package sheet

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Table is one sheet: a header and its rows.
type Table struct {
	Name   string     `json:"name"`
	Header []string   `json:"header"`
	Rows   [][]string `json:"rows"`
}

// FromRows makes a table of rows whose first non-empty row is the header.
// Short rows are padded; a header cell left empty gets its column letter.
func FromRows(name string, rows [][]string) (Table, error) {
	var kept [][]string

	for _, r := range rows {
		empty := true

		for _, c := range r {
			if strings.TrimSpace(c) != "" {
				empty = false

				break
			}
		}

		if !empty {
			kept = append(kept, r)
		}
	}

	if len(kept) == 0 {
		return Table{}, errors.New("the table is empty")
	}

	width := 0
	for _, r := range kept {
		width = max(width, len(r))
	}

	header := make([]string, width)
	seen := map[string]int{}

	for i := range header {
		h := ""
		if i < len(kept[0]) {
			h = strings.TrimSpace(kept[0][i])
		}

		if h == "" {
			h = columnLetter(i)
		}

		// Two columns of one name would make "Amount" mean either.
		if n := seen[strings.ToLower(h)]; n > 0 {
			h = fmt.Sprintf("%s (%d)", h, n+1)
		}

		seen[strings.ToLower(h)]++
		header[i] = h
	}

	t := Table{Name: name, Header: header}

	for _, r := range kept[1:] {
		row := make([]string, width)
		for i := range row {
			if i < len(r) {
				row[i] = strings.TrimSpace(r[i])
			}
		}

		t.Rows = append(t.Rows, row)
	}

	return t, nil
}

func columnLetter(i int) string {
	name := ""

	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}

	return name
}

// Column is a column by its name, whatever its case; or by its letter.
func (t Table) Column(name string) (int, bool) {
	name = strings.TrimSpace(name)

	for i, h := range t.Header {
		if strings.EqualFold(h, name) {
			return i, true
		}
	}

	for i := range t.Header {
		if strings.EqualFold(columnLetter(i), name) {
			return i, true
		}
	}

	return -1, false
}

/*
 * Numbers.
 *
 * "1 234,56", "1,234.56", "1.234,56", "€ 12", "12 лв.", "(12.50)" for a
 * negative, "−7", "15 %". The one that cannot be read alone is "1,250" —
 * a thousand and a quarter in English, one and a quarter in Bulgarian — so
 * a column decides: the other cells of the same column say which mark is
 * the decimal one, and when none of them does, the cell is not guessed at.
 */

// Style is which mark a column writes decimals with.
type Style int

const (
	// Unknown is a column whose cells never say.
	Unknown Style = iota
	// Point is 1,234.56.
	Point
	// Comma is 1 234,56 or 1.234,56.
	Comma
)

var (
	currency  = regexp.MustCompile(`(?i)^(?:[€$£]|bgn|eur|usd|gbp|лв\.?|лева|eur\.?)\s*|\s*(?:[€$£]|bgn|eur|usd|gbp|лв\.?|лева|ст\.?)$`)
	plainNum  = regexp.MustCompile(`^\d+$`)
	groupedPt = regexp.MustCompile(`^\d{1,3}(,\d{3})+(\.\d+)?$`)
	groupedCm = regexp.MustCompile(`^\d{1,3}(\.\d{3})+(,\d+)?$`)
	spaced    = regexp.MustCompile(`^\d{1,3}([ \x{00a0}\x{202f}']\d{3})+([.,]\d+)?$`)
	decimalPt = regexp.MustCompile(`^\d+\.\d+$`)
	decimalCm = regexp.MustCompile(`^\d+,\d+$`)
)

// clean takes the sign, the currency and a percent sign off a cell.
func clean(cell string) (body string, negative, percent bool) {
	s := strings.TrimSpace(cell)
	s = strings.NewReplacer("−", "-", "–", "-").Replace(s)

	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		negative, s = true, strings.TrimSpace(s[1:len(s)-1])
	}

	for i := 0; i < 2; i++ {
		s = strings.TrimSpace(currency.ReplaceAllString(s, ""))
	}

	if strings.HasSuffix(s, "%") {
		percent, s = true, strings.TrimSpace(strings.TrimSuffix(s, "%"))
	}

	if strings.HasPrefix(s, "-") {
		negative, s = !negative, strings.TrimSpace(s[1:])
	} else if strings.HasPrefix(s, "+") {
		s = strings.TrimSpace(s[1:])
	}

	for i := 0; i < 2; i++ {
		s = strings.TrimSpace(currency.ReplaceAllString(s, ""))
	}

	return s, negative, percent
}

// StyleOf is what one cell says about its column's decimal mark.
func StyleOf(cell string) Style {
	s, _, _ := clean(cell)

	switch {
	case groupedPt.MatchString(s) && !(strings.Count(s, ",") == 1 && !strings.Contains(s, ".")):
		return Point
	case groupedCm.MatchString(s) && !(strings.Count(s, ".") == 1 && !strings.Contains(s, ",")):
		return Comma
	case spaced.MatchString(s) && strings.ContainsAny(s[len(s)-3:], ".,"):
		if strings.Contains(s, ",") {
			return Comma
		}

		return Point
	case decimalPt.MatchString(s) && len(s)-strings.IndexByte(s, '.') != 4:
		return Point
	case decimalCm.MatchString(s) && len(s)-strings.IndexByte(s, ',') != 4:
		return Comma
	}

	return Unknown
}

// Number reads a cell as an exact number, given its column's style. The
// second result is whether it is a number at all; empty is not.
func Number(cell string, style Style) (*big.Rat, bool) {
	s, negative, _ := clean(cell)
	if s == "" {
		return nil, false
	}

	var text string

	switch {
	case plainNum.MatchString(s):
		text = s
	case spaced.MatchString(s):
		s = strings.NewReplacer(" ", "", " ", "", " ", "", "'", "").Replace(s)
		text = strings.Replace(s, ",", ".", 1)
	case groupedPt.MatchString(s) && (style != Comma || strings.Contains(s, ".")):
		if style == Unknown && !strings.Contains(s, ".") && strings.Count(s, ",") == 1 {
			return nil, false // "1,250": a thousand or one and a quarter
		}

		text = strings.ReplaceAll(s, ",", "")
	case groupedCm.MatchString(s) && (style != Point || strings.Contains(s, ",")):
		if style == Unknown && !strings.Contains(s, ",") && strings.Count(s, ".") == 1 {
			return nil, false // "1.250": the same question the other way
		}

		text = strings.Replace(strings.ReplaceAll(s, ".", ""), ",", ".", 1)
	case decimalPt.MatchString(s):
		text = s
	case decimalCm.MatchString(s):
		text = strings.Replace(s, ",", ".", 1)
	default:
		return nil, false
	}

	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, false
	}

	if negative {
		r.Neg(r)
	}

	return r, true
}

// ColumnStyle is the decimal mark most of a column's cells say they use.
func ColumnStyle(t Table, col int) Style {
	point, comma := 0, 0

	for _, r := range t.Rows {
		switch StyleOf(r[col]) {
		case Point:
			point++
		case Comma:
			comma++
		}
	}

	switch {
	case point > comma:
		return Point
	case comma > point:
		return Comma
	}

	return Unknown
}

// totalWords mark a row that is itself a total of the rows above it.
var totalWords = []string{"total", "subtotal", "sum", "grand total", "общо", "всичко", "сума", "итого", "тотал"}

// IsTotalRow is whether a row's first words say it is a total.
func IsTotalRow(row []string) bool {
	for _, c := range row {
		c = strings.ToLower(strings.TrimSpace(strings.Trim(c, "*:")))
		if c == "" {
			continue
		}

		for _, w := range totalWords {
			if c == w || strings.HasPrefix(c, w+" ") || strings.HasPrefix(c, w+":") {
				return true
			}
		}

		// Only the first filled cell names a row.
		return false
	}

	return false
}

// Profile is what one column holds.
type Profile struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"` // number, text, empty
	Filled  int    `json:"filled"`
	Blank   int    `json:"blank"`
	Numbers int    `json:"numbers"`
	Sum     string `json:"sum,omitempty"`
	Min     string `json:"min,omitempty"`
	Max     string `json:"max,omitempty"`
	Mean    string `json:"mean,omitempty"`

	// Unread are the rows (as the sheet numbers them) of a number column
	// whose cell could not be read as a number.
	Unread []int `json:"unread,omitempty"`

	// Ambiguous is a column with numbers like 1,250 and nothing to say
	// which mark is the decimal one.
	Ambiguous bool `json:"ambiguous,omitempty"`
}

// Finding is one thing the checks found: in English, and as a key and its
// values, so the page can say it in the reader's language.
type Finding struct {
	OK   bool              `json:"ok"`
	What string            `json:"what"`
	Key  string            `json:"key"`
	Args map[string]string `json:"args,omitempty"`
}

// findings are what each kind of finding says, with {name} for its values.
var findings = map[string]string{
	"unread":         "{col}: {n} cells are not numbers and are left out of its sums (rows {rows})",
	"ambiguous":      "{col}: numbers like 1,250 could be a thousand or one and a quarter, and nothing in the column says which; they are not counted until written unambiguously",
	"duplicate":      "rows {rows} are the same in every cell — counted twice?",
	"clean":          "Every number could be read; no repeated rows; no total rows to compare.",
	"totalOK":        "row {row}, {col}: the sheet's total {value} matches its rows",
	"totalWrong":     "row {row}, {col}: the sheet says {stated} but rows {from}–{to} add up to {sum} (a difference of {diff})",
	"skipped":        "rows {rows} are the sheet's own totals and were left out, so nothing is counted twice",
	"filterMismatch": "the filter kept {kept} rows, but {again} rows match it when checked again",
	"filterOK":       "{n} of {of} rows match the filter, checked twice",
	"sumLimited":     "{col}: the groups shown add up to {shown} of {total} in all (only the first {limit} groups are shown)",
	"sumOK":          "{col}: the groups add up to {total}, the same as the rows added one by one",
	"sumMismatch":    "{col}: the groups add up to {shown} but the rows add up to {total}",
	"sumUnread":      "{col}: {n} cells are not numbers and are not in the sum (rows {rows})",
	"groupsOK":       "all {n} rows are in exactly one of {groups} groups",
	"groupsMismatch": "{n} rows in {groups} groups, but {passed} rows passed the filter",
	"nothing":        "{n} rows used; nothing needed checking a second way",
}

// found is a finding of a kind, with its values given as name, value pairs.
func found(ok bool, key string, kv ...any) Finding {
	f := Finding{OK: ok, Key: key, Args: map[string]string{}}
	what := findings[key]

	for i := 0; i+1 < len(kv); i += 2 {
		name, value := fmt.Sprint(kv[i]), fmt.Sprint(kv[i+1])
		f.Args[name] = value
		what = strings.ReplaceAll(what, "{"+name+"}", value)
	}

	f.What = what

	return f
}

// Summary is a table looked over: every column, and what is wrong.
type Summary struct {
	Name       string    `json:"name"`
	Rows       int       `json:"rows"`
	Columns    []Profile `json:"columns"`
	TotalRows  []int     `json:"total_rows,omitempty"`
	Duplicates [][]int   `json:"duplicates,omitempty"`
	Findings   []Finding `json:"findings"`
}

// SheetRow is the row number the sheet itself shows for row i of the
// table: the header is row 1.
func SheetRow(i int) int { return i + 2 }

// Summarise looks a table over without a model.
func Summarise(t Table) Summary {
	s := Summary{Name: t.Name, Rows: len(t.Rows)}

	totals := map[int]bool{}

	for i, r := range t.Rows {
		if IsTotalRow(r) {
			totals[i] = true
			s.TotalRows = append(s.TotalRows, SheetRow(i))
		}
	}

	for c, name := range t.Header {
		p := Profile{Name: name}
		style := ColumnStyle(t, c)

		var (
			sum, lo, hi *big.Rat
			unread      []int
			ambiguous   bool
		)

		for i, r := range t.Rows {
			if totals[i] {
				continue
			}

			cell := r[c]
			if cell == "" {
				p.Blank++

				continue
			}

			p.Filled++

			n, ok := Number(cell, style)
			if !ok {
				if looksNumeric(cell) {
					ambiguous = true
				}

				unread = append(unread, SheetRow(i))

				continue
			}

			p.Numbers++

			if sum == nil {
				sum, lo, hi = new(big.Rat), new(big.Rat).Set(n), new(big.Rat).Set(n)
			}

			sum.Add(sum, n)

			if n.Cmp(lo) < 0 {
				lo.Set(n)
			}

			if n.Cmp(hi) > 0 {
				hi.Set(n)
			}
		}

		switch {
		case p.Filled == 0:
			p.Kind = "empty"
		case p.Numbers*2 > p.Filled:
			p.Kind = "number"
			p.Sum, p.Min, p.Max = Format(sum), Format(lo), Format(hi)
			p.Mean = Format(new(big.Rat).Quo(sum, big.NewRat(int64(p.Numbers), 1)))
			p.Unread = unread
			p.Ambiguous = ambiguous
		default:
			p.Kind = "text"
		}

		s.Columns = append(s.Columns, p)
	}

	s.Duplicates = duplicates(t, totals)
	s.Findings = summaryFindings(t, s)

	return s
}

func looksNumeric(cell string) bool {
	digits := 0

	for _, r := range cell {
		if unicode.IsDigit(r) {
			digits++
		}
	}

	return digits > 0 && digits*2 >= len([]rune(strings.TrimSpace(cell)))
}

// duplicates are groups of rows that are the same in every cell — the
// commonest way an expense is counted twice.
func duplicates(t Table, skip map[int]bool) [][]int {
	at := map[string][]int{}

	var order []string

	for i, r := range t.Rows {
		if skip[i] {
			continue
		}

		key := strings.ToLower(strings.Join(r, "\x1f"))
		if _, ok := at[key]; !ok {
			order = append(order, key)
		}

		at[key] = append(at[key], SheetRow(i))
	}

	var out [][]int

	for _, k := range order {
		if len(at[k]) > 1 {
			out = append(out, at[k])
		}
	}

	return out
}

func summaryFindings(t Table, s Summary) []Finding {
	var out []Finding

	for _, p := range s.Columns {
		if p.Kind != "number" {
			continue
		}

		if len(p.Unread) > 0 {
			out = append(out, found(false, "unread", "col", p.Name, "n", len(p.Unread), "rows", rowList(p.Unread)))
		}

		if p.Ambiguous {
			out = append(out, found(false, "ambiguous", "col", p.Name))
		}
	}

	for _, d := range s.Duplicates {
		out = append(out, found(false, "duplicate", "rows", rowList(d)))
	}

	// A total row is checked against what its rows really add up to.
	out = append(out, CheckTotalRows(t)...)

	if len(out) == 0 {
		out = append(out, found(true, "clean"))
	}

	return out
}

/*
 * CheckTotalRows compares each row that calls itself a total with what the
 * rows above it — back to the previous total — add up to, column by
 * column. A sheet whose own total is wrong is the most expensive mistake a
 * table can make, and the easiest to miss.
 */
func CheckTotalRows(t Table) []Finding {
	var out []Finding

	start := 0

	for i, r := range t.Rows {
		if !IsTotalRow(r) {
			continue
		}

		for c := range t.Header {
			style := ColumnStyle(t, c)

			stated, ok := Number(r[c], style)
			if !ok {
				continue
			}

			sum := new(big.Rat)
			n := 0

			for j := start; j < i; j++ {
				if v, ok := Number(t.Rows[j][c], style); ok {
					sum.Add(sum, v)
					n++
				}
			}

			if n == 0 {
				continue
			}

			if sum.Cmp(stated) == 0 {
				out = append(out, found(true, "totalOK", "row", SheetRow(i), "col", t.Header[c], "value", Format(stated)))
			} else {
				out = append(out, found(false, "totalWrong", "row", SheetRow(i), "col", t.Header[c], "stated", Format(stated),
					"from", SheetRow(start), "to", SheetRow(i-1), "sum", Format(sum), "diff", Format(new(big.Rat).Sub(stated, sum))))
			}
		}

		start = i + 1
	}

	return out
}

func rowList(rows []int) string {
	var parts []string

	for i, r := range rows {
		if i == 12 {
			parts = append(parts, fmt.Sprintf("and %d more", len(rows)-12))

			break
		}

		parts = append(parts, strconv.Itoa(r))
	}

	return strings.Join(parts, ", ")
}

// Format writes an exact number plainly: no exponent, at most four
// decimals, trailing zeros taken off. The rounding is only in the writing.
func Format(r *big.Rat) string {
	if r == nil {
		return ""
	}

	s := r.FloatString(4)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}

	if s == "-0" {
		s = "0"
	}

	return s
}

// sortRows orders rows by a column: numbers as numbers, the rest as text.
func sortRows(rows [][]string, col int, desc bool, style Style) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, aok := Number(rows[i][col], style)
		b, bok := Number(rows[j][col], style)

		var less bool

		switch {
		case aok && bok:
			less = a.Cmp(b) < 0
		case aok != bok:
			less = aok // numbers before text
		default:
			less = strings.ToLower(rows[i][col]) < strings.ToLower(rows[j][col])
		}

		if desc {
			return !less && !(aok && bok && a.Cmp(b) == 0)
		}

		return less
	})
}

func ratFloat(r *big.Rat) float64 {
	f, _ := r.Float64()

	return f
}

// FindingKeys are every kind of finding, for checking each is said in
// every language the interface speaks.
func FindingKeys() []string {
	out := make([]string, 0, len(findings))
	for k := range findings {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}
