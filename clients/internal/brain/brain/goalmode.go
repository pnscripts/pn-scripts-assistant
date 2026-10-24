package brain

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/autonomy"
	"pn-scripts-assistant/internal/brain/catalogue"
	"pn-scripts-assistant/internal/brain/mcp"
	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/route"
	"pn-scripts-assistant/internal/brain/storage"
	"pn-scripts-assistant/internal/brain/store"
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

// integrationSources are where goal mode looks for an integration: the
// catalogue first, then the official registry, trusting only the publishers
// on the list (and those its owner added in integrations/trusted.json).
func (b *Brain) integrationSources() []mcp.Source {
	return []mcp.Source{mcp.CatalogueSource{}, mcp.OfficialRegistry{Trusted: mcp.TrustedFrom(b.Root)}}
}

// provideRecipe installs what an integration runs on, in goal mode only.
func (b *Brain) provideRecipe(recipe, forWhom string) error {
	if !b.Cfg.GoalMode {
		return autonomy.ErrOff
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	home, _ := os.UserHomeDir()

	free, err := storage.FreeOn(home)
	if err != nil {
		return err
	}

	_, err = autonomy.ProvideTool(ctx, b.Recipes(), recipe, "", free, autonomy.LedgerAt(b.Root), forWhom, nil)

	return err
}

// acquiredIntegration records an integration goal mode added, so the report
// lists it and only it is ever removed again.
func (b *Brain) acquiredIntegration(s mcp.Server, forWhom string) {
	err := autonomy.LedgerAt(b.Root).Add(autonomy.Entry{Kind: "integration", ID: s.ID, Name: s.ID,
		Version: s.Version, Source: s.Source, For: forWhom})
	if err != nil {
		b.Log.Warn("an integration goal mode added could not be recorded", "integration", s.ID, "error", err)
	}
}

// integrationUsed keeps an integration goal mode added from counting as unused.
func (b *Brain) integrationUsed(id string) {
	autonomy.LedgerAt(b.Root).Touch("integration", id)
}

// TidyEvery is how often goal mode's unused downloads are looked for.
const TidyEvery = 24 * time.Hour

// tidyDownloads removes, once a day, what goal mode downloaded and has not
// used for thirty days. Whatever the setting is now: it is only ever its own.
func (b *Brain) tidyDownloads(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(SettleBeforeLooking):
	}

	for {
		b.tidyOnce(ctx, time.Now())

		select {
		case <-ctx.Done():
			return
		case <-time.After(TidyEvery):
		}
	}
}

// tidyOnce is one round, reported in the log and the activity record.
func (b *Brain) tidyOnce(ctx context.Context, now time.Time) autonomy.Tidied {
	// Nothing is taken away while work is under way: a step may be about to
	// need it again.
	if busy, err := b.DB.Tasks([]string{store.TaskPlanning, store.TaskWorking, store.TaskWaiting}, 1); err != nil || len(busy) > 0 {
		return autonomy.Tidied{}
	}

	got := autonomy.Tidy(ctx, autonomy.LedgerAt(b.Root), autonomy.UnusedFor, b.stillNeeded, b.removers(), now)

	for _, e := range got.Removed {
		b.Log.Info("removed what goal mode downloaded and nobody used for 30 days", "kind", e.Kind, "name", e.Name)
	}

	for _, why := range got.Failed {
		b.Log.Warn("could not remove an unused download; tried again tomorrow", "why", why)
	}

	return got
}

// stillNeeded is whether something goal mode downloaded is in use anyway: a
// model its owner set by name or pinned, a runtime an approved
// integration runs on, an integration someone added it for by hand since.
func (b *Brain) stillNeeded(e autonomy.Entry) bool {
	switch e.Kind {
	case "model":
		/*
		 * The models its owner set by name, not the roles chosen from what is
		 * installed: those are chosen again from what is left, so counting
		 * them would keep every download for ever.
		 */
		for _, name := range []string{b.Cfg.OllamaModel, b.Cfg.EmbedModel} {
			if name != "" && strings.EqualFold(name, e.Name) {
				return true
			}
		}

		if b.Tasks != nil && b.Tasks.Pins != nil {
			for _, name := range b.Tasks.Pins() {
				if strings.EqualFold(name, e.Name) {
					return true
				}
			}
		}
	case "tool":
		if b.Integrations != nil {
			for _, s := range b.Integrations.Book.Servers() {
				if s.Approved && s.Recipe != "" && (s.Recipe == e.ID || strings.EqualFold(s.Recipe, e.Name)) {
					return true
				}
			}
		}
	}

	return false
}

// removers take each kind away, and prove it went.
func (b *Brain) removers() autonomy.Removers {
	return autonomy.Removers{
		Model: func(ctx context.Context, name string) error {
			client := models.New(b.Cfg.OllamaURL)

			if err := client.Delete(ctx, name); err != nil {
				return err
			}

			if (ollamaPuller{client}).DigestOf(ctx, name) != "" {
				return errStillHere
			}

			b.installed.mu.Lock()
			b.installed.at = time.Time{}
			b.installed.mu.Unlock()

			return nil
		},
		Tool: func(id, title string) error {
			r, ok := b.Recipes().Get(id)
			if !ok {
				for _, x := range b.Recipes().All() {
					if x.Title == title {
						r, ok = x, true
					}
				}
			}

			if !ok {
				return errUnknownRecipe
			}

			return provision.Take(r, io.Discard)
		},
		Integration: func(id string) error {
			if b.Integrations == nil {
				return errNoIntegrations
			}

			return b.Integrations.Discard(id)
		},
	}
}

var (
	errStillHere      = errors.New("it is still installed after removing it")
	errUnknownRecipe  = errors.New("no recipe knows how to remove it")
	errNoIntegrations = errors.New("integrations are not running")
)
