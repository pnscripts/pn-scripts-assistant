package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/research"
	"pn-scripts-assistant/internal/brain/store"
)

type keptResearch struct {
	mu   sync.Mutex
	list []store.ResearchReport
}

func (k *keptResearch) SaveResearch(r store.ResearchReport) (store.ResearchReport, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	r.ID = int64(len(k.list) + 1)
	k.list = append(k.list, r)

	return r, nil
}

// fakeWeb is a search engine and the pages it finds.
type fakeWeb struct {
	mu      sync.Mutex
	queries []string
	fetched []string
}

var fakePages = map[string]string{
	"https://nra.bg/vat": "Updated 2025-02-01. The standard VAT rate in Bulgaria is 20 %. A reduced rate of 9 % " +
		"applies to hotel accommodation. Registration is compulsory once taxable turnover reaches 166 000 leva " +
		"in twelve months. " + strings.Repeat("Further guidance on invoices and returns. ", 10),
	"https://blog.example.com/vat": strings.Repeat("Our opinion on taxes. ", 20),
}

func (w *fakeWeb) search(_ context.Context, q string) ([]SearchResult, error) {
	w.mu.Lock()
	w.queries = append(w.queries, q)
	w.mu.Unlock()

	return []SearchResult{
		{Title: "Blog", URL: "https://blog.example.com/vat", Snippet: "opinion"},
		{Title: "VAT rates — NRA", URL: "https://nra.bg/vat", Snippet: "rates"},
		{Title: "Gone", URL: "https://gone.example.org/x", Snippet: "A page that is gone but whose snippet says a little about VAT."},
	}, nil
}

func (w *fakeWeb) fetch(_ context.Context, u string) (string, error) {
	w.mu.Lock()
	w.fetched = append(w.fetched, u)
	w.mu.Unlock()

	if p, ok := fakePages[u]; ok {
		return p, nil
	}

	return "", errors.New("404")
}

// writer answers each brief the way a model would, telling them apart by
// the system prompt.
func writer(report string, asked *[]string) func(context.Context, string, string) (string, string, error) {
	var mu sync.Mutex

	return func(_ context.Context, system, user string) (string, string, error) {
		mu.Lock()
		*asked = append(*asked, system)
		mu.Unlock()

		switch {
		case strings.HasPrefix(system, "You plan web research"):
			return `["Bulgaria VAT rate", "НАП ДДС ставка"]`, "m", nil
		case strings.HasPrefix(system, "You write a research report"):
			return report, "writer-model", nil
		case strings.HasPrefix(system, "You are a fact checker"):
			claim, _, _ := strings.Cut(user, "\n")
			if strings.Contains(claim, "Hotels") {
				return "SUPPORTED: the source names hotel accommodation at 9 %", "m", nil
			}

			return "NOT SUPPORTED: the source does not say this", "m", nil
		}

		return "", "", errors.New("unexpected brief")
	}
}

func TestResearchReadsTheBestSourcesWritesAndChecks(t *testing.T) {
	web := &fakeWeb{}
	book := &keptResearch{}

	var asked []string

	report := "## Short answer\nThe standard VAT rate in Bulgaria is 20 % [1]. Hotels enjoy generous discounts granted by treasury ministers personally [1].\n\n" +
		"## What the sources say\n- Registration is compulsory from 100 000 leva of turnover in twelve months [1].\n" +
		"- Bloggers think taxes are too high everywhere in Europe [5].\n\n" +
		"## Where they disagree\n—\n\n## What is not known\nNothing about next year.\n"

	r := Researching{
		Book: book, Search: web.search, Fetch: web.fetch, Ask: writer(report, &asked),
		Now: func() time.Time { return time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC) },
	}

	done, err := r.Research(context.Background(), "What is the VAT rate in Bulgaria?", "quick", "en")
	if err != nil {
		t.Fatal(err)
	}

	if len(web.queries) != 3 || web.queries[0] != "What is the VAT rate in Bulgaria?" {
		t.Errorf("searched %q", web.queries)
	}

	if len(done.Sources) != 3 || done.Sources[0].URL != "https://nra.bg/vat" || done.Sources[0].Date != "2025-02-01" {
		t.Fatalf("the official source should be first and dated: %+v", done.Sources)
	}

	if !done.Sources[2].Partial {
		t.Errorf("a page that could not be read should be kept as its snippet only: %+v", done.Sources[2])
	}

	if done.Summary.Removed != 1 {
		t.Errorf("the citation to source 5 should have been removed: %+v", done.Summary)
	}

	// The hotel sentence was weak by rule and confirmed by the fact checker;
	// the 100 000 figure is not in the source and the checker agrees.
	for _, c := range done.Claims {
		switch {
		case strings.HasPrefix(c.Text, "Hotels"):
			if c.Verdict != research.Confirmed || c.Flagged() {
				t.Errorf("%+v", c)
			}
		case strings.HasPrefix(c.Text, "Registration"):
			if c.Status != research.Number || !c.Flagged() {
				t.Errorf("%+v", c)
			}
		}
	}

	if len(book.list) != 1 || book.list[0].Model != "writer-model" || !strings.Contains(book.list[0].Report, "## Claims check") ||
		!strings.Contains(book.list[0].Report, "1. [VAT rates — NRA](https://nra.bg/vat) — official body · dated 2025-02-01 · read 2026-10-26") {
		t.Fatalf("%+v", book.list)
	}

	// Sources are kept with what was read of them, so they can be checked again.
	var kept []research.Source
	if json.Unmarshal(book.list[0].Sources, &kept) != nil || kept[0].Text == "" {
		t.Error("the sources were not kept with their text")
	}
}

func TestResearchSaysPlainlyWhyItCannot(t *testing.T) {
	web := &fakeWeb{}

	var asked []string

	ask := writer("x", &asked)

	if _, err := (Researching{Search: web.search, Ask: ask, Web: func() bool { return false }}).
		Research(context.Background(), "q?", "", ""); !errors.Is(err, ErrNoWeb) {
		t.Errorf("with the web off: %v", err)
	}

	if _, err := (Researching{Search: web.search}).Research(context.Background(), "q?", "", ""); !errors.Is(err, ErrNoResearchModel) {
		t.Errorf("with no model: %v", err)
	}

	if _, err := (Researching{Search: web.search, Ask: ask}).Research(context.Background(), "q?", "deep", ""); err == nil {
		t.Error("an unknown depth was accepted")
	}

	nothing := func(context.Context, string) ([]SearchResult, error) { return nil, nil }
	if _, err := (Researching{Search: nothing, Ask: ask}).Research(context.Background(), "q?", "", ""); !errors.Is(err, ErrNothingRead) {
		t.Errorf("with nothing found: %v", err)
	}
}

func TestCheckClaimsReadsFilesAndKeepsNothing(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes.md")

	if err := os.WriteFile(notes, []byte("The museum opened in 1892 and has 12 000 items."), 0o600); err != nil {
		t.Fatal(err)
	}

	var asked []string

	r := Researching{Ask: writer("", &asked), Web: func() bool { return false }}

	done, err := r.CheckClaims(context.Background(),
		"The museum opened in 1892 [1]. It holds 15 000 items in its collection [1]. Its web page is popular with tourists [2].",
		[]string{notes, "https://example.com/page"}, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(done.Unread) != 1 || !strings.Contains(done.Unread[0], "privacy") {
		t.Errorf("a web source with the web off should be reported unread: %q", done.Unread)
	}

	if len(done.Claims) != 3 || done.Claims[0].Flagged() || !done.Claims[1].Flagged() || !done.Claims[2].Flagged() {
		t.Errorf("%+v", done.Claims)
	}

	for _, s := range done.Sources {
		if s.Text != "" {
			t.Error("the sources' text was handed back")
		}
	}

	out, err := CheckClaimsTool{With: r}.Execute(context.Background(),
		json.RawMessage(`{"text":"The museum opened in 1892 [1].","sources":["`+notes+`"]}`))
	if err != nil || !strings.Contains(out, "1 claims checked: 1 supported") || !strings.Contains(out, "OK: The museum") {
		t.Errorf("%v %s", err, out)
	}

	if _, err := r.CheckClaims(context.Background(), "x [1]", []string{"relative/path.txt"}, ""); err != nil {
		t.Fatal(err)
	}
}

func TestTheResearchToolHidesWithTheWebAndSaysWhatItSends(t *testing.T) {
	var tool Tool = ResearchTool{}

	if l, ok := tool.(interface{ LeavesTheMachine() bool }); !ok || !l.LeavesTheMachine() {
		t.Error("research must hide with the web")
	}

	if leaves, what := Leaves(tool); !leaves || !strings.Contains(what, "search") {
		t.Errorf("%v %q", leaves, what)
	}

	if got := Reaches(CheckClaimsTool{}, json.RawMessage(`{"sources":["https://www.nra.bg/x","/home/you/a.txt"]}`)); got != "nra.bg" {
		t.Errorf("%q", got)
	}
}
