package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"pn-brain/internal/brain/llm"
)

/*
 * Connecting to a model that is not on this machine.
 *
 * The keys have been in the settings file since the beginning and there has
 * never been anywhere to type one. So the only way to use a hosted model was
 * to find the file, know its name, and edit it — which is not a feature this
 * program has, it is a feature its author has.
 *
 * A key is never sent back. The page is told whether one is set and what its
 * last four characters are, which is enough to tell two keys apart and useless
 * to anybody reading over a shoulder. Sending the whole key back to be shown
 * in a form is how a secret ends up in a screenshot.
 */

// tail is the recognisable end of a key, for telling one from another.
//
// Four characters: enough to answer "is that the one I meant", short enough
// that it is not a key.
func tail(key string) string {
	key = strings.TrimSpace(key)

	if len(key) < 4 {
		return ""
	}

	return key[len(key)-4:]
}

// handleProviders describes every model provider and how to reach it.
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	cfg := s.brain.Cfg

	reach := map[string]llm.Availability{}

	for _, a := range s.brain.Router.Availabilities(r.Context()) {
		reach[a.Name] = a
	}

	type provider struct {
		Name string `json:"name"`

		// What it is, in one line, because "OpenRouter" means nothing to
		// somebody who has not met it.
		What string `json:"what"`

		// NeedsKey separates the local one from the rest. Ollama needs an
		// address; the others need a secret.
		NeedsKey bool `json:"needs_key"`

		HasKey  bool   `json:"has_key"`
		KeyTail string `json:"key_tail,omitempty"`

		Model string `json:"model,omitempty"`

		// Where it lives, for the one that is on this machine.
		URL string `json:"url,omitempty"`

		Reachable bool `json:"reachable"`

		// Permitted is whether privacy allows it at all — a provider with a
		// key that privacy forbids is a different state from one with no key,
		// and they look identical unless the page is told.
		Permitted bool `json:"permitted"`
		Default   bool `json:"default"`
	}

	list := []provider{
		{
			Name:      "ollama",
			What:      "Runs models on this machine. Nothing leaves.",
			URL:       cfg.OllamaURL,
			Model:     cfg.OllamaModel,
			Reachable: reach["ollama"].Reachable,
			Permitted: reach["ollama"].Permitted,
			Default:   reach["ollama"].Default,
		},
		{
			Name:      "anthropic",
			What:      "Claude, on Anthropic's computers. Conversations leave this machine.",
			NeedsKey:  true,
			HasKey:    cfg.AnthropicKey != "",
			KeyTail:   tail(cfg.AnthropicKey),
			Model:     cfg.AnthropicModel,
			Reachable: reach["anthropic"].Reachable,
			Permitted: reach["anthropic"].Permitted,
			Default:   reach["anthropic"].Default,
		},
		{
			Name:      "openai",
			What:      "GPT, on OpenAI's computers. Conversations leave this machine.",
			NeedsKey:  true,
			HasKey:    cfg.OpenAIKey != "",
			KeyTail:   tail(cfg.OpenAIKey),
			Model:     cfg.OpenAIModel,
			Reachable: reach["openai"].Reachable,
			Permitted: reach["openai"].Permitted,
			Default:   reach["openai"].Default,
		},
		{
			Name: "openrouter",
			What: "One key for many companies' models — which means the request " +
				"may be served by any of them.",
			NeedsKey:  true,
			HasKey:    cfg.OpenRouterKey != "",
			KeyTail:   tail(cfg.OpenRouterKey),
			Model:     cfg.OpenRouterModel,
			Reachable: reach["openrouter"].Reachable,
			Permitted: reach["openrouter"].Permitted,
			Default:   reach["openrouter"].Default,
		},
	}

	ok(w, map[string]any{
		"privacy":   cfg.Privacy,
		"providers": list,
	})
}

/*
 * handleConnectProvider stores a key, an address or a model for one provider.
 *
 * Every field optional and applied only when present, so saving a model does
 * not blank a key somebody typed a minute earlier. The same rule the settings
 * endpoint learned the hard way.
 */
func (s *Server) handleConnectProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string  `json:"provider"`
		Key      *string `json:"key"`
		Model    *string `json:"model"`
		URL      *string `json:"url"`

		// Forget empties the key, which is the only way to take one back —
		// and it has to be its own field, because an empty key and a key that
		// was not sent must not mean the same thing.
		Forget bool `json:"forget"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "I could not read that.")

		return
	}

	cfg := s.brain.Cfg

	set := func(field *string, value *string) {
		if value != nil {
			*field = strings.TrimSpace(*value)
		}
	}

	switch strings.TrimSpace(strings.ToLower(body.Provider)) {
	case "ollama":
		set(&cfg.OllamaURL, body.URL)
		set(&cfg.OllamaModel, body.Model)

	case "anthropic":
		set(&cfg.AnthropicKey, body.Key)
		set(&cfg.AnthropicModel, body.Model)

		if body.Forget {
			cfg.AnthropicKey = ""
		}

	case "openai":
		set(&cfg.OpenAIKey, body.Key)
		set(&cfg.OpenAIModel, body.Model)

		if body.Forget {
			cfg.OpenAIKey = ""
		}

	case "openrouter":
		set(&cfg.OpenRouterKey, body.Key)
		set(&cfg.OpenRouterModel, body.Model)

		if body.Forget {
			cfg.OpenRouterKey = ""
		}

	default:
		fail(w, http.StatusBadRequest, "I do not know a provider by that name.")

		return
	}

	if err := cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, "I could not write it down: "+err.Error())

		return
	}

	s.brain.Cfg = cfg

	/*
	 * Rebuilt, not merely recorded.
	 *
	 * The router is assembled from the keys when the brain is built, so a key
	 * typed into a running program did nothing at all until it was restarted.
	 * That is the same bug privacy had, and it presents the same way: the
	 * setting is saved, the page says so, and nothing works.
	 */
	s.brain.ReloadProviders()

	s.brain.Log.Info("provider connection changed", "provider", body.Provider)

	// Never the key, only whether there is one.
	ok(w, map[string]any{"provider": body.Provider, "saved": true})
}

// handleTestProvider tries to reach one provider and says what happened.
//
// Its own endpoint because "I typed a key, is it right" is the question
// somebody has at that exact moment, and the alternative is asking the brain
// something and watching it fail for reasons that could be anything.
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(strings.ToLower(r.PathValue("name")))

	for _, a := range s.brain.Router.Availabilities(r.Context()) {
		if a.Name != name {
			continue
		}

		switch {
		case !a.Permitted:
			ok(w, map[string]any{
				"name":      name,
				"reachable": false,
				"why": "Your privacy setting (" + s.brain.Cfg.Privacy +
					") does not allow this one, so I have not tried to reach it.",
			})

		case a.Reachable:
			ok(w, map[string]any{"name": name, "reachable": true, "why": "Answered."})

		default:
			ok(w, map[string]any{
				"name": name, "reachable": false,
				"why": "No answer. Check the key, or whether this machine is online.",
			})
		}

		return
	}

	fail(w, http.StatusNotFound, "I do not know a provider by that name.")
}
