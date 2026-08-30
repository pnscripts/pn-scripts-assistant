package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"pn-brain/internal/brain/machine"
)

/*
 * What this machine can actually run.
 *
 * The models worth choosing are completely different on a laptop with four
 * cores and on a workstation with a graphics card, and until now the choice was
 * written for the first of those because that is what it was developed on. A
 * seven-billion-parameter model is the sensible ceiling here and an obviously
 * poor one on a machine that could hold ten times that in video memory.
 *
 * So the machine is asked rather than assumed. Ollama already knows the answer
 * and reports it per loaded model as size_vram: how much of that model it put
 * on the graphics card. Zero across the board means everything is running on
 * the processor, whatever hardware is nominally present — which is the fact
 * that matters, and is more honest than looking for a card and hoping the
 * drivers work.
 */

// Power is what the machine can be asked to do.
type Power struct {
	// Accelerated is true when Ollama is actually putting models on a graphics
	// card, rather than merely having one.
	Accelerated bool

	// VRAMBytes is how much it put there, across everything loaded. A lower
	// bound on the card rather than its size, and the useful number: it is what
	// has been demonstrated to work.
	VRAMBytes int64

	// Cores and RAMBytes describe the processor side, which is what carries
	// everything when there is no acceleration.
	Cores    int
	RAMBytes uint64
}

/*
 * Tier is the size of model this machine should be running.
 *
 * Named rather than numeric so that the reason survives: what matters is not
 * "level 2" but that the thing has a graphics card with room on it.
 */
type Tier string

const (
	// Modest is a processor and nothing else. Every answer costs real seconds,
	// so the smallest model that can do the job is the right one.
	Modest Tier = "modest"

	// Capable is a real graphics card, where a larger model costs memory rather
	// than minutes and is simply better.
	Capable Tier = "capable"

	// Generous is a card big enough that the largest models worth running fit
	// on it with room to spare.
	Generous Tier = "generous"
)

// WhatItCanRun asks the machine what it is capable of.
func WhatItCanRun(ctx context.Context, c *Client) Power {
	load := machine.Current()

	power := Power{Cores: load.Cores, RAMBytes: load.MemoryTotalBytes}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/ps", nil)
	if err != nil {
		return power
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return power
	}

	defer res.Body.Close()

	var out struct {
		Models []struct {
			SizeVRAM int64 `json:"size_vram"`
		} `json:"models"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return power
	}

	for _, m := range out.Models {
		power.VRAMBytes += m.SizeVRAM
	}

	power.Accelerated = power.VRAMBytes > 0

	return power
}

/*
 * Tier decides which models this machine should be offered.
 *
 * Deliberately conservative about claiming acceleration. Nothing loaded means
 * nothing measured, and guessing "capable" on a machine that turns out to be a
 * laptop produces an assistant that takes four minutes to say good morning —
 * which is a far worse first impression than a small model on a fast machine.
 */
func (p Power) Tier() Tier {
	const gb = 1 << 30

	switch {
	case p.Accelerated && p.VRAMBytes >= 16*gb:
		return Generous
	case p.Accelerated:
		return Capable
	default:
		return Modest
	}
}

// Describe says what was found, for a log or a settings page.
func (p Power) Describe() string {
	if !p.Accelerated {
		return fmt.Sprintf("%d cores, %.0fGB memory, no graphics acceleration",
			p.Cores, float64(p.RAMBytes)/(1<<30))
	}

	return fmt.Sprintf("%d cores, %.0fGB memory, %.0fGB on the graphics card",
		p.Cores, float64(p.RAMBytes)/(1<<30), float64(p.VRAMBytes)/(1<<30))
}
