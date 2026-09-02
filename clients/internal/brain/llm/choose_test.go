package llm

import (
	"strings"
	"testing"
)

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

/*
 * A model that cannot use tools must never be given the job of using them.
 *
 * Measured on this machine rather than assumed: gemma3:12b is the largest model
 * installed and ollama refuses it outright — "does not support tools" — so
 * choosing it for anything that means doing something would produce a brain
 * that talks well and cannot act at all. deepseek-r1:8b accepts the schema and
 * then never reaches for it, which is worse, because nothing reports an error.
 *
 * So the list of models considered for that job is a list, not a preference for
 * whichever is biggest.
 */
func TestOnlyModelsThatCanActGetTheActingJob(t *testing.T) {
	for _, cannot := range []string{"gemma3:12b", "gemma3:4b", "deepseek-r1:8b"} {
		for _, want := range WorkCandidates {
			if want == cannot {
				t.Errorf("%s is offered the job of using tools, and it cannot", cannot)
			}
		}
	}

	// And the ones that can are there.
	installed := []string{"gemma3:12b", "deepseek-r1:8b", "qwen3:latest", "qwen2.5-coder:7b"}

	if got := PickWork(installed); got != "qwen2.5-coder:7b" && got != "qwen3:latest" {
		t.Errorf("picked %q for doing things, which was not measured as able to", got)
	}
}

/*
 * A machine with no graphics card is offered a model it can actually finish a
 * turn with.
 *
 * The ordering elsewhere ranks models by how well they use a tool, which is
 * the right question when any of them runs quickly. On four cores it is the
 * wrong one: a seven-billion-parameter model produces a few tokens a second
 * here, and a spoken question was measured still on its first round after four
 * and a half minutes. That is not a slow answer, it is no answer — the person
 * has stopped waiting.
 */
func TestModestMachinesLeadWithSomethingQuick(t *testing.T) {
	work, _, _ := ForMachine("modest")

	if len(work) == 0 {
		t.Fatal("a modest machine was offered no model that can use a tool")
	}

	if work[0] != "llama3.2:3b" {
		t.Errorf("a machine with no card leads with %q; want the quick one first", work[0])
	}

	// Ahead of the others, never instead of them: a machine that has qwen and
	// not llama must still be given qwen rather than nothing.
	var hasFallback bool

	for _, m := range work {
		if m == "qwen2.5-coder:7b" {
			hasFallback = true

			break
		}
	}

	if !hasFallback {
		t.Error("the modest list dropped the models it used to offer, so a machine " +
			"without the small one is left with nothing that can call a tool")
	}
}

// A better machine is unaffected, which is the whole point of the tiers.
func TestBetterMachinesStillLeadWithTheSmartModels(t *testing.T) {
	for _, c := range []struct{ tier, want string }{
		{"capable", "qwen3:14b"},
		{"generous", "qwen3:32b"},
	} {
		work, _, _ := ForMachine(c.tier)

		if len(work) == 0 || work[0] != c.want {
			got := "nothing"
			if len(work) > 0 {
				got = work[0]
			}

			t.Errorf("a %s machine leads with %q; want %q", c.tier, got, c.want)
		}
	}
}

/*
 * A hard request goes to the better model; an ordinary one does not.
 *
 * On a machine without a graphics card the fastest model and the ablest are
 * different by minutes per round, so ordinary turns take the quick one. Some
 * requests plainly need more, and for those the wait is the right price — a
 * fast wrong answer costs the asking again as well as the time.
 *
 * Both halves matter. Escalating everything would undo the reason the quick
 * model was chosen at all.
 */
func TestHardWorkGoesToTheBetterModel(t *testing.T) {
	sizes := Sizes{
		Work:  "llama3.2:3b",
		Quick: "llama3.2:3b",
		Best:  "qwen2.5-coder:7b",
		Talk:  "llama3.2:3b",
	}

	for _, ordinary := range []string{
		"open my downloads folder",
		"what files are in /tmp",
		"remind me to call the dentist",
	} {
		if got := ChooseModel(ordinary, sizes); got.Model != "llama3.2:3b" {
			t.Errorf("an ordinary request %q went to %q, and will now take minutes",
				ordinary, got.Model)
		}
	}

	for _, hard := range []string{
		"refactor this and explain the architecture",
		"debug why the tests fail and fix it properly",
		"analyse this algorithm carefully",
	} {
		if got := ChooseModel(hard, sizes); got.Model != "qwen2.5-coder:7b" {
			t.Errorf("a demanding request %q went to %q rather than the best model",
				hard, got.Model)
		}
	}
}

// With only one model there is nothing to escalate to, and it must not pretend
// otherwise.
func TestNoEscalationWhenThereIsOnlyOneModel(t *testing.T) {
	sizes := Sizes{Work: "llama3.2:3b", Quick: "llama3.2:3b", Best: "llama3.2:3b"}

	if got := ChooseModel("refactor this carefully", sizes); got.Model != "llama3.2:3b" {
		t.Errorf("escalated to %q when that is the only model installed", got.Model)
	}
}

/*
 * A question about a website goes to a model that can open one.
 *
 * "What do you think about pnscripts.com?" reads as a request for an opinion,
 * and the word "think" sent it to the reasoning model — which is deliberately
 * offered no tools, because reasoning is for turning something over rather
 * than going and looking. So it was asked about a website it had no way to
 * see. It floundered, produced something shaped like a tool call, and the turn
 * ended with "the model returned a tool call as text" and nothing to show.
 *
 * Nobody can have an opinion about a site they cannot open. A domain in the
 * sentence means the answer is outside the model, whatever verb is used.
 */
func TestAQuestionAboutAWebsiteCanReachTheWeb(t *testing.T) {
	sizes := Sizes{
		Work:   "llama3.2:3b",
		Quick:  "llama3.2:3b",
		Talk:   "llama3.2:3b",
		Reason: "deepseek-r1:8b",
		Best:   "qwen2.5-coder:7b",
	}

	for _, asked := range []string{
		"what do you think about websites that is pnscripts.com?",
		"what do you think about pnscripts.com? It's a website.",
		"is example.org any good",
		"have a look at https://pnscripts.com and tell me",
	} {
		got := ChooseModel(asked, sizes)

		if got.Model == sizes.Reason {
			t.Errorf("%q went to the reasoning model, which is given no tools "+
				"and cannot open a website", asked)
		}

		if !got.Tools {
			t.Errorf("%q was answered with no tools, so the site could never "+
				"be looked at", asked)
		}
	}

	// A genuine reasoning question, with nothing to go and look at, still goes
	// to the model that reasons — that route is worth keeping.
	pondering := ChooseModel("why do you think people procrastinate so much", sizes)

	if pondering.Model != sizes.Reason {
		t.Errorf("a question with nothing to look up went to %q rather than the "+
			"reasoning model", pondering.Model)
	}
}

/*
 * Asking what it thinks is a question, not a request for deliberation.
 *
 * "What do you think" and "have you considered" are how people phrase ordinary
 * questions. Both used to send the turn to the reasoning model, which on a
 * machine without a graphics card is several minutes and is offered no tools —
 * so a casual question became a long wait that ended with nothing to show.
 */
func TestCasualPhrasingIsNotTreatedAsDeepThought(t *testing.T) {
	sizes := Sizes{
		Work: "llama3.2:3b", Quick: "llama3.2:3b", Talk: "llama3.2:3b",
		Reason: "deepseek-r1:8b", Best: "qwen2.5-coder:7b",
	}

	for _, casual := range []string{
		"what do you think about that idea",
		"have you considered the weather",
	} {
		if got := ChooseModel(casual, sizes); got.Model == sizes.Reason {
			t.Errorf("%q went to the reasoning model and will take minutes", casual)
		}
	}

	// Something genuinely analytical still does.
	for _, deep := range []string{
		"why does the disk fill up every week",
		"explain how the echo canceller works here",
	} {
		if got := ChooseModel(deep, sizes); got.Model != sizes.Reason {
			t.Errorf("%q went to %q rather than the model that reasons",
				deep, got.Model)
		}
	}
}

/*
 * A greeting is answered, not investigated.
 *
 * "Say hello in four words" produced a request to create
 * /home/Petar/greetings.txt, and the turn ended in an approval prompt instead
 * of an answer. Told not to write files, it went looking for that same file
 * and reported it missing. Neither response contains a greeting.
 *
 * A model shown thirty tools and given a greeting finds something to do with
 * them. The prompt asks it to answer conversation in words; a
 * three-billion-parameter model does not reliably listen, and the empty list
 * is the instruction that holds.
 */
func TestConversationIsAnsweredWithoutTools(t *testing.T) {
	sizes := Sizes{
		Work: "qwen2.5-coder:7b", Quick: "llama3.2:3b",
		Talk: "llama3.2:3b", Reason: "deepseek-r1:8b", Best: "qwen2.5-coder:7b",
	}

	for _, chat := range []string{
		"Say hello in four words.",
		"good evening",
		"how are you",
		"thank you",
		"what can you help me with",
	} {
		if got := ChooseModel(chat, sizes); got.Tools {
			t.Errorf("%q was given tools, and a model holding thirty of them "+
				"will use one", chat)
		}
	}
}

/*
 * Anything that might need looking up keeps its tools.
 *
 * The cost of getting this wrong falls entirely on this side: a tool withheld
 * when it was needed produces "I cannot", which is worse than a moment wasted
 * offering one that was not.
 */
func TestAnythingThatMightNeedLookingUpKeepsItsTools(t *testing.T) {
	sizes := Sizes{
		Work: "qwen2.5-coder:7b", Quick: "llama3.2:3b",
		Talk: "llama3.2:3b", Reason: "deepseek-r1:8b", Best: "qwen2.5-coder:7b",
	}

	for _, real := range []string{
		"what is the weather in Sofia",
		"read my notes",
		"what is waiting for me",
		"check my email",
		"what do you think about pnscripts.com?",
		"list the files in my downloads folder",
		"remind me to call the dentist",
		"what models are installed",
		"summarise this and write it into a report for the team meeting tomorrow",
	} {
		if got := ChooseModel(real, sizes); !got.Tools {
			t.Errorf("%q was answered with no tools, so it cannot be answered "+
				"at all", real)
		}
	}
}

/*
 * A question about what it knows must never be answered without tools.
 *
 * "What do you know for me?" was six words with nothing in the vocabulary that
 * forces tools, so it was treated as small talk and sent to the model with no
 * tools and no facts. A model given neither says the only thing it can — "I
 * know nothing about you. I have no memory of our conversations. I am a fresh
 * start" — while the store held one thousand and twenty-nine things about the
 * person asking.
 *
 * That is the worst answer this program can produce: false, about the single
 * capability it exists for, and leaving somebody no reason to believe anything
 * else it says.
 */
func TestAskingWhatItKnowsAlwaysKeepsItsTools(t *testing.T) {
	for _, asked := range []string{
		"what do you know for me?",
		"what do you know about me?",
		"do you know me",
		"what do you remember",
		"tell me what you know",
		"do you have any memory of me",
		"какво знаеш за мен",
		"помниш ли ме",
	} {
		words := strings.Fields(strings.ToLower(asked))

		if nothingToLookUp(strings.ToLower(asked), words) {
			t.Errorf("%q would be answered with no tools at all", asked)
		}
	}
}
