package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheInvoiceRegister(t *testing.T) {
	db := openDeeds(t)

	if _, err := db.AddInvoice(Invoice{Kind: "gift", Date: "2026-09-01"}); err == nil {
		t.Error("an invoice of no kind was kept")
	}

	if _, err := db.AddInvoice(Invoice{Kind: "sale", Date: "1.9.2026"}); err == nil {
		t.Error("a date that is not YYYY-MM-DD was kept")
	}

	a, _ := db.AddInvoice(Invoice{Kind: "sale", Number: "0000000001", Date: "2026-09-02", Party: "Алфа", Net: "100.00", Rate: "20", VAT: "20.00", Gross: "120.00"})
	_, _ = db.AddInvoice(Invoice{Kind: "purchase", Number: "F1", Date: "2026-08-30", Party: "Бета", Net: "10.00", Rate: "9", VAT: "0.90", Gross: "10.90", Reverse: true})
	_, _ = db.AddInvoice(Invoice{Kind: "sale", Number: "0000000002", Date: "2026-10-01", Net: "1"})

	all, _ := db.Invoices("", "")
	if len(all) != 3 || all[0].Number != "F1" || !all[0].Reverse {
		t.Fatalf("%+v", all)
	}

	sept, _ := db.Invoices("2026-09-01", "2026-09-30")
	if len(sept) != 1 || sept[0].ID != a.ID || sept[0].Gross != "120.00" {
		t.Errorf("%+v", sept)
	}

	if err := db.RemoveInvoice(a.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.RemoveInvoice(a.ID); !errors.Is(err, ErrNoInvoice) {
		t.Errorf("%v", err)
	}

	_ = db.SetBooksSettings(map[string]string{"country": "bg", "vat_id": "BG123456789"})
	_ = db.SetBooksSettings(map[string]string{"vat_id": ""})

	if s, _ := db.BooksSettings(); s["country"] != "bg" || len(s) != 1 {
		t.Errorf("%v", s)
	}
}

func TestTheInvoiceRegisterIsSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	_, _ = d.AddInvoice(Invoice{Kind: "sale", Number: "0000004711", Date: "2026-09-02", Party: "Petrov Consulting", PartyVAT: "BG987654321",
		Net: "98765.43", VAT: "19753.09", Gross: "118518.52", Note: "secret project"})
	_ = d.SetBooksSettings(map[string]string{"vat_id": "BG111222333"})

	if back, _ := d.Invoices("", ""); len(back) != 1 || back[0].Party != "Petrov Consulting" {
		t.Fatalf("%+v", back)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	for _, word := range []string{"0000004711", "Petrov Consulting", "BG987654321", "98765.43", "118518.52", "secret project", "BG111222333"} {
		if strings.Contains(string(raw)+string(wal), word) {
			t.Errorf("%q is readable on disk", word)
		}
	}
}
