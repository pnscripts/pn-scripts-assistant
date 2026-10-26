package sheet

import (
	"math/big"
	"strings"
	"testing"
)

func rat(s string) *big.Rat {
	r, _ := new(big.Rat).SetString(s)

	return r
}

func TestNumbersAreReadHoweverTheyAreWritten(t *testing.T) {
	cases := []struct {
		cell  string
		style Style
		want  string
		ok    bool
	}{
		{"1 234,56", Unknown, "1234.56", true},
		{"1,234.56", Unknown, "1234.56", true},
		{"1.234,56", Unknown, "1234.56", true},
		{"€ 12", Unknown, "12", true},
		{"12 лв.", Unknown, "12", true},
		{"BGN 1 200", Unknown, "1200", true},
		{"(12.50)", Unknown, "-12.5", true},
		{"−7", Unknown, "-7", true},
		{"15 %", Unknown, "15", true},
		{"12,5", Unknown, "12.5", true},
		{"1,250", Unknown, "", false},
		{"1,250", Point, "1250", true},
		{"1,250", Comma, "1.25", true},
		{"1.250", Comma, "1250", true},
		{"1.250", Point, "1.25", true},
		{"abc", Unknown, "", false},
		{"", Unknown, "", false},
		{"2024-03", Unknown, "", false},
	}

	for _, c := range cases {
		got, ok := Number(c.cell, c.style)
		if ok != c.ok || (ok && got.Cmp(rat(c.want)) != 0) {
			t.Errorf("%q (%d): %v %v, want %s %v", c.cell, c.style, got, ok, c.want, c.ok)
		}
	}
}

func TestAColumnDecidesItsDecimalMark(t *testing.T) {
	bg, _ := FromRows("x", [][]string{{"Сума"}, {"1 250,50"}, {"12,5"}, {"1,250"}})
	en, _ := FromRows("x", [][]string{{"Amount"}, {"1,250.50"}, {"12.5"}, {"1,250"}})

	if ColumnStyle(bg, 0) != Comma || ColumnStyle(en, 0) != Point {
		t.Fatal("the column style was not seen")
	}

	if s := Summarise(bg); s.Columns[0].Sum != "1264.25" {
		t.Errorf("Bulgarian: %+v", s.Columns[0])
	}

	if s := Summarise(en); s.Columns[0].Sum != "2513" {
		t.Errorf("English: %+v", s.Columns[0])
	}
}

var expenses = [][]string{
	{"Date", "Category", "Amount", "Note"},
	{"2026-01-05", "Food", "120,50", ""},
	{"2026-01-09", "Rent", "800", ""},
	{"2026-01-15", "Food", "79,50", ""},
	{"2026-02-03", "Food", "110", ""},
	{"2026-02-04", "Fuel", "n/a", "receipt lost"},
	{"2026-02-04", "Fuel", "60", ""},
	{"2026-02-04", "Fuel", "60", ""},
	{"Total", "", "1 230", ""},
}

func TestASummaryFindsWhatIsWrong(t *testing.T) {
	tab, err := FromRows("expenses", expenses)
	if err != nil {
		t.Fatal(err)
	}

	s := Summarise(tab)

	amount := s.Columns[2]
	if amount.Kind != "number" || amount.Sum != "1230" || len(amount.Unread) != 1 || amount.Unread[0] != 6 {
		t.Errorf("%+v", amount)
	}

	if len(s.TotalRows) != 1 || s.TotalRows[0] != 9 {
		t.Errorf("total rows %v", s.TotalRows)
	}

	if len(s.Duplicates) != 1 || s.Duplicates[0][0] != 7 || s.Duplicates[0][1] != 8 {
		t.Errorf("duplicates %v", s.Duplicates)
	}

	all := ""
	for _, f := range s.Findings {
		all += f.What + "\n"
	}

	for _, want := range []string{"1 cells are not numbers", "rows 7, 8 are the same", "the sheet's total 1230 matches"} {
		if !strings.Contains(all, want) {
			t.Errorf("lacks %q:\n%s", want, all)
		}
	}

	wrong := append([][]string(nil), expenses...)
	wrong[len(wrong)-1] = []string{"Общо", "", "1 300", ""}

	tab2, _ := FromRows("x", wrong)

	found := false
	for _, f := range CheckTotalRows(tab2) {
		if !f.OK && strings.Contains(f.What, "the sheet says 1300 but rows 2–8 add up to 1230 (a difference of 70)") {
			found = true
		}
	}

	if !found {
		t.Errorf("a wrong total was not found: %+v", CheckTotalRows(tab2))
	}
}

func TestARecipeIsCarriedOutAndCheckedASecondWay(t *testing.T) {
	tab, _ := FromRows("expenses", expenses)

	r, err := ParseRecipe("Here you go:\n```json\n{\"filter\":[{\"column\":\"category\",\"op\":\"!=\",\"value\":\"Rent\"}]," +
		"\"add\":[{\"name\":\"With VAT\",\"expr\":\"round(Amount * 1.2, 2)\"}]," +
		"\"group_by\":[\"Category\"],\"aggregate\":[{\"column\":\"Amount\",\"fn\":\"sum\"},{\"column\":\"With VAT\",\"fn\":\"sum\",\"as\":\"VAT incl.\"},{\"fn\":\"count\"}]," +
		"\"sort\":{\"column\":\"sum of Amount\",\"desc\":true}}\n```")
	if err != nil {
		t.Fatal(err)
	}

	res, err := Run(tab, r)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(res.Header, "|") != "Category|sum of Amount|VAT incl.|count" {
		t.Errorf("header %q", res.Header)
	}

	if len(res.Rows) != 2 || strings.Join(res.Rows[0], "|") != "Food|310|372|3" || strings.Join(res.Rows[1], "|") != "Fuel|120|144|3" {
		t.Errorf("rows %q", res.Rows)
	}

	if len(res.Skipped) != 1 || res.Skipped[0] != 9 || res.Used != 6 {
		t.Errorf("skipped %v used %d", res.Skipped, res.Used)
	}

	all := ""
	bad := 0

	for _, f := range res.Checks {
		all += f.What + "\n"

		if !f.OK {
			bad++
		}
	}

	for _, want := range []string{"rows 9 are the sheet's own totals", "6 of 7 rows match the filter", "sum of Amount: the groups add up to 430, the same as the rows",
		"VAT incl.: the groups add up to 516", "Amount: 1 cells are not numbers and are not in the sum (rows 6)", "all 6 rows are in exactly one of 2 groups"} {
		if !strings.Contains(all, want) {
			t.Errorf("lacks %q:\n%s", want, all)
		}
	}

	if bad != 1 {
		t.Errorf("%d problems, want only the unread cell:\n%s", bad, all)
	}
}

func TestColumnsAreNamedInTheTablesLanguage(t *testing.T) {
	bg, _ := FromRows("x", [][]string{{"Категория", "Сума"}, {"Храна", "10"}, {"Храна", "5"}})

	res, err := Run(bg, Recipe{GroupBy: []string{"Категория"}, Aggregate: []Aggregate{{Column: "Сума"}, {Fn: "count"}}})
	if err != nil || strings.Join(res.Header, "|") != "Категория|сума на Сума|брой" || res.Rows[0][1] != "15" {
		t.Errorf("%v %q %q", err, res.Header, res.Rows)
	}

	ok := false
	for _, c := range res.Checks {
		if c.Key == "sumOK" && c.Args["col"] == "сума на Сума" {
			ok = true
		}
	}

	if !ok {
		t.Errorf("the sum was not checked under its name: %+v", res.Checks)
	}
}

func TestARecipeNamingAMissingColumnSaysWhichAreThere(t *testing.T) {
	tab, _ := FromRows("x", expenses)

	_, err := Run(tab, Recipe{GroupBy: []string{"Kategorie"}})
	if err == nil || !strings.Contains(err.Error(), "Date, Category, Amount, Note") {
		t.Errorf("%v", err)
	}

	if _, err := Run(tab, Recipe{Add: []Add{{Name: "x", Expr: "Price * 2"}}}); err == nil {
		t.Error("a formula over a missing column was accepted")
	}

	if _, err := Run(tab, Recipe{Aggregate: []Aggregate{{Column: "Amount", Fn: "median"}}}); err == nil {
		t.Error("an unknown aggregate was accepted")
	}
}

func TestFormulasAreArithmeticOnly(t *testing.T) {
	get := func(name string) (*big.Rat, bool) {
		switch strings.ToLower(name) {
		case "price":
			return rat("19.99"), true
		case "qty", "quantity sold":
			return rat("3"), true
		}

		return nil, false
	}

	cases := map[string]string{
		"Price * Qty":                  "59.97",
		`"Quantity sold" * price`:      "59.97",
		"round(Price * 1.2, 2)":        "23.99",
		"round(2.5)":                   "3",
		"round(-2.5)":                  "-3",
		"Price * 20%":                  "3.998",
		"-(Price - 20) * 2":            "0.02",
		"max(Price, 25) + min(Qty, 1)": "26",
		"abs(Qty - 10) / 7":            "1",
		"(Price + 0.01) × Qty":         "60",
	}

	for f, want := range cases {
		e, err := Parse(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)

			continue
		}

		v, err := e.Eval(get)
		if err != nil || v.Cmp(rat(want)) != 0 {
			t.Errorf("%s = %v (%v), want %s", f, v, err, want)
		}
	}

	for _, bad := range []string{"", "Price *", "(Price", "system('rm')", "Price; Qty", "round(1,2,3)", `"Price`} {
		if e, err := Parse(bad); err == nil {
			if _, err := e.Eval(get); err == nil {
				t.Errorf("%q was accepted", bad)
			}
		}
	}

	e, _ := Parse("Price / (Qty - 3)")
	if _, err := e.Eval(get); err == nil {
		t.Error("division by zero was not refused")
	}
}

func TestAForecastIsAnEstimateFromThePast(t *testing.T) {
	tab, _ := FromRows("sales", [][]string{
		{"Month", "Sales"}, {"2026-03", "130"}, {"2026-01", "100"}, {"2026-02", "115"}, {"2026-02", "0"},
	})

	res, err := Run(tab, Recipe{Forecast: &Forecast{Column: "Sales", By: "Month", Periods: 2, Method: "trend"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Forecast) != 3 || strings.Join(res.Forecast[1], "|") != "2026-04|145|trend" || res.Forecast[2][0] != "2026-05" {
		t.Errorf("%q", res.Forecast)
	}

	res, _ = Run(tab, Recipe{Forecast: &Forecast{Column: "Sales", By: "Month", Periods: 1}})
	if strings.Join(res.Forecast[1], "|") != "2026-04|115|average" {
		t.Errorf("%q", res.Forecast)
	}

	one, _ := FromRows("x", [][]string{{"Month", "Sales"}, {"2026-01", "1"}})
	if _, err := Run(one, Recipe{Forecast: &Forecast{Column: "Sales", By: "Month"}}); err == nil {
		t.Error("a forecast from one period was made")
	}
}

func TestRoundingIsHalfAwayFromZero(t *testing.T) {
	for in, want := range map[string]string{"1.005": "1.01", "-1.005": "-1.01", "2.344": "2.34", "0.125": "0.13"} {
		if got := Format(Round(rat(in), 2)); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}
