package otherprograms

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestOnlyTheStatusIsAnsweredWithoutASwitch(t *testing.T) {
	if got := strings.Join(Tools(), " "); got != "assistant_status list_programs ask_assistant search_memory" {
		t.Fatalf("the questions other programs may ask: %s", got)
	}

	if s, known := SubjectOf("assistant_status"); !known || s != "" {
		t.Errorf("whether it is running: subject %q, known %v", s, known)
	}

	if s, known := SubjectOf("list_programs"); !known || s != Programs {
		t.Errorf("which programs are installed: subject %q, known %v", s, known)
	}

	// Asking the assistant and searching its memory each have a switch of
	// their own (1.2.0 step 4): allowing one never allows the other.
	if s, known := SubjectOf("ask_assistant"); !known || s != Ask {
		t.Errorf("a question to the assistant: subject %q, known %v", s, known)
	}

	if s, known := SubjectOf("search_memory"); !known || s != Memory {
		t.Errorf("a search of the memory: subject %q, known %v", s, known)
	}

	for _, tool := range []string{"", "use_program", "read_file", "LIST_PROGRAMS", "list_programs ", "what_you_know", "recall"} {
		if _, known := SubjectOf(tool); known {
			t.Errorf("%q is not a question other programs may ask", tool)
		}
	}

	// Every question but the status is behind a switch somebody can see.
	for _, tool := range Tools() {
		s, _ := SubjectOf(tool)
		if tool != "assistant_status" && !Known(s) {
			t.Errorf("%s is answered with no switch", tool)
		}
	}

	// Three switches, the one that hands out what is personal last.
	if got := fmt.Sprint(Subjects()); got != "[programs ask memory]" || Known("") || Known("Memory") || Known("Programs") || Known("files") {
		t.Errorf("the switches: %v", got)
	}
}

func TestTheNameAProgramGivesIsMadeSafeToShow(t *testing.T) {
	for _, c := range []struct{ name, version, want string }{
		{"claude-code", "2.1.0", "claude-code 2.1.0"},
		{"  Cursor  ", "", "Cursor"},
		{"", "", Unnamed},
		{"   ", "9", Unnamed},
		{"\x00\x1b\x07", "", Unnamed},
		{"\x1b[31mred", "", "[31mred"},
		{"line one\nApproved by you", "1", "line one Approved by you 1"},
		{"tab\there", "", "tab here"},
		{"Кодер", "1.0", "Кодер 1.0"},
		{"a‮b​c", "", "abc"},
		{strings.Repeat("n", 200), strings.Repeat("7", 200), strings.Repeat("n", 60) + " " + strings.Repeat("7", 30)},
	} {
		if got := Caller(c.name, c.version); got != c.want {
			t.Errorf("Caller(%q, %q) = %q, want %q", c.name, c.version, got, c.want)
		}
	}

	// Marks stacked on one letter would draw over the lines above and
	// below: two stay, the rest go. Letters that need a mark keep it.
	if got := Caller("Z"+strings.Repeat("\u0301", 50)+"algo", ""); got != "Z\u0301\u0301algo" {
		t.Errorf("stacked marks: %q", got)
	}

	if got := Caller("Вѝдео Naïve हिन्दी", ""); got != "Вѝдео Naïve हिन्दी" {
		t.Errorf("letters that carry a mark: %q", got)
	}

	if got := Caller(strings.Repeat("\u0301", 30), "\ufe0f\ufe0f"); got != Unnamed {
		t.Errorf("marks with no letter under them: %q", got)
	}

	// Cut on a letter, never inside one.
	if got := Caller(strings.Repeat("я", 100), ""); !utf8.ValidString(got) || utf8.RuneCountInString(got) != 60 {
		t.Errorf("a long Cyrillic name: %q", got)
	}
}

func TestEveryQuestionIsSaidInBothLanguages(t *testing.T) {
	for _, tool := range Tools() {
		en, bg := Asked(tool)
		if en == "" || bg == "" || en == bg {
			t.Errorf("%s: %q / %q", tool, en, bg)
		}

		if strings.ContainsAny(en+bg, "–—") {
			t.Errorf("%s is said with a dash: %q / %q", tool, en, bg)
		}
	}

	if en, bg := Asked("nothing"); en != "" || bg != "" {
		t.Errorf("an unknown question is said as %q / %q", en, bg)
	}

	for _, s := range Subjects() {
		for _, allowed := range []bool{true, false} {
			if en, bg := Switched(s, allowed); en == "" || bg == "" || en == bg || strings.ContainsAny(en+bg, "–—") {
				t.Errorf("%s switched %v: %q / %q", s, allowed, en, bg)
			}
		}

		en, bg := NotAllowed(s)
		if en == "" || bg == "" || en == bg || !strings.Contains(en, "Programs") {
			t.Errorf("%s refused: %q / %q", s, en, bg)
		}
	}
}

// Each switch is said in its own words: a line about one must not read as
// a line about another.
func TestEachSwitchIsSaidInItsOwnWords(t *testing.T) {
	seen := map[string]Subject{}

	for _, s := range Subjects() {
		on, onBG := Switched(s, true)
		off, offBG := Switched(s, false)
		no, noBG := NotAllowed(s)

		for _, line := range []string{on, onBG, off, offBG, no, noBG} {
			if other, twice := seen[line]; twice {
				t.Errorf("%s and %s are said with the same line: %q", s, other, line)
			}

			seen[line] = s
		}
	}

	if en, _ := NotAllowed(Memory); !strings.Contains(en, "may not search what I remember") {
		t.Errorf("the memory refused: %q", en)
	}

	if en, bg := NotAllowed("nothing"); en != "" || bg != "" {
		t.Errorf("an unknown switch refused: %q / %q", en, bg)
	}
}

func TestAProgramThatAsksInALoopIsStoppedAndListedOnce(t *testing.T) {
	var p Pace

	start := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

	for i := 0; i < MostAMinute; i++ {
		if within, first := p.Next(start.Add(time.Duration(i) * time.Second)); !within || first {
			t.Fatalf("question %d of the minute: within %v, first %v", i+1, within, first)
		}
	}

	// Over the number: refused, and only the first refusal is for the record.
	if within, first := p.Next(start.Add(40 * time.Second)); within || !first {
		t.Fatalf("the first over the number: within %v, first %v", within, first)
	}

	for i := 0; i < 500; i++ {
		if within, first := p.Next(start.Add(41 * time.Second)); within || first {
			t.Fatalf("one more over the number: within %v, first %v", within, first)
		}
	}

	// A minute after the first question the count starts again, and a
	// second flood is listed once again.
	if within, _ := p.Next(start.Add(time.Minute)); !within {
		t.Fatal("still refused a minute later")
	}

	for i := 1; i < MostAMinute; i++ {
		p.Next(start.Add(time.Minute))
	}

	if within, first := p.Next(start.Add(time.Minute + time.Second)); within || !first {
		t.Errorf("the second flood: within %v, first %v", within, first)
	}

	// A clock set back does not lock the door.
	if within, _ := p.Next(start.Add(-time.Hour)); !within {
		t.Error("refused after the clock was set back")
	}

	en, bg := TooMany()
	if !strings.Contains(en, fmt.Sprintf("more than %d questions", MostAMinute)) || !strings.Contains(bg, fmt.Sprintf("повече от %d въпроса", MostAMinute)) ||
		strings.ContainsAny(en+bg, "–—") {
		t.Errorf("the line for too many: %q / %q", en, bg)
	}
}

func TestPaceIsSafeFromManyAtOnce(t *testing.T) {
	var p Pace

	now := time.Now()
	within := make(chan bool, 200)

	for i := 0; i < 200; i++ {
		go func() {
			ok, _ := p.Next(now)
			within <- ok
		}()
	}

	answered := 0

	for i := 0; i < 200; i++ {
		if <-within {
			answered++
		}
	}

	if answered != MostAMinute {
		t.Errorf("%d of 200 at once were answered, the number is %d", answered, MostAMinute)
	}
}

func TestWordsFromAnotherProgramAreMadeSafeToKeep(t *testing.T) {
	cases := []struct{ in, want string }{
		{"What is the capital of Bulgaria?", "What is the capital of Bulgaria?"},
		{"  two\r\nlines \t here \n\n\n and a third ", "two\nlines here\nand a third"},
		{"Кой е най-високият връх?", "Кой е най-високият връх?"},
		{"right\u202eto left\u200b\x00\x1b[31m", "rightto left[31m"},
		{"z\u0301\u0301\u0301\u0301\u0301algo", "z\u0301\u0301algo"},
		{"\u200b\u202e\n\t", ""},
		{"", ""},
	}

	for _, c := range cases {
		if got := Words(c.in, MostQuestion); got != c.want {
			t.Errorf("%q became %q, want %q", c.in, got, c.want)
		}
	}

	long := strings.Repeat("я", MostQuestion+50)
	if got := Words(long, MostQuestion); utf8.RuneCountInString(got) != MostQuestion || !utf8.ValidString(got) {
		t.Errorf("a long question is kept as %d characters", utf8.RuneCountInString(got))
	}

	if got := Words(strings.Repeat("a\n", 500), MostSearch); utf8.RuneCountInString(got) > MostSearch {
		t.Errorf("many short lines are kept as %d characters", utf8.RuneCountInString(got))
	}
}
