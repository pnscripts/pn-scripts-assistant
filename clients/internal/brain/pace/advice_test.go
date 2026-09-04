package pace

import (
	"strings"
	"testing"
)

/*
 * What to tell somebody about why it is slow.
 *
 * The advice is the part that can be wrong in a way the timings cannot. A
 * number is just a number; "install a smaller model" said to somebody with a
 * graphics card is worse than silence, because they will do it and it will not
 * help, and then the program has spent its credibility.
 */

func fourCores() Machine {
	return Machine{Cores: 4, MemoryGB: 31, Working: "qwen3:8b"}
}

func withCard() Machine {
	return Machine{Cores: 16, MemoryGB: 64, HasGPU: true,
		GPUName: "NVIDIA RTX 4090", Working: "qwen3:32b", Quick: "llama3.2:3b"}
}

func slow() Milestones {
	return Milestones{Hearing: 800, Thinking: 42000, Writing: 900, Speaking: 700,
		ToFirst: 44400, Model: "qwen3:8b", Spoken: true}
}

/*
 * On a processor with no small model installed, that is the whole finding.
 *
 * It is also the true state of the machine this was written on: a small model
 * would answer a greeting several times quicker and the program has never
 * mentioned it, because nothing was measuring.
 */
func TestNoQuickModelIsTheFirstThingSaidOnAProcessor(t *testing.T) {
	found := Findings(slow(), 6, fourCores())

	if len(found) == 0 {
		t.Fatal("said nothing about a 42-second wait")
	}

	first := found[0]

	if first.Stage != "thinking" {
		t.Fatalf("led with %q rather than the model", first.Stage)
	}

	if !strings.Contains(first.Change, "small model") {
		t.Fatalf("did not suggest a small model:\n%s", first.Change)
	}

	if first.Doing != "install-quick-model" {
		t.Fatal("named no action the program could take about it")
	}
}

// With one already installed, suggesting it again would be nonsense — and the
// honest answer is that the rest is arithmetic.
func TestWithASmallModelAlreadyInstalledItSaysWhatIsLeft(t *testing.T) {
	machine := fourCores()
	machine.Quick = "llama3.2:3b"

	found := Findings(slow(), 6, machine)

	if strings.Contains(found[0].Change, "install") {
		t.Fatalf("suggested installing what is already there:\n%s", found[0].Change)
	}

	if !strings.Contains(found[0].Change, "llama3.2:3b") {
		t.Fatalf("did not say what is already handling conversation:\n%s", found[0].Change)
	}
}

/*
 * On a card, a smaller model is the wrong advice.
 *
 * The reasoning inverts: a larger model costs memory rather than minutes, and
 * a slow answer there means something else — too much prompt, or the card not
 * being used at all.
 */
func TestOnACardItDoesNotSuggestASmallerModel(t *testing.T) {
	found := Findings(slow(), 6, withCard())

	if len(found) == 0 {
		t.Fatal("said nothing")
	}

	if strings.Contains(found[0].Change, "install a small") {
		t.Fatalf("told somebody with a card to install a small model:\n%s", found[0].Change)
	}

	if !strings.Contains(found[0].Because, "RTX 4090") {
		t.Fatalf("did not mention the card it is failing to be quick on:\n%s", found[0].Because)
	}
}

// A quick machine is told nothing, because there is nothing to fix and a list
// of tiny findings would only teach somebody to ignore the list.
func TestAQuickMachineIsToldNothing(t *testing.T) {
	quick := Milestones{Hearing: 200, Thinking: 300, Writing: 100, Speaking: 150,
		ToFirst: 750, Model: "llama3.2:3b"}

	if found := Findings(quick, 10, withCard()); len(found) != 0 {
		t.Fatalf("invented %d findings about a machine answering in 750ms: %+v",
			len(found), found)
	}
}

// Nothing measured means nothing said. Advice from no data is a guess wearing
// a number.
func TestNothingMeasuredMeansNoAdvice(t *testing.T) {
	if found := Findings(slow(), 0, fourCores()); found != nil {
		t.Fatalf("gave advice with nothing measured: %+v", found)
	}
}

/*
 * The program's own costs are reported even when the model dwarfs them.
 *
 * They are what is left when the model gets quick, and somebody deciding
 * whether to buy a card wants to know what will still be there afterwards.
 */
func TestTheProgramsOwnCostsAreStillReportedBehindTheModel(t *testing.T) {
	m := slow()
	m.Speaking = 1400
	m.Hearing = 900

	found := Findings(m, 6, fourCores())

	var stages []string
	for _, f := range found {
		stages = append(stages, f.Stage)
	}

	for _, want := range []string{"thinking", "speaking", "hearing"} {
		if !contains(stages, want) {
			t.Errorf("%q was not reported: %v", want, stages)
		}
	}
}

func contains(all []string, one string) bool {
	for _, s := range all {
		if s == one {
			return true
		}
	}

	return false
}

/*
 * The verdict is blunt on purpose.
 *
 * A page of stages invites somebody to find the comforting number in it. The
 * question being answered is whether an answer starts before they begin
 * wondering whether it heard them.
 */
func TestTheVerdictSaysPlainlyWhetherThisIsConversationSpeed(t *testing.T) {
	quick := Verdict(Milestones{ToFirst: 600}, 12)

	if !strings.Contains(quick, "conversation speed") {
		t.Fatalf("600ms was not called quick: %s", quick)
	}

	middling := Verdict(Milestones{ToFirst: 2000}, 12)

	if strings.Contains(middling, "conversation speed") {
		t.Fatalf("two seconds was called conversation speed: %s", middling)
	}

	slow := Verdict(Milestones{ToFirst: 44400}, 12)

	if !strings.Contains(slow, "wonder whether I heard you") {
		t.Fatalf("forty seconds was described gently: %s", slow)
	}

	if !strings.Contains(slow, "44.4s") {
		t.Fatalf("the verdict did not say how long: %s", slow)
	}
}

func TestNothingMeasuredSaysSoRatherThanZero(t *testing.T) {
	said := Verdict(Milestones{}, 0)

	if strings.Contains(said, "0ms") || !strings.Contains(said, "Nothing measured") {
		t.Fatalf("reported an empty measurement as a fast one: %s", said)
	}
}

/*
 * The button and the sentence beside it must name the same model.
 *
 * They are written in different places — the paragraph here, the button in the
 * page — and a button that fetches something other than what the paragraph
 * promised is worse than no button at all, because somebody clicks it and
 * waits for two gigabytes of the wrong thing.
 */
func TestTheModelOfferedIsTheModelNamed(t *testing.T) {
	machine := fourCores()
	machine.Suggest = "llama3.2:3b"

	found := Findings(slow(), 6, machine)

	if found[0].Fetch != "llama3.2:3b" {
		t.Fatalf("offered %q", found[0].Fetch)
	}

	if !strings.Contains(found[0].Change, "llama3.2:3b") {
		t.Fatalf("the sentence does not name what the button fetches:\n%s",
			found[0].Change)
	}

	if !strings.Contains(found[0].Change, "qwen3:8b") {
		t.Fatalf("did not say that real work still goes to the large model:\n%s",
			found[0].Change)
	}
}

// With nothing to suggest, it still says what would help — it simply cannot
// offer to do it, and must not offer a button with no model behind it.
func TestWithNothingToSuggestItStillExplains(t *testing.T) {
	found := Findings(slow(), 6, fourCores())

	if found[0].Fetch != "" {
		t.Fatalf("offered to fetch %q, which nothing chose", found[0].Fetch)
	}

	if !strings.Contains(found[0].Change, "small model") {
		t.Fatalf("stopped explaining as well:\n%s", found[0].Change)
	}
}

// A machine with a card is never offered a small model, button or otherwise.
func TestACardIsNeverOfferedASmallModel(t *testing.T) {
	machine := withCard()
	machine.Quick = ""
	machine.Suggest = "llama3.2:3b"

	for _, f := range Findings(slow(), 6, machine) {
		if f.Fetch != "" {
			t.Fatalf("offered %q to a machine with an RTX 4090", f.Fetch)
		}
	}
}
