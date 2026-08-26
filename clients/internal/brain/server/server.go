// Package server exposes the brain over HTTP on the loopback interface.
//
// The interface is a local web page rendered inside a native window, not a site.
// That distinction is why everything here binds to 127.0.0.1 and why there is
// no authentication: the only thing that can reach it is a process on this
// machine. Binding to every interface once put this brain's knowledge endpoints
// on the local network, which is a mistake worth making impossible rather than
// remembering not to make — see Listen.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pn-brain/internal/brain/brain"
	"pn-brain/internal/brain/speech"
	"pn-brain/internal/brain/storage"
)

// Server serves the interface and the API.
type Server struct {
	brain  *brain.Brain
	log    *slog.Logger
	mux    *http.ServeMux
	assets http.Handler
}

// New builds the server and registers its routes.
func New(b *brain.Brain, logger *slog.Logger) *Server {
	s := &Server{brain: b, log: logger, mux: http.NewServeMux(), assets: assetHandler(logger)}

	s.mux.HandleFunc("POST /api/chat", s.handleChat)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/memory-map", s.handleMemoryMap)
	s.mux.HandleFunc("GET /api/knowledge", s.handleKnowledge)
	s.mux.HandleFunc("GET /api/activity", s.handleActivity)
	s.mux.HandleFunc("GET /api/conversations/latest", s.handleLatestConversation)
	s.mux.HandleFunc("GET /api/approvals", s.handleApprovals)
	s.mux.HandleFunc("POST /api/approvals/{id}/{decision}", s.handleDecision)
	s.mux.HandleFunc("GET /api/lessons", s.handleLessons)
	s.mux.HandleFunc("POST /api/lessons/{id}/{decision}", s.handleLessonDecision)
	s.mux.HandleFunc("GET /api/drives", s.handleDrives)
	s.mux.HandleFunc("POST /api/speak", s.handleSpeak)
	s.mux.HandleFunc("POST /api/listen", s.handleListen)
	s.mux.HandleFunc("GET /health", s.handleHealth)

	s.mux.HandleFunc("GET /", s.handleRoot)

	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

// Listen binds the address, refusing anything that is not loopback.
//
// This is checked rather than documented because the failure is silent: a brain
// bound to 0.0.0.0 works perfectly for its owner while serving everything it
// knows to the network around it.
func Listen(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("address %q is not host:port: %w", addr, err)
	}

	if !isLoopback(host) {
		return nil, fmt.Errorf(
			"refusing to listen on %q: the brain serves everything it knows without "+
				"authentication, so it must stay on the loopback interface", addr,
		)
	}

	return net.Listen("tcp", addr)
}

func isLoopback(host string) bool {
	// An empty host is not "unspecified, therefore harmless" — in ":8790" it
	// means every interface, which is precisely the case this refuses. Treating
	// it as loopback made the guard pass the one address most likely to be
	// typed by somebody who wanted a shortcut.
	if host == "" {
		return false
	}

	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// Serve runs until the context is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler: s.mux,
		// A reply waits on a language model. On a machine without a GPU a single
		// answer can take longer than a minute on its own, so a write timeout
		// tuned for ordinary web traffic would cut off correct answers.
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		<-ctx.Done()

		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		srv.Shutdown(shutdown)
	}()

	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

/* ---------- handlers ---------- */

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req brain.ChatRequest

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "That message could not be read: "+err.Error())

		return
	}

	reply, err := s.brain.Chat(r.Context(), req)
	if err != nil {
		// The message is shown to a person, so it has to say what happened
		// rather than "internal error".
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, reply)
}

// handleRoot serves the interface, and only for the exact root path: Go's
// "GET /" pattern also matches every unclaimed path, so anything else is an
// asset request or a genuine miss.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.renderIndex(w, r)

		return
	}

	s.assets.ServeHTTP(w, r)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	facts, err := s.brain.DB.CountFacts()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	categories, _ := s.brain.DB.FactsByCategory()
	pending, _ := s.brain.DB.CountPendingLessons()
	conversations, _ := s.brain.DB.CountConversations()

	ok(w, map[string]any{
		"name":         s.brain.Cfg.Name,
		"owner":        s.brain.Cfg.Owner,
		"provider":     s.brain.Cfg.DefaultProvider,
		"model":        s.brain.Cfg.OllamaModel,
		"privacy":      s.brain.Mode.Describe(),
		"providers":    s.brain.Router.Availabilities(r.Context()),
		"capabilities": s.brain.Capabilities(),
		"storage":      s.brain.Storage(),
		"memory": map[string]any{
			"facts":           facts,
			"categories":      categories,
			"pending_lessons": pending,
			"conversations":   conversations,
		},
	})
}

// handleApprovals lists actions waiting for a person to allow them.
func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	pending, err := s.brain.PendingApprovals()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// A bare array, because that is what the interface expects to count.
	ok(w, pending)
}

// handleDecision records an approval or denial and carries out what was allowed.
func (s *Server) handleDecision(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "That is not an action id.")

		return
	}

	decision := r.PathValue("decision")

	if decision != "approve" && decision != "deny" {
		fail(w, http.StatusBadRequest, `The decision must be "approve" or "deny".`)

		return
	}

	invocation, execErr := s.brain.Decide(r.Context(), id, decision == "approve")

	// An action that ran and failed is still a completed decision, so the
	// outcome is reported rather than turned into an HTTP error — the person
	// needs to see what happened, not a status code.
	body := map[string]any{"invocation": invocation}

	if execErr != nil {
		body["error"] = execErr.Error()
	}

	ok(w, body)
}

// handleLessons lists what the brain proposes to remember but has not been
// allowed to. These are model inferences, which nothing can verify — the whole
// reason they wait for a person rather than promoting themselves.
func (s *Server) handleLessons(w http.ResponseWriter, r *http.Request) {
	lessons, err := s.brain.DB.LessonsByStatus("proposed", 100)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, lessons)
}

// handleLessonDecision accepts or rejects a proposed lesson.
func (s *Server) handleLessonDecision(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "That is not a lesson id.")

		return
	}

	decision := r.PathValue("decision")

	if decision != "accept" && decision != "reject" {
		fail(w, http.StatusBadRequest, `The decision must be "accept" or "reject".`)

		return
	}

	result, err := s.brain.DecideLesson(r.Context(), id, decision == "accept")
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, result)
}

func (s *Server) handleMemoryMap(w http.ResponseWriter, r *http.Request) {
	m, err := s.brain.DB.BuildMap()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, m)
}

func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	limit := 50

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	facts, err := s.brain.DB.RecentFacts(limit)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"facts": facts})
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	items, err := s.brain.DB.RecentActivity(25)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"activity": items})
}

func (s *Server) handleLatestConversation(w http.ResponseWriter, r *http.Request) {
	c, err := s.brain.DB.LatestConversation()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if c == nil {
		ok(w, map[string]any{"conversation": nil, "messages": []any{}})

		return
	}

	history, err := s.brain.DB.History(c.ID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// The system prompt is machinery, not conversation; showing it would put
	// the assistant's own instructions in the transcript.
	visible := make([]any, 0, len(history))

	for _, m := range history {
		if m.Role == "system" {
			continue
		}

		visible = append(visible, m)
	}

	ok(w, map[string]any{"conversation": c, "messages": visible})
}

// handleDrives lists where the brain could live.
//
// Read-only on purpose. Moving is not exposed over HTTP: it copies gigabytes,
// deletes the original, and must happen while nothing is writing to the
// database — none of which belongs behind a request that can be retried,
// cancelled by a closed window, or fired twice by an impatient click.
func (s *Server) handleDrives(w http.ResponseWriter, r *http.Request) {
	drives, err := storage.Drives(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"current": s.brain.Root,
		"drives":  drives,
		"storage": s.brain.Storage(),
		"how":     "Run 'pn-brain move <folder>' to relocate the brain. It verifies every byte before removing the original.",
	})
}

// handleSpeak reads a reply aloud through a local engine.
//
// Safe and ungated: it plays sound on the machine the request came from, which
// is the same machine that made it. Nothing leaves, and nothing changes.
func (s *Server) handleSpeak(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "Nothing to say.")

		return
	}

	if err := speech.Speak(r.Context(), body.Text); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"speaking": true})
}

// handleListen records from the microphone and returns what was said.
//
// It does not send the result anywhere: the transcript comes back to the
// interface, which puts it in the input box for a person to read before it is
// asked. A recogniser that mishears "delete the backups" should not have that
// go straight to a brain with tools.
func (s *Server) handleListen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seconds int `json:"seconds"`
	}

	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)

	if body.Seconds <= 0 {
		body.Seconds = 6
	}

	text, err := speech.Listen(r.Context(), body.Seconds)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"text": text})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"status": "ok"})
}

/* ---------- helpers ---------- */

func ok(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
