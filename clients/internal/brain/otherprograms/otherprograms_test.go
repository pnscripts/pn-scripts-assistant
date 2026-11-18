package otherprograms

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOnlyTheStatusIsAnsweredWithoutASwitch(t *testing.T) {
	if got := strings.Join(Tools(), " "); got != "assistant_status list_programs" {
		t.Fatalf("the questions other programs may ask: %s", got)
	}

	if s, known := SubjectOf("assistant_status"); !known || s != "" {
		t.Errorf("whether it is running: subject %q, known %v", s, known)
	}

	if s, known := SubjectOf("list_programs"); !known || s != Programs {
		t.Errorf("which programs are installed: subject %q, known %v", s, known)
	}

	for _, tool := range []string{"", "ask_assistant", "read_file", "LIST_PROGRAMS", "list_programs "} {
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

	if len(Subjects()) != 1 || Subjects()[0] != Programs || Known("") || Known("memory") || Known("Programs") {
		t.Errorf("the switches: %v", Subjects())
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
