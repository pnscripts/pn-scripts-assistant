package brain

import (
	"context"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/aidisclosure"
	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * The first thing it says to somebody new says that it is an AI (EU AI Act
 * article 50(1): at the latest at the first interaction). The opener is the
 * model's, and is both shown and spoken (server handleGreeting speaks Text),
 * so the notice is in Text whatever the model chose to say.
 */
func TestTheFirstMeetingSaysItIsAnAI(t *testing.T) {
	t.Setenv("PN_TEST_WORDING", "1")

	cases := []struct {
		name, language, model, want string
	}{
		{"English words without it", "en", "I live on this computer. What should I call you?",
			aidisclosure.Notice("en") + " I live on this computer. What should I call you?"},
		{"Bulgarian owner, Bulgarian words", "bg", "Живея на този компютър. Как да ви наричам?",
			aidisclosure.Notice("bg") + " Живея на този компютър. Как да ви наричам?"},
		{"the model already said it", "en", "I am an AI assistant on this computer. What should I call you?",
			"I am an AI assistant on this computer. What should I call you?"},
		{"Bulgarian already said it", "bg", "Аз съм асистент с изкуствен интелект. Как да ви наричам?",
			"Аз съм асистент с изкуствен интелект. Как да ви наричам?"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := quietBrain(t)
			b.Cfg.Owner = ""
			b.Cfg.Language = c.language

			if !b.FirstMeeting() {
				t.Fatal("not a first meeting")
			}

			b.Router = llm.NewRouter(llm.ModeOpen, "ollama", phrasing{said: c.model})

			g := b.GreetInWords(context.Background())
			if g.Text != c.want {
				t.Errorf("the first meeting opened with %q, want %q", g.Text, c.want)
			}

			if !aidisclosure.Discloses(g.Text) {
				t.Errorf("the opener does not say it is an AI: %q", g.Text)
			}
		})
	}
}

// The model is asked to say it, too: the brief and the facts name it.
func TestTheFirstMeetingBriefAsksToSayItIsAnAI(t *testing.T) {
	t.Setenv("PN_TEST_WORDING", "1")

	b := quietBrain(t)
	b.Cfg.Owner = ""

	asked := &listening{}
	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", asked)

	_ = b.GreetInWords(context.Background())

	if !strings.Contains(asked.prompt, "AI assistant") {
		t.Errorf("the model was not told it is an AI assistant: %q", asked.prompt)
	}

	if !strings.Contains(asked.prompt, "say that you are an AI") {
		t.Errorf("the model was not asked to say it: %q", asked.prompt)
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

// With no model nothing is said, and the page's own line is the notice.
func TestTheFirstMeetingWithNoModelAddsNothing(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.Owner = ""
	b.Router = llm.NewRouter(llm.ModeOpen, "nobody")

	if said := b.GreetInWords(context.Background()).Text; strings.Contains(said, aidisclosure.Notice("en")) {
		t.Errorf("with no model talking, it still said %q", said)
	}
}
