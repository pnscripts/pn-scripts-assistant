package glossary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTermsAreAddedReplacedAndRemoved(t *testing.T) {
	root := t.TempDir()

	if terms, err := Load(root); err != nil || len(terms) != 0 {
		t.Fatalf("a new brain's glossary: %v %v", terms, err)
	}

	if _, replaced, err := Add(root, Term{Source: " invoice ", Target: "фактура", From: "en", To: "bg"}); err != nil || replaced {
		t.Fatal(replaced, err)
	}

	if _, replaced, err := Add(root, Term{Source: "Invoice", Target: "фактура (данъчна)", From: "en", To: "bg"}); err != nil || !replaced {
		t.Fatal("the same term in other letters did not replace the first", err)
	}

	if _, _, err := Add(root, Term{Source: "PN Scripts", Target: "PN Scripts", To: "bg"}); err != nil {
		t.Fatal(err)
	}

	terms, _ := Load(root)
	if len(terms) != 2 || terms[0].Target != "фактура (данъчна)" && terms[1].Target != "фактура (данъчна)" {
		t.Fatalf("%+v", terms)
	}

	if info, err := os.Stat(filepath.Join(root, FileName)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the glossary file is not private: %v %v", info, err)
	}

	if found, err := Remove(root, Term{Source: "invoice", From: "en", To: "bg"}); !found || err != nil {
		t.Fatal(found, err)
	}

	if found, _ := Remove(root, Term{Source: "nothing", To: "bg"}); found {
		t.Error("removing a term that is not there reported it found")
	}

	terms, _ = Load(root)
	if len(terms) != 1 {
		t.Errorf("%+v", terms)
	}
}

func TestBadTermsAreRefused(t *testing.T) {
	for _, bad := range []Term{
		{Source: "", Target: "x", To: "bg"},
		{Source: "x", Target: "", To: "bg"},
		{Source: "x", Target: "y", To: "bulgarian"},
		{Source: "x", Target: "y", From: "bg", To: "bg"},
		{Source: strings.Repeat("x", 201), Target: "y", To: "bg"},
	} {
		if _, _, err := Add(t.TempDir(), bad); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
}

func TestATranslationIsCheckedAgainstTheGlossary(t *testing.T) {
	terms := []Term{
		{Source: "invoice", Target: "фактура", From: "en", To: "bg"},
		{Source: "contract", Target: "договор", To: "bg"},
		{Source: "PN Scripts", Target: "PN Scripts", To: "bg"},
		{Source: "receipt", Target: "разписка", To: "bg"},
		{Source: "car", Target: "кола", From: "de", To: "bg"},
	}

	original := "Send the invoice and the contract to office@example.com by 12.5 at https://example.com/a?b=1. PN Scripts. A carpet."
	good := "Изпратете фактурата и договора на office@example.com до 12,5 на https://example.com/a?b=1. PN Scripts. Килим."
	bad := "Изпратете сметката и договора на офиса до 13 на сайта. ПН Скриптс."

	pair := For(terms, "en", "bg")
	if len(pair) != 4 {
		t.Fatalf("for en→bg: %+v", pair)
	}

	if miss := Check(original, good, pair); len(miss) != 0 {
		t.Errorf("a good translation missed %+v", miss)
	}

	miss := Check(original, bad, pair)

	var got []string
	for _, m := range miss {
		got = append(got, m.Kind+":"+m.Source)
	}

	want := "term:invoice term:PN Scripts kept:office@example.com kept:12.5 kept:https://example.com/a?b=1"
	if strings.Join(got, " ") != want {
		t.Errorf("missed %v\nwant %s", got, want)
	}
}

func TestGlossariesGoInAndOutAsSpreadsheets(t *testing.T) {
	terms := []Term{{Source: "invoice, tax", Target: "данъчна фактура", From: "en", To: "bg", Note: "закон за ДДС"}}

	back, problems := FromCSV(CSV(terms), "", "")
	if len(problems) != 0 || len(back) != 1 || back[0].Source != "invoice, tax" || back[0].Note != "закон за ДДС" {
		t.Fatalf("%+v %v", back, problems)
	}

	typed := "договор;contract\nфактура;invoice;bg;en;\nбез превод\n;празно\n"

	got, problems := FromCSV(typed, "bg", "en")
	if len(got) != 2 || got[0].To != "en" || got[0].From != "bg" || len(problems) != 2 {
		t.Errorf("%+v %v", got, problems)
	}
}
