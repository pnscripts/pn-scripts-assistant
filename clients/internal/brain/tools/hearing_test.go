package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

/*
 * What the brain says when it is asked why it did that.
 *
 * These read like a lot of assertions about wording, and they are. The whole
 * value of this tool is that somebody who is annoyed at their assistant gets
 * an answer they can act on — "the browser was playing and that was not your
 * voice" is worth something, "recognition_state: degraded" is worth nothing.
 * So the wording is the behaviour, and the wording is what is tested.
 */

func answer(t *testing.T, state HearingState, heard ...Overheard) string {
	t.Helper()

	tool := Hearing{
		State:  func() HearingState { return state },
		Recent: func() []Overheard { return heard },
	}

	said, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("asking what it hears: %v", err)
	}

	return said
}

// The state everything else varies from: taught, listening to everybody,
// hearing the room raw.
func ready() HearingState {
	return HearingState{
		VoiceModel:  true,
		VoiceTaught: true,
		Match:       0.55,
		WakeWord:    "brain",
	}
}

func TestItSaysWhenItCannotTellVoicesApartAtAll(t *testing.T) {
	said := answer(t, HearingState{WakeWord: "brain"})

	if !strings.Contains(said, "not installed") {
		t.Fatalf("a machine with no voice model should say so:\n%s", said)
	}

	if !strings.Contains(said, "Privacy") {
		t.Fatalf("it should say where to fix it:\n%s", said)
	}
}

/*
 * The state Petar's machine is actually in as this is written: the model is
 * there, nobody has taught it a voice yet.
 *
 * This is the one most likely to be hit and the easiest to describe wrongly.
 * "Voice recognition: enabled" would be true and would leave somebody
 * wondering for a week why it never recognises them.
 */
func TestItSaysWhenItKnowsHowToTellVoicesApartButHasNotBeenTaught(t *testing.T) {
	said := answer(t, HearingState{VoiceModel: true, WakeWord: "brain"})

	if !strings.Contains(said, "not been taught") {
		t.Fatalf("it should say it has not been taught a voice:\n%s", said)
	}

	if !strings.Contains(said, "three sentences") {
		t.Fatalf("it should say what teaching it costs:\n%s", said)
	}
}

func TestItSaysWhetherItIsAnsweringOnlyOneVoice(t *testing.T) {
	state := ready()
	state.OnlyOwner = true

	if said := answer(t, state); !strings.Contains(said, "only you") {
		t.Fatalf("with only-me on it should say so:\n%s", said)
	}

	state.OnlyOwner = false

	said := answer(t, state)

	if !strings.Contains(said, "any voice") {
		t.Fatalf("with only-me off it should say it answers anybody:\n%s", said)
	}

	// And it must not read as though the switch were on, which is the whole
	// difference between "it ignored me" and "it answered the television".
	if strings.Contains(said, "only you") {
		t.Fatalf("with only-me off it must not claim it answers only one voice:\n%s", said)
	}
}

func TestItSaysWhetherThisMachinesOwnSoundReachesTheMicrophone(t *testing.T) {
	state := ready()

	if said := answer(t, state); !strings.Contains(said, "mixed with whoever is talking") {
		t.Fatalf("it should admit the music reaches it:\n%s", said)
	}

	state.CancellingUs = true

	said := answer(t, state)

	if !strings.Contains(said, "subtracted") {
		t.Fatalf("with the rerouting on it should say so:\n%s", said)
	}

	if strings.Contains(said, "mixed with whoever is talking") {
		t.Fatalf("with the rerouting on it must not still complain about music:\n%s", said)
	}
}

// "It never answers unless I say its name" and "it answers everything" are
// both settings, and both get reported as complaints.
func TestItSaysWhatItTakesToGetItsAttention(t *testing.T) {
	state := ready()
	state.AlwaysName = true

	if said := answer(t, state); !strings.Contains(said, `"brain"`) {
		t.Fatalf("it should name the word:\n%s", said)
	}

	state.AlwaysName = false

	if said := answer(t, state); !strings.Contains(said, "stay in the conversation") {
		t.Fatalf("it should explain that the name is not needed every time:\n%s", said)
	}

	state.WakeWord = ""

	if said := answer(t, state); !strings.Contains(said, "no name is needed") {
		t.Fatalf("with no wake word it should say anything reaches it:\n%s", said)
	}
}

/*
 * The complaint this was built for, end to end.
 *
 * Music playing, somebody talking over it, and a line the assistant answered
 * that nobody said to it. The answer has to contain all three facts, because
 * any two of them explain nothing.
 */
func TestItExplainsAnsweringTheMusic(t *testing.T) {
	said := answer(t, ready(),
		Overheard{
			Text:           "and we'll be right back after these messages",
			Addressed:      true,
			KnownVoice:     true,
			Owner:          false,
			Voice:          0.11,
			MachinePlaying: true,
			Playing:        []string{"Brave"},
			PeakRMS:        4100,
			NoiseFloor:     900,
			Ago:            "just now",
		})

	for _, want := range []string{
		"right back after these messages", // what it heard
		"answered",                        // what it did
		"not your voice",                  // whose it was
		"Brave",                           // where it came from
		"4100",                            // and how loud
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the account of answering the television is missing %q:\n%s", want, said)
		}
	}
}

// The other half of the same complaint: it heard perfectly well and decided
// not to. Somebody sitting there sees no difference from a dead microphone.
func TestItExplainsIgnoringSomebody(t *testing.T) {
	said := answer(t, ready(),
		Overheard{
			Text:       "what time is it",
			Addressed:  false,
			Why:        "not your voice",
			KnownVoice: true,
			Owner:      false,
			Voice:      0.21,
			PeakRMS:    3300,
			NoiseFloor: 700,
		})

	if !strings.Contains(said, "not answered: not your voice") {
		t.Fatalf("it should say it heard the words and why it let them go:\n%s", said)
	}

	if !strings.Contains(said, "what time is it") {
		t.Fatalf("it should quote what it heard:\n%s", said)
	}
}

// A brand new brain, or one just restarted. There is nothing to report and
// saying nothing would look like a fault.
func TestNothingOverheardIsSaidPlainly(t *testing.T) {
	said := answer(t, ready())

	if !strings.Contains(said, "Nothing has been overheard") {
		t.Fatalf("an empty log should be stated:\n%s", said)
	}
}

// Eight is what somebody can hold in their head. Half an hour of a busy room
// is not an answer to a question.
func TestOnlyTheLastFewAreRecounted(t *testing.T) {
	var many []Overheard

	for i := 0; i < 40; i++ {
		many = append(many, Overheard{Text: "line " + string(rune('a'+i%26)), PeakRMS: 2000})
	}

	said := answer(t, ready(), many...)

	if lines := strings.Count(said, "peak 2000"); lines != 8 {
		t.Fatalf("expected eight turns recounted, got %d:\n%s", lines, said)
	}
}

// A voice it has never been taught to place is reported without a claim about
// whose it was, rather than as a stranger's.
func TestAnUnjudgedVoiceIsNotCalledSomebodyElses(t *testing.T) {
	said := answer(t, ready(), Overheard{
		Text:      "hello",
		Addressed: true,
		PeakRMS:   3000,
	})

	// Only the account of the turn itself; the paragraph above it says that it
	// knows the owner's voice, which is a different fact and a true one.
	_, recounted, _ := strings.Cut(said, "made of the room:")

	if strings.Contains(recounted, "your voice") {
		t.Fatalf("it should make no claim about a voice it did not judge:\n%s", said)
	}
}

// The machine was making a noise but nothing said which program. "Something on
// this machine" is still the fact that explains the confusion.
func TestPlayingWithNoNameIsStillReported(t *testing.T) {
	said := answer(t, ready(), Overheard{
		Text:           "buy now",
		MachinePlaying: true,
		PeakRMS:        2500,
	})

	if !strings.Contains(said, "something on this machine") {
		t.Fatalf("it should say the machine was playing even unnamed:\n%s", said)
	}
}

// Two programs at once, which is a laptop with a video paused in one tab and
// music in another.
func TestSeveralProgramsPlayingAreBothNamed(t *testing.T) {
	said := answer(t, ready(), Overheard{
		Text:           "chorus",
		MachinePlaying: true,
		Playing:        []string{"Brave", "Spotify"},
		PeakRMS:        5000,
	})

	if !strings.Contains(said, "Brave and Spotify") {
		t.Fatalf("both should be named:\n%s", said)
	}
}

// Nothing listening at all — packaging without the voice parts, or a headless
// run. It says so instead of reporting an empty room as a quiet one.
func TestItRefusesToInventAnAnswerWhenNothingIsListening(t *testing.T) {
	if _, err := (Hearing{}).Execute(context.Background(), nil); err == nil {
		t.Fatal("with nothing wired up it should say so, not report silence")
	}
}

func TestTheHearingToolChangesNothing(t *testing.T) {
	if got := (Hearing{}).Risk(); got != Safe {
		t.Fatalf("reading what it heard should be safe, got %v", got)
	}
}

/*
 * And the switch that fixes the commonest cause.
 *
 * Mutating on purpose: it moves every program's sound on the machine. A
 * sentence misheard as a request must not silently rewire somebody's speakers
 * in the middle of a film.
 */
func TestReroutingTheMachinesSoundAsksFirst(t *testing.T) {
	if got := (Quieten{}).Risk(); got != Mutating {
		t.Fatalf("rerouting every program's sound should ask first, got %v", got)
	}
}

func TestRerouteOnAndOffBothReachTheMachine(t *testing.T) {
	var asked []bool

	tool := Quieten{
		Reroute: func(_ context.Context, on bool) error {
			asked = append(asked, on)

			return nil
		},
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"off":true}`)); err != nil {
		t.Fatal(err)
	}

	if len(asked) != 2 || !asked[0] || asked[1] {
		t.Fatalf("expected on then off, got %v", asked)
	}
}

// The sentence somebody is shown before they agree has to say what will
// happen, in both directions.
func TestTheRerouteSummaryDistinguishesOnFromOff(t *testing.T) {
	on := Quieten{}.Summarize(json.RawMessage(`{}`))
	off := Quieten{}.Summarize(json.RawMessage(`{"off":true}`))

	if on == off {
		t.Fatal("switching it on and off must not be described the same way")
	}

	if !strings.Contains(off, "back") {
		t.Fatalf("putting it back should say so: %q", off)
	}
}

/*
 * It must not promise more than it can do.
 *
 * The canceller subtracts what this machine played. A phone on the desk, a
 * radio in the kitchen, somebody else in the room — none of that is
 * subtractable, and saying "now I only hear you" would be a lie that costs an
 * evening of confusion.
 */
func TestItDoesNotPromiseSilenceItCannotDeliver(t *testing.T) {
	tool := Quieten{Reroute: func(context.Context, bool) error { return nil }}

	said, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "somewhere else") {
		t.Fatalf("it should say what it still cannot subtract:\n%s", said)
	}
}

// A machine with no PipeWire, or a canceller that would not load. The failure
// is reported rather than swallowed into a cheerful confirmation.
func TestAFailedRerouteIsNotReportedAsSuccess(t *testing.T) {
	tool := Quieten{
		Reroute: func(context.Context, bool) error {
			return errors.New("no echo canceller on this machine")
		},
	}

	said, err := tool.Execute(context.Background(), json.RawMessage(`{}`))

	if err == nil {
		t.Fatalf("a failed reroute should be an error, said %q", said)
	}

	if said != "" {
		t.Fatalf("it should not also claim to have done it: %q", said)
	}
}

func TestRerouteWithNothingWiredUpSaysSo(t *testing.T) {
	if _, err := (Quieten{}).Execute(context.Background(), nil); err == nil {
		t.Fatal("with no way to reroute it should say so")
	}
}

// Both tools carry a shape the model can call, which is a mistake made once
// per tool and never noticed until a conversation fails.
func TestBothToolsDescribeThemselvesToTheModel(t *testing.T) {
	for _, tool := range []Tool{Hearing{}, Quieten{}} {
		var shape map[string]any

		if err := json.Unmarshal(tool.Parameters(), &shape); err != nil {
			t.Errorf("%s: unreadable parameters: %v", tool.Name(), err)
		}

		if shape["type"] != "object" {
			t.Errorf("%s: parameters are not an object", tool.Name())
		}

		if len(tool.Description()) < 60 {
			t.Errorf("%s: the description is too thin to pick it out", tool.Name())
		}

		if tool.Summarize(json.RawMessage(`{}`)) == "" {
			t.Errorf("%s: nothing to show somebody before it runs", tool.Name())
		}
	}
}
