package books

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func keys(fs []Finding) string {
	var k []string
	for _, f := range fs {
		k = append(k, f.Key)
	}

	return strings.Join(k, ",")
}

func TestAnInvoiceIsFilledInAndChecked(t *testing.T) {
	bg := PackFor("bg", nil, "")

	inv := Normalise(Invoice{Kind: "sale", Number: "0000000012", Date: "2026-09-03", Party: "Алфа ООД", PartyVAT: "bg 123456789", Net: "1 000,00", Rate: "20 %"})
	if inv.VAT != "200.00" || inv.Gross != "1200.00" || inv.PartyVAT != "BG123456789" || inv.Rate != "20" {
		t.Fatalf("%+v", inv)
	}

	if got := keys(Check(inv, bg, now)); got != "invoiceOK" {
		t.Errorf("a right invoice: %s", got)
	}

	cases := map[string]Invoice{
		"vatMath":     {Number: "0000000013", Date: "2026-09-04", Party: "x", Net: "100", Rate: "20", VAT: "18", Gross: "118"},
		"grossMath":   {Number: "0000000014", Date: "2026-09-04", Party: "x", Net: "100", Rate: "20", VAT: "20", Gross: "121"},
		"rateUnknown": {Number: "0000000015", Date: "2026-09-04", Party: "x", Net: "100", Rate: "7", VAT: "7", Gross: "107"},
		"zeroBasis":   {Number: "0000000016", Date: "2026-09-04", Party: "x", Net: "100", Rate: "0", VAT: "0", Gross: "100"},
		"numberForm":  {Number: "INV-17", Date: "2026-09-04", Party: "x", Net: "100", Rate: "9", VAT: "9", Gross: "109"},
		"vatID":       {Number: "0000000018", Date: "2026-09-04", Party: "x", PartyVAT: "BG12", Net: "100", Rate: "20", VAT: "20", Gross: "120"},
		"future":      {Number: "0000000019", Date: "2027-01-01", Party: "x", Net: "100", Rate: "20", VAT: "20", Gross: "120"},
		"noParty":     {Number: "0000000020", Date: "2026-09-04", Net: "100", Rate: "20", VAT: "20", Gross: "120"},
		"reverseVAT":  {Number: "0000000021", Date: "2026-09-04", Party: "x", Net: "100", Reverse: true, VAT: "20", Gross: "120"},
	}

	for want, inv := range cases {
		inv.Kind = Sale
		if got := keys(Check(inv, bg, now)); got != want {
			t.Errorf("%s: got %s", want, got)
		}
	}

	// A purchase invoice's number is the supplier's, in any form.
	p := Invoice{Kind: Purchase, Number: "F-2026/77", Date: "2026-09-05", Party: "Доставчик", Net: "50", Rate: "9", VAT: "4,50", Gross: "54,50"}
	if got := keys(Check(Normalise(p), bg, now)); got != "invoiceOK" {
		t.Errorf("a purchase: %s", got)
	}
}

func TestTheRegisterFindsNumbersUsedTwiceAndGaps(t *testing.T) {
	list := []Invoice{
		{Kind: Sale, Number: "0000000001"}, {Kind: Sale, Number: "0000000002"}, {Kind: Sale, Number: "0000000002"},
		{Kind: Sale, Number: "0000000005"}, {Kind: Sale, Number: "0000200000"}, {Kind: Purchase, Number: "0000000003"},
	}

	got := RegisterFindings(list)
	if keys(got) != "duplicate,gap" || got[1].Args["missing"] != "0000000003, 0000000004" {
		t.Errorf("%+v", got)
	}
}

func TestAPeriodsVATIsWorkedOutWithItsDueDate(t *testing.T) {
	bg := PackFor("bg", nil, "")

	list := []Invoice{
		Normalise(Invoice{Kind: Sale, Number: "0000000001", Date: "2026-09-01", Party: "a", Net: "1000", Rate: "20"}),
		Normalise(Invoice{Kind: Sale, Number: "0000000002", Date: "2026-09-15", Party: "b", Net: "200", Rate: "9"}),
		Normalise(Invoice{Kind: Sale, Number: "0000000003", Date: "2026-09-30", Party: "c", Net: "500", Reverse: true}),
		Normalise(Invoice{Kind: Purchase, Number: "X1", Date: "2026-09-10", Party: "d", Net: "300", Rate: "20"}),
		Normalise(Invoice{Kind: Sale, Number: "0000000004", Date: "2026-10-01", Party: "e", Net: "999", Rate: "20"}),
	}

	p, err := Summarise(list, "2026-09", bg, now)
	if err != nil {
		t.Fatal(err)
	}

	if p.Output != "218.00" || p.Input != "60.00" || p.Result != "158.00" || !p.ToPay || p.Due != "2026-10-14" || p.Invoices != 4 {
		t.Errorf("%+v", p)
	}

	if len(p.Sales) != 2 || p.Sales[0].Rate != "20" || p.Sales[1].VAT != "18.00" || p.Reverse.Net != "500.00" {
		t.Errorf("%+v", p.Sales)
	}

	if !strings.HasPrefix(keys(p.Findings), "periodOK") {
		t.Errorf("%s", keys(p.Findings))
	}

	q, _ := Summarise(list, "2026-Q4", bg, now)
	if q.Due != "" || q.Output != "199.80" {
		t.Errorf("%+v", q)
	}

	if _, err := Summarise(list, "September", bg, now); err == nil {
		t.Error("a period that is not one was accepted")
	}
}

func TestTheThresholdIsWatched(t *testing.T) {
	bg := PackFor("bg", nil, "")

	list := []Invoice{
		{Kind: Sale, Number: "0000000001", Date: "2026-02-01", Net: "30000"},
		{Kind: Sale, Number: "0000000002", Date: "2026-05-01", Net: "21130"},
		{Kind: Purchase, Number: "p", Date: "2026-05-02", Net: "90000"},
	}

	w, ok := ThresholdWatch(list, bg, 2026)
	if !ok || w.Sales != "51130.00" || w.Finding.Key != "threshold" || w.Finding.Args["pct"] != "100" {
		t.Errorf("at the threshold is not over it: %+v", w)
	}

	list = append(list, Invoice{Kind: Sale, Number: "0000000003", Date: "2026-06-01", Net: "0.01"})

	w, _ = ThresholdWatch(list, bg, 2026)
	if w.Finding.Key != "overThreshold" || w.Finding.Args["date"] != "2026-06-01" {
		t.Errorf("%+v", w)
	}

	if _, ok := ThresholdWatch(list, PackFor("custom", []string{"21"}, "EUR"), 2026); ok {
		t.Error("a country without rules was given a threshold")
	}
}

func TestAnotherCountryUsesItsOwnersRatesAndClaimsNothing(t *testing.T) {
	p := PackFor("custom", []string{"21", "10 %", "x"}, "EUR")

	if len(p.Rates) != 2 || p.Number != nil || p.Due != 0 || len(p.Sources) != 0 {
		t.Errorf("%+v", p)
	}

	inv := Normalise(Invoice{Kind: Sale, Number: "A-1", Date: "2026-09-01", Party: "x", Net: "100", Rate: "21"})
	if got := keys(Check(inv, p, now)); got != "invoiceOK" {
		t.Errorf("%s", got)
	}

	per, _ := Summarise([]Invoice{inv}, "2026-09", p, now)
	if per.Due != "" {
		t.Error("a due date was claimed for a country without rules")
	}
}

func TestEveryBulgarianRuleSaysWhereItComesFrom(t *testing.T) {
	bg := PackFor("bg", nil, "")

	for _, r := range bg.Rates {
		if !strings.Contains(r.Rule, "ЗДДС") || r.Covers[0] == "" || r.Covers[1] == "" {
			t.Errorf("%+v", r)
		}
	}

	for _, rule := range [][2]string{bg.ThresholdRule, bg.DueRule, bg.InvoiceRule, bg.NumberRule} {
		if !strings.Contains(rule[0], "ЗДДС чл.") || !strings.Contains(rule[1], "ЗДДС чл.") {
			t.Errorf("%q", rule)
		}
	}

	if len(bg.Sources) == 0 || bg.Sources[0].Checked == "" || bg.Checked == "" || len(bg.Fields[0]) != len(bg.Fields[1]) {
		t.Errorf("%+v", bg.Sources)
	}
}
