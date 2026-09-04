package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// a conversation, as far as the tool is concerned.
type talk struct {
	forgot  int
	deleted bool
	renamed string
	lines   []Message
	err     error
}

func (t *talk) ForgetLastExchange(int64) (int, error) { t.forgot++; return 3, t.err }
func (t *talk) DeleteConversation(int64) error        { t.deleted = true; return t.err }
func (t *talk) RenameConversation(_ int64, title string) error {
	t.renamed = title

	return t.err
}

func (t *talk) History(int64) ([]Message, error) { return t.lines, t.err }

func revising(t *testing.T, in *talk) Revise {
	t.Helper()

	return Revise{Talk: in, Now: func() int64 { return 7 }}
}

/*
 * The four things people say when they want the last minute undone.
 *
 * A conversation that cannot be corrected is a transcript. Until this existed
 * the brain could be stopped and could be asked something new, and that was the
 * whole of it — a question asked wrongly stayed exactly as it landed.
 */
func TestChangingWhatJustHappened(t *testing.T) {
	for _, c := range []struct {
		action string
		want   func(*talk) bool
		says   string
	}{
		{"forget_the_last_exchange", func(x *talk) bool { return x.forgot == 1 }, "Forgotten"},
		{"delete_this_conversation", func(x *talk) bool { return x.deleted }, "deleted"},
	} {
		in := &talk{}

		said, err := revising(t, in).Execute(context.Background(),
			json.RawMessage(`{"action":"`+c.action+`"}`))
		if err != nil {
			t.Fatalf("%s: %v", c.action, err)
		}

		if !c.want(in) {
			t.Errorf("%s did not reach the conversation", c.action)
		}

		if !strings.Contains(said, c.says) {
			t.Errorf("%s said %q", c.action, said)
		}
	}

	in := &talk{}

	if _, err := revising(t, in).Execute(context.Background(),
		json.RawMessage(`{"action":"rename_this_conversation","title":"the drive plan"}`)); err != nil {
		t.Fatal(err)
	}

	if in.renamed != "the drive plan" {
		t.Errorf("renamed to %q", in.renamed)
	}
}

/*
 * Answering again means the question before last.
 *
 * The last thing said is "say that again", and answering that would be
 * answering the request rather than the thing it refers to.
 */
func TestAnsweringAgainReachesPastTheRequest(t *testing.T) {
	in := &talk{lines: []Message{
		{Role: "user", Content: "how big is the drive"},
		{Role: "assistant", Content: "a wrong answer"},
		{Role: "user", Content: "no, say that again properly"},
	}}

	said, err := revising(t, in).Execute(context.Background(),
		json.RawMessage(`{"action":"answer_again"}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "how big is the drive") {
		t.Errorf("it brought back the wrong question: %s", said)
	}

	if strings.Contains(said, "say that again properly") {
		t.Errorf("it brought back the request instead of the question: %s", said)
	}

	// A conversation with nothing before it says so rather than inventing one.
	thin := &talk{lines: []Message{{Role: "user", Content: "say that again"}}}

	said, err = revising(t, thin).Execute(context.Background(),
		json.RawMessage(`{"action":"answer_again"}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "no earlier question") {
		t.Errorf("an empty history produced %q", said)
	}
}

/*
 * Two of these delete things a person said, so they wait for a decision.
 *
 * A misheard "forget that" would otherwise take a real exchange with it, and
 * the whole point of the tool is undoing mistakes rather than making a new
 * kind of them.
 */
func TestChangingAConversationWaitsForAPerson(t *testing.T) {
	if (Revise{}).Risk() != Mutating {
		t.Error("a tool that deletes what was said runs without being approved")
	}

	said := (Revise{}).Summarize(json.RawMessage(`{"action":"delete_this_conversation"}`))

	if !strings.Contains(said, "Delete") {
		t.Errorf("the approval would read %q", said)
	}
}
