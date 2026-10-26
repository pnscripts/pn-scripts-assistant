package tools

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/books"
	"pn-scripts-assistant/internal/brain/store"
)

func keeperFor(t *testing.T, reply string) Bookkeeping {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	k := Bookkeeping{Book: db, Now: func() time.Time { return time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC) }}

	if reply != "" {
		k.Ask = func(context.Context, string, string) (string, string, error) { return reply, "m", nil }
	}

	return k
}

func TestInvoicesAreRecordedCheckedAndNeverTwice(t *testing.T) {
	k := keeperFor(t, "")

	r, err := k.Record(books.Invoice{Kind: "sale", Number: "0000000001", Date: "2026-09-02", Party: "Алфа ООД", Net: "1000", Rate: "20"}, true)
	if err != nil || !r.Kept || r.Invoice.VAT != "200.00" || r.Invoice.Currency != "EUR" || r.Findings[0].Key != "invoiceOK" {
		t.Fatalf("%v %+v", err, r)
	}

	if _, err := k.Record(books.Invoice{Kind: "sale", Number: "0000000001", Date: "2026-09-03", Party: "Бета", Net: "5", Rate: "20"}, true); !errors.Is(err, ErrDuplicateInvoice) {
		t.Errorf("a second invoice of one number: %v", err)
	}

	// A purchase may share a number with a sale: it is the supplier's.
	if _, err := k.Record(books.Invoice{Kind: "purchase", Number: "0000000001", Date: "2026-09-04", Party: "Гама", Net: "100", Rate: "20"}, true); err != nil {
		t.Errorf("%v", err)
	}

	only, _ := k.Record(books.Invoice{Kind: "sale", Number: "17", Date: "2026-09-05", Party: "x", Net: "10", Rate: "20", VAT: "3"}, false)
	if only.Kept || len(only.Findings) != 2 || only.Findings[0].Key != "vatMath" || only.Findings[1].Key != "numberForm" {
		t.Errorf("checking only: %+v", only)
	}

	out, err := VATSummary{With: k}.Execute(context.Background(), json.RawMessage(`{"period":"2026-09"}`))
	if err != nil || !strings.Contains(out, "VAT on sales 200.00, on purchases 20.00: 180.00 to pay") ||
		!strings.Contains(out, "Due by 2026-10-14") || !strings.Contains(out, "Information, not tax advice") ||
		!strings.Contains(out, "lex.bg") {
		t.Errorf("%v\n%s", err, out)
	}

	if err := k.SetSettings(BooksSettings{Country: "mars"}); err == nil {
		t.Error("an unknown country was kept")
	}

	_ = k.SetSettings(BooksSettings{Country: "custom", Rates: "21, 10", Currency: "EUR"})

	if p := k.Pack(); p.Country != "custom" || len(p.Rates) != 2 || p.Due != 0 {
		t.Errorf("%+v", p)
	}
}

func TestATableOfInvoicesIsImported(t *testing.T) {
	k := keeperFor(t, "")

	path := filepath.Join(t.TempDir(), "продажби.csv")
	_ = os.WriteFile(path, []byte("Номер;Дата;Клиент;ДДС номер;Данъчна основа;Ставка;ДДС;Общо\n"+
		"0000000001;01.09.2026;Алфа;BG123456789;100,00;20;20,00;120,00\n"+
		"0000000002;02.09.2026;Бета;;50,00;9;4,50;54,50\n"+
		"0000000004;03.09.2026;Гама;;10,00;20;3,00;13,00\n"+
		";;;;;;;\n"+
		"Общо;;;;160,00;;27,50;187,50\n"+
		"0000000001;05.09.2026;Повтор;;1;20;0,2;1,2\n"), 0o600)

	im, err := k.Import(context.Background(), path, "", "sale")
	if err != nil {
		t.Fatal(err)
	}

	if im.Kept != 3 || len(im.Skipped) != 1 {
		t.Errorf("%+v", im)
	}

	var all []string
	for _, f := range im.Findings {
		all = append(all, f.Key)
	}

	if strings.Join(all, ",") != "vatMath" {
		t.Errorf("%q", all)
	}

	list, _ := k.Register("", "")
	if len(list) != 3 || list[0].PartyVAT != "BG123456789" || list[1].Date != "2026-09-02" {
		t.Errorf("%+v", list)
	}

	s, _ := k.Summary("2026-09")
	gap := false
	for _, f := range s.Findings {
		if f.Key == "gap" && f.Args["missing"] == "0000000003" {
			gap = true
		}
	}

	if !gap {
		t.Errorf("the missing number was not found: %+v", s.Findings)
	}

	if _, err := k.Import(context.Background(), path, "", "sale"); err != nil {
		t.Fatal(err)
	}

	if again, _ := k.Register("", ""); len(again) != 3 {
		t.Errorf("importing again doubled the register: %d", len(again))
	}
}

func TestAnInvoiceIsReadFromADocument(t *testing.T) {
	k := keeperFor(t, `Here: {"number":"0000000042","date":"2026-09-10","party":"Делта ЕООД","party_vat":"BG204000000",`+
		`"net":"250,00","rate":20,"vat":"50,00","gross":"300,00","currency":"EUR","reverse":false}`)

	doc := filepath.Join(t.TempDir(), "invoice.odt")
	writeODT(t, doc, "ФАКТУРА № 0000000042 от 10.09.2026 …")

	r, err := k.ReadInvoice(context.Background(), doc, "purchase")
	if err != nil || r.Kept || r.Invoice.Number != "0000000042" || r.Invoice.Rate != "20" || r.Invoice.Gross != "300.00" || r.Findings[0].Key != "invoiceOK" {
		t.Fatalf("%v %+v", err, r)
	}

	if _, err := keeperFor(t, "").ReadInvoice(context.Background(), doc, "sale"); !errors.Is(err, ErrNoBooksModel) {
		t.Errorf("%v", err)
	}
}

func TestDatesAreReadInEveryUsualForm(t *testing.T) {
	for in, want := range map[string]string{"2026-09-03": "2026-09-03", "03.09.2026": "2026-09-03", "3.9.2026 г.": "2026-09-03",
		"46268": "2026-09-03", "yesterday": ""} {
		if got := dateOf(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func writeODT(t *testing.T, path, text string) {
	t.Helper()

	f, _ := os.Create(path)
	z := zip.NewWriter(f)
	w, _ := z.Create("content.xml")
	_, _ = w.Write([]byte(`<office:document-content><office:body><office:text><text:p>` + text + `</text:p></office:text></office:body></office:document-content>`))
	z.Close()
	f.Close()
}
