package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

/*
 * The reason first, then the question, then the readings.
 *
 * "I found X, so which did you mean" is a question somebody can answer.
 * "Which did you mean" on its own is one they have to reconstruct the context
 * for, which is most of the cost of being asked anything.
 */
func TestAQuestionReadsAsSomethingAnswerable(t *testing.T) {
	q, ok := ReadQuestion(json.RawMessage(`{
		"question": "Which of them did you mean?",
		"why": "There are two folders called Skillo on this machine",
		"options": ["~/Projects/Skillo", "/media/drive/DEV/Skillo"]
	}`))

	if !ok {
		t.Fatal("a well formed question was not read")
	}

	text := q.Text()

	if !strings.HasPrefix(text, "There are two folders") {
		t.Errorf("the reason does not come first:\n%s", text)
	}

	for _, want := range []string{"Which of them did you mean?", "~/Projects/Skillo", "/media/drive/DEV/Skillo"} {
		if !strings.Contains(text, want) {
			t.Errorf("the question does not say %q:\n%s", want, text)
		}
	}
}

// A question with no reason and no options is still a question.
func TestTheBarestQuestionStillWorks(t *testing.T) {
	q, ok := ReadQuestion(json.RawMessage(`{"question":"Which drive?"}`))

	if !ok || q.Text() != "Which drive?" {
		t.Errorf("a bare question came out as %q (ok=%v)", q.Text(), ok)
	}
}

// An empty question is not one, and must not end a turn with silence.
func TestAnEmptyQuestionIsRefused(t *testing.T) {
	for _, args := range []string{`{}`, `{"question":""}`, `{"question":"   "}`, `not json`} {
		if _, ok := ReadQuestion(json.RawMessage(args)); ok {
			t.Errorf("%s was accepted as a question", args)
		}
	}
}

// Asking must never itself need permission, or the brain would need approval
// to admit that it is unsure.
func TestAskingNeedsNoPermission(t *testing.T) {
	var asking Ask

	if asking.Risk() != Safe {
		t.Error("asking a question was treated as changing something")
	}
}
