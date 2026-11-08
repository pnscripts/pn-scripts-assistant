package polish

import (
	"strings"
	"testing"
	"time"
)

var today = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC) // a Sunday

func keys(r Result) map[string]int {
	out := map[string]int{}
	for _, f := range r.Findings {
		out[f.Key]++
	}

	return out
}

func TestWhatTheReaderAlreadySeesIsTakenOut(t *testing.T) {
	r := Polish(Draft{
		From:    "Anna Petrova",
		Subject: "Invoice for October",
		Text: "Invoice for October\n\nHi Boris,\n\nThis is Anna writing. I hope this email finds you well. " +
			"The invoice is attached. The invoice is attached.\n\nPlease don't hesitate to contact me.\n\nAnna",
	}, today)

	want := "Hi Boris,\n\nThe invoice is attached.\n\nAnna"
	if r.Text != want {
		t.Fatalf("got\n%q\nwant\n%q", r.Text, want)
	}

	k := keys(r)
	if k[AlreadyVisible] != 2 || k[Filler] != 2 || k[Repeated] != 1 || !r.Changed || r.Blocking {
		t.Errorf("%+v", r)
	}

	for _, f := range r.Findings {
		if !f.Fixed {
			t.Errorf("left open: %+v", f)
		}
	}
}

func TestBulgarianIsPolishedToo(t *testing.T) {
	r := Polish(Draft{
		From: "Мария",
		Text: "Здравейте,\n\nПише ви Мария от магазина. Надявам се, че сте добре. Пратката тръгна днес днес.\n\n" +
			"Не се колебайте да се свържете с мен при въпроси.",
	}, today)

	want := "Здравейте,\n\nПратката тръгна днес."
	if r.Text != want {
		t.Fatalf("got\n%q\nwant\n%q", r.Text, want)
	}

	k := keys(r)
	if k[AlreadyVisible] != 1 || k[Filler] != 2 || k[Slip] < 1 {
		t.Errorf("%+v", r.Findings)
	}
}

func TestAPlaceholderStopsTheTextGoingOut(t *testing.T) {
	for _, text := range []string{
		"Dear [Name], thank you.",
		"Срещата е на {{date}}.",
		"Поздрави, [Име]",
		"TODO: add the price.",
	} {
		r := Polish(Draft{Text: text}, today)
		if !r.Blocking || keys(r)[Placeholder] != 1 {
			t.Errorf("%q: %+v", text, r)
		}
	}

	if r := Polish(Draft{Text: "Thanks, see you on [the usual] terrace."}, today); r.Blocking {
		t.Errorf("an ordinary bracket blocked: %+v", r)
	}
}

func TestADateWithTheWrongWeekdayIsPointedAt(t *testing.T) {
	r := Polish(Draft{Text: "See you on Monday, 10 November 2026 at noon. Or on Tuesday, 10 November 2026."}, today)

	var wrong []Finding

	for _, f := range r.Findings {
		if f.Key == WrongWeekday {
			wrong = append(wrong, f)
		}
	}

	if len(wrong) != 1 || wrong[0].Hint != "Tuesday" || !strings.HasPrefix(wrong[0].Found, "Monday") {
		t.Fatalf("%+v", r.Findings)
	}

	r = Polish(Draft{Text: "Срещата е в понеделник, 10 ноември."}, today)
	if k := keys(r); k[WrongWeekday] != 1 || r.Findings[len(r.Findings)-1].Hint != "вторник" {
		t.Errorf("%+v", r.Findings)
	}

	// Said without a year and already past this year: next year's, so
	// Thursday is right for 4 March (2027) and Wednesday is not.
	if r = Polish(Draft{Text: "Back on Thursday, 4 March."}, today); keys(r)[WrongWeekday] != 0 {
		t.Errorf("%+v", r.Findings)
	}

	if r = Polish(Draft{Text: "Back on Wednesday, 4 March."}, today); keys(r)[WrongWeekday] != 1 {
		t.Errorf("%+v", r.Findings)
	}

	if r = Polish(Draft{Text: "Като езиков модел не мога да подпиша."}, today); keys(r)[MachineVoice] != 1 {
		t.Errorf("%+v", r.Findings)
	}
}

func TestAMachineTalkingAboutItselfIsPointedAt(t *testing.T) {
	r := Polish(Draft{Text: "As an AI, I cannot sign this for you."}, today)
	if keys(r)[MachineVoice] != 1 || r.Text != "As an AI, I cannot sign this for you." {
		t.Errorf("%+v", r)
	}
}

func TestNothingToSendIsSaid(t *testing.T) {
	if r := Polish(Draft{Text: "  \n "}, today); !r.Blocking || keys(r)[Empty] != 1 {
		t.Errorf("%+v", r)
	}
}

func TestAGoodTextIsLeftAlone(t *testing.T) {
	text := "Hi Boris,\n\nThe invoice for October is attached; it is due on Friday, 13 November 2026.\n\nAnna"

	r := Polish(Draft{Text: text, From: "Anna"}, today)
	if r.Text != text || r.Changed || len(r.Findings) != 0 {
		t.Errorf("%+v", r)
	}

	// A sentence that only starts like a filler says something.
	text = "I hope you are well enough to come on Friday."
	if r := Polish(Draft{Text: text}, today); r.Text != text {
		t.Errorf("%q", r.Text)
	}

	// An introduction that goes on to say something stays.
	text = "Hi, this is Anna from the shop, about your order of 3 November."
	if r := Polish(Draft{Text: text, From: "Anna"}, today); r.Text != text {
		t.Errorf("%q", r.Text)
	}

	// Introducing somebody else is not introducing the sender.
	text = "This is Boris, our new accountant."
	if r := Polish(Draft{Text: text, From: "Anna"}, today); r.Text != text {
		t.Errorf("%q", r.Text)
	}
}
