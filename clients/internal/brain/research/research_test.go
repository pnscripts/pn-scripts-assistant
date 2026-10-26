package research

import (
	"strings"
	"testing"
	"time"
)

func TestSourcesAreGradedByWhatKindOfPlaceTheyAre(t *testing.T) {
	cases := map[string]string{
		"https://ec.europa.eu/eurostat/web/main":       "official",
		"https://www.nsi.bg/bg/content/2815":           "official",
		"https://www.gov.uk/vat-rates":                 "official",
		"https://data.census.gov/table":                "official",
		"https://www.mit.edu/research":                 "scholarly",
		"https://www.ox.ac.uk/news":                    "scholarly",
		"https://pubmed.ncbi.nlm.nih.gov/123/":         "scholarly",
		"https://en.wikipedia.org/wiki/Bulgaria":       "reference",
		"https://bg.wikipedia.org/wiki/България":       "reference",
		"https://www.reuters.com/world/":               "news",
		"https://www.reddit.com/r/bulgaria/":           "forum",
		"https://old.reddit.com/r/x":                   "forum",
		"https://developer.mozilla.org/en-US/docs/Web": "standards",
		"https://example.com/blog/post":                "other",
		"https://shop.example.com/item":                "commercial",
		"not a url":                                    "other",
	}

	for u, want := range cases {
		if got := GradeURL(u).Kind; got != want {
			t.Errorf("%s graded %s, want %s", u, got, want)
		}
	}

	if GradeURL("https://europa.eu").Score <= GradeURL("https://reddit.com").Score {
		t.Error("an official body should outrank a forum")
	}
}

func TestPickReadsBetterPlacesFirstAndNotOneSiteOverAndOver(t *testing.T) {
	hits := []Hit{
		{Title: "a", URL: "https://www.reddit.com/r/a"},
		{Title: "b", URL: "https://blog.example.com/1"},
		{Title: "c", URL: "https://blog.example.com/2"},
		{Title: "d", URL: "https://blog.example.com/3"},
		{Title: "e", URL: "https://ec.europa.eu/x"},
		{Title: "e again", URL: "https://ec.europa.eu/x/"},
		{Title: "f", URL: "ftp://files.example.com/x"},
		{Title: "g", URL: "http://plain.example.org/x"},
	}

	got := Pick(hits, 5)

	if got[0].URL != "https://ec.europa.eu/x" {
		t.Errorf("the official page should come first: %+v", got)
	}

	blog := 0

	for _, h := range got {
		if strings.Contains(h.URL, "blog.example.com") {
			blog++
		}

		if strings.HasPrefix(h.URL, "ftp:") {
			t.Error("an address that is not a web page was picked")
		}
	}

	if blog != 2 {
		t.Errorf("one site picked %d times, want at most 2", blog)
	}

	if len(got) != 5 || got[len(got)-1].Title != "a" {
		t.Errorf("the forum should be read last: %+v", got)
	}
}

func TestThePlanAlwaysSearchesTheQuestion(t *testing.T) {
	q := "What is the VAT rate in Bulgaria?"

	got := ParsePlan("Sure! Here you go:\n```json\n[\"Bulgaria VAT rate 2026\", \"НАП ДДС ставка\", \"Bulgaria VAT rate 2026\"]\n```", q, 3)
	if len(got) != 3 || got[0] != q || got[1] != "Bulgaria VAT rate 2026" || got[2] != "НАП ДДС ставка" {
		t.Errorf("%q", got)
	}

	if got := ParsePlan("I cannot help with that.", q, 3); len(got) < 1 || got[0] != q {
		t.Errorf("an unreadable plan: %q", got)
	}

	if got := ParsePlan("1. first query\n2. second query", q, 5); len(got) != 3 {
		t.Errorf("a numbered list: %q", got)
	}
}

func TestDatesNearTheTopAreFoundAndNeverGuessed(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)

	cases := map[string]string{
		"Published 2025-03-14 by staff":           "2025-03-14",
		"Обновено на 14.03.2025 г.":               "2025-03-14",
		"14 March 2025 — The ministry said":       "2025-03-14",
		"March 14, 2025. Report":                  "2025-03-14",
		"Публикувано на 3 октомври 2024":          "2024-10-03",
		"No date here at all":                     "",
		"Founded in 1999 and still going":         "",
		"Event on 2031-01-01 is in the future":    "",
		"Not a day: 2025-02-30, real: 01.02.2025": "2025-02-01",
	}

	for text, want := range cases {
		if got := FindDate(text, now); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}

	if got := FindDate(strings.Repeat("x ", 1500)+"2024-01-01", now); got != "" {
		t.Errorf("a date deep in the page was taken for the page's own: %q", got)
	}
}

func sources() []Source {
	return []Source{
		{N: 1, Title: "VAT rates in Bulgaria", URL: "https://nra.bg/vat", Grade: GradeURL("https://nra.bg/vat"),
			Text: "The standard VAT rate in Bulgaria is 20 %. A reduced rate of 9 % applies to hotel accommodation. " +
				"Registration is compulsory once taxable turnover reaches 166 000 leva in twelve months."},
		{N: 2, Title: "Restaurant VAT", URL: "https://news.example.com/vat", Grade: GradeURL("https://news.example.com/vat"),
			Text: "From 1 January 2025 restaurant and catering services returned to the standard rate of 20 %."},
	}
}

func TestTheCheckFlagsWhatTheSourcesDoNotSay(t *testing.T) {
	report := "## Short answer\n" +
		"The standard VAT rate in Bulgaria is 20 % [1]. Hotel accommodation is taxed at a reduced rate of 9 % [1].\n\n" +
		"## What the sources say\n" +
		"- Registration becomes compulsory when taxable turnover reaches 100 000 leva in twelve months [1].\n" +
		"- Restaurant services went back to the standard rate from January 2025 [2][7].\n" +
		"- Books are taxed at a reduced rate of nine percent across the whole country.\n" +
		"- Farmers enjoy special agricultural compensation schemes for livestock production [2].\n\n" +
		"## Where they disagree\n—\n\n" +
		"## What is not known\nThe sources do not say whether the rate will change next year.\n"

	fixed, claims, sum := Check(report, sources(), "en")

	if strings.Contains(fixed, "[7]") || sum.Removed != 1 {
		t.Errorf("a citation to a source that does not exist was kept: removed %d\n%s", sum.Removed, fixed)
	}

	status := map[string]string{}
	for _, c := range claims {
		status[c.Text[:20]] = c.Status
	}

	want := map[string]string{
		"The standard VAT rat": Supported,
		"Hotel accommodation ": Supported,
		"Registration becomes": Number,
		"Restaurant services ": Supported,
		"Books are taxed at a": Unsourced,
		"Farmers enjoy specia": Weak,
	}

	for k, v := range want {
		if status[k] != v {
			t.Errorf("%q: got %q, want %q (%+v)", k, status[k], v, claims)
		}
	}

	for _, c := range claims {
		if strings.HasPrefix(c.Text, "The sources do not say") {
			t.Error("what is not known was checked as a claim")
		}
	}

	if sum.Claims != 6 || sum.Supported != 3 || sum.Flagged != 3 {
		t.Errorf("%+v", sum)
	}
}

// A short sentence is still a claim, and one whose only citation pointed
// nowhere is a claim with no source.
func TestShortClaimsAreStillClaims(t *testing.T) {
	src := []Source{{N: 1, Title: "x", Text: "The rate is 20 %."}}

	_, claims, _ := Check("The rate is 20 % [1]. Hotels pay 30 % [1]. Everyone pays the same [2]. See below.", src, "en")

	if len(claims) != 3 || claims[0].Status != Supported || claims[1].Status != Number || claims[2].Status != Unsourced {
		t.Errorf("%+v", claims)
	}
}

func TestFiguresMatchHoweverTheyAreWritten(t *testing.T) {
	src := []Source{{N: 1, Title: "x", Text: "Population: 6 445 481 people; growth 1,50 % in 2024."}}

	_, claims, _ := Check("## Short answer\nThe country had 6,445,481 people and grew by 1.5 % in 2024 [1].", src, "en")

	if len(claims) != 1 || claims[0].Status != Supported {
		t.Errorf("%+v", claims)
	}
}

func TestBulgarianReportsAreCheckedToo(t *testing.T) {
	src := []Source{{N: 1, Title: "ДДС", Text: "Стандартната ставка на ДДС в България е 20 %, а за настаняване в хотели е 9 %."}}

	report := "## Накратко\nСтандартната ставка на ДДС в България е 20 % [1]. За настаняване в хотели ставката е 7 % [1].\n" +
		"## Какво не е известно\nИзточниците не казват нищо за следващата година и промените.\n"

	_, claims, sum := Check(report, src, "bg")

	if len(claims) != 2 || claims[0].Status != Supported || claims[1].Status != Number {
		t.Errorf("%+v", claims)
	}

	out := Render("Каква е ставката на ДДС?", report, "bg", src, claims, sum, time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC))

	for _, want := range []string{"# Каква е ставката на ДДС?", "## Проверка на твърденията", "число, което го няма", "## Източници", "1. [ДДС]", "прочетен"} {
		if !strings.Contains(out, want) {
			t.Errorf("the Bulgarian report lacks %q:\n%s", want, out)
		}
	}
}

func TestSentencesKeepTheirCitations(t *testing.T) {
	got := sentences("The rate is 20 %. [1] It rose in 2025 [2]. Dr. Smith agreed [1].")

	if len(got) != 3 || got[0] != "The rate is 20 %. [1]" || got[2] != "Dr. Smith agreed [1]." {
		t.Errorf("%q", got)
	}
}

func TestCitationsAreReadInEveryForm(t *testing.T) {
	text, removed := FixCitations("a [1, 3] b [2-4] c [9] d [1;2]", 4)

	if text != "a [1][3] b [2][3][4] c  d [1][2]" || removed != 1 {
		t.Errorf("%q %d", text, removed)
	}
}

func TestTheFactCheckerIsShownTheEvidenceAndItsVerdictIsRead(t *testing.T) {
	long := strings.Repeat("Unrelated filler about weather and sport. ", 200) +
		"The standard VAT rate in Bulgaria is 20 percent. " + strings.Repeat("More filler text here. ", 200)

	src := []Source{{N: 1, Title: "x", Text: long}}
	c := Claim{Text: "The standard VAT rate in Bulgaria is 20 % [1].", Cites: []int{1}}

	in := JudgeInput(c, src)
	if !strings.Contains(in, "standard VAT rate in Bulgaria is 20 percent") || len(in) > 2000 {
		t.Errorf("the passage shown is not the one the claim came from (%d bytes)", len(in))
	}

	cases := map[string]string{
		"SUPPORTED: the source says 20 %":       Confirmed,
		"**Not supported** — the source says 9": NotSupported,
		"PARTLY: right rate, wrong year":        Partly,
		"I think so":                            "",
	}

	for reply, want := range cases {
		if got, _ := ParseVerdict(reply); got != want {
			t.Errorf("%q: %q, want %q", reply, got, want)
		}
	}

	c.Status, c.Verdict = Weak, Confirmed
	if c.Flagged() {
		t.Error("a claim the fact checker confirmed is still flagged")
	}

	c.Status, c.Verdict = Supported, NotSupported
	if !c.Flagged() || Recount([]Claim{c}, 0).Flagged != 1 {
		t.Error("a claim the fact checker rejected is not flagged")
	}
}

func TestCutNeverBreaksALetter(t *testing.T) {
	got := Cut(strings.Repeat("думи ", 100), 101)
	if !strings.HasSuffix(got, "…") || len(got) > 110 {
		t.Errorf("%q", got)
	}

	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatal("a letter was broken")
		}
	}
}
