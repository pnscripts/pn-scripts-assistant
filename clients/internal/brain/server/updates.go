package server

import (
	"context"
	"net/http"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"pn-brain/internal/brain/models"
	"pn-brain/internal/brain/speech"
)

// What the brain is made of, and what any of it costs.
//
// The cost column is the point of this. Everything here runs on this machine
// and is free to use, and that is worth stating plainly rather than leaving
// somebody to assume there is a meter running somewhere. Where a paid service
// is configured it says so and says it is billed by whoever provides it — it
// does not print a rate, because a rate copied into source is a rate that goes
// out of date without anybody noticing, and a wrong price is worse than none.

// Part is one piece the brain depends on.
type Part struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
	Cost    string `json:"cost"`
	Note    string `json:"note"`
}

// handleUpdates reports what is installed and what it costs.
func (s *Server) handleUpdates(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	parts := []Part{brainPart(), ollamaPart(ctx, s.brain.Cfg.OllamaURL)}

	parts = append(parts, modelParts(ctx, s.brain.Cfg.OllamaURL, s.brain.Cfg.OllamaModel, s.brain.Cfg.EmbedModel)...)
	parts = append(parts, voicePart(), earsPart(), cloudPart(s.brain.Cfg.AnthropicKey != "", s.brain.Cfg.AnthropicModel))

	ok(w, map[string]any{"parts": parts})
}

// brainPart describes this program.
//
// The version comes from the build rather than a constant somebody has to
// remember to bump, which is the version that is actually running.
func brainPart() Part {
	version := "built from source"

	if info, readable := debug.ReadBuildInfo(); readable {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
				version = setting.Value[:7]
			}
		}
	}

	return Part{
		Name:    "PN Brain",
		Version: version,
		Status:  "running",
		Cost:    "free",
		Note:    "Yours. One file, no account, nothing to renew.",
	}
}

func ollamaPart(ctx context.Context, url string) Part {
	part := Part{Name: "Ollama", Cost: "free", Note: "Runs the models on this machine."}

	out, err := exec.CommandContext(ctx, "ollama", "--version").Output()
	if err == nil {
		part.Version = strings.TrimSpace(strings.TrimPrefix(string(out), "ollama version is "))
	}

	if models.New(url).Has(ctx, "") || part.Version != "" {
		part.Status = "installed"
	} else {
		part.Status = "not found"
		part.Note = "Without it the brain cannot think. Install it from ollama.com."
	}

	return part
}

func modelParts(ctx context.Context, url, chat, embed string) []Part {
	installed, err := models.New(url).List(ctx)
	if err != nil {
		return []Part{{
			Name: "Models", Status: "unknown", Cost: "free",
			Note: "Ollama is not answering, so what is installed cannot be read.",
		}}
	}

	var parts []Part

	for _, m := range installed {
		role := "installed"

		switch {
		case m.Name == chat || strings.TrimSuffix(m.Name, ":latest") == strings.TrimSuffix(chat, ":latest"):
			role = "answering"
		case m.Name == embed || strings.TrimSuffix(m.Name, ":latest") == strings.TrimSuffix(embed, ":latest"):
			role = "remembering"
		}

		parts = append(parts, Part{
			Name:    m.Name,
			Version: m.Size,
			Status:  role,
			Cost:    "free",
			Note:    "Downloaded once, runs here, costs nothing to use.",
		})
	}

	return parts
}

// voicePart asks the speech package rather than looking on PATH.
//
// The first version checked PATH and reported "not found" for a piper that was
// installed and working — it lives under the owner's home directory, which is
// where the tarball unpacks and where the brain has always looked for it. A
// panel that reports a working part as missing is the exact failure this
// program is built to avoid, and it took reading the output to catch.
func voicePart() Part {
	engine := speech.Available()

	if engine == nil {
		return Part{
			Name: "Voice", Status: "not found", Cost: "free",
			Note: "Nothing installed that can speak, so replies are text only.",
		}
	}

	note := "The voice."

	if engine.Name != "piper" {
		note = "A plainer system voice. Piper sounds considerably better if you install it."
	}

	return Part{
		Name:    strings.ToUpper(engine.Name[:1]) + engine.Name[1:],
		Status:  "installed",
		Cost:    "free",
		Note:    note,
		Version: "",
	}
}

// earsPart asks the speech package too, for the same reason as voicePart.
func earsPart() Part {
	listening, detail := speech.Listening()

	if !listening {
		return Part{
			Name: "Whisper", Status: "not found", Cost: "free",
			Note: "Without it the brain cannot listen, only read what you type. " + detail,
		}
	}

	return Part{Name: "Whisper", Status: "installed", Cost: "free", Note: "Hearing you speak. " + detail}
}

// cloudPart is the only thing here that can cost money.
func cloudPart(configured bool, model string) Part {
	if !configured {
		return Part{
			Name: "Cloud model", Status: "not configured", Cost: "—",
			Note: "Nothing is sent anywhere. Everything above runs on this machine.",
		}
	}

	return Part{
		Name: model, Status: "configured", Cost: "billed by usage",
		Note: "The one paid part. Charged per token by the provider — see their pricing " +
			"page for current rates; a rate printed here would go out of date silently.",
	}
}
