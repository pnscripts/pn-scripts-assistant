package brain

import (
	"context"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Keeping both models in memory, so switching between them is free.
 *
 * Choosing a smaller model for small talk only helps if the smaller model is
 * already loaded. If it is not, the saving is spent several times over loading
 * it — measured here at forty-four seconds to say good morning, which is worse
 * than never having switched at all.
 *
 * So both are loaded at the start and touched often enough that neither falls
 * out. What decides whether this can work is a setting on the machine rather
 * than anything in this program: Ollama holds a fixed number of models at once,
 * and if that number is smaller than the models in play, every switch evicts
 * something. When it is too small this says so, by name, rather than quietly
 * thrashing.
 */

// howOftenToTouchModels keeps a model from ageing out. Well inside the
// thirty-minute lifetime the requests ask for.
const howOftenToTouchModels = 8 * time.Minute

// keepModelsWarm loads the models the conversation uses and keeps them loaded.
func (b *Brain) keepModelsWarm(ctx context.Context) {
	warm := func() {
		/*
		 * The models the conversation actually moves between.
		 *
		 * The reasoning model is deliberately left out: it is reached for
		 * rarely, it is large, and holding it resident would evict one of the
		 * two that are used constantly. It is worth its load time when it is
		 * asked for, and worth nothing sitting idle.
		 */
		roles := b.modelRoles()

		wanted := []string{roles.Work}

		if roles.Talk != "" && roles.Talk != roles.Work {
			wanted = append(wanted, roles.Talk)
		}

		for _, model := range wanted {
			if ctx.Err() != nil {
				return
			}

			/*
			 * One at a time and never while somebody is waiting.
			 *
			 * Loading a model is minutes of the whole processor on this
			 * machine, and doing it underneath a turn makes the turn slower —
			 * which is the opposite of the point.
			 */
			b.waitUntilIdle(ctx)

			if err := b.ollama.WarmModel(ctx, model); err != nil {
				b.Log.Info("could not keep a model loaded", "model", model, "error", err)
			}
		}

		b.reportResidency(ctx, wanted)
	}

	warm()

	ticker := time.NewTicker(howOftenToTouchModels)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			warm()
		}
	}
}

/*
 * reportResidency says when the machine cannot hold what is being asked of it.
 *
 * The failure is otherwise invisible and looks like the program being slow: a
 * switch evicts the other model, the next turn reloads several gigabytes, and
 * nothing anywhere says why. Named here with the setting that fixes it.
 */
func (b *Brain) reportResidency(ctx context.Context, wanted []string) {
	if len(wanted) < 2 {
		return
	}

	loaded, err := b.ollama.Resident(ctx)
	if err != nil {
		return
	}

	held := map[string]bool{}

	for _, m := range loaded {
		held[m.Name] = true
	}

	var missing []string

	for _, want := range wanted {
		if !held[want] && !held[want+":latest"] {
			missing = append(missing, want)
		}
	}

	b.mu.Lock()
	b.modelsResident = len(missing) == 0
	b.mu.Unlock()

	if len(missing) == 0 {
		return
	}

	b.Log.Info(
		"the machine will not hold both models at once, so switching between "+
			"them reloads one every time",
		"missing", strings.Join(missing, ", "),
		"fix", "raise OLLAMA_MAX_LOADED_MODELS to 3")
}

// waitUntilIdle holds off while a turn is in flight.
func (b *Brain) waitUntilIdle(ctx context.Context) {
	for i := 0; i < 120; i++ {
		if ctx.Err() != nil {
			return
		}

		if !progress.Answering() {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// ModelsBothResident reports whether switching model is currently free.
func (b *Brain) ModelsBothResident() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.modelsResident
}

// Models lists what is installed, for the tool that reports it.
func (b *Brain) Models(ctx context.Context) ([]tools.InstalledModel, error) {
	installed, err := models.New(b.Cfg.OllamaURL).List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]tools.InstalledModel, 0, len(installed))

	for _, m := range installed {
		out = append(out, tools.InstalledModel{Name: m.Name, Size: m.Size})
	}

	return out, nil
}

// Roles is which model does what, for the interface to show.
func (b *Brain) Roles() (work, talk, reason string) {
	r := b.modelRoles()

	return r.Work, r.Talk, r.Reason
}

var _ = llm.Loaded{}
