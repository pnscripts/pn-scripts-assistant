package rules

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStandingInstructionsAreNoticedInBothLanguages(t *testing.T) {
	cases := []struct {
		said    string
		text    string
		kind    Kind
		actions []string
	}{
		{"Always ask me before you send an email.", "Always ask me before you send an email.", AskFirst, []string{Send}},
		{"From now on, never delete files without asking me", "From now on, never delete files without asking me.", AskFirst, []string{Delete}},
		{"Never post on Sundays.", "Never post on Sundays.", Never, []string{Post}},
		{"please never push to main", "Please never push to main.", Never, []string{Commit}},
		{"Thanks. Never send emails to anna@example.org.", "Never send emails to anna@example.org.", Never, []string{Send}},
		{"It's pointless to write that you are writing on my behalf in an email.",
			"It's pointless to write that you are writing on my behalf in an email.", Never, []string{Send}},
		{"I hate it when you buy things without telling me", "I hate it when you buy things without telling me.", Never, []string{Purchase}},
		{"Stop deleting my drafts.", "Stop deleting my drafts.", Never, []string{Delete}},
		{"Винаги проверявай текста, преди да изпратиш имейл.", "Винаги проверявай текста, преди да изпратиш имейл.", Always, []string{Send}},
		{"Никога не трий файлове без да ме питаш.", "Никога не трий файлове без да ме питаш.", AskFirst, []string{Delete}},
		{"Винаги ме питай, преди да изтриеш нещо.", "Винаги ме питай, преди да изтриеш нещо.", AskFirst, []string{Delete}},
		{"Първо ме питай, преди да плащаш.", "Първо ме питай, преди да плащаш.", AskFirst, []string{Purchase}},
		{"Питай ме преди да публикуваш нещо.", "Питай ме преди да публикуваш нещо.", AskFirst, []string{Post}},
		{"Не изпращай повече съобщения в неделя.", "Не изпращай повече съобщения в неделя.", Never, []string{Send}},
		{"Тъпо е да пишеш „Пише ви“ в началото на писмото.", "Тъпо е да пишеш „Пише ви“ в началото на писмото.", Never, []string{Send}},
		{"Отсега нататък пиши на Мария само на ти.", "Отсега нататък пиши на Мария само на ти.", Always, []string{Send}},
		{"Спри да плащаш абонаменти.", "Спри да плащаш абонаменти.", Never, []string{Purchase}},
		{"Whenever you send an invoice, attach the PDF.", "Whenever you send an invoice, attach the PDF.", Always, []string{Send}},
		{"Всеки път, когато изтриваш файл, ми казвай.", "Всеки път, когато изтриваш файл, ми казвай.", Always, []string{Delete}},
		{"Always answer in Bulgarian.", "Always answer in Bulgarian.", Always, []string{}},
	}

	for _, c := range cases {
		p, ok := Notice(c.said)
		if !ok {
			t.Errorf("%q was not noticed", c.said)

			continue
		}

		if p.Text != c.text || p.Kind != c.kind || !reflect.DeepEqual(p.Actions, c.actions) {
			t.Errorf("%q\n got %q %s %v\nwant %q %s %v", c.said, p.Text, p.Kind, p.Actions, c.text, c.kind, c.actions)
		}

		if p.Quote != strings.TrimSpace(c.said) {
			t.Errorf("%q: the quote is %q, not the message", c.said, p.Quote)
		}
	}
}

func TestOrdinarySentencesAreNotRules(t *testing.T) {
	for _, said := range []string{
		"Send this email to Anna.",
		"Don't send it yet.",
		"Do you always answer this slowly?",
		"Never mind.",
		"I always forget my keys.",
		"Always the same with this printer.",
		"Винаги забравям паролата си.",
		"Никога не съм бил в Париж.",
		"Не го изпращай още.",
		"Изтрий файла report.txt.",
		"Don't do that again.",
		"Не прави повече така.",
		"Can you never send it on Sundays?",
		"Never heard of it.", "Never seen that before.", "Always happy to help.", "Never knew that.",
		"Always, thanks!", "Always on time, that train.", "I never said that.", "Don't worry about it.",
		"Никога не сме били там.", "Винаги имаше проблеми с това.", "Никога не е късно.",
		"Винаги се радвам да помогна.", "Винаги, когато вали, ме боли глава.", "Whenever it rains, I stay in.",
		"Спри музиката.", "Stop it.", "It's stupid to think that way?",
		"",
	} {
		if p, ok := Notice(said); ok {
			t.Errorf("%q was proposed as a rule: %+v", said, p)
		}
	}
}

func TestWhatARuleNamesNarrowsIt(t *testing.T) {
	p, _ := Notice("Never send emails to anna@example.org or to Boris.")
	if !reflect.DeepEqual(p.Words, []string{"anna@example.org", "Boris"}) {
		t.Errorf("words %v", p.Words)
	}

	p, _ = Notice("Never write in English to clients.")
	for _, w := range p.Words {
		if w == "English" {
			t.Errorf("a language is not a name: %v", p.Words)
		}
	}

	p, _ = Notice("Never post on LinkedIn on weekends.")
	if !reflect.DeepEqual(p.Words, []string{"LinkedIn"}) || !reflect.DeepEqual(p.Days, []time.Weekday{time.Saturday, time.Sunday}) {
		t.Errorf("words %v days %v", p.Words, p.Days)
	}

	p, _ = Notice("Никога не публикувай в петък и събота.")
	if !reflect.DeepEqual(p.Days, []time.Weekday{time.Friday, time.Saturday}) || len(p.Words) != 0 {
		t.Errorf("days %v words %v", p.Days, p.Words)
	}
}

func kept(p Proposal) Rule {
	r := p.Rule(1, time.Now())
	r.Status, r.On = Kept, true

	return Normalise(r)
}

func mustNotice(t *testing.T, said string) Rule {
	t.Helper()

	p, ok := Notice(said)
	if !ok {
		t.Fatalf("%q was not noticed", said)
	}

	return kept(p)
}

func TestAPreciseNeverStopsAndNamesTheRule(t *testing.T) {
	r := mustNotice(t, "Never send emails to anna@example.org.")

	v := Check([]Rule{r}, Act{Actions: []string{Send}, Text: "Send to Anna@Example.org\nSubject: hello"})
	if v.Stop == nil || v.Stop.Text != r.Text {
		t.Fatalf("not stopped: %+v", v)
	}

	// Somebody else, or another kind of action, is not its business.
	if v := Check([]Rule{r}, Act{Actions: []string{Send}, Text: "Send to boris@example.org"}); !v.Clear() {
		t.Errorf("stopped a message to somebody else: %+v", v)
	}

	if v := Check([]Rule{r}, Act{Actions: []string{Files}, Text: "anna@example.org.txt"}); !v.Clear() {
		t.Errorf("stopped a file: %+v", v)
	}
}

func TestANeverWithConditionsItCannotSeeAsksInstead(t *testing.T) {
	r := mustNotice(t, "Never send emails to clients after dark.")

	if s := ScopeOf(r); s.Precise || !reflect.DeepEqual(s.Unsure, []string{"clients", "dark"}) {
		t.Fatalf("scope %+v", s)
	}

	v := Check([]Rule{r}, Act{Actions: []string{Send}, Text: "Send to someone"})
	if v.Stop != nil || len(v.Ask) != 1 {
		t.Errorf("want one ask, got %+v", v)
	}

	if Mode(r) != "ask" {
		t.Errorf("mode %s", Mode(r))
	}

	// Words set by hand say exactly when it applies.
	r.Words = []string{"client.example"}
	if Mode(r) != "stop" {
		t.Errorf("mode with words %s", Mode(r))
	}
}

func TestDaysAreChecked(t *testing.T) {
	r := mustNotice(t, "Never post on Sundays.")

	sunday := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	monday := sunday.AddDate(0, 0, 1)

	if v := Check([]Rule{r}, Act{Actions: []string{Post}, At: sunday}); v.Stop == nil {
		t.Errorf("a Sunday post went ahead: %+v", v)
	}

	if v := Check([]Rule{r}, Act{Actions: []string{Post}, At: monday}); !v.Clear() {
		t.Errorf("a Monday post was held: %+v", v)
	}
}

func TestAskFirstAndAlwaysNeverStop(t *testing.T) {
	ask := mustNotice(t, "Ask me before you delete anything.")
	always := mustNotice(t, "Always proofread before you send an email.")

	v := Check([]Rule{ask, always}, Act{Actions: []string{Delete, Send}, Text: "rm x"})
	if v.Stop != nil || len(v.Ask) != 1 || len(v.Remind) != 1 {
		t.Errorf("%+v", v)
	}

	if Mode(ask) != "ask" || Mode(always) != "remind" {
		t.Errorf("modes %s %s", Mode(ask), Mode(always))
	}
}

func TestOnlyLiveRulesCount(t *testing.T) {
	r := mustNotice(t, "Never push to main.")

	off := r
	off.On = false

	proposed := r
	proposed.Status = Proposed

	declined := r
	declined.Status = Declined

	act := Act{Actions: []string{Commit}, Text: "git push origin main"}

	for _, x := range []Rule{off, proposed, declined} {
		if v := Check([]Rule{x}, act); !v.Clear() {
			t.Errorf("a rule that is not live acted: %+v", x)
		}
	}

	if Block([]Rule{off, proposed, declined}, 0) != "" {
		t.Error("a rule that is not live went into the prompt")
	}
}

func TestAfterSeesWhatTheActionDid(t *testing.T) {
	r := mustNotice(t, "Never send emails to anna@example.org.")

	if got := After([]Rule{r}, Act{Actions: []string{Send}, Text: "sent to anna@example.org"}); len(got) != 1 {
		t.Errorf("after missed it: %v", got)
	}
}

func TestMentionsFindsWholeWordsOnly(t *testing.T) {
	if !Mentions("A note to Anna's team", []string{"anna"}) {
		t.Error("missed Anna's")
	}

	if Mentions("A note to Joanna", []string{"anna"}) {
		t.Error("found anna inside Joanna")
	}

	if !Mentions("към Мария", []string{"мария"}) {
		t.Error("missed a Cyrillic name")
	}
}

func TestTheBlockListsLiveRulesWithinItsLength(t *testing.T) {
	var list []Rule

	for i, said := range []string{"Never push to main.", "Always ask me before you send an email.", "Never post on Sundays."} {
		r := mustNotice(t, said)
		r.ID = int64(i + 1)
		list = append(list, r)
	}

	b := Block(list, 0)
	for _, r := range list {
		if !strings.Contains(b, r.Text) {
			t.Errorf("%q is missing from\n%s", r.Text, b)
		}
	}

	short := Block(list, 230)
	if !strings.Contains(short, "more on the Rules page") || strings.Contains(short, "Sundays") {
		t.Errorf("not cut by whole rules:\n%s", short)
	}
}

func TestNormaliseDropsWhatIsNotKnown(t *testing.T) {
	r := Normalise(Rule{
		Text:    "  never   push to main ",
		Kind:    "sometimes",
		Actions: []string{"commit", "launch", "COMMIT", "send"},
		Words:   []string{" main ", "Main", "", "x"},
		Days:    []time.Weekday{time.Sunday, 9, time.Monday},
	})

	if r.Text != "Never push to main." || r.Kind != Never {
		t.Errorf("text %q kind %s", r.Text, r.Kind)
	}

	if !reflect.DeepEqual(r.Actions, []string{Send, Commit}) || !reflect.DeepEqual(r.Words, []string{"main", "x"}) ||
		!reflect.DeepEqual(r.Days, []time.Weekday{time.Monday, time.Sunday}) {
		t.Errorf("%+v", r)
	}
}

func TestCleanKeepsARuleShort(t *testing.T) {
	long := strings.Repeat("дума ", 200)

	if got := Clean(long); len([]rune(got)) > MaxText+1 {
		t.Errorf("%d runes", len([]rune(got)))
	}

	if Clean("   ") != "" {
		t.Error("an empty rule is not empty")
	}
}
