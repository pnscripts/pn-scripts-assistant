package setup

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/desktop"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/protect"
	"pn-scripts-assistant/internal/brain/speech"
)

/*
 * Proving it works, rather than saying so.
 *
 * The last step used to list what had been installed and call that ready. It
 * is not the same claim: a model can be on disk and still fail to load, a key
 * can be well-formed and revoked, a folder can exist and not be writable. Each
 * of those produces a program that starts, accepts a question, and answers
 * nothing — with the person looking for the fault in the wrong place, because
 * the last thing they were told was that everything was fine.
 *
 * So the slow ones are actually run. The model is asked something and timed;
 * nothing it says is shown, because the answer is not the point and a first
 * impression of a small model improvising is a bad one. The number is the
 * point: on a processor with no graphics card, "answered in 24s" is the single
 * most useful fact in the whole wizard.
 */

// A tick is one thing checked, in the words somebody would use.
type tick struct {
	ID   string `json:"id"`
	What string `json:"what"`

	// State is "yes", "no", "checking" or "skip".
	State string `json:"state"`
	Note  string `json:"note,omitempty"`
}

type readiness struct {
	mu    sync.Mutex
	slow  map[string]tick
	doing bool
	when  time.Time
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.ready.mu.Lock()
		s.ready.slow = nil
		s.ready.when = time.Time{}
		s.ready.mu.Unlock()
	}

	cfg, _ := config.LoadFrom(s.envPath)

	out := s.quickTicks(cfg)

	s.ready.mu.Lock()

	/*
	 * Started on the first ask, and never while an install is running.
	 *
	 * An install has the disk and the processor, and a model measured against
	 * that contention would report a number that says more about the download
	 * than about the machine.
	 */
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()

	if !busy && !s.ready.doing && s.ready.when.IsZero() {
		s.ready.doing = true

		go s.probeSlowly(cfg)
	}

	for _, id := range []string{"think", "remember"} {
		if got, ok := s.ready.slow[id]; ok {
			out = append(out, got)

			continue
		}

		note := ""
		if busy {
			note = "waiting for the install to finish"
		}

		out = append(out, tick{ID: id, What: slowNames[id], State: "checking", Note: note})
	}

	s.ready.mu.Unlock()

	s.writeJSON(w, map[string]any{"ticks": out})
}

var slowNames = map[string]string{
	"think":    "It can think",
	"remember": "It can remember what you tell it",
}

// quickTicks are the ones that cost nothing, computed on every ask.
func (s *Server) quickTicks(cfg config.Config) []tick {
	out := []tick{}

	root := s.chosenRoot()

	home := tick{ID: "home", What: "It has somewhere to keep what it learns"}

	if canWriteInto(root) {
		home.State, home.Note = "yes", root
	} else {
		home.State, home.Note = "no", "that folder cannot be written to"
	}

	out = append(out, home)

	speak := tick{ID: "speak", What: "It can speak"}

	if speech.Available() != nil {
		speak.State = "yes"
		speak.Note = speech.VoiceKind(cfg.Voice)
	} else {
		speak.State = "no"
		speak.Note = "no voice is installed"
	}

	out = append(out, speak)

	hear := tick{ID: "hear", What: "It can hear you"}

	if ok, why := speech.Listening(); ok {
		hear.State = "yes"
	} else {
		hear.State, hear.Note = "skip", why
	}

	out = append(out, hear)

	guard := tick{ID: "protect", What: "It will ask before reading your keys"}
	chosen := protect.Load(root)

	// Set to act freely, nothing asks — these included. Saying otherwise on
	// the last step would be promising a question that setting has turned off.
	if permits.Freedom(cfg.Asking()) == permits.Everything {
		guard.State, guard.Note = "skip", "not while it is set to act freely — reading them is recorded instead"
	} else if len(chosen.Off) < len(protect.BuiltIn) {
		guard.State = "yes"
		guard.Note = fmt.Sprintf("%d of %d places", len(protect.BuiltIn)-len(chosen.Off),
			len(protect.BuiltIn))
	} else {
		guard.State, guard.Note = "no", "every rule is switched off"
	}

	out = append(out, guard)

	// Who does the work, and whether they can do it here. See organisation.go.
	out = append(out, s.organisationTicks(cfg)...)

	menu := tick{ID: "menu", What: "You can start it again from the menu"}

	if desktop.Installed() {
		menu.State = "yes"
	} else {
		menu.State, menu.Note = "skip", "not added yet"
	}

	return append(out, menu)
}

/*
 * probeSlowly asks the model something, in the background.
 *
 * Its own context, not the request's: the page asks for this on a two-second
 * poll and that request is long gone by the time a model on a processor has
 * finished thinking. Cancelling with it would mean the check never completes
 * on exactly the machines where it matters most.
 */
func (s *Server) probeSlowly(cfg config.Config) {
	defer func() {
		s.ready.mu.Lock()
		s.ready.doing = false
		s.ready.when = time.Now()
		s.ready.mu.Unlock()
	}()

	got := map[string]tick{}

	got["think"] = s.probeThinking(cfg)
	got["remember"] = s.probeMemory(cfg)

	s.ready.mu.Lock()
	s.ready.slow = got
	s.ready.mu.Unlock()
}

func (s *Server) probeThinking(cfg config.Config) tick {
	t := tick{ID: "think", What: slowNames["think"]}

	if cfg.HasPaidProvider() {
		// The key's shape was checked when it was saved; whether it works is a
		// different question and this is the last chance to ask it cheaply.
		t.State, t.Note = "yes", "using a paid service"

		return t
	}

	model := strings.TrimSpace(cfg.OllamaModel)
	if model == "" {
		t.State, t.Note = "no", "no model is installed"

		return t
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	measured, err := models.New(cfg.OllamaURL).Measure(ctx, model)
	if err != nil {
		t.State, t.Note = "no", plainly(err)

		return t
	}

	t.State = "yes"
	t.Note = fmt.Sprintf("%s answered in %.0fs", model, measured.Seconds)

	return t
}

func (s *Server) probeMemory(cfg config.Config) tick {
	t := tick{ID: "remember", What: slowNames["remember"]}

	if strings.TrimSpace(cfg.EmbedModel) == "" {
		t.State, t.Note = "skip", "no embedding model — it will answer but not remember"

		return t
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	/*
	 * Actually embedded, not merely present.
	 *
	 * Memory is the thing this program is for, and an embedding model that
	 * downloaded but will not load leaves an assistant that answers every
	 * question and remembers none of it — which looks like forgetfulness
	 * rather than like a fault, and so goes unreported for weeks.
	 */
	if _, err := llm.NewOllama(cfg.OllamaURL, "", cfg.EmbedModel).Embed(ctx, "a short line"); err != nil {
		t.State, t.Note = "skip", "it will answer but not remember — "+plainly(err)

		return t
	}

	t.State = "yes"

	return t
}

/*
 * plainly turns a transport error into something worth reading.
 *
 * The honest text of a failure here is a Go error about a POST to a port, and
 * that is the wrong register for the last screen of a wizard: it names the
 * symptom in the vocabulary of the thing that failed rather than saying what
 * somebody should do. The three that actually happen are worth translating;
 * anything else is passed through rather than guessed at, because a wrong
 * explanation is worse than a technical one.
 */
func plainly(err error) string {
	text := err.Error()

	switch {
	case strings.Contains(text, "connection refused"), strings.Contains(text, "dial tcp"):
		return "Ollama is not running — it is what runs a model on this machine"
	case strings.Contains(text, "context deadline exceeded"):
		return "it did not answer in time, which on a processor can mean it is simply slow"
	case strings.Contains(text, "not found"), strings.Contains(text, "404"):
		return "that model is not installed"
	}

	return text
}
