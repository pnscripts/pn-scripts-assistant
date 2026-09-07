package server

import (
	"net/http"
	"strings"

	"pn-brain/internal/brain/models"
)

/*
 * The models somebody can install, judged against the machine they are on.
 *
 * /api/models says what is installed. Nothing said what could be, so choosing
 * a different model meant knowing ollama's catalogue by heart and typing a
 * name into a box — a capability that exists for whoever already knows the
 * answer, which is the same failure the API keys had.
 *
 * The list is written down; the opinion is not. Which of these is a good idea
 * is entirely a fact about the computer it is being asked on, and this program
 * will run on machines with no graphics card and machines with two. A fixed
 * recommendation would be right on one of them and wrong everywhere else — so
 * every model carries what it needs, and what to say about it is worked out
 * here from what this machine actually measured.
 *
 * The list itself is short and stable on purpose. Every model in the world is
 * not a list, it is a search problem; and a name that is retired fails to pull
 * and says so, which is a better failure than an empty page because a remote
 * index moved.
 */

// Model is one somebody could install.
type Model struct {
	Name string `json:"name"`

	// Job is what it is for, in the terms this program uses: work is tools and
	// reasoning, talk is short answers quickly, memory is recall.
	Job string `json:"job"`

	Size string `json:"size"`
	What string `json:"what"`

	/*
	 * NeedsGB is roughly what it wants in memory while it runs.
	 *
	 * Not the download size, which is what everybody quotes and is the wrong
	 * number: a 4.7GB model needs about six gigabytes free to answer, and a
	 * machine with eight will swap itself to a standstill trying.
	 */
	NeedsGB float64 `json:"needs_gb"`

	// Filled in per machine, which is the whole point.
	Installed bool   `json:"installed"`
	Verdict   string `json:"verdict"`
	Fits      bool   `json:"fits"`
}

// catalogue is the short honest set. Sizes are the download; NeedsGB is what
// it wants free while running.
var catalogue = []Model{
	{
		Name: "llama3.2:3b", Job: "talk", Size: "2.0GB", NeedsGB: 3,
		What: "Small and quick. Good for short answers and small talk, " +
			"which is most of what gets said to an assistant.",
	},
	{
		Name: "gemma3:4b", Job: "work", Size: "3.3GB", NeedsGB: 4.5,
		What: "Half the size of the others and holds a conversation well.",
	},
	{
		Name: "mistral:7b", Job: "work", Size: "4.1GB", NeedsGB: 5.5,
		What: "Fast for its size, and steady at ordinary writing.",
	},
	{
		Name: "qwen2.5-coder:7b", Job: "work", Size: "4.7GB", NeedsGB: 6,
		What: "Reads and writes code, and uses tools reliably.",
	},
	{
		Name: "qwen3:8b", Job: "work", Size: "5.2GB", NeedsGB: 7,
		What: "Newer, reasons more carefully, and is slower for it.",
	},
	{
		Name: "deepseek-r1:8b", Job: "reason", Size: "5.2GB", NeedsGB: 7,
		What: "Thinks step by step before answering. Better at problems that " +
			"need working through, and it shows its working, which costs time.",
	},
	{
		Name: "gpt-oss:20b", Job: "work", Size: "13GB", NeedsGB: 16,
		What: "Large. Worth it only on a machine with a real graphics card.",
	},
	{
		Name: "nomic-embed-text", Job: "memory", Size: "274MB", NeedsGB: 1,
		What: "Turns memories into numbers so they can be recalled by meaning. " +
			"Nothing is learned or recalled without one of these.",
	},
	{
		Name: "mxbai-embed-large", Job: "memory", Size: "670MB", NeedsGB: 1.5,
		What: "A larger memory model. Recalls a little better, and switching " +
			"to it re-indexes everything already stored.",
	},
}

// handleCatalogue lists the models with what this machine will make of them.
func (s *Server) handleCatalogue(w http.ResponseWriter, r *http.Request) {
	client := models.New(s.brain.Cfg.OllamaURL)

	installed := map[string]bool{}

	here, err := client.List(r.Context())
	if err == nil {
		for _, m := range here {
			installed[m.Name] = true

			// ollama reports "qwen3:8b"; somebody may have pulled "qwen3". The
			// family counts as present for the purpose of not offering it
			// twice.
			if family, _, found := strings.Cut(m.Name, ":"); found {
				installed[family] = true
			}
		}
	}

	/*
	 * Asked of the machine now, not remembered from startup.
	 *
	 * A graphics card can be busy with something else, memory can be full, and
	 * somebody looking at this page is deciding what to download — so the
	 * answer has to be about the machine as it is rather than as it was when
	 * the program opened.
	 */
	power := models.WhatItCanRun(r.Context(), client)

	out := make([]Model, 0, len(catalogue))

	for _, m := range catalogue {
		m.Installed = installed[m.Name]
		m.Fits, m.Verdict = judge(m, power)

		out = append(out, m)
	}

	ok(w, map[string]any{
		"machine": power.Describe(),
		"tier":    string(power.Tier()),
		"models":  out,
	})
}

/*
 * judge says what this machine will make of one model.
 *
 * Three different answers for three different machines, and the same model
 * gets all three depending on where it is asked. A card with room means larger
 * is simply better; a processor alone means every gigabyte is seconds per
 * answer; and not enough memory at all means the machine will swap itself to a
 * standstill rather than fail honestly, which is the worst of the three and
 * the one worth warning about.
 */
func judge(m Model, p models.Power) (fits bool, verdict string) {
	const gb = 1 << 30

	room := float64(p.RAMBytes) / gb

	if p.Accelerated && p.VRAMBytes > 0 {
		// What the card can hold is what decides, when there is a card.
		card := float64(p.VRAMBytes) / gb

		switch {
		case m.NeedsGB <= card:
			return true, "fits on your graphics card — this will be quick"

		case m.NeedsGB <= room:
			return true, "too big for your graphics card, so it runs on the processor — slower"

		default:
			return false, "larger than this machine's memory"
		}
	}

	switch {
	case m.NeedsGB > room:
		return false, "larger than this machine's memory — it would swap and crawl"

	case m.Job == "memory":
		// These run once per thing remembered and are small; the tier does not
		// change the answer.
		return true, "fine on any machine"

	case m.NeedsGB <= 3.5:
		return true, "quick enough on a processor"

	case m.NeedsGB <= 6:
		return true, "usable on a processor — expect seconds, not instant"

	default:
		return true, "slow on a processor without a graphics card — minutes per answer"
	}
}
