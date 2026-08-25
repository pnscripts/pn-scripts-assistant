package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"pn-brain/internal/preflight"
)

// setupServer is a small HTTP server the desktop app runs itself, so first-run
// setup happens inside the application window rather than in a terminal.
//
// It has to exist separately from the brain because of an ordering problem: the
// brain cannot serve its own setup page on a machine that is missing the things
// the brain needs to start. So the app carries just enough of a web server to
// explain the situation and fix it, then hands the window over to the brain.
type setupServer struct {
	mu       sync.Mutex
	log      bytes.Buffer
	busy     bool
	envPath  string
	listener net.Listener
}

func newSetupServer(envPath string) (*setupServer, error) {
	// Port 0: the OS picks a free one. Hardcoding a port would collide with
	// whatever else the user happens to be running.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	return &setupServer{listener: l, envPath: envPath}, nil
}

func (s *setupServer) url() string {
	return "http://" + s.listener.Addr().String()
}

func (s *setupServer) serve(onReady func()) {
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
		go s.install("__model__")
		s.writeJSON(w, map[string]any{"started": true})
	})

	mux.HandleFunc("/api-key", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		if err := s.saveAPIKey(strings.TrimSpace(body.Key)); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})

			return
		}

		s.writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, map[string]any{"ok": true})
		go onReady()
	})

	_ = http.Serve(s.listener, mux)
}

func (s *setupServer) writeJSON(w http.ResponseWriter, v any) {
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

func (s *setupServer) state() map[string]any {
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
		"has_api_key": s.hasAPIKey(),
	}
}

func (s *setupServer) install(name string) {
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

	if name == "__model__" {
		hw := preflight.DetectHardware()
		choice := preflight.RecommendModel(hw)
		s.runModelPull(choice.Model)

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

func (s *setupServer) runModelPull(model string) {
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

// saveAPIKey writes the key into .env. It is never logged or echoed back: the
// setup log is displayed in the window, and a key that appears there would be
// a key shown to anyone looking over the user's shoulder.
func (s *setupServer) saveAPIKey(key string) error {
	if key == "" {
		return fmt.Errorf("no key given")
	}

	if !strings.HasPrefix(key, "sk-ant-") {
		return fmt.Errorf("that does not look like an Anthropic key (they start with sk-ant-)")
	}

	data, err := os.ReadFile(s.envPath)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", s.envPath, err)
	}

	lines := strings.Split(string(data), "\n")
	replaced := false

	for i, line := range lines {
		if strings.HasPrefix(line, "ANTHROPIC_API_KEY=") {
			lines[i] = "ANTHROPIC_API_KEY=" + key
			replaced = true

			break
		}
	}

	if !replaced {
		lines = append(lines, "ANTHROPIC_API_KEY="+key)
	}

	// 0600: this file now holds a credential.
	return os.WriteFile(s.envPath, []byte(strings.Join(lines, "\n")), 0o600)
}

func (s *setupServer) hasAPIKey() bool {
	data, err := os.ReadFile(s.envPath)
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ANTHROPIC_API_KEY=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "ANTHROPIC_API_KEY=")) != ""
		}
	}

	return false
}

// syncWriter funnels install output into the buffer the window polls.
type syncWriter struct {
	s *setupServer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()

	return w.s.log.Write(p)
}

func defaultEnvPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ".env"
	}

	// cmd/desktop/<binary> → repo root
	return filepath.Join(filepath.Dir(exe), "..", "..", "..", ".env")
}
