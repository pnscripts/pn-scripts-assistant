package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/books"
)

// The Invoices page: the register, a month's VAT with its rules and
// sources, the threshold; nothing kept twice, nothing filed.
func TestTheInvoicesPage(t *testing.T) {
	ts, _, b := newServer(t)

	b.Router = nil

	var page struct {
		Settings struct{ Country string } `json:"settings"`
		Pack     struct {
			Sources []struct{ URL, Checked string } `json:"sources"`
		} `json:"pack"`
		Packs []map[string]any `json:"packs"`
	}

	getJSON(t, ts.URL+"/api/books", &page)

	if page.Settings.Country != "bg" || len(page.Pack.Sources) == 0 || page.Pack.Sources[0].Checked == "" || len(page.Packs) != 2 {
		t.Fatalf("%+v", page)
	}

	var rec struct {
		Invoice struct {
			ID  int64  `json:"id"`
			VAT string `json:"vat"`
		} `json:"invoice"`
		Kept bool `json:"kept"`
	}

	inv := map[string]any{"kind": "sale", "number": "0000000001", "date": "2026-09-02", "party": "Алфа", "net": "1000", "rate": "20", "keep": true}

	if code := sendJSON(t, ts.URL+"/api/books/invoices", inv, &rec); code != http.StatusOK || !rec.Kept || rec.Invoice.VAT != "200.00" {
		t.Fatalf("%d %+v", code, rec)
	}

	if code := sendJSON(t, ts.URL+"/api/books/invoices", inv, nil); code != http.StatusConflict {
		t.Errorf("a number used twice answered %d", code)
	}

	var sum struct {
		Result string `json:"result"`
		Due    string `json:"due"`
		Watch  struct {
			Sales string `json:"sales"`
		} `json:"watch"`
	}

	if code := sendJSON(t, ts.URL+"/api/books/summary", map[string]any{"period": "2026-09"}, &sum); code != http.StatusOK ||
		sum.Result != "200.00" || sum.Due != "2026-10-14" || sum.Watch.Sales != "1000.00" {
		t.Errorf("%d %+v", code, sum)
	}

	if code := sendJSON(t, ts.URL+"/api/books/summary", map[string]any{"period": "soon"}, nil); code != http.StatusBadRequest {
		t.Errorf("a period that is not one answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/books/read", map[string]any{"path": "/nowhere/x.pdf", "kind": "sale"}, nil); code != http.StatusBadRequest {
		t.Errorf("reading a missing document answered %d", code)
	}

	path := filepath.Join(t.TempDir(), "purchases.csv")
	_ = os.WriteFile(path, []byte("Date,Supplier,Number,Net,Rate,VAT,Total\n2026-09-05,Beta,F-9,100,20,20,120\n"), 0o600)

	var im struct {
		Kept int `json:"kept"`
	}

	if code := sendJSON(t, ts.URL+"/api/books/import", map[string]any{"path": path, "kind": "purchase"}, &im); code != http.StatusOK || im.Kept != 1 {
		t.Errorf("%d %+v", code, im)
	}

	if code := sendJSON(t, ts.URL+"/api/books/settings", map[string]any{"country": "custom", "rates": "21", "currency": "EUR"}, nil); code != http.StatusOK {
		t.Errorf("settings answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/books/invoices/"+strconv.FormatInt(rec.Invoice.ID, 10)+"/remove", map[string]any{}, nil); code != http.StatusOK {
		t.Errorf("remove answered %d", code)
	}
}

// Every kind of finding is said in both languages.
func TestEveryInvoiceFindingIsTranslated(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("assets", "js", "i18n.js"))
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range books.FindingKeys() {
		if !strings.Contains(string(js), "'books.f."+key+"': [") {
			t.Errorf("books.f.%s is not in i18n.js", key)
		}
	}
}
