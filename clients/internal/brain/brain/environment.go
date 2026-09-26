package brain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"pn-scripts-assistant/internal/brain/environs"
	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * What is on this machine, as the assistant knows it.
 *
 * The assistant was never told. It had its own tool descriptions and nothing
 * else: no Godot, no Claude Code, no Go, no git, no browser — all of it
 * discovered elsewhere in this program and none of it reaching the model.
 * Asked "can you make a Godot game", it answered from a tool schema rather
 * than from the engine sitting at a known path, and asked "what can you do"
 * it read out a list written in Go eighteen months ago.
 *
 * This is the other half of environs: that package reads the machine, and
 * this one adds what only the brain knows — which models it can think with,
 * which integrations were approved, which services have keys, and which of
 * its own abilities are waiting for something to be set up.
 */

// world holds the one reading of the machine, built once.
type world struct {
	once sync.Once
	w    *environs.World
}

/*
 * World is what this machine has, kept and shared.
 *
 * One per brain, because the reading is a second of work and every turn, the
 * interface and every proposal want it within moments of each other.
 */
func (b *Brain) World() *environs.World {
	b.world.once.Do(func() {
		b.world.w = environs.New(environs.Sources{
			More: []func(context.Context) []environs.Thing{
				b.thinkingWith,
				b.abilitiesWaiting,
			},
			Hidden: b.turnedOff,
		})
	})

	return b.world.w
}

/*
 * WarmWorld reads the machine in the background at start-up.
 *
 * The first question of a session should not pay for it. A second and a half
 * is nothing at start-up and is noticeable in front of somebody who has just
 * asked something.
 */
func (b *Brain) WarmWorld() {
	go func() {
		ctx, stop := context.WithTimeout(context.Background(), environs.HowLongToAsk*4)
		defer stop()

		b.World().All(ctx)
	}()
}

/*
 * thinkingWith is what can answer a question: the model on this machine and
 * every service with a key.
 *
 * A service without a key is still named, as something that wants setting up
 * — that is the whole point of the change. The assistant could not say "I
 * could use Claude if you put a key in", because a service without a key did
 * not exist as far as it knew.
 */
func (b *Brain) thinkingWith(ctx context.Context) []environs.Thing {
	var out []environs.Thing

	if name := strings.TrimSpace(b.Cfg.OllamaModel); name != "" {
		out = append(out, environs.Thing{
			ID: "model:" + name, Kind: environs.Model, Title: name,
			State: environs.Here, Local: true,
		})
	}

	for _, service := range llm.Services() {
		if service.ID == "custom" {
			continue
		}

		thing := environs.Thing{
			ID: "service:" + service.ID, Kind: environs.Service, Title: service.Name,
			State: environs.WantsSetting,
			Needs: "a key for " + service.Name + ", in the System tab",
		}

		if strings.TrimSpace(b.Cfg.ProviderKeys[service.ID]) != "" {
			thing.State, thing.Needs = environs.Here, ""

			/*
			 * Configured and still unusable while privacy keeps this machine
			 * to itself. Said rather than hidden: somebody who paid for a
			 * service and cannot see why it is idle deserves the reason, and
			 * the model cannot offer the reason it is not told.
			 */
			if !b.Mode.AllowsProvider(service.ID) {
				thing.State = environs.WantsSetting
				thing.Needs = "privacy is set to keep everything on this machine"
			}
		}

		out = append(out, thing)
	}

	return out
}

/*
 * abilitiesWaiting is what this program can do that is waiting for something.
 *
 * Only the waiting ones. What it can already do is in its tools, which the
 * model is given in full; repeating seventy-four of them in the inventory
 * would double the cost of every turn to say what the next section already
 * says. What is missing from the tools is exactly what could not be spoken
 * about before: mail with no mailbox, a house with no address, the web while
 * privacy is closed.
 */
func (b *Brain) abilitiesWaiting(ctx context.Context) []environs.Thing {
	var out []environs.Thing

	if !mailAccount(b.Cfg).Configured() {
		out = append(out, environs.Thing{
			ID: "ability:mail", Kind: environs.Ability, Title: "Reading and sending your mail",
			State: environs.WantsSetting, Local: true,
			Needs: "the mailbox address, user and password, in the System tab",
		})
	}

	if strings.TrimSpace(b.Cfg.HomeAssistantURL) == "" {
		out = append(out, environs.Thing{
			ID: "ability:home", Kind: environs.Ability, Title: "Turning things on and off in the house",
			State: environs.WantsSetting, Local: true,
			Needs: "a Home Assistant address and token, in the System tab",
		})
	}

	/*
	 * Which robot is doing the speaking, because there are two and they are
	 * not alike.
	 *
	 * The voice is a machine by delivery rather than by treatment: the
	 * clearest neural voice on this machine, spoken flat. Where no neural
	 * voice is installed it falls back to espeak, which is formant synthesis
	 * and sounds like 1985 — a worse robot, not a different kind of thing.
	 *
	 * Said, because nothing said it. Speaking counts as working the moment
	 * espeak is there, so setup never offers the better one and somebody
	 * concludes the voice is simply poor.
	 */
	if !neuralVoiceHere() {
		out = append(out, environs.Thing{
			ID: "ability:clear-voice", Kind: environs.Ability,
			Title: "Speaking in the clear machine voice",
			State: environs.WantsSetting, Local: true,
			Why:   "it is speaking through espeak, which is formant synthesis from the 1980s",
			Needs: "the neural voice (63MB) from Setup — the robot is built from it and is far clearer",
		})
	}

	/*
	 * The web, which is a decision rather than a missing part.
	 *
	 * The tools exist and are hidden while privacy keeps this machine to
	 * itself — deliberately, and that stays. What changes is that the model
	 * is told, so it can say "I could look that up if you open privacy"
	 * instead of behaving as though searching had never been built.
	 */
	if !b.Mode.AllowsWeb() {
		out = append(out, environs.Thing{
			ID: "ability:web", Kind: environs.Ability, Title: "Searching and reading the web",
			State: environs.WantsSetting, Local: false,
			Needs: "privacy set to research or open — it is private, so nothing is asked of anybody",
		})
	}

	return out
}

/*
 * turnedOff is what this brain's owner has switched off.
 *
 * Empty for everybody until somebody switches something off: the default is
 * that what exists is available. This is the one place a decision like that
 * is applied, so that every reader of the world agrees about it without each
 * having to remember.
 */
func (b *Brain) turnedOff(id string) bool {
	for _, off := range b.Cfg.TurnedOff {
		if strings.EqualFold(strings.TrimSpace(off), id) {
			return true
		}
	}

	return false
}

/*
 * neuralVoiceHere reports whether a neural voice is installed for the robot
 * to be built from.
 *
 * The same two folders preflight looks in, asked the same way: a voice is a
 * model file beside piper, and anything else is espeak.
 */
func neuralVoiceHere() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	for _, dir := range []string{
		filepath.Join(home, ".local", "src", "piper"),
		filepath.Join(home, ".local", "share", "piper"),
	} {
		if voices, _ := filepath.Glob(filepath.Join(dir, "voices", "*.onnx")); len(voices) > 0 {
			return true
		}
	}

	return false
}
