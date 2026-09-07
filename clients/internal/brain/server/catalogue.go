package server

import (
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/catalogue"
	"pn-scripts-assistant/internal/brain/models"
)

/*
 * Every model that can be installed, judged against the machine asking.
 *
 * This began as nine models written into a file, which is a list of what one
 * person had heard of — wrong within a month, and wrong in the direction that
 * matters: somebody looking for the model they read about yesterday does not
 * find it and concludes the program cannot run it. The list comes from
 * ollama's own library now, which is where the answer lives.
 *
 * The list is theirs; the opinion is this machine's. Which of two hundred and
 * thirty-nine models is a good idea is entirely a fact about the computer
 * being asked, and this program runs on machines with no graphics card and
 * machines with two. So every size carries what it wants in memory while
 * running — not its download size, which is the number everybody quotes and
 * the wrong one — and the verdict is worked out from what the machine
 * measured, now rather than at startup.
 */

// Variant is one size of one model: the thing somebody actually installs.
type Variant struct {
	// Name is what to pull: "qwen3:8b", or just "nomic-embed-text" for the
	// ones published in a single size.
	Name string `json:"name"`

	Size string `json:"size,omitempty"`

	// NeedsGB is roughly what it wants in memory while it runs. An estimate,
	// and the interface says so.
	NeedsGB float64 `json:"needs_gb"`

	Installed bool   `json:"installed"`
	Fits      bool   `json:"fits"`
	Verdict   string `json:"verdict"`
}

// Listed is one model with every size it comes in.
type Listed struct {
	Name     string    `json:"name"`
	What     string    `json:"what"`
	Can      []string  `json:"can,omitempty"`
	Variants []Variant `json:"variants"`

	// AnyInstalled saves the interface working it out to decide what to show
	// first: a model somebody already has is the one they came to look at.
	AnyInstalled bool `json:"any_installed"`
}

// handleCatalogue lists everything available and what this machine makes of it.
func (s *Server) handleCatalogue(w http.ResponseWriter, r *http.Request) {
	/*
	 * The library is on the internet, so privacy decides whether it is asked
	 * for at all.
	 *
	 * A model list is a small thing to leak and it is still a request that
	 * says this machine exists and is looking for models. Somebody who set
	 * their brain to keep to itself did not make an exception for shopping.
	 */
	if !s.brain.Router.Mode().AllowsWeb() {
		ok(w, map[string]any{
			"models": []Listed{},
			"why": "The list of models lives on the internet, and your privacy " +
				"setting keeps this machine to itself. Change it on the Privacy " +
				"page to see what can be installed.",
		})

		return
	}

	client := models.New(s.brain.Cfg.OllamaURL)

	installed := map[string]bool{}

	if here, err := client.List(r.Context()); err == nil {
		for _, m := range here {
			installed[m.Name] = true

			// ollama reports "qwen3:8b"; somebody may have pulled "qwen3",
			// which is the same download under its default tag.
			if family, _, found := strings.Cut(m.Name, ":"); found {
				installed[family] = true
			}
		}
	}

	library, err := catalogue.Fetch(r.Context(), nil)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())

		return
	}

	/*
	 * Asked of the machine now, not remembered from startup.
	 *
	 * A graphics card can be busy with something else and memory can be full,
	 * and somebody looking at this page is deciding what to download — so the
	 * answer has to be about the machine as it is.
	 */
	power := models.WhatItCanRun(r.Context(), client)

	out := make([]Listed, 0, len(library))

	for _, m := range library {
		listed := Listed{Name: m.Name, What: m.What, Can: m.Can}

		sizes := m.Sizes

		// Published in one size only — the name is the whole of it.
		if len(sizes) == 0 {
			sizes = []string{""}
		}

		for _, size := range sizes {
			name := m.Name

			if size != "" {
				name += ":" + size
			}

			v := Variant{
				Name:      name,
				Size:      size,
				NeedsGB:   catalogue.MemoryFor(size),
				Installed: installed[name],
			}

			// An embedding model is small and runs once per thing remembered;
			// the size list is usually empty and the machine is never the
			// limit.
			if m.Embedding() || v.NeedsGB == 0 {
				v.NeedsGB = 1
			}

			v.Fits, v.Verdict = judge(v.NeedsGB, m.Embedding(), power)

			listed.AnyInstalled = listed.AnyInstalled || v.Installed
			listed.Variants = append(listed.Variants, v)
		}

		out = append(out, listed)
	}

	ok(w, map[string]any{
		"machine": power.Describe(),
		"tier":    string(power.Tier()),
		"models":  out,
	})
}

/*
 * judge says what this machine will make of a model that wants this much.
 *
 * Three different answers for three different machines, and the same model
 * gets all three depending on where it is asked. A card with room means larger
 * is simply better; a processor alone means every gigabyte is seconds per
 * answer; and not enough memory at all means the machine will swap itself to a
 * standstill rather than fail honestly, which is the worst of the three and
 * the one worth warning about.
 */
func judge(needsGB float64, embedding bool, p models.Power) (fits bool, verdict string) {
	const gb = 1 << 30

	room := float64(p.RAMBytes) / gb

	if embedding {
		// Small, and run once per thing remembered rather than per answer.
		// The tier does not change the answer.
		return true, "fine on any machine"
	}

	if p.Accelerated && p.VRAMBytes > 0 {
		card := float64(p.VRAMBytes) / gb

		switch {
		case needsGB <= card:
			return true, "fits on your graphics card — this will be quick"

		case needsGB <= room:
			return true, "too big for your graphics card, so it runs on the processor — slower"

		default:
			return false, "larger than this machine's memory"
		}
	}

	switch {
	case needsGB > room:
		return false, "larger than this machine's memory — it would swap and crawl"

	case needsGB <= 3.5:
		return true, "quick enough on a processor"

	case needsGB <= 6:
		return true, "usable on a processor — seconds, not instant"

	default:
		return true, "slow without a graphics card — minutes per answer"
	}
}
