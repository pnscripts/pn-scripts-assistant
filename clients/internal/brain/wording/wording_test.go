package wording

import (
	"context"
	"errors"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

// a model that answers with whatever it was told to, and remembers what it
// was asked.
type saying struct {
	said string
	err  error

	asked llm.Request
}

func (s *saying) Name() string                   { return "saying" }
func (s *saying) Available(context.Context) bool { return true }

func (s *saying) Chat(_ context.Context, req llm.Request) (llm.Response, error) {
	s.asked = req

	if s.err != nil {
		return llm.Response{}, s.err
	}

	return llm.Response{Content: s.said}, nil
}

func allowed(t *testing.T) {
	t.Helper()
	t.Setenv("PN_TEST_WORDING", "1")
}

/*
 * The assistant says it, and it is given nothing but the facts.
 *
 * Both halves matter. The first is the point of the package; the second is
 * what keeps it honest, because a model that is handed the conversation will
 * eventually say something that was in the conversation rather than in the
 * facts.
 */
func TestItSaysItItself(t *testing.T) {
	allowed(t)

	model := &saying{said: "Four things are waiting for you."}

	said := Say(context.Background(), model, "qwen", Want{
		Brief: "a greeting",
		Facts: map[string]any{"waiting": 4},
		Plain: "There are 4 things waiting.",
	})

	if said != "Four things are waiting for you." {
		t.Errorf("it said %q", said)
	}

	if len(model.asked.Messages) != 2 || model.asked.Messages[1].Content != `{"waiting":4}` {
		t.Fatalf("the model was given something other than the facts: %+v", model.asked.Messages)
	}

	brief := model.asked.Messages[0].Content
	for _, rule := range []string{"Use only the facts", "a greeting", "unless a fact says so"} {
		if !strings.Contains(brief, rule) {
			t.Errorf("the brief does not say %q:\n%s", rule, brief)
		}
	}
}

/*
 * When there is no model, or it cannot answer, what the program would have
 * said is what gets said.
 *
 * This is the whole reason the plain sentences stay in the code: an assistant
 * that goes quiet because a model is missing is worse than one that speaks
 * plainly.
 */
func TestWithNoModelItStillSpeaks(t *testing.T) {
	allowed(t)

	plain := "Good evening. Four things are waiting."

	for _, model := range []llm.Provider{nil, &saying{err: errors.New("no model here")}, &saying{said: "   "}} {
		if said := Say(context.Background(), model, "qwen", Want{Brief: "a greeting", Plain: plain}); said != plain {
			t.Errorf("with %T it said %q", model, said)
		}
	}

	// And an answer far longer than what was asked for is not an answer.
	long := &saying{said: strings.Repeat("and then it said a great deal more. ", 40)}
	if said := Say(context.Background(), long, "qwen", Want{Brief: "one line", Plain: plain, Most: 120}); said != plain {
		t.Errorf("a runaway answer was used: %q", said)
	}
}

// What a small model wraps around its answer is not part of the answer.
func TestTheUsualDecorationsComeOff(t *testing.T) {
	allowed(t)

	for said, want := range map[string]string{
		"```\nFour things are waiting.\n```":     "Four things are waiting.",
		"```text\nFour things are waiting.\n```": "Four things are waiting.",
		"Assistant: Four things are waiting.":    "Four things are waiting.",
		"\"Four things are waiting.\"":           "Four things are waiting.",
		"  Four things are waiting.  ":           "Four things are waiting.",
	} {
		if got := Say(context.Background(), &saying{said: said}, "qwen",
			Want{Brief: "a greeting", Plain: "plain"}); got != want {
			t.Errorf("%q came out as %q", said, got)
		}
	}
}

// A test binary says the plain thing unless it asked not to, so that no test
// of anything else quietly depends on a model being installed.
func TestATestDoesNotTalkToAModelByAccident(t *testing.T) {
	model := &saying{said: "a model answered"}

	if said := Say(context.Background(), model, "qwen", Want{Brief: "a greeting", Plain: "plain"}); said != "plain" {
		t.Errorf("a test reached a model without asking: %q", said)
	}
}
