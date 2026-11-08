package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"pn-scripts-assistant/internal/brain/jobs"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/speech"
)

/*
 * handlePulse tells the page, as it happens, that the assistant has started
 * or stopped doing something — so the page can rest instead of asking.
 *
 * The page used to find out by polling: thirty-odd timers, the microphone
 * level eleven times a second, each one waking the program, the page and the
 * display whether anything had changed or not (see pace.js, measured
 * 08.11.2026). Now a page nobody is using holds every timer and keeps only
 * this stream open, and the stream says something only when the answer to
 * "is anything happening?" changes.
 *
 * It looks once a second, and only while a page is connected: with no window
 * open nothing here runs at all. The first message says where things stand;
 * every later one is a change.
 */
func (s *Server) handlePulse(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, http.StatusInternalServerError, "This connection cannot stream.")

		return
	}

	// A stream outlives the server's write timeout by design.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	every := s.pulseEvery
	if every <= 0 {
		every = time.Second
	}

	tick := time.NewTicker(every)
	defer tick.Stop()

	last := ""
	memory, looked := -1, 0

	for {
		// What it knows is counted once a minute, not every second: it
		// changes all the time while it learns, and a page that rests only
		// needs its numbers brought up to date now and then.
		if looked%pulseMemoryEvery == 0 {
			memory = s.pulseMemory()
		}

		looked++

		now := s.pulseNow(memory)

		if now != last {
			if _, err := fmt.Fprintf(w, "event: pulse\ndata: %s\n\n", now); err != nil {
				return
			}

			flusher.Flush()

			last = now
		}

		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

// pulse is what the page needs to know to decide whether to wake.
type pulse struct {
	// Busy: a question is being answered, a tool is running, or the
	// assistant is speaking. The page stays awake while it is.
	Busy bool `json:"busy"`
	// Kind of work, when there is some ("thinking", "tool", "speaking").
	Kind string `json:"kind,omitempty"`
	// Heard is somebody talking in the room, loud enough to be speech.
	Heard bool `json:"heard,omitempty"`
	// Waiting is how many actions wait for a decision.
	Waiting int `json:"waiting"`
	// Jobs is how many background jobs are running: a job starting or
	// finishing wakes the page, a job running does not keep it awake.
	Jobs int `json:"jobs"`
	// Memory is how many things it knows, counted once a minute. A change
	// in this alone does not wake the page; it refreshes its numbers once
	// and goes on resting.
	Memory int `json:"memory"`
}

// pulseMemoryEvery is how many looks pass between counts of what it knows.
const pulseMemoryEvery = 60

func (s *Server) pulseMemory() int {
	if s.brain == nil || s.brain.DB == nil {
		return 0
	}

	n, err := s.brain.DB.CountFacts()
	if err != nil {
		return 0
	}

	return n
}

func (s *Server) pulseNow(memory int) string {
	p := pulse{Memory: memory}

	step := progress.Now()

	// Listening is not work: the microphone is open whenever the assistant is
	// running, and a silent room is the resting state. Nor is learning in the
	// background, which can go on for an hour with nobody watching it.
	if step.InATurn || (step.Busy && step.Kind != "listening" && !step.Background) {
		p.Busy = true
		p.Kind = step.Kind
	}

	level := speech.LiveLevel()

	switch {
	case level.Source == "voice":
		p.Busy, p.Kind = true, "speaking"
	case level.Source == "mic" && level.Speech > 0 && level.Level >= level.Speech:
		p.Heard = true
	}

	if s.brain != nil && s.brain.DB != nil {
		if pending, err := s.brain.PendingApprovals(); err == nil {
			p.Waiting = len(pending)
		}
	}

	if s.brain != nil && s.brain.Jobs != nil {
		for _, job := range s.brain.Jobs.List() {
			if job.State == jobs.Running {
				p.Jobs++
			}
		}
	}

	out, _ := json.Marshal(p)

	return string(out)
}
