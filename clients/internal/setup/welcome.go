package setup

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/environs"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/wording"
)

/*
 * The first thing anybody reads, written for the machine it is read on.
 *
 * The page carried two paragraphs about a private assistant that listens and
 * remembers — true, the same for everybody, and written eighteen months ago.
 * Somebody opening this on a machine with Godot, Claude Code and four
 * languages on it is being told the same thing as somebody opening it on a
 * bare install, which is the shape of introduction this whole change is
 * about.
 *
 * So the machine is read and the model writes the welcome from it. With one
 * honest limit: on a true first run there is no model — that is what the
 * wizard is for — and then the paragraphs the page already has are what
 * there is. That is not a failure. It is the only run where the program
 * genuinely has nothing to write with, and every run after it is written.
 */

/*
 * HowLongToWrite bounds the writing.
 *
 * Generous, because nobody is waiting on it: the page shows the words it
 * ships with and replaces them if an answer arrives. Two minutes was not
 * enough on this machine — a paragraph from a seven-billion-parameter model
 * on four cores is a minute or two of generation alone, and the first attempt
 * came back empty every time.
 */
const HowLongToWrite = 6 * time.Minute

/*
 * handleWelcome hands over the opening if it has been written, and starts
 * writing it if it has not.
 *
 * Never waits. An empty answer means "not yet, or not on this machine", and
 * the page keeps its own words either way — which is the right behaviour on
 * the first run of all, where there is no model because installing one is
 * what the wizard is for.
 */
func (s *Server) handleWelcome(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	said, writing := s.welcome, s.writingWelcome
	s.mu.Unlock()

	if said == "" && !writing {
		s.mu.Lock()
		s.writingWelcome = true
		s.mu.Unlock()

		go s.writeWelcome()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"text": said, "writing": said == "" && writing})
}

// writeWelcome composes the opening in the background and keeps it.
func (s *Server) writeWelcome() {
	ctx, stop := context.WithTimeout(context.Background(), HowLongToWrite)
	defer stop()

	said := s.welcomeInWords(ctx)

	s.mu.Lock()
	s.welcome, s.writingWelcome = said, false
	s.mu.Unlock()
}

func (s *Server) welcomeInWords(ctx context.Context) string {
	cfg, err := config.Load(s.root)
	if err != nil {
		cfg = config.Default()
	}

	model := strings.TrimSpace(cfg.OllamaModel)
	if model == "" {
		model = chatModelHere(ctx, cfg.OllamaURL)
	}

	if model == "" {
		// No model on this machine yet: the run the wizard exists for.
		return ""
	}

	here := environs.Words(environs.New(environs.Sources{}).All(ctx), 0)
	if strings.TrimSpace(here) == "" {
		return ""
	}

	provider := llm.NewOllama(cfg.OllamaURL, model, cfg.EmbedModel)

	return wording.Say(ctx, provider, model, wording.Want{
		Brief: "the opening of a setup wizard: what this program is — a private " +
			"assistant that keeps everything on this computer — and what it will " +
			"be able to work with here, naming two or three of the things found",
		Facts: map[string]any{
			"what is on this machine": here,
			"what setting up involves": "choosing where the brain lives, which model " +
				"does the thinking, and optionally keys for paid services; it says " +
				"what each piece costs and where it goes before fetching anything",
			"what is true of it always": "nothing said to it leaves this machine " +
				"unless a paid service is chosen, and it works with the network " +
				"unplugged",
		},
		Plain:  "",
		Most:   600,
		Within: HowLongToWrite,
	})
}

/*
 * chatModelHere is any model the machine already has that can hold a
 * conversation, for the run where the settings have not named one yet.
 *
 * Asked of Ollama rather than of the list of programs: "ollama" is a program
 * on this machine and not a model, and the first version of this took it for
 * one — which is the same mistake as reporting a kitchen as a meal.
 */
func chatModelHere(ctx context.Context, url string) string {
	ctx, stop := context.WithTimeout(ctx, environs.HowLongToAsk)
	defer stop()

	installed, err := models.New(url).List(ctx)
	if err != nil {
		return ""
	}

	for _, model := range installed {
		// An embedding model cannot hold a conversation, and counting one
		// would have the wizard write its welcome with something that cannot
		// write.
		if !strings.Contains(strings.ToLower(model.Name), "embed") {
			return model.Name
		}
	}

	return ""
}
