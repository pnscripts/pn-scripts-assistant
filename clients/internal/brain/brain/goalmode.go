package brain

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"pn-scripts-assistant/internal/brain/autonomy"
	"pn-scripts-assistant/internal/brain/catalogue"
	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/route"
	"pn-scripts-assistant/internal/brain/storage"
)

/*
 * Goal mode on this machine: which models it may download for which work,
 * where they go, and how much room there is. The choosing, checking and
 * recording are the autonomy package's; this is the wiring.
 */

// modelFamilies are what goal mode downloads for each kind of need, smallest
// first: open models from the Ollama library, free to use. The size that fits
// the machine is chosen by autonomy.Choose.
var modelFamilies = map[string][]string{
	"coding":  {"qwen2.5-coder:1.5b", "qwen2.5-coder:7b", "qwen2.5-coder:14b", "qwen2.5-coder:32b"},
	"vision":  {"qwen2.5vl:3b", "qwen2.5vl:7b", "gemma3:12b", "gemma3:27b"},
	"general": {"llama3.2:3b", "qwen3:4b", "qwen3:8b", "qwen3:14b", "qwen3:32b"},
}

// modelOptions are the downloads that could meet a need, with their sizes.
func modelOptions(needs route.Needs) []autonomy.Option {
	family := "general"

	switch {
	case needs.Vision:
		family = "vision"
	case needs.Coding >= 2:
		family = "coding"
	}

	var out []autonomy.Option

	for _, name := range modelFamilies[family] {
		size := name[len(name)-1:]
		if i := lastColon(name); i >= 0 {
			size = name[i+1:]
		}

		out = append(out, autonomy.Option{Name: name, GB: catalogue.MemoryFor(size)})
	}

	return out
}

func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}

	return -1
}

// ollamaPuller is Ollama as autonomy.Puller.
type ollamaPuller struct{ c *models.Client }

func (p ollamaPuller) Pull(ctx context.Context, name string, progress func(string)) error {
	return p.c.Pull(ctx, name, progress)
}

func (p ollamaPuller) DigestOf(ctx context.Context, name string) string {
	list, err := p.c.List(ctx)
	if err != nil {
		return ""
	}

	for _, m := range list {
		if m.Name == name {
			return m.Digest
		}
	}

	return ""
}

// modelsFolder is where Ollama keeps models, for its free space.
func modelsFolder() string {
	if dir := os.Getenv("OLLAMA_MODELS"); dir != "" {
		return dir
	}

	home, _ := os.UserHomeDir()

	for _, dir := range []string{filepath.Join(home, ".ollama", "models"), "/usr/share/ollama/.ollama/models", home} {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	return home
}

// roomForModels is the memory a model may use and the disk it may fill.
func (b *Brain) roomForModels(ctx context.Context, client *models.Client) autonomy.Room {
	power := models.WhatItCanRun(ctx, client)

	room := autonomy.Room{}

	if power.Accelerated {
		room.Memory = uint64(power.VRAMBytes)
	} else if power.RAMBytes > 4<<30 {
		// The system and this program keep about four gigabytes.
		room.Memory = power.RAMBytes - 4<<30
	}

	if free, err := storage.FreeOn(modelsFolder()); err == nil {
		room.Disk = free
	}

	return room
}

// provideModel downloads the model that fits a need, in goal mode only.
func (b *Brain) provideModel(expert string, needs route.Needs) error {
	if !b.Cfg.GoalMode {
		return autonomy.ErrOff
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	client := models.New(b.Cfg.OllamaURL)

	e, err := autonomy.PullModel(ctx, ollamaPuller{client}, modelOptions(needs), b.roomForModels(ctx, client),
		autonomy.LedgerAt(b.Root), expert, func(line string) { b.Log.Info("goal mode model", "progress", line) })
	if err != nil {
		return err
	}

	b.Log.Info("goal mode downloaded a model", "model", e.Name, "digest", e.Digest, "for", expert)

	// Seen at the next choice, not in half a minute.
	b.installed.mu.Lock()
	b.installed.at = time.Time{}
	b.installed.mu.Unlock()

	return nil
}

// goalFolder is a job's own folder, in goal mode only.
func (b *Brain) goalFolder(id int64, name string) (string, error) {
	if !b.Cfg.GoalMode {
		return "", autonomy.ErrOff
	}

	return autonomy.GoalFolder(b.Root, id, name)
}

// prepared is what goal mode put here since a moment, as report lines.
func (b *Brain) prepared(since time.Time) []string {
	return autonomy.Lines(autonomy.LedgerAt(b.Root).Since(since))
}
