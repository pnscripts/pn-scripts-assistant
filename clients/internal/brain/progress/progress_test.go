package progress

import (
	"encoding/json"
	"strings"
	"testing"
)

/*
 * Work the brain gave itself is told apart from work somebody is waiting on.
 *
 * The core is coloured by this. Learning from the last conversation runs a
 * minute after every exchange, so without the distinction its owner opens the
 * program and finds it already amber and apparently thinking about a question
 * nobody asked.
 */
func TestBackgroundWorkIsMarkedAsSuch(t *testing.T) {
	Done()

	SetBackground("learning", "Learning from the last conversation")

	step := Now()

	if !step.Busy {
		t.Fatal("background work is not reported at all")
	}

	if !step.Background {
		t.Error("learning after a conversation is presented as work somebody is waiting for")
	}

	if step.Note != "Learning from the last conversation" {
		t.Errorf("it does not say what it is doing: %q", step.Note)
	}

	// And anything somebody is waiting on clears the flag again, so a turn
	// following background work is not mistaken for more of it.
	Set("thinking", "Thinking")

	if Now().Background {
		t.Error("a turn after background work was still marked as background")
	}

	Begin()

	if Now().Background {
		t.Error("beginning a turn left the background mark set")
	}

	Done()
}

/*
 * A running tool reports which tool it is, in the JSON the interface reads.
 *
 * The interface decides what to say out loud from this field. When it was
 * missing, the guess used instead was the first word of the summary written
 * for a person — "Read /etc/hosts" gives "read" — which matched nothing, so
 * the line covering a slow tool never played once in the life of the program.
 */
func TestARunningToolSaysWhichToolItIs(t *testing.T) {
	Done()
	defer Done()

	SetTool("look_at_screen", "Look at your screen: weather in Sofia")

	step := Now()

	if step.Tool != "look_at_screen" {
		t.Errorf("the step reports tool %q, want look_at_screen", step.Tool)
	}

	if step.Kind != "tool" {
		t.Errorf("the step reports kind %q, want tool", step.Kind)
	}

	body, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("could not encode the step: %v", err)
	}

	if !strings.Contains(string(body), `"tool":"look_at_screen"`) {
		t.Errorf("the encoded step has no tool field, so the interface cannot "+
			"tell which tool is running: %s", body)
	}

	// And it must not linger once something else starts, or the interface
	// announces a tool that finished minutes ago.
	Set("thinking", "Thinking")

	if got := Now().Tool; got != "" {
		t.Errorf("the tool name survived into the next step as %q", got)
	}
}

/*
 * Detail lands on the step it belongs to, not on whatever is running when it
 * arrives.
 *
 * A model call outlives the step that started it: it begins under "Thinking",
 * and by the time it returns the brain has moved through answering and
 * speaking and back to listening. Attaching its running time to the current
 * step put "1m 26s" under "Listening" — a step that had not been running and
 * could not have taken it, which is worse than reporting nothing because it is
 * a measurement of the wrong thing.
 */
func TestLateDetailFindsItsOwnStep(t *testing.T) {
	Done()
	defer Done()

	Set("thinking", "Thinking")

	thinking := Mark()

	// The turn moves on while the model is still working.
	Set("answering", "Answering")
	SetBackground("listening", "Listening")

	DetailOn(thinking, "llama3.2:3b · 240 words in 1m 26s")

	var found bool

	for _, entry := range Recent() {
		for _, detail := range entry.Detail {
			if !strings.Contains(detail, "1m 26s") {
				continue
			}

			found = true

			if entry.Kind != "thinking" {
				t.Errorf("the model's timing was attached to %q, which was not "+
					"the step that ran it", entry.Kind)
			}
		}
	}

	if !found {
		t.Error("the detail was dropped entirely")
	}
}

/*
 * A listening turn is only known to be empty after the recogniser has run.
 *
 * It looks identical while it happens whether somebody spoke or a chair
 * creaked — the level crossed the threshold either way — so it is announced
 * before anybody can know it holds nothing. Reported at full volume, those
 * turns read as the assistant hearing voices in an empty room.
 */
func TestAStepCanBeQuietenedAfterTheFact(t *testing.T) {
	Begin()
	Set("transcribing", "Making out the words")

	mark := Mark()

	before := Recent()
	if len(before) == 0 {
		t.Fatal("nothing was remembered")
	}

	for _, e := range before {
		if e.Kind == "transcribing" && e.Background {
			t.Fatal("the step was quiet before anything quietened it")
		}
	}

	QuietenOn(mark)

	var found bool

	for _, e := range Recent() {
		if e.Kind == "transcribing" {
			found = true

			if !e.Background {
				t.Error("the step was not quietened")
			}
		}
	}

	if !found {
		t.Error("the step disappeared instead of being demoted; it still belongs " +
			"in the record of what the microphone made of the room")
	}
}
