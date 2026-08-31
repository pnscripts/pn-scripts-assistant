package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"pn-brain/internal/preflight"
	"pn-brain/internal/starter"
)

// setupServer is a small HTTP server the desktop app runs itself, so first-run
// setup happens inside the application window rather than in a terminal.
//
// It has to exist separately from the brain because of an ordering problem: the
// brain cannot serve its own setup page on a machine that is missing the things
// the brain needs to start. So the app carries just enough of a web server to
// explain the situation and fix it, then hands the window over to the brain.
type Server struct {
	mu       sync.Mutex
	log      bytes.Buffer
	busy     bool
	envPath  string
	listener net.Listener
}

func New(envPath string) (*Server, error) {
	// Port 0: the OS picks a free one. Hardcoding a port would collide with
	// whatever else the user happens to be running.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	return &Server{listener: l, envPath: envPath}, nil
}

func (s *Server) URL() string {
	return "http://" + s.listener.Addr().String()
}

func (s *Server) Serve(onReady func()) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, setupPage)
	})

	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, s.state())
	})

	mux.HandleFunc("/install", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		go s.install(name)
		s.writeJSON(w, map[string]any{"started": true})
	})

	mux.HandleFunc("/choose-model", func(w http.ResponseWriter, r *http.Request) {
		/*
		 * Which model, when one is named.
		 *
		 * An empty body still means the recommended one, so the page works
		 * unchanged and an older client is not broken by this.
		 */
		var body struct {
			Model string `json:"model"`
		}

		json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)

		go s.install(modelRequest(body.Model))
		s.writeJSON(w, map[string]any{"started": true})
	})

	mux.HandleFunc("/api-key", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`

			// Which service the key belongs to. Empty means Anthropic, which
			// is what the page sent before there was anywhere else to send.
			Provider string `json:"provider"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		if err := s.saveAPIKey(body.Provider, strings.TrimSpace(body.Key)); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}

		s.writeJSON(w, map[string]any{"ok": true})
	})

	// The starter answers questions while the real brain is still downloading.
	// That gap is exactly when someone has the most questions about what is
	// being installed on their machine, and the least reason to trust it.
	mux.HandleFunc("/ask", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Question string `json:"question"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		reply := starter.Ask(body.Question)

		s.writeJSON(w, map[string]any{
			"answer":   reply.Answer,
			"matched":  reply.Matched,
			"followup": reply.Followup,
		})
	})

	mux.HandleFunc("/suggestions", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, map[string]any{"suggestions": starter.Suggestions()})
	})

	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, map[string]any{"ok": true})
		go onReady()
	})

	_ = http.Serve(s.listener, mux)
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type requirementView struct {
	Name        string `json:"name"`
	Why         string `json:"why"`
	Consequence string `json:"consequence"`
	State       string `json:"state"`
	Detail      string `json:"detail"`
	Optional    bool   `json:"optional"`
	Installable bool   `json:"installable"`
	ManualHint  string `json:"manual_hint"`
}

func (s *Server) state() map[string]any {
	results := preflight.Check()
	views := make([]requirementView, 0, len(results))

	for _, r := range results {
		views = append(views, requirementView{
			Name:        r.Requirement.Name,
			Why:         r.Requirement.Why,
			Consequence: r.Requirement.Consequence,
			State:       r.State.Label(),
			Detail:      r.Detail,
			Optional:    r.Requirement.Optional,
			Installable: r.Requirement.Installable(),
			ManualHint:  r.Requirement.ManualHint,
		})
	}

	hw := preflight.DetectHardware()
	model := preflight.RecommendModel(hw)

	s.mu.Lock()
	logText, busy := s.log.String(), s.busy
	s.mu.Unlock()

	return map[string]any{
		"requirements": views,
		"blocking":     preflight.BlockingCount(results),
		"busy":         busy,
		"log":          logText,
		"hardware": map[string]any{
			"cores":     hw.CPUCores,
			"ram_gb":    hw.RAMGB,
			"gpu":       hw.GPUName,
			"has_gpu":   hw.HasGPU,
			"can_local": preflight.CanRunLocalModels(hw),
		},
		"recommended_model": map[string]any{
			"model": model.Model,
			"size":  model.SizeNote,
			"speed": model.SpeedNote,
		},

		/*
		 * And the alternatives, because a single suggestion makes a trade on
		 * somebody's behalf that they may not want.
		 *
		 * The recommendation is still marked. What changes is that a person
		 * who would rather wait two seconds than get the better answer can now
		 * see that the option exists, which they could not before.
		 */
		"model_options": modelOptions(hw),
		"has_api_key":   s.hasAPIKey(),
	}
}

func (s *Server) install(name string) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()

		return
	}

	s.busy = true
	s.log.Reset()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	if chosen, ok := strings.CutPrefix(name, modelPrefix); ok {
		if chosen == "" {
			chosen = preflight.RecommendModel(preflight.DetectHardware()).Model
		}

		s.runModelPull(chosen)

		return
	}

	for _, r := range preflight.Check() {
		if r.Requirement.Name != name {
			continue
		}

		if err := preflight.Install(r.Requirement, &syncWriter{s: s}); err != nil {
			fmt.Fprintf(&syncWriter{s: s}, "\nFailed: %v\n", err)
		}

		return
	}
}

func (s *Server) runModelPull(model string) {
	w := &syncWriter{s: s}
	fmt.Fprintf(w, "Pulling %s — this downloads a few GB and can take a while.\n\n", model)

	req := preflight.Requirement{
		Name:       "model",
		InstallCmd: func() []string { return []string{"ollama", "pull", model} },
	}

	if err := preflight.Install(req, w); err != nil {
		fmt.Fprintf(w, "\nFailed: %v\n", err)

		return
	}

	fmt.Fprintf(w, "\nDone. %s is ready.\n", model)
}

// saveAPIKey writes the key into the brain's settings file. It is never logged or echoed back: the
// setup log is displayed in the window, and a key that appears there would be
// a key shown to anyone looking over the user's shoulder.
func (s *Server) saveAPIKey(provider, key string) error {
	setting := envKeyFor(provider)

	if key == "" {
		return fmt.Errorf("no key given")
	}

	if !strings.HasPrefix(key, "sk-ant-") {
		return fmt.Errorf("that does not look like an Anthropic key (they start with sk-ant-)")
	}

	// On a first run the settings file does not exist yet — which is exactly
	// when somebody is most likely to be pasting in a key. A missing file means
	// "no settings", not a failure.
	data, err := os.ReadFile(s.envPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not read %s: %w", s.envPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(s.envPath), 0o755); err != nil {
		return fmt.Errorf("could not create the settings folder: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	replaced := false

	for i, line := range lines {
		if strings.HasPrefix(line, setting+"=") {
			lines[i] = setting + "=" + key
			replaced = true

			break
		}
	}

	if !replaced {
		lines = append(lines, setting+"="+key)
	}

	// 0600: this file now holds a credential.
	return os.WriteFile(s.envPath, []byte(strings.Join(lines, "\n")), 0o600)
}

func (s *Server) hasAPIKey() bool {
	data, err := os.ReadFile(s.envPath)
	if err != nil {
		return false
	}

	/*
	 * Any of them counts.
	 *
	 * This decides whether setup can be finished without a local model, and
	 * that question is about having somewhere to send a request — not about
	 * which company. Checking only Anthropic would have told somebody with an
	 * OpenAI key that they still had nothing.
	 */
	for _, line := range strings.Split(string(data), "\n") {
		for _, setting := range []string{
			"ANTHROPIC_API_KEY=", "OPENAI_API_KEY=", "OPENROUTER_API_KEY=",
		} {
			if !strings.HasPrefix(line, setting) {
				continue
			}

			if strings.TrimSpace(strings.TrimPrefix(line, setting)) != "" {
				return true
			}
		}
	}

	return false
}

// syncWriter funnels install output into the buffer the window polls.
type syncWriter struct {
	s *Server
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()

	return w.s.log.Write(p)
}

// modelOptions renders the models worth offering this machine.
func modelOptions(hw preflight.Hardware) []map[string]any {
	options := preflight.ModelOptions(hw)

	out := make([]map[string]any, 0, len(options))

	for _, o := range options {
		out = append(out, map[string]any{
			"model":       o.Model,
			"size":        o.SizeNote,
			"speed":       o.SpeedNote,
			"label":       o.Label,
			"recommended": o.Recommended,
		})
	}

	return out
}

/*
 * modelPrefix marks an install request as being for a model rather than for
 * one of the machine's requirements.
 *
 * A prefix rather than the old "__model__" sentinel, because the name of the
 * model now travels with it and a sentinel has nowhere to put one.
 */
const modelPrefix = "model:"

func modelRequest(model string) string { return modelPrefix + strings.TrimSpace(model) }

/*
 * envKeyFor is the setting a provider's key is written to.
 *
 * Separate settings rather than one, because they are separate decisions:
 * privacy is judged by which company a request goes to, and somebody who
 * agreed to send conversation to OpenAI has not thereby agreed to OpenRouter.
 * An unknown name falls back to Anthropic, which is what every key meant
 * before there was a choice.
 */
func envKeyFor(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return "OPENAI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	default:
		return "ANTHROPIC_API_KEY"
	}
}
