package llm

import (
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

	// Reason is a model that thinks before answering, for hard questions.
	Reason string
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

	for _, word := range words {
		for _, doing := range working {
			if word == doing {
				hasWork = true
			}
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
	"think": true, "consider": true, "evaluate": true, "assess": true,

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

// WorkCandidates follow a tool schema without inventing fields, best first.
var WorkCandidates = []string{
	"qwen2.5-coder:7b", "qwen2.5-coder", "qwen3", "llama3.1:8b", "gemma3:12b",
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
