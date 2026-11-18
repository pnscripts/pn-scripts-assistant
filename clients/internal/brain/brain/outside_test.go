package brain

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/protect"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

// readsIt is a tool that reads a file by its path, by counting.
type readsIt struct{ ran *int }

func (readsIt) Name() string                     { return "zz_read_it" }
func (readsIt) Description() string              { return "read a file" }
func (readsIt) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (readsIt) Risk() tools.Risk                 { return tools.Safe }
func (readsIt) Summarize(json.RawMessage) string { return "Read a file" }
func (r readsIt) Execute(context.Context, json.RawMessage) (string, error) {
	*r.ran++

	return "read", nil
}

func learned(path string) bool {
	for _, allowed := range protect.InForce().Allowed {
		if strings.EqualFold(allowed, path) {
			return true
		}
	}

	return false
}

/*
 * A yes to what another program asked for is for that once. For the
 * person's own request, approving a protected file teaches the assistant
 * not to ask about that file again; the same yes on another program's
 * card must teach it nothing, or one question from outside would open the
 * file to every request after it.
 */
func TestAYesForAnotherProgramTeachesNothingLasting(t *testing.T) {
	b := testBrain(t)

	ran := 0
	if err := b.Agent.Registry.Register(readsIt{ran: &ran}); err != nil {
		t.Fatal(err)
	}

	theirs := filepath.Join(b.Root, "theirs", ".env")
	mine := filepath.Join(b.Root, "mine", ".env")

	if _, _, held := protect.InArguments(json.RawMessage(`{"path":"` + theirs + `"}`)); !held {
		t.Fatal("the test's file is not a protected one")
	}

	t.Cleanup(func() {
		protect.Forget(b.Root, theirs)
		protect.Forget(b.Root, mine)
	})

	asked := func(path, by string) int64 {
		id, err := b.DB.RecordInvocation(0, "zz_read_it", `{"path":"`+path+`"}`, "Read a file", string(tools.Mutating))
		if err != nil {
			t.Fatal(err)
		}

		if by != "" {
			if err := b.DB.InvocationAskedBy(id, by); err != nil {
				t.Fatal(err)
			}
		}

		return id
	}

	if _, err := b.Decide(context.Background(), asked(theirs, "claude-code 2.1.0"), true); err != nil {
		t.Fatal(err)
	}

	if ran != 1 {
		t.Fatalf("the approved call ran %d times", ran)
	}

	if learned(theirs) {
		t.Error("a yes for another program taught the assistant not to ask about the file again")
	}

	// The person's own request still teaches it, as before.
	if _, err := b.Decide(context.Background(), asked(mine, ""), true); err != nil {
		t.Fatal(err)
	}

	if !learned(mine) {
		t.Error("a yes for the person's own request was not learned")
	}

	// And the record of the yes says who had asked.
	deeds, _ := b.DB.Deeds(store.DeedFilter{Outside: true})
	if len(deeds) != 1 || deeds[0].AskedBy != "claude-code 2.1.0" || deeds[0].Outcome != "done" {
		t.Errorf("the record: %+v", deeds)
	}
}

// The switch through the brain: off until allowed, a change recorded once,
// nothing allowed that is not a switch.
func TestWhatOtherProgramsMayAskThroughTheBrain(t *testing.T) {
	b := testBrain(t)

	if b.OutsideMay(otherprograms.Programs) || b.OutsideMay("memory") || b.OutsideMay("") {
		t.Fatal("allowed on a new brain")
	}

	if err := b.AllowOutside("memory", true); err == nil {
		t.Error("something that is not a switch was allowed")
	}

	for i := 0; i < 2; i++ {
		if err := b.AllowOutside(otherprograms.Programs, true); err != nil {
			t.Fatal(err)
		}
	}

	if !b.OutsideMay(otherprograms.Programs) {
		t.Fatal("not allowed after allowing")
	}

	deeds, _ := b.DB.Deeds(store.DeedFilter{})
	if len(deeds) != 1 || deeds[0].Tool != SwitchTool || deeds[0].Specialist != "" {
		t.Errorf("the record: %+v", deeds)
	}

	// A record that cannot be written: the question is not noted, and the
	// caller is told so.
	if err := b.AskedFromOutside("read_file", "x", "", "done", ""); err == nil {
		t.Error("a question there is not was written down")
	}

	if err := b.AskedFromOutside("assistant_status", "", "", "done", ""); err == nil {
		t.Error("a question from nobody was written down")
	}
}
