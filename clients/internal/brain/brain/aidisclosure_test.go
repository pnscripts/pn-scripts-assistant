package brain

import (
	"context"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * The first thing it says to somebody new is the model's own opener and
 * nothing more: it behaves as an assistant, and the terms accepted before
 * the first conversation (package terms) are where it says it is an AI.
 * 1.0.3 put "Before anything else: I am an AI assistant..." in front of
 * every first meeting; its owner asked for that to go (13.11.2026).
 */
func TestTheFirstMeetingSaysWhatTheModelSaidAndNothingMore(t *testing.T) {
	t.Setenv("PN_TEST_WORDING", "1")

	for _, c := range []struct{ name, language, model string }{
		{"English", "en", "I live on this computer. What should I call you?"},
		{"Bulgarian", "bg", "Живея на този компютър. Как да ви наричам?"},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := quietBrain(t)
			b.Cfg.Owner = ""
			b.Cfg.Language = c.language

			if !b.FirstMeeting() {
				t.Fatal("not a first meeting")
			}

			b.Router = llm.NewRouter(llm.ModeOpen, "ollama", phrasing{said: c.model})

			if g := b.GreetInWords(context.Background()); g.Text != c.model {
				t.Errorf("the first meeting opened with %q, want the model's own %q", g.Text, c.model)
			}
		})
	}
}

// Nor is the model asked to announce it.
func TestTheFirstMeetingBriefDoesNotAskToAnnounceAnAI(t *testing.T) {
	t.Setenv("PN_TEST_WORDING", "1")

	b := quietBrain(t)
	b.Cfg.Owner = ""

	asked := &listening{}
	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", asked)

	_ = b.GreetInWords(context.Background())

	if asked.prompt == "" {
		t.Fatal("the model was not asked for an opener")
	}

	for _, never := range []string{"say that you are an AI", "AI assistant (a language model)"} {
		if strings.Contains(asked.prompt, never) {
			t.Errorf("the model was asked %q: %q", never, asked.prompt)
		}
	}
}

// A model that remembers everything it was asked.
type listening struct{ prompt string }

func (*listening) Name() string                   { return "ollama" }
func (*listening) Available(context.Context) bool { return true }

func (l *listening) Chat(_ context.Context, r llm.Request) (llm.Response, error) {
	for _, m := range r.Messages {
		l.prompt += m.Content + "\n"
	}

	return llm.Response{Content: "Hello there."}, nil
}
