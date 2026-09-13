package setup

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/catalogue"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * Every model there is, for somebody who wants more than the eight.
 *
 * The list on the page is eight models chosen for being the ones most people
 * should pick between, and that is the right thing to show first — a few
 * hundred entries is not a choice, it is a search problem. It is the wrong
 * thing to show *only*, because the honest answer to "are those the only
 * ones?" is no, and a program that lists eight without saying so has answered
 * a question with a smaller truth.
 *
 * Fetched rather than written down: ollama's library changes, and a copy of it
 * in this file would be a list that is wrong by the month.
 */

type libraryEntry struct {
	Name string   `json:"name"`
	What string   `json:"what"`
	Can  []string `json:"can,omitempty"`

	// Size is the variant this row installs — a model published in five sizes
	// becomes five rows, because the size is the whole of the decision.
	Size string `json:"size"`

	// NeedsGB is roughly what it wants to answer at a sensible speed, and Fits
	// is whether this machine has it.
	NeedsGB int  `json:"needs_gb"`
	Fits    bool `json:"fits"`

	Installed bool   `json:"installed"`
	Pulls     string `json:"pulls,omitempty"`
}

/*
 * handleLibrary lists the whole library, judged against this machine.
 *
 * Not gated, unlike the same list inside the running program, and the reason
 * is that there is nothing here to gate it on: the permission that governs
 * looking things up is a setting, and setup is what runs before there are
 * settings. It is also consistent with the rest of this screen, which reaches
 * the network to fetch every piece it installs — a page that downloads a
 * gigabyte on request but will not ask a website which models exist would be
 * drawing a line in a strange place.
 *
 * It sends nothing about anybody: it asks ollama.com what it publishes.
 */
func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	models, err := catalogue.Fetch(ctx, &http.Client{Timeout: 15 * time.Second})
	if err != nil {
		s.writeJSON(w, map[string]any{
			"ok": false,
			"error": "I could not reach ollama's library just now. " +
				"The eight above are installable without it.",
		})

		return
	}

	hw := preflight.DetectHardware()
	here := modelsHere()

	out := []libraryEntry{}

	for _, m := range models {
		/*
		 * Embedding models are left out.
		 *
		 * They turn text into numbers and cannot hold a conversation, and this
		 * list sits under the heading "run one here" — offering one as a brain
		 * would produce a program that starts and answers nothing. The one
		 * embedding model this needs is installed as a requirement, on the
		 * step before.
		 */
		if m.Embedding() {
			continue
		}

		sizes := m.Sizes
		if len(sizes) == 0 {
			sizes = []string{""}
		}

		for _, size := range sizes {
			name := m.Name
			if size != "" {
				name = m.Name + ":" + size
			}

			needs := int(catalogue.MemoryFor(size) + 0.5)

			out = append(out, libraryEntry{
				Name:      name,
				What:      m.What,
				Can:       m.Can,
				Size:      size,
				NeedsGB:   needs,
				Fits:      needs == 0 || hw.RAMGB >= needs,
				Installed: here[name] || here[strings.TrimSuffix(name, ":latest")],
				Pulls:     m.Pulls,
			})
		}
	}

	// What fits first, then by name: a list whose first screen is models this
	// machine cannot run is a list that has to be scrolled past before it
	// becomes useful.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Fits != out[j].Fits {
			return out[i].Fits
		}

		return out[i].Name < out[j].Name
	})

	s.writeJSON(w, map[string]any{"ok": true, "models": out})
}
