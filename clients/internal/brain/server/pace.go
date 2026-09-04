package server

import (
	"net/http"

	"encoding/json"

	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/pace"
	"pn-brain/internal/brain/speech"
	"pn-brain/internal/preflight"
)

/*
 * handlePace says how long answering takes, broken into the four waits.
 *
 * The point of showing it is that the same program is slow for opposite
 * reasons on different machines. On four processor cores the model is
 * effectively all of it and the rest is a rounding error; on a card the model
 * drops under a second and what is left is the silence this program waits out
 * and the process it starts to speak with. Both look like "it is slow", and a
 * single total cannot tell them apart.
 */
func (s *Server) handlePace(w http.ResponseWriter, r *http.Request) {
	typical, over := pace.Typical()

	recent := pace.Recent()

	turns := make([]pace.Milestones, 0, len(recent))

	for i, t := range recent {
		if i >= 12 {
			break
		}

		turns = append(turns, t.Milestones())
	}

	hardware := preflight.DetectHardware()
	roles := s.brain.ModelRoles()

	machine := pace.Machine{
		Cores:    hardware.CPUCores,
		MemoryGB: hardware.RAMGB,
		HasGPU:   hardware.HasGPU,
		GPUName:  hardware.GPUName,
		Working:  roles.Work,
		Quick:    roles.Talk,
		Suggest:  llm.TalkCandidates[0],
	}

	ok(w, map[string]any{
		"typical":  typical,
		"over":     over,
		"turns":    turns,
		"fixed":    s.fixedCosts(),
		"verdict":  pace.Verdict(typical, over),
		"findings": pace.Findings(typical, over, machine),
		"target":   pace.RealTime,
	})
}

/*
 * fixedCosts is what this program costs before any model is involved.
 *
 * These are the numbers that do not improve when the machine does, which makes
 * them the whole answer to "what would it take to be quick on better
 * hardware". A card can take the model from ninety seconds to half a second
 * and leave a two-second answer with all of it spent here.
 */
func (s *Server) fixedCosts() map[string]any {
	return map[string]any{
		"silence_ms": int(speech.SilenceToEnd.Milliseconds()),
		"tool_words": s.toolPromptSize(),
	}
}

/*
 * toolPromptSize is roughly how much prompt the tools take up, in tokens.
 *
 * Counted from the live registry rather than written down, because it grows
 * every time a tool is added and nobody would remember to update a constant.
 * It is read before the model reads it: the whole of this has to be processed
 * before the first word of an answer can be produced, on every turn that is
 * offered tools.
 *
 * A rough four characters to the token. Precise enough for the only decision
 * it informs — whether this is a large part of the wait — and getting it
 * exactly right would mean shipping a tokeniser to answer a question that is
 * asked in orders of magnitude.
 */
func (s *Server) toolPromptSize() int {
	total := 0

	for _, tool := range s.brain.Agent.Registry.All() {
		described, err := json.Marshal(map[string]any{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  json.RawMessage(tool.Parameters()),
		})
		if err != nil {
			continue
		}

		total += len(described)
	}

	return total / 4
}
