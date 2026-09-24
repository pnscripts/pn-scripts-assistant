package brain

import (
	"context"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/environs"
	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * The model is told what is on the machine.
 *
 * It never was. The persona ended with where the folders are and said nothing
 * about what is in them, so "can you make a Godot game" was answered from a
 * tool description rather than from the engine at a known path.
 */
func TestThePromptSaysWhatIsOnTheMachine(t *testing.T) {
	b := &Brain{}

	b.world.w = environs.New(environs.Sources{
		Programs: []environs.Program{},
		More: []func(context.Context) []environs.Thing{
			func(context.Context) []environs.Thing {
				return []environs.Thing{
					{ID: "godot", Kind: environs.Engine, Title: "Godot", State: environs.Here, Version: "4.7.1"},
					{ID: "claude-code", Kind: environs.Coder, Title: "Claude Code", State: environs.WantsSignIn},
					{ID: "go", Kind: environs.Runtime, Title: "Go", State: environs.Here, Version: "1.26.6"},
				}
			},
		},
	})

	b.world.once.Do(func() {}) // the world is already built; do not build another

	b.World().All(context.Background())

	prompt := b.SystemPrompt()

	for _, want := range []string{"On this machine, now:", "Godot 4.7.1", "Go 1.26.6", "Claude Code (not signed in)"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not say %q", want)
		}
	}
}

/*
 * And it says nothing at all while the machine is still being read.
 *
 * The first reading takes a second or two. A turn that stalls in front of
 * somebody in order to describe their own machine has made a poor trade —
 * every turn before this change had no inventory at all and was fine.
 */
func TestATurnDuringStartUpDoesNotWaitForTheMachine(t *testing.T) {
	b := &Brain{}

	b.world.w = environs.New(environs.Sources{
		Programs: []environs.Program{},
		More: []func(context.Context) []environs.Thing{
			func(ctx context.Context) []environs.Thing {
				<-ctx.Done() // a reading that never finishes

				return nil
			},
		},
	})

	b.world.once.Do(func() {})

	if got := b.whatIsHere(); got != "" {
		t.Errorf("it waited and then said: %q", got)
	}
}

/*
 * Something here and waiting for setting up is named, with what it wants.
 *
 * This is the whole difference between "not configured" and "cannot": the
 * assistant can now say that it could read the mail once there is a mailbox,
 * where before the tool simply did not exist as far as it knew.
 */
func TestWhatIsWaitingForSettingUpIsSaid(t *testing.T) {
	// The privacy a fresh brain has: everything stays on this machine.
	b := &Brain{Mode: llm.ModePrivate}

	things := b.abilitiesWaiting(context.Background())

	var mail, web bool

	for _, thing := range things {
		switch thing.ID {
		case "ability:mail":
			mail = thing.State == environs.WantsSetting && thing.Needs != ""
		case "ability:web":
			web = thing.State == environs.WantsSetting && strings.Contains(thing.Needs, "privacy")
		}
	}

	if !mail {
		t.Error("an unconfigured mailbox is not mentioned as something that could be set up")
	}

	if !web {
		t.Error("the web, switched off by privacy, is not mentioned as a setting")
	}
}

/*
 * Everything is available; one thing can be taken away.
 *
 * The opposite of the architecture this replaces, where nothing existed until
 * somebody switched it on. A list of exceptions is short and readable; a list
 * of permissions is neither — and the default here is that the list is empty.
 */
func TestSomethingSwitchedOffIsTakenAwayFromTheToolsToo(t *testing.T) {
	b := &Brain{}
	b.Cfg.TurnedOff = []string{"godot", "tool:run_command"}

	if !b.turnedOff("godot") {
		t.Error("something named in the settings is not switched off")
	}

	if !b.turnedOff("GODOT") {
		t.Error("it is case-sensitive, which nobody typing a settings file expects")
	}

	if b.turnedOff("docker") {
		t.Error("something not named is switched off")
	}

	// And nothing is switched off by default, which is the whole point.
	fresh := &Brain{}

	for _, id := range []string{"godot", "docker", "tool:run_command", "service:openai"} {
		if fresh.turnedOff(id) {
			t.Errorf("%s is off on a brain nobody has configured", id)
		}
	}
}
