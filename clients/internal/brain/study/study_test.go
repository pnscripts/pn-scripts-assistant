package study

import (
	"strings"
	"testing"
	"time"
)

func TestLevels(t *testing.T) {
	if l, ok := LevelNamed(""); !ok || l.Name != "beginner" {
		t.Errorf("no level should be a beginner, got %+v", l)
	}

	if _, ok := LevelNamed("Child"); !ok {
		t.Error("a level is found whatever its case")
	}

	if _, ok := LevelNamed("genius"); ok {
		t.Error("an unknown level was accepted")
	}

	for _, l := range Levels {
		if l.EN == "" || l.BG == "" || l.Brief == "" {
			t.Errorf("%s lacks a name or a brief", l.Name)
		}
	}
}

func TestBriefsAreInTheLearnersLanguage(t *testing.T) {
	lv, _ := LevelNamed("child")

	bg := LessonBrief(lv, "bg", 0)
	if !strings.Contains(bg, "Bulgarian") || !strings.Contains(bg, "Проверете се") || !strings.Contains(bg, "15 minutes") {
		t.Errorf("Bulgarian lesson brief: %s", bg)
	}

	if !strings.Contains(LessonBrief(lv, "en", 500), "90 minutes") {
		t.Error("a lesson is capped at ninety minutes")
	}

	q := QuizBrief(lv, "en", 50)
	if !strings.Contains(q, "Write 10 questions") || !strings.Contains(q, "never invent") {
		t.Errorf("quiz brief: %s", q)
	}
}

/*
 * The questions a small model writes, with everything small models do to
 * them: prose before, a fence, a letter for the answer, "12 cm" for a number,
 * the same question twice, a choice whose answer is none of its options.
 */
func TestParseQuizKeepsOnlyWhatCanBeMarked(t *testing.T) {
	reply := "Here are your questions:\n```json\n[" +
		`{"kind":"choice","question":"Which river flows through Vienna?","options":["A) Rhine","B) Danube","C) Elbe"],"answer":"B"},` +
		`{"kind":"number","question":"How many centimetres are in 1.2 m?","answer":"120 cm","explain":"100 cm in a metre."},` +
		`{"type":"short","question":"Capital of Bulgaria?","answer":["Sofia","София"]},` +
		`{"kind":"choice","question":"which river flows through vienna","options":["Danube","Rhine"],"answer":"Danube"},` +
		`{"kind":"choice","question":"Pick one","options":["x","y"],"answer":"z"},` +
		`{"kind":"essay","question":"Discuss.","answer":"…"},` +
		`{"kind":"number","question":"How many?","answer":"many"}` +
		"]\n```\nGood luck!"

	qs, dropped, err := ParseQuiz(reply)
	if err != nil {
		t.Fatal(err)
	}

	if len(qs) != 3 || dropped != 4 {
		t.Fatalf("kept %d, dropped %d: %+v", len(qs), dropped, qs)
	}

	if qs[0].Answer != "Danube" || qs[0].Options[0] != "Rhine" {
		t.Errorf("the letter was not read: %+v", qs[0])
	}

	if qs[1].Answer != "120" {
		t.Errorf("the number was not read: %+v", qs[1])
	}

	if qs[2].Kind != Short || qs[2].Answer != "Sofia|София" {
		t.Errorf("the alternatives were not kept: %+v", qs[2])
	}

	if _, _, err := ParseQuiz(`{"questions":[{"kind":"short","question":"2+2 in words?","answer":"four"}]}`); err != nil {
		t.Errorf("a wrapped list was not read: %v", err)
	}

	if _, _, err := ParseQuiz("I cannot write questions on that."); err == nil {
		t.Error("prose was taken for questions")
	}
}

func TestParseNumber(t *testing.T) {
	for text, want := range map[string]float64{
		"12.5": 12.5, "12,5": 12.5, "1 234,5": 1234.5, "1,234.5": 1234.5, "1.234,5": 1234.5,
		"-3": -3, "3/4": 0.75, "25 %": 25, "12 cm": 12, "about 7": 7, "1,000,000": 1e6, "−2": -2,
		"1 000 000": 1e6, "0.5.": 0.5,
	} {
		if got, ok := ParseNumber(text); !ok || got != want {
			t.Errorf("%q read as %v %v, want %v", text, got, ok, want)
		}
	}

	for _, text := range []string{"", "many", "1/0", "-"} {
		if _, ok := ParseNumber(text); ok {
			t.Errorf("%q was read as a number", text)
		}
	}
}

func TestCheck(t *testing.T) {
	choice := Question{Kind: Choice, Question: "?", Options: []string{"Rhine", "Danube", "Elbe"}, Answer: "Danube"}

	for given, right := range map[string]bool{
		"Danube": true, "the danube.": true, "B": true, "b)": true, "2": true, "Б": true,
		"A": false, "Rhine": false, "3": false,
	} {
		if v := Check(choice, given); !v.Sure || v.Right != right {
			t.Errorf("choice %q: %+v", given, v)
		}
	}

	if v := Check(choice, "the long one"); v.Sure {
		t.Error("an answer naming no option was marked by a rule")
	}

	if v := Check(choice, "A"); v.Expected != "B) Danube" {
		t.Errorf("the expected answer reads %q", v.Expected)
	}

	third := Question{Kind: Number, Answer: "0.333333"}
	for given, right := range map[string]bool{"0.33": true, "0,333": true, "1/3": true, "0.3": false, "3": false} {
		if v := Check(third, given); !v.Sure || v.Right != right {
			t.Errorf("number %q: %+v", given, v)
		}
	}

	if v := Check(Question{Kind: Number, Answer: "120"}, "120 cm"); !v.Right {
		t.Error("a number with its unit was marked wrong")
	}

	if v := Check(third, "a third"); v.Sure {
		t.Error("words were marked as a number")
	}

	short := Question{Kind: Short, Answer: "Sofia|София"}
	for _, given := range []string{"sofia", "София.", "SOFIA "} {
		if v := Check(short, given); !v.Right || !v.Sure {
			t.Errorf("short %q: %+v", given, v)
		}
	}

	if v := Check(short, "the capital is Sofia"); v.Right || v.Sure || v.Expected != "Sofia" {
		t.Errorf("a longer answer must go to a judge: %+v", v)
	}

	if v := Check(short, "  "); v.Right || !v.Sure {
		t.Errorf("no answer is wrong, surely: %+v", v)
	}
}

func TestNormal(t *testing.T) {
	for in, want := range map[string]string{
		"The Danube.": "danube", "Café  au lait!": "cafe au lait", "ѝ": "и", "A": "a", "an apple": "apple",
	} {
		if got := Normal(in); got != want {
			t.Errorf("Normal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSchedule(t *testing.T) {
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

	box, due := Next(1, true, now)
	if box != 2 || due != now.Add(24*time.Hour) {
		t.Errorf("first right answer: box %d due %v", box, due)
	}

	box, due = Next(4, true, now)
	if box != 5 || !Learnt(box) || due != now.Add(30*24*time.Hour) {
		t.Errorf("fourth box: %d %v", box, due)
	}

	if box, _ = Next(5, true, now); box != 5 {
		t.Error("the last box is the last")
	}

	box, due = Next(5, false, now)
	if box != 1 || due != now.Add(Again) {
		t.Errorf("a wrong answer starts again: %d %v", box, due)
	}
}
