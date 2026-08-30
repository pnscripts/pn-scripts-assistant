package llm

import "testing"

var here = Sizes{Work: "qwen2.5-coder:7b", Talk: "llama3.2:3b", Reason: "deepseek-r1:8b"}

/*
 * Small talk is answered quickly.
 *
 * Every answer is computed on four cores with no graphics card, so the size of
 * the model is the whole of the wait. "Good morning" through a seven-billion
 * parameter coding model costs most of a minute to say good morning back.
 */
func TestSmallTalkGoesToTheQuickModel(t *testing.T) {
	for _, said := range []string{
		"hello", "Hi!", "good morning", "How are you?", "thanks",
		"thank you very much", "goodbye", "who are you",
		"can you hear me", "здравей", "добро утро", "как си", "благодаря",
	} {
		if got := ChooseModel(said, here); got.Model != here.Talk {
			t.Errorf("%q went to %s, which will take a minute to say hello",
				said, got.Model)
		}
	}
}

/*
 * Anything that means doing something goes to the model that can.
 *
 * This is the direction that matters. A slow greeting is a small cost; a
 * request quietly handed to a model that cannot carry it out produces an
 * assistant that says it will do something and does not, which is the exact
 * failure this program has already been through.
 */
func TestAnythingToDoGoesToTheCapableModel(t *testing.T) {
	for _, said := range []string{
		"read that file",
		"Brain, open the notes",
		"approve everything",
		"remind me in ten minutes",
		"what is on my screen",
		"check my email",
		"run the tests",
		"search for the function",
		"remember this",
		"напиши файл",
		"провери имейла",
		"покажи ми",
		"what does /etc/hosts say",
		"fix this: ```go\nfunc main() {}\n```",
		"Could you look through the project and tell me which parts still need work",
	} {
		if got := ChooseModel(said, here); got.Model != here.Work {
			t.Errorf("%q went to the small model, which cannot carry it out", said)
		}
	}
}

// Anything long is carrying content, whatever words it uses.
func TestALongSentenceIsNotSmallTalk(t *testing.T) {
	said := "hello there, I was wondering whether you might be able to help me " +
		"with something rather involved today"

	if got := ChooseModel(said, here); got.Model != here.Work {
		t.Errorf("a long message beginning with a greeting went to %s", got.Model)
	}
}

// With no small model installed, everything goes to the one that is.
func TestWithNothingSmallEverythingGoesToTheUsualModel(t *testing.T) {
	only := Sizes{Work: "qwen2.5-coder:7b"}

	if got := ChooseModel("hello", only); got.Model != only.Work {
		t.Errorf("chose %q when only one model exists", got.Model)
	}

	same := Sizes{Work: "qwen2.5-coder:7b", Talk: "qwen2.5-coder:7b"}

	if got := ChooseModel("hello", same); got.Model != same.Work {
		t.Errorf("chose %q when both are the same model", got.Model)
	}
}

// The small model is one that is actually installed.
func TestPickingModelsThatExist(t *testing.T) {
	installed := []string{"qwen2.5-coder:7b", "gemma3:4b", "nomic-embed-text:latest"}

	if got := PickTalk(installed); got != "gemma3:4b" {
		t.Errorf("picked %q from %v", got, installed)
	}

	// Preference order: llama3.2:3b is better at plain conversation than a
	// small coding model, so it wins when both are there.
	both := []string{"qwen2.5-coder:1.5b", "llama3.2:3b"}

	if got := PickTalk(both); got != "llama3.2:3b" {
		t.Errorf("picked %q from %v", got, both)
	}

	if got := PickTalk([]string{"qwen2.5-coder:7b"}); got != "" {
		t.Errorf("found a small model where there is none: %q", got)
	}
}

// The reason travels with the choice, so a person can see why an answer was
// quick or slow rather than guessing at it.
func TestTheChoiceSaysWhy(t *testing.T) {
	if got := ChooseModel("hello", here); got.Why == "" {
		t.Error("no reason given for using the quick model")
	}

	if got := ChooseModel("read the file", here); got.Why == "" {
		t.Error("no reason given for using the usual model")
	}
}

/*
 * The same judgement decides whether to describe the tools.
 *
 * This used to be decided by whether the turn was spoken, which meant nothing
 * said out loud could ever do anything — "approve everything", said aloud,
 * could not have worked however well the rest of the chain behaved. And
 * describing every tool costs hundreds of tokens of prompt on a processor that
 * manages ten a second, so a greeting should not pay for them.
 */
func TestToolsAreOfferedForWorkAndNotForGreetings(t *testing.T) {
	for _, said := range []string{"hello", "good morning", "thanks", "здравей"} {
		if ChooseModel(said, here).Tools {
			t.Errorf("%q was sent the whole tool list to answer with a greeting", said)
		}
	}

	for _, said := range []string{
		"approve everything", "read that file", "remind me in ten minutes",
		"check my email", "напиши файл",
	} {
		if !ChooseModel(said, here).Tools {
			t.Errorf("%q was offered no tools, so it can only talk about doing it", said)
		}
	}

	// And with no small model at all, tools are still offered for work.
	only := Sizes{Work: "qwen2.5-coder:7b"}

	if !ChooseModel("approve everything", only).Tools {
		t.Error("with one model, a request to do something was offered no tools")
	}

	if !ChooseModel("hello", only).Tools {
		t.Error("with one model the turn must still be able to act if it needs to")
	}
}

/*
 * The quick model is told not to invent things it did.
 *
 * Asked "good morning", llama3.2:3b answered "Good morning, sir. I have
 * prepared your usual breakfast in the dining room." It had not. Small models
 * fill a conversational opening with whatever an assistant sounds like in the
 * text they were trained on, and a quick answer that invents actions is worse
 * than a slow one that does not.
 */
func TestTheQuickModelIsHeldToItsJob(t *testing.T) {
	choice := ChooseModel("good morning", here)

	if choice.Guidance == "" {
		t.Fatal("the quick model is given no guidance at all")
	}

	// The usual model has the whole conversation and its tools, and needs no
	// extra instruction — which is prompt it would have to read every turn.
	if ChooseModel("read that file", here).Guidance != "" {
		t.Error("the usual model is being sent guidance it does not need")
	}
}

/*
 * A question worth working out goes to the model that reasons.
 *
 * It is slower by design, which is the right trade for "why is this happening"
 * and the wrong one for "open that file" — so anything that means doing
 * something wins over it, however thoughtfully it is phrased.
 */
func TestHardQuestionsGoToTheModelThatReasons(t *testing.T) {
	for _, said := range []string{
		"why is the disk filling up so quickly",
		"explain how the recall actually works",
		"compare these two approaches for me",
		"защо това не работи",
	} {
		if got := ChooseModel(said, here); got.Model != here.Reason {
			t.Errorf("%q went to %s rather than the model that thinks", said, got.Model)
		}
	}

	// Doing something wins, however thoughtful the phrasing.
	for _, said := range []string{
		"explain what is in that file",
		"why did the build fail, check the log",
	} {
		if got := ChooseModel(said, here); got.Model != here.Work {
			t.Errorf("%q went to %s, and it needs tools", said, got.Model)
		}
	}

	// With none installed, a hard question still gets answered.
	without := Sizes{Work: "qwen2.5-coder:7b", Talk: "llama3.2:3b"}

	if got := ChooseModel("why is this happening", without); got.Model != without.Work {
		t.Errorf("with no reasoning model, the question went to %q", got.Model)
	}
}

// Each role picks the best of what is actually on the machine.
func TestEachRoleTakesTheBestInstalled(t *testing.T) {
	installed := []string{
		"qwen2.5-coder:7b", "llama3.2:3b", "deepseek-r1:8b",
		"gemma3:4b", "nomic-embed-text:latest",
	}

	if got := PickTalk(installed); got != "llama3.2:3b" {
		t.Errorf("talking picked %q", got)
	}

	if got := PickReason(installed); got != "deepseek-r1:8b" {
		t.Errorf("reasoning picked %q", got)
	}

	if got := PickWork(installed); got != "qwen2.5-coder:7b" {
		t.Errorf("doing things picked %q", got)
	}

	// A machine with only the one model gets no roles it cannot fill.
	one := []string{"qwen2.5-coder:7b"}

	if got := PickTalk(one); got != "" {
		t.Errorf("invented a small model: %q", got)
	}

	if got := PickReason(one); got != "" {
		t.Errorf("invented a reasoning model: %q", got)
	}
}
