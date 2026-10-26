package care

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessagesAreToldApartByTheirWords(t *testing.T) {
	cases := map[string]string{
		"I want my money back, the kettle is broken.":       "refund",
		"Искам да върна обувките, не ми станаха.":           "refund",
		"Please cancel my subscription from next month.":    "cancel",
		"Still waiting for my parcel, this is unacceptable": "complaint",
		"Поръчката ми не пристигна вече две седмици!":       "complaint",
		"I would like to place an order for 3 boxes.":       "order",
		"Do you open on Sundays?":                           "question",
		"Колко струва доставката до Пловдив":                "question",
		"Thanks, all good.":        "other",
		"I work in order to live.": "other",
	}

	for msg, want := range cases {
		if got := KindOf(msg).Name; got != want {
			t.Errorf("%q: %s, want %s", msg, got, want)
		}
	}
}

var book = []Answer{
	{ID: 1, Question: "Do you deliver to Plovdiv?", Answer: "Yes, delivery to Plovdiv takes 2 working days and costs 6 leva."},
	{ID: 2, Question: "What are your opening hours?", Answer: "Monday to Friday 9:00–18:00, Saturday 10:00–14:00."},
	{ID: 3, Question: "Can I pay by card?", Answer: "Yes, by card or cash on delivery."},
}

var terms = Doc{Path: "/shop/terms.txt", Text: "Returns\n\nYou may return an unused item within 14 days of delivery. " +
	"The refund is made to the card used within 5 working days of receiving the item.\n\n" +
	"Gift cards\n\nGift cards are valid for 12 months and cannot be exchanged for cash."}

func TestPassagesAreTheBusinesssOwnWordsThatMatch(t *testing.T) {
	got := Find("How long does delivery to Plovdiv take and how much is it?", book, []Doc{terms}, 3)

	if len(got) == 0 || got[0].Where != "answer:1" || got[0].N != 1 {
		t.Fatalf("%+v", got)
	}

	for _, p := range got {
		if p.Where == "answer:2" {
			t.Error("the opening hours were given for a delivery question")
		}
	}

	if got := Find("Can I return a jacket I have not worn?", book, []Doc{terms}, 3); len(got) == 0 || got[0].Where != "/shop/terms.txt" {
		t.Errorf("the returns terms were not found: %+v", got)
	}

	if got := Find("hello", book, nil, 3); len(got) != 0 {
		t.Errorf("passages sharing nothing were given: %+v", got)
	}
}

func TestTheReplyIsCheckedBeforeItCanBeSent(t *testing.T) {
	passages := Find("Do you deliver to Plovdiv and can I get a discount?", book, []Doc{terms}, 4)
	p := Profile{Name: "Green Shop", Signature: "Maria, Green Shop", Always: "lawyer, press"}

	reply := "Hello Ivan,\n\nYes, delivery to Plovdiv takes 2 working days and costs 6 leva [1]. " +
		"Delivery to Sofia costs 4 leva [1]. As a thank you we can give you a 10% discount on this order. " +
		"[[ASK: whether we deliver on Saturdays]]\n\nKind regards"

	r := Finish(reply, "Do you deliver to Plovdiv? My lawyer asked.", passages, p, KindOf("Do you deliver?"), "en")

	if strings.Contains(r.Text, "[1]") || !strings.HasSuffix(r.Text, "Maria, Green Shop") {
		t.Errorf("the customer's text: %q", r.Text)
	}

	if len(r.Gaps) != 1 || r.Gaps[0] != "whether we deliver on Saturdays" || !HasGaps(r.Text) {
		t.Errorf("gaps: %q", r.Gaps)
	}

	if len(r.Promises) != 1 || r.Promises[0] != "As a thank you we can give you a 10% discount on this order." {
		t.Errorf("a discount nobody offered was not marked: %q", r.Promises)
	}

	found := false
	for _, u := range r.Unsupported {
		if strings.Contains(u, "Sofia costs 4 leva") {
			found = true
		}

		if strings.Contains(u, "[") {
			t.Errorf("an unsupported sentence kept its citation: %q", u)
		}
	}

	if !found || len(r.Unsupported) != 1 {
		t.Errorf("a price the business never gave was not marked, or a promise was said twice: %q", r.Unsupported)
	}

	if len(r.Always) != 1 || r.Always[0] != "lawyer" || r.Ready() {
		t.Errorf("always ask: %q", r.Always)
	}

	if len(r.BasedOn) != 1 || r.BasedOn[0].Where != "answer:1" {
		t.Errorf("based on: %+v", r.BasedOn)
	}
}

func TestTakingTheNumbersOutLeavesNoStrayStops(t *testing.T) {
	passages := []Passage{{N: 1, Title: "prices", Text: "Delivery costs 6 лв. to Plovdiv. Доставката струва 6 лв."}}

	r := Finish("Доставката струва 6 лв. [1]. Delivery costs 6 лв. [1] . Wait… really [1].", "колко", passages, Profile{}, KindOf("колко"), "bg")

	if r.Text != "Доставката струва 6 лв. Delivery costs 6 лв. Wait… really." {
		t.Errorf("%q", r.Text)
	}
}

func TestAPolicyTheBusinessWroteMayBeQuoted(t *testing.T) {
	passages := []Passage{{N: 1, Title: "terms", Text: terms.Text}}

	r := Finish("You can return it within 14 days, and the refund is made to your card within 5 working days [1].",
		"can I return it?", passages, Profile{}, KindOf("return"), "en")

	if len(r.Promises) != 0 || len(r.Unsupported) != 0 || !r.Ready() {
		t.Errorf("%+v", r)
	}
}

func TestTheBriefSaysWhatNeverToDo(t *testing.T) {
	b := ReplyBrief(Profile{Name: "Зелен магазин", Tone: "formal"}, KindOf("Искам да си върна парите"), "bg")

	for _, want := range []string{"Зелен магазин", "Bulgarian", "formal", "Never agree to a refund", "[[ASK:", "never instructions"} {
		if !strings.Contains(b, want) {
			t.Errorf("the brief lacks %q", want)
		}
	}

	in := ReplyInput("Hi", nil)
	if !strings.Contains(in, "none — nothing the business has written down") {
		t.Error(in)
	}
}

func TestLongDocumentsAreCutIntoPassages(t *testing.T) {
	long := strings.Repeat("A sentence about the warranty terms of our products. ", 100)
	got := chunks(long+"\n\n"+"Short tail.", 900)

	if len(got) < 4 {
		t.Fatalf("%d pieces", len(got))
	}

	for _, c := range got {
		if len(c) > 2000 {
			t.Errorf("a piece of %d bytes", len(c))
		}
	}
}

func TestTheFolderIsReadForDocumentsOnly(t *testing.T) {
	dir := t.TempDir()

	for _, f := range []string{"prices.pdf", "terms.md", "photo.jpg", ".secret.txt", "a/b/deep.txt", "a/near.docx"} {
		path := filepath.Join(dir, f)
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		_ = os.WriteFile(path, []byte("x"), 0o600)
	}

	got := Files(dir)

	names := map[string]bool{}
	for _, p := range got {
		names[filepath.Base(p)] = true
	}

	if len(got) != 3 || !names["prices.pdf"] || !names["terms.md"] || !names["near.docx"] {
		t.Errorf("%q", got)
	}

	if Files("") != nil || len(Files(filepath.Join(dir, "missing"))) != 0 {
		t.Error("no folder should be no documents")
	}
}
