package environs

import (
	"context"
	"strings"
	"testing"
)

/*
 * The block the model is given, against the machine it is running on.
 *
 * Both halves matter: that it says what is here, and that it is small. A turn
 * on this machine is already six thousand tokens and ten minutes of reading;
 * an inventory that added a thousand more would be paid for on every single
 * message.
 */
func TestTheBlockSaysWhatIsHereAndStaysSmall(t *testing.T) {
	things := New(Sources{}).All(context.Background())

	block := Words(things, 0)

	if len(block) > HowMuchToSay {
		t.Errorf("the block is %d characters, over the budget of %d", len(block), HowMuchToSay)
	}

	if !strings.HasPrefix(block, "On this machine, now:") {
		t.Errorf("it does not open by saying what it is:\n%s", block)
	}

	// Go compiled this test, so Go is in it.
	if !strings.Contains(block, "Go ") {
		t.Errorf("Go is missing from the machine that built this:\n%s", block)
	}

	t.Logf("%d characters:\n%s", len(block), block)
}

// Over budget, the least useful line goes first and the block stays whole.
func TestABlockOverBudgetLosesItsLeastUsefulLine(t *testing.T) {
	things := []Thing{
		{ID: "go", Kind: Runtime, Title: "Go", State: Here, Version: "1.26.6"},
		{ID: "godot", Kind: Engine, Title: "Godot", State: Here, Version: "4.3"},
		{ID: "unreal", Kind: Engine, Title: "Unreal Engine", State: WantsInstalling},
	}

	full := Words(things, 0)

	if !strings.Contains(full, "Not installed: Unreal Engine") {
		t.Fatalf("the full block does not mention what is missing:\n%s", full)
	}

	short := Words(things, len(full)-10)

	if strings.Contains(short, "Not installed") {
		t.Errorf("over budget, it kept the least useful line:\n%s", short)
	}

	if !strings.Contains(short, "Go 1.26.6") {
		t.Errorf("it cut something that mattered:\n%s", short)
	}
}

/*
 * A thing that is here and not ready is named, with why.
 *
 * This is the whole point of the change: the assistant could not say "I could
 * search the web if you switched privacy" or "Unity is here but not licensed",
 * because anything not usable was simply absent from what it was told.
 */
func TestSomethingHereButNotReadyIsStillSaid(t *testing.T) {
	block := Words([]Thing{
		{ID: "claude-code", Kind: Coder, Title: "Claude Code", State: WantsSignIn, Version: "2.0.1"},
		{ID: "unity", Kind: Engine, Title: "Unity", State: WantsLicence, Version: "6000.0"},
		{ID: "anthropic", Kind: Service, Title: "Anthropic", State: WantsSetting},
		{ID: "codex", Kind: Coder, Title: "Codex", State: Spent},
	}, 0)

	for _, want := range []string{
		"Claude Code 2.0.1 (not signed in)",
		"Unity 6000.0 (not licensed)",
		"Anthropic (needs setting up)",
		"Codex (nothing left today)",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("%q is not in the block:\n%s", want, block)
		}
	}
}

// The same world twice is the same block, because a prefix that changes is a
// prompt that cannot be reused — minutes, on a machine with no graphics card.
func TestTheBlockDoesNotReshuffleItself(t *testing.T) {
	world := New(Sources{})
	ctx := context.Background()

	first := Words(world.All(ctx), 0)
	second := Words(world.Again(ctx), 0)

	if first != second {
		t.Errorf("two readings gave two blocks:\n%s\n---\n%s", first, second)
	}
}
