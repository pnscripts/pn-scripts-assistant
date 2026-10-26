package cite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/store"
)

func fact(id int64, content, source string) store.Scored {
	return store.Scored{Fact: store.Fact{ID: id, Content: content, Source: source}}
}

// Two facts from one report are one source; a thing somebody said is its own.
func TestFactsFromOnePlaceShareANumber(t *testing.T) {
	l := NewLedger()

	numbers := l.Facts([]store.Scored{
		fact(1, "The invoice is due on the 5th.", "document:/home/you/report.pdf"),
		fact(2, "Totals are in euro.", "document:/home/you/report.pdf"),
		fact(3, "Alex prefers short answers.", ""),
	})

	if numbers[0] != 1 || numbers[1] != 1 || numbers[2] != 2 {
		t.Fatalf("numbers %v, expected [1 1 2]", numbers)
	}

	sources := l.Sources()

	if len(sources) != 2 {
		t.Fatalf("%d sources, expected 2: %+v", len(sources), sources)
	}

	if sources[0].Kind != Document || sources[0].Title != "report.pdf" || sources[0].Where != "/home/you/report.pdf" {
		t.Errorf("the report reads %+v", sources[0])
	}

	if sources[1].Kind != Memory || sources[1].FactID != 3 {
		t.Errorf("the remembered thing reads %+v", sources[1])
	}
}

// The model reads the number beside the words, and is told what to do with it.
func TestThePreambleNumbersEveryFactAndNamesThePlace(t *testing.T) {
	l := NewLedger()

	facts := []store.Scored{
		fact(1, "The invoice is due on the 5th.", "document:/home/you/report.pdf"),
		fact(2, "Alex prefers short answers.", ""),
	}

	text := Preamble(facts, l.Facts(facts), 200, func(s string, _ int) string { return s })

	for _, want := range []string{
		"[1] The invoice is due on the 5th. (from: report.pdf)",
		"[2] Alex prefers short answers.\n",
		"never invent one",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the preamble lacks %q:\n%s", want, text)
		}
	}
}

// A page and a file read while answering are sources; a failed read is not.
func TestReadingWhileAnsweringIsNumberedWhereTheModelSeesIt(t *testing.T) {
	l := NewLedger()

	l.Facts([]store.Scored{fact(1, "Something remembered.", "")})

	page := l.Note("read_a_page", json.RawMessage(`{"url":"https://www.example.org/guide/start"}`), "The guide says…")

	if !strings.HasPrefix(page, "Source [2].") {
		t.Errorf("the page reads %q", page)
	}

	if out := l.Note("read_file", json.RawMessage(`{"path":"/x/y.txt"}`), "Error: no such file"); out != "Error: no such file" {
		t.Errorf("a failed read was labelled: %q", out)
	}

	if out := l.Note("write_file", json.RawMessage(`{"path":"/x/y.txt"}`), "written"); out != "written" {
		t.Errorf("a write was labelled as a source: %q", out)
	}

	sources := l.Sources()

	if len(sources) != 2 || sources[1].Kind != Page || sources[1].Title != "example.org/guide/start" {
		t.Fatalf("sources %+v", sources)
	}

	// The same page read twice is one source.
	again := l.Note("fetch_url", json.RawMessage(`{"url":"https://www.example.org/guide/start"}`), "Again.")

	if !strings.HasPrefix(again, "Source [2].") {
		t.Errorf("the same page got a new number: %q", again)
	}
}

// A search's own numbering would collide with the ledger's, so it is replaced.
func TestSearchResultsAreRenumberedAsSources(t *testing.T) {
	l := NewLedger()

	l.Facts([]store.Scored{fact(1, "Remembered.", "")})

	out := l.Note("web_search", json.RawMessage(`{"query":"vat rate"}`),
		"1. VAT rates\n   https://a.example/vat\n   Standard rate is 20 %.\n"+
			"2. Tax office\n   https://b.example/tax\n")

	if !strings.Contains(out, "[2] VAT rates") || !strings.Contains(out, "[3] Tax office") {
		t.Fatalf("results were not renumbered:\n%s", out)
	}

	if strings.Contains(out, "1. VAT rates") {
		t.Errorf("the search's own number was kept:\n%s", out)
	}

	if got := l.Sources(); len(got) != 3 || got[2].Kind != Result || got[2].Where != "https://b.example/tax" {
		t.Errorf("sources %+v", got)
	}
}

// Invented numbers are removed; real ones mark their source as cited.
func TestCheckKeepsRealCitationsAndRemovesInventedOnes(t *testing.T) {
	sources := []Source{{N: 1, Kind: Memory}, {N: 2, Kind: Page}, {N: 3, Kind: Document}}

	reply, out := Check("It is due on the 5th [1]. The rate is 20 % [2, 7]. Nobody said this [9].", sources)

	if reply != "It is due on the 5th [1]. The rate is 20 % [2]. Nobody said this." {
		t.Errorf("checked reply: %q", reply)
	}

	if !out[0].Cited || !out[1].Cited || out[2].Cited {
		t.Errorf("cited flags %+v", out)
	}
}

// Code is code: indexes and anything inside backticks are left alone.
func TestCheckLeavesCodeAlone(t *testing.T) {
	sources := []Source{{N: 1}}

	in := "Use `list[4]` or matrix[2][3], as the guide says [1].\n```\nx = a[7]\n```"

	reply, _ := Check(in, sources)

	if reply != in {
		t.Errorf("code was changed:\n%s", reply)
	}

	// And an answer with no sources at all is not touched.
	if got, _ := Check("Option [5] is best.", nil); got != "Option [5] is best." {
		t.Errorf("an answer with no sources was changed: %q", got)
	}
}

func TestStripRemovesCitationsForSpeech(t *testing.T) {
	if got := Strip("Due on the 5th [1]. Rate 20 % [2, 3]."); got != "Due on the 5th. Rate 20 %." {
		t.Errorf("stripped: %q", got)
	}
}

func TestTheLedgerTravelsInTheContext(t *testing.T) {
	if From(context.Background()) != nil {
		t.Fatal("an empty context has a ledger")
	}

	l := NewLedger()

	if From(With(context.Background(), l)) != l {
		t.Fatal("the ledger was lost on the way")
	}

	// A nil ledger notes nothing and changes nothing.
	var none *Ledger

	if out := none.Note("read_a_page", json.RawMessage(`{"url":"https://x.example"}`), "text"); out != "text" {
		t.Errorf("a nil ledger changed the output: %q", out)
	}
}

// A research report numbers its sources from 1; in a turn that already has
// a [1], they are given the turn's numbers, in the text and in the list.
func TestAResearchReportTakesTheTurnsNumbers(t *testing.T) {
	l := NewLedger()

	l.Facts([]store.Scored{fact(1, "Alex lives in Sofia.", "")})

	out := l.Note("research", json.RawMessage(`{"question":"q"}`),
		"## Short answer\nThe rate is 20 % [1]. Hotels pay 9 % [2][1].\n\n## Sources\n\n"+
			"1. [VAT rates](https://nra.bg/vat) — official body\n2. [Hotels [2024]](https://news.example.com/h) — other site\n")

	for _, want := range []string{"20 % [2].", "9 % [3][2].", "2. [VAT rates](https://nra.bg/vat)", "3. [Hotels"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}

	if got := l.Sources(); len(got) != 3 || got[1].Where != "https://nra.bg/vat" || got[1].Kind != Page {
		t.Errorf("%+v", got)
	}
}
