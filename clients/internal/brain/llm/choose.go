package llm

import (
	"regexp"
	"strings"
	"unicode"
)

/*
 * Choosing a model for the turn.
 *
 * Every answer here is computed on four processor cores with no graphics card,
 * so the size of the model is the whole of the wait. A greeting answered by a
 * seven-billion-parameter coding model costs the better part of a minute, and
 * the answer is "good morning".
 *
 * The direction of the risk is not symmetric, and that decides the design.
 * Sending small talk to a big model wastes time. Sending real work to a small
 * one produces an assistant that says it will do something and does not — which
 * is the exact failure this program has already been through, and it is far
 * worse than a slow hello.
 *
 * So this is an allowlist, not a classifier. A turn goes to the small model
 * only when it is recognisably one of a short list of conversational things.
 * Everything else — anything mentioning a file, a command, an action, anything
 * long, anything with a path in it, anything at all in doubt — goes to the
 * model that can use tools properly.
 */

// Choice is the model to use for a turn, and the reason, which is shown so a
// person can see why an answer was quick or slow.
type Choice struct {
	Model string
	Why   string

	/*
	 * Tools is whether to offer the model its tools this turn.
	 *
	 * The same judgement, used twice. Describing every tool costs hundreds of
	 * tokens of prompt on a processor that manages ten a second, and a greeting
	 * does not need any of them — but this was previously decided by whether
	 * the turn was spoken, which meant nothing said out loud could ever do
	 * anything at all. Saying "approve everything" to it could not have worked
	 * however well the rest of the chain behaved.
	 *
	 * So it is decided by what was said, not by how.
	 */
	Tools bool

	/*
	 * Guidance is added for this turn only, when the model needs holding to
	 * its job.
	 *
	 * A three-billion-parameter model asked "good morning" answered "Good
	 * morning, sir. I have prepared your usual breakfast in the dining room."
	 * It has not prepared anything. Small models fill a conversational opening
	 * with whatever an assistant sounds like in the text they were trained on,
	 * and a quick answer that invents things it did is worse than a slow one
	 * that does not.
	 */
	Guidance string
}

/*
 * Sizes names the model for each kind of turn.
 *
 * Three jobs, because they want genuinely different things. Talking wants to be
 * quick above all — a greeting is not improved by a larger model, only delayed.
 * Doing something wants the model that follows a tool schema without inventing
 * fields. Working something out wants the one that reasons, and is worth
 * waiting for precisely because somebody asked a hard question.
 *
 * Any of them may be empty, and then that kind of turn falls back to Work,
 * which every machine running this has.
 */
type Sizes struct {
	// Work answers anything that means doing something, and uses the tools.
	// This is the fallback for everything.
	Work string

	// Talk is a small quick model for conversation.
	Talk string

	/*
	 * Quick is the fastest model here that can still call a tool.
	 *
	 * Distinct from Best, and the distinction is the point. On a machine
	 * without a graphics card the fastest and the ablest are different models
	 * by minutes per round, so ordinary turns take Quick and the few that
	 * genuinely need care take Best. Drawing both from one list made them the
	 * same model and there was nothing left to escalate to.
	 */
	Quick string

	// Reason is a model that thinks before answering, for hard questions.
	Reason string

	/*
	 * Best is the smartest installed model this machine could reasonably run,
	 * which is not always the one in use.
	 *
	 * Kept apart from Work because the working model is its owner's choice and
	 * this is only a recommendation. Overriding somebody's setting because a
	 * better model exists is how a program stops being predictable.
	 */
	Best string
}

/*
 * TalkCandidates are small models worth using for conversation, best first.
 *
 * Small enough to answer in a moment on a processor, and good enough that
 * "good morning" comes back as a greeting rather than an attempt to run a
 * shell command — which llama3.2:3b was measured doing when it was tried as a
 * general default, and which is why it is only ever given small talk.
 */
var TalkCandidates = []string{
	"llama3.2:3b", "gemma3:4b", "qwen2.5-coder:1.5b", "llama3.2", "phi3:mini",
}

/*
 * The same three jobs again, for a machine with a graphics card.
 *
 * Everything above was chosen against four processor cores, where the size of
 * the model is the whole of the wait and the smallest one that can do the job
 * is the right one. That reasoning inverts on a card: a larger model costs
 * memory rather than minutes, and there is no reason to run a small one.
 *
 * Ordered smartest first, and every entry in the working list can actually
 * call a tool — gemma3 is absent for that reason however large it gets, since
 * ollama refuses it tools outright.
 */
var (
	// On a card with room to spare.
	GenerousWork   = []string{"qwen3:32b", "qwen2.5-coder:32b", "llama3.3:70b", "qwen3:14b"}
	GenerousTalk   = []string{"qwen3:8b", "llama3.1:8b", "llama3.2:3b"}
	GenerousReason = []string{"deepseek-r1:32b", "qwq:32b", "deepseek-r1:14b"}

	// On a card, without room for the largest.
	CapableWork   = []string{"qwen3:14b", "qwen2.5-coder:14b", "qwen3:8b", "qwen2.5-coder:7b"}
	CapableTalk   = []string{"llama3.2:3b", "qwen3:4b", "gemma3:4b"}
	CapableReason = []string{"deepseek-r1:14b", "deepseek-r1:8b", "qwq"}
)

/*
 * conversational is what may go to the small model.
 *
 * Openings, closings, courtesies, and questions about the brain itself. Written
 * out rather than inferred, because every entry here is a decision to spend
 * less thought on something, and that should be a list somebody can read.
 *
 * Bulgarian as well as English: this brain is spoken to in both.
 */
var conversational = []string{
	"hello", "hi", "hey", "good morning", "good afternoon", "good evening",
	"good night", "morning", "how are you", "how are things", "are you there",
	"are you awake", "can you hear me", "do you hear me", "thanks", "thank you",
	"cheers", "goodbye", "bye", "see you", "well done", "nice", "great",
	"who are you", "what is your name", "what are you", "what can you do",
	"tell me a joke", "how old are you",

	"здравей", "здравейте", "добро утро", "добър ден", "добър вечер",
	"лека нощ", "как си", "как сте", "чуваш ли ме", "тук ли си", "благодаря",
	"мерси", "чао", "довиждане", "браво", "как се казваш", "коя си ти",
	"кой си ти", "какво можеш",
}

/*
 * working are words that mean something is to be done.
 *
 * Present in a turn, it goes to the capable model however short and however
 * friendly it looks. "Brain, read that file" is four words and is not small
 * talk.
 */
var working = []string{
	"file", "folder", "directory", "path", "read", "write", "edit", "change",
	"open", "save", "delete", "remove", "create", "make", "run", "command",
	"install", "build", "test", "compile", "code", "function", "error", "log",
	"search", "find", "look", "screen", "screenshot", "email", "mail", "send",
	"remind", "reminder", "calendar", "approve", "reject", "waiting", "pending",
	"remember", "forget", "learn", "list", "show", "check", "fix", "start",
	"stop", "set", "change", "turn", "call", "download", "update", "git",
	"document", "pdf", "spreadsheet", "note", "write down",

	"файл", "папка", "прочети", "запиши", "напиши", "изтрий", "създай",
	"стартирай", "спри", "потърси", "намери", "покажи", "провери", "поправи",
	"запомни", "забрави", "научи", "напомни", "имейл", "изпрати", "екран",
	"промени", "отвори", "инсталирай", "обнови",
}

// mostWordsForSmallTalk is where a turn stops being a greeting. Anything longer
// is carrying content, whatever words it uses.
const mostWordsForSmallTalk = 10

/*
 * ChooseModel picks the model for one turn.
 *
 * message is what the person said. Anything not plainly conversational gets
 * the capable model, which is the safe direction: a slow greeting is a small
 * cost, and a request quietly answered by a model that cannot carry it out is
 * the failure this whole program is trying not to have.
 */
func ChooseModel(message string, sizes Sizes) Choice {
	work := Choice{Model: sizes.Work, Why: "the model that does things", Tools: true}

	text := normalise(message)

	if text == "" {
		return work
	}

	words := strings.Fields(text)

	// A path, an extension or a code fence means doing something, whatever
	// else is in the sentence.
	hasWork := strings.ContainsAny(message, "/\\{}") || strings.Contains(message, "```")

	/*
	 * A website named in the question also means doing something.
	 *
	 * "What do you think about pnscripts.com?" reads as an opinion, and the
	 * word "think" sent it to the reasoning model — which is offered no tools
	 * by design, because reasoning is for turning something over rather than
	 * going and looking. So it was asked about a website with no way to see
	 * one. It floundered, emitted something shaped like a tool call, and the
	 * turn ended with nothing to show.
	 *
	 * Nobody can have an opinion about a site they cannot open. A domain in
	 * the sentence means the answer is outside the model, whatever verb the
	 * question happens to use.
	 */
	if mentionsSomewhere(message) {
		hasWork = true
	}

	for _, word := range words {
		for _, doing := range working {
			if word == doing {
				hasWork = true
			}
		}
	}

	/*
	 * Something demanding goes to the better model, where there is one.
	 *
	 * Checked before anything else, because whether a request needs care is
	 * independent of whether it happens to mention a file. "Refactor this and
	 * explain the architecture" names no path and contains no code fence, so
	 * the working heuristic passes it by — and it is exactly the kind of
	 * request the small model answers plausibly and badly.
	 *
	 * The quick model still answers ordinary turns, because on this hardware a
	 * seven-billion-parameter model takes minutes per round and most requests
	 * do not need it. For the few that do, the wait is the right price: a fast
	 * wrong answer costs the asking again as well as the time.
	 */
	if sizes.Best != "" && sizes.Best != sizes.Work && needsTheBest(text, words) {
		return Choice{
			Model: sizes.Best,
			Why:   "the best model here, because this needs care",
			Tools: true,
		}
	}

	if hasWork {
		return work
	}

	/*
	 * A question worth thinking about.
	 *
	 * Only when there is a model for it, and only when nothing in the sentence
	 * means doing something — a reasoning model is slower by design, which is
	 * the right trade for "why is this happening" and the wrong one for "open
	 * that file".
	 */
	if sizes.Reason != "" && sizes.Reason != sizes.Work && len(words) >= 4 {
		for _, word := range words {
			if thinking[word] {
				return Choice{
					Model: sizes.Reason,
					Why:   "a question worth working out, answered by the model that reasons",
					Tools: false,
				}
			}
		}
	}

	if sizes.Talk == "" || sizes.Talk == sizes.Work {
		return work
	}
	/*
	 * Nothing here needs looking up, so nothing is offered to look with.
	 *
	 * A model shown thirty tools and given a greeting will find something to
	 * do with them. "Say hello in four words" produced a request to create
	 * /home/Petar/greetings.txt; told not to, it went looking for that file
	 * instead and reported it missing. Neither answer contains a greeting.
	 *
	 * The prompt asks it to answer conversation in words, and a
	 * three-billion-parameter model does not reliably listen. What it is
	 * shown, it uses — so the reliable instruction is the empty list, not the
	 * paragraph asking it to restrain itself.
	 *
	 * Deliberately narrow: only messages with no sign whatever of needing
	 * something the model does not already have. Anything ambiguous keeps its
	 * tools, because a tool withheld when it was needed produces "I cannot",
	 * which is a worse failure than a moment wasted.
	 */
	if nothingToLookUp(text, words) {
		return Choice{
			Model:    sizes.Talk,
			Why:      "conversation, which needs no tools",
			Tools:    false,
			Guidance: SmallTalkGuidance,
		}
	}

	if len(words) > mostWordsForSmallTalk {
		return work
	}

	for _, phrase := range conversational {
		if text == phrase || strings.HasPrefix(text, phrase+" ") ||
			strings.HasSuffix(text, " "+phrase) {
			return Choice{
				Model:    sizes.Talk,
				Why:      "small talk, answered by the quick model",
				Tools:    false,
				Guidance: SmallTalkGuidance,
			}
		}
	}

	return work
}

/*
 * thinking are words that mean the answer has to be worked out.
 *
 * Deliberately narrow. A reasoning model takes noticeably longer, so it is
 * worth reaching for when somebody has asked something that deserves it and a
 * poor trade for everything else.
 */
var thinking = map[string]bool{
	"why": true, "explain": true, "compare": true, "analyse": true,
	"analyze": true, "reason": true, "prove": true, "solve": true,
	"design": true, "plan": true, "strategy": true, "tradeoff": true,
	"tradeoffs": true, "implications": true, "consequences": true,
	"evaluate": true, "assess": true,

	/*
	 * "think" and "consider" are deliberately absent.
	 *
	 * They read as requests for deliberation and are mostly just how people
	 * phrase an ordinary question — "what do you think about this", "have you
	 * considered". Either one sent the turn to the reasoning model, which on
	 * this hardware is several minutes and is offered no tools at all, so a
	 * casual question became a long wait ending in nothing.
	 *
	 * A question that genuinely needs working out almost always carries one of
	 * the words above as well: why, explain, compare, prove.
	 */

	"защо": true, "обясни": true, "сравни": true, "анализирай": true,
	"измисли": true, "прецени": true, "план": true,
}

/*
 * SmallTalkGuidance holds the quick model to answering rather than performing.
 *
 * Short, because every word of it is prompt the processor has to read before
 * the answer starts, and blunt, because a small model follows a blunt
 * instruction better than a polite one.
 */
const SmallTalkGuidance = "This is small talk. Reply in one short friendly " +
	"sentence. Never claim to have done anything, prepared anything or taken " +
	"any action. Do not invent facts about the person or the house. If asked " +
	"for anything that needs doing, say you will need a moment and stop."

/*
 * ReasonCandidates are models that work an answer out before giving it, best
 * first.
 *
 * They are slower by design — that is what they are for — so one is only ever
 * used when the question asked for it.
 */
var ReasonCandidates = []string{
	"deepseek-r1:8b", "deepseek-r1", "qwen3", "qwq", "gemma3:12b",
}

/*
 * WorkCandidates can actually call a tool, best first.
 *
 * Measured on this machine rather than assumed from size, and the measurements
 * are worth writing down because they are all counter-intuitive.
 *
 * gemma3 is the largest model installed here and ollama refuses it outright —
 * "does not support tools" — so a brain given that job would talk well and be
 * unable to act at all. deepseek-r1 is worse: it accepts the schema and then
 * never reaches for it, and nothing anywhere reports an error.
 *
 * qwen3 asks properly and leads for that reason. It was rejected once, and the
 * measurement behind that was real: with tools and a conversation behind it, it
 * did not finish a single question in twenty-five minutes, because it wraps its
 * working in <think> tags and on a processor the deliberation is the whole of
 * the cost.
 *
 * The deliberation can simply be switched off. Asked not to think, on the same
 * four cores with no graphics card, it answered a tool question in 44 seconds
 * with a proper tool_calls field. qwen2.5-coder took 38 on the same prompt and
 * wrote the call out as prose to be recovered by reading its reply — six
 * seconds cheaper for an answer in the wrong shape, on the one thing this list
 * exists to rank.
 *
 * See quietThinking in ollama.go, which is what makes the ordering here
 * possible at all.
 */
var WorkCandidates = []string{
	"qwen3:8b", "qwen3", "qwen2.5-coder:7b", "qwen2.5-coder", "llama3.1:8b",
}

/*
 * ModestWork leads on a machine with no graphics card.
 *
 * The list above is ordered by how well each model uses a tool, which is the
 * right question on a machine that can run any of them quickly. On four cores
 * and no card it is the wrong question, because the answer arrives after the
 * person has given up: a seven-billion-parameter model here produces two or
 * three tokens a second, and a turn that calls a tool is several rounds of
 * that. Measured on this machine, one spoken question — "what is the weather
 * in Sofia" — was still on its first round after four and a half minutes.
 *
 * llama3.2:3b is less able and answers in a fraction of the time, and it turns
 * out not to be the compromise it looks like: asked the same question with the
 * same tool, it emits a proper tool_calls field, while qwen2.5-coder writes
 * the call out as prose that has to be recovered by reading its reply. So on
 * this class of machine the smaller model is both quicker and more reliable at
 * the one thing this list exists to rank.
 *
 * Only ahead of the others, never instead of them. A machine that has qwen and
 * not this still gets qwen, and anything with a card takes the capable or
 * generous list first — which is what makes the same program worth running on
 * better hardware without changing anything.
 */
var ModestWork = []string{"llama3.2:3b", "llama3.2", "qwen2.5:3b"}

/*
 * ForMachine is the order to prefer models in, given what the machine is.
 *
 * The better models are tried first and the modest ones remain behind them as
 * a fallback, so a machine with a card uses what it can and a machine without
 * one is never left with nothing because the smart list was all it was offered.
 */
func ForMachine(tier string) (work, talk, reason []string) {
	switch tier {
	case "generous":
		return append(append([]string{}, GenerousWork...), WorkCandidates...),
			append(append([]string{}, GenerousTalk...), TalkCandidates...),
			append(append([]string{}, GenerousReason...), ReasonCandidates...)
	case "capable":
		return append(append([]string{}, CapableWork...), WorkCandidates...),
			append(append([]string{}, CapableTalk...), TalkCandidates...),
			append(append([]string{}, CapableReason...), ReasonCandidates...)
	default:
		return append(append([]string{}, ModestWork...), WorkCandidates...),
			TalkCandidates, ReasonCandidates
	}
}

// PickTalk returns the best small model that is actually installed.
func PickTalk(installed []string) string {
	return pick(TalkCandidates, installed)
}

// PickReason returns the best installed model that reasons, or empty.
func PickReason(installed []string) string {
	return pick(ReasonCandidates, installed)
}

// PickWork returns the best installed model for doing things, or empty.
func PickWork(installed []string) string {
	return pick(WorkCandidates, installed)
}

// PickFor chooses from a given order, which is how the machine's tier reaches
// the decision.
func PickFor(order, installed []string) string { return pick(order, installed) }

func pick(wanted, installed []string) string {
	for _, want := range wanted {
		for _, have := range installed {
			if have == want || strings.HasPrefix(have, want+":") ||
				strings.TrimSuffix(have, ":latest") == want {
				return have
			}
		}
	}

	return ""
}

// normalise lowers the case and drops punctuation, so "Hello!" and "hello" are
// the same greeting.
func normalise(text string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

/*
 * SmartestOrder is the capability ranking, ignoring how fast anything is.
 *
 * ForMachine puts the quick model first on a machine without a graphics card,
 * which is right for the model that answers ordinary turns and wrong for the
 * one held in reserve. If both come from the same list then on such a machine
 * they are the same model, there is nothing to escalate to, and a hard
 * question is answered by the small model simply because it was the only one
 * offered.
 *
 * So the reserve is chosen by capability alone. It is slower — on four cores,
 * minutes rather than seconds — and that is the correct trade for the handful
 * of requests that actually need it.
 */
func SmartestOrder(tier string) []string {
	switch tier {
	case "generous":
		return append(append([]string{}, GenerousWork...), WorkCandidates...)
	case "capable":
		return append(append([]string{}, CapableWork...), WorkCandidates...)
	default:
		return WorkCandidates
	}
}

/*
 * needsTheBest reports a request worth waiting longer for.
 *
 * Kept narrow on purpose. Escalating is expensive here — the difference is
 * measured in minutes — so it is reserved for the cases where the small model
 * visibly struggles: writing real code, reasoning through several steps, or
 * being asked in so many words to take care.
 */
func needsTheBest(text string, words []string) bool {
	if strings.Contains(text, "```") {
		return true
	}

	for _, word := range words {
		switch word {
		case "carefully", "properly", "thoroughly", "detailed", "debug",
			"refactor", "architecture", "algorithm", "optimise", "optimize",
			"analyse", "analyze", "review", "prove", "derive":
			return true
		}
	}

	// A long, involved request is one somebody spent time writing, and
	// answering it badly wastes more of their time than the wait would.
	return len(words) > 60
}

/*
 * somewhereToLook matches a domain or a URL named in a sentence.
 *
 * Bare domains as well as full addresses, because people say "pnscripts.com"
 * far more often than they say "https://pnscripts.com" — and it was exactly
 * the bare form that slipped past the older check, which looked for a slash.
 */
var somewhereToLook = regexp.MustCompile(
	`\b(?:[a-zA-Z0-9][a-zA-Z0-9-]*\.)+(?:com|net|org|io|dev|local|bg|co|uk|app|sh|me|ai)\b`)

// mentionsSomewhere reports that the sentence names a place to go and look.
func mentionsSomewhere(message string) bool {
	return somewhereToLook.MatchString(message)
}

/*
 * asking are the words that mean the answer is outside the model.
 *
 * Anything about this machine, this person's files, the world today, or a
 * thing that has to be fetched. One of these in a sentence is enough to keep
 * the tools available.
 */
var asking = map[string]bool{
	"file": true, "files": true, "folder": true, "directory": true,
	"read": true, "open": true, "find": true, "search": true, "look": true,
	"weather": true, "news": true, "price": true, "today": true, "now": true,
	"latest": true, "current": true, "website": true, "site": true, "page": true,
	"email": true, "mail": true, "inbox": true, "reminder": true, "reminders": true,
	"waiting": true, "model": true, "models": true, "screen": true, "window": true,
	"remember": true, "remind": true, "learn": true, "check": true, "list": true,
	"my": true, "mine": true, "our": true,

	/*
	 * Asking what it knows is asking about the store, always.
	 *
	 * "What do you know for me?" was six words with none of the above in them,
	 * so it was treated as small talk and answered with no tools at all — and
	 * a model handed no tools and no facts says the only thing it can: "I know
	 * nothing about you. I have no memory of our conversations. I am a fresh
	 * start." Said by a brain holding one thousand and twenty-nine things
	 * about the person asking.
	 *
	 * That is the worst answer this program can give. It is false, it is about
	 * the one capability the whole thing exists for, and somebody hearing it
	 * has no reason to believe anything else it says.
	 */
	"know": true, "knows": true, "knew": true, "knowledge": true,
	"memory": true, "memories": true, "forget": true, "forgot": true,
	"recall": true, "about": true,

	"файл": true, "файлове": true, "папка": true, "новини": true, "време": true,
	"провери": true, "намери": true, "напомни": true,
	"знаеш": true, "помниш": true, "памет": true, "забрави": true,
}

/*
 * nothingToLookUp reports a message that can be answered from the model alone.
 *
 * Short, and free of any word suggesting the answer lives somewhere else. The
 * length limit matters as much as the vocabulary: a long request is a request,
 * whatever words it happens to use, and the cost of getting this wrong falls
 * entirely on the side of withholding a tool that was needed.
 */
func nothingToLookUp(text string, words []string) bool {
	if len(words) == 0 || len(words) > 14 {
		return false
	}

	for _, word := range words {
		// Anything whose answer lives outside the model.
		if asking[word] {
			return false
		}

		/*
		 * And anything genuinely worth working out.
		 *
		 * "Why is this happening" is four words with nothing to look up in
		 * them, and it is not small talk — it is the kind of question this
		 * program exists for. Reaching the reasoning model matters more than
		 * saving it the tools, and where there is no reasoning model it
		 * belongs with the one that does the work.
		 */
		if thinking[word] {
			return false
		}
	}

	return true
}
