package ask

import (
	"strings"
	"testing"
)

var where = Question{
	Text:        "Where should the brain live?",
	Options:     []string{"On a USB stick", "On this computer", "In a folder I choose"},
	Recommended: 1,
}

func TestAQuestionIsOneQuestionWithTwoToFourAnswers(t *testing.T) {
	if err := Validate(where); err != nil {
		t.Fatal(err)
	}

	bad := []Question{
		{Text: "Which model? And which voice?", Options: []string{"a", "b"}},
		{Text: "Which model?", Options: []string{"a"}},
		{Text: "Which model?", Options: []string{"a", "b", "c", "d", "e"}},
		{Text: "Which model?", Options: []string{"a", "b"}, Recommended: 2},
		{Text: "Which model?", Options: []string{"a", " "}},
	}

	for _, q := range bad {
		if Validate(q) == nil {
			t.Errorf("accepted %+v", q)
		}
	}
}

func TestTheRecommendedAnswerComesFirstAndOtherLast(t *testing.T) {
	bg := Show(where, "bg")
	want := "Where should the brain live?\n1. On this computer (Препоръчително)\n2. On a USB stick\n3. In a folder I choose\n4. Друго — кажете го с ваши думи"

	if bg != want {
		t.Fatalf("shown as\n%s\nwant\n%s", bg, want)
	}

	if en := Show(where, "en"); !strings.Contains(en, "1. On this computer (Recommended)") {
		t.Fatalf("English shown as\n%s", en)
	}
}

func TestAnswersByNumberOrdinalOrWordsInBothLanguages(t *testing.T) {
	cases := map[string]string{
		"1":                    "On this computer",
		"2.":                   "On a USB stick",
		"второто":              "On a USB stick",
		"the third one":        "In a folder I choose",
		"първото":              "On this computer",
		"препоръчителното":     "On this computer",
		"on a usb stick":       "On a USB stick",
		"usb":                  "On a USB stick",
		"Избирам трето, моля.": "In a folder I choose",
	}

	for reply, want := range cases {
		got, other := Answer(where, reply)
		if other || got != want {
			t.Errorf("%q → %q (other=%v), want %q", reply, got, other, want)
		}
	}

	if got, other := Answer(where, "on my NAS at home"); !other || got != "on my NAS at home" {
		t.Errorf("free text was not taken as Other: %q %v", got, other)
	}

	if _, other := Answer(where, "4"); !other {
		t.Error("the number of Other was taken as an answer")
	}
}

func TestSeveralQuestionsBecomeAQueue(t *testing.T) {
	got := Split("Before I start:\n1. Which engine should I use? 2. Should it have sound?\n- Who is it for?")
	want := []string{"Before I start:\n1. Which engine should I use?", "Should it have sound?", "Who is it for?"}

	if len(got) != 3 || got[1] != want[1] || got[2] != want[2] || !strings.HasSuffix(got[0], "Which engine should I use?") {
		t.Fatalf("split into %q", got)
	}
}
