// Package starter is the small brain that exists before the real one does.
//
// Nothing about PN Scripts Assistant works until a language model and a speech recogniser
// are in place, which on a fresh machine is a download of several gigabytes
// during which the application can say nothing at all. That is the worst possible moment to
// be silent: it is when someone has the most questions and the least reason to
// trust what is being installed on their computer.
//
// So the desktop binary carries this. It is a few kilobytes of written answers
// with keyword matching over them — deliberately not a model, and it says so
// rather than implying intelligence it does not have. It knows one subject:
// PN Scripts Assistant, what it needs, and why. Once the real brain is running the
// application hands over and this is never used again.
package starter

import (
	"sort"
	"strings"
)

// Topic is one thing the starter can explain.
type Topic struct {
	// Keys are matched against the question. Multi-word keys score higher,
	// because "how long" is far more telling than "long" on its own.
	Keys   []string
	Answer string
}

// Reply is an answer with the confidence behind it, so the caller can decline
// to answer rather than confidently returning something unrelated.
type Reply struct {
	Answer   string
	Matched  bool
	Followup []string
}

var topics = []Topic{
	{
		Keys: []string{"what is this", "what is pn scripts assistant", "what is pn brain", "what does this do", "what are you", "purpose"},
		Answer: "PN Scripts Assistant is a personal assistant that runs on your own computer. " +
			"It remembers things about your work across conversations and gets more useful over time.\n\n" +
			"Unlike a website, it keeps what it learns on your machine, and by default nothing " +
			"is sent anywhere else.",
	},
	{
		Keys: []string{"why ollama", "what is ollama", "need ollama", "local model", "why model"},
		Answer: "Ollama runs a language model on your own machine. That is what lets PN Scripts Assistant " +
			"think without sending your conversations to a company.\n\n" +
			"It is free and works offline. The trade is speed: without a graphics card, " +
			"expect a few seconds per reply rather than instant.",
	},
	{
		Keys: []string{"how long", "how much time", "take long", "wait", "download size", "how big"},
		Answer: "The setup itself takes a minute or two. The language model is the slow part — " +
			"roughly 2GB to download, so anywhere from two minutes to twenty depending on your " +
			"connection.\n\n" +
			"You only do this once. After that, starting PN Scripts Assistant takes a few seconds.",
	},
	{
		Keys: []string{"private", "privacy", "my data", "sent anywhere", "secure", "safe", "who sees"},
		Answer: "By default nothing leaves your computer. The model runs locally, the memory " +
			"lives on your disk, and web access is switched off.\n\n" +
			"If you later turn on web search or a paid model, PN Scripts Assistant still never sends what it " +
			"has learned about you — only what you type in that conversation. That rule is " +
			"enforced in the code, not just promised.",
	},
	{
		Keys: []string{"cost", "pay", "free", "subscription", "price", "api key"},
		Answer: "Nothing here costs money. The model runs on your own hardware.\n\n" +
			"You can optionally add an Anthropic API key for stronger answers, which is paid " +
			"per use — but it is entirely optional and PN Scripts Assistant works fully without one.",
	},
	{
		Keys: []string{"internet", "offline", "no connection", "without internet"},
		Answer: "After setup, PN Scripts Assistant works with no internet connection at all. The model and " +
			"your memory are both local.\n\n" +
			"You need a connection now, to download the model.",
	},
	{
		Keys: []string{"where stored", "where is my data", "which drive", "storage", "disk space", "space"},
		Answer: "Everything the brain knows lives in one folder you choose, and it can sit on an " +
			"external drive.\n\n" +
			"If that drive starts filling up, PN Scripts Assistant tells you before it becomes a problem and " +
			"can move itself to a bigger one.",
	},
	{
		Keys: []string{"delete", "uninstall", "remove", "get rid"},
		Answer: "Delete the data folder and the brain is gone — there is no account and nothing " +
			"stored elsewhere.\n\n" +
			"Removing Ollama and the speech models is separate, if you want those gone too.",
	},
	{
		Keys: []string{"password", "why password", "sudo", "root", "permission"},
		Answer: "Installing system packages needs your password, the same as installing any " +
			"software. You will see a normal system prompt, not a box inside this window.\n\n" +
			"PN Scripts Assistant never stores it.",
	},
	{
		Keys: []string{"gpu", "graphics card", "slow", "speed", "fast", "performance"},
		Answer: "A graphics card makes replies much faster, but is not required.\n\n" +
			"Without one, a small model still works — expect a few seconds per reply. " +
			"PN Scripts Assistant picks a model size that suits the machine it finds.",
	},
	{
		Keys: []string{"what can it do", "capabilities", "features", "what will it", "tools"},
		Answer: "It answers questions, remembers what matters across conversations, and can read " +
			"files and folders you point it at.\n\n" +
			"It can also write files, run searches and control smart-home devices — but anything " +
			"that changes something asks your permission first, every time.",
	},
	{
		Keys: []string{"stuck", "not working", "failed", "error", "broken", "help"},
		Answer: "If a step failed, the details are written to a log file rather than shown here — " +
			"the message under the failure points at it.\n\n" +
			"The most common cause by far is the model download: it is several gigabytes, " +
			"and a connection that drops part-way leaves it unfinished.",
	},
}

const unmatched = "I only know about setting up PN Scripts Assistant — I am a short list of written " +
	"answers, not the assistant itself. That comes once setup finishes.\n\n" +
	"Try asking about privacy, what this needs, how long it takes, or what it can do."

// Suggestions are shown before anything is asked, so the box does not look like
// a search field someone has to guess at.
func Suggestions() []string {
	return []string{
		"What is this?",
		"Is my data private?",
		"How long does setup take?",
	}
}

// Ask returns the best answer for a question, or says plainly that it does not
// know. Guessing would be worse than useless here: someone deciding whether to
// trust software with their files deserves an honest "I can't answer that".
func Ask(question string) Reply {
	q := strings.ToLower(strings.TrimSpace(question))

	if q == "" {
		return Reply{Answer: unmatched, Followup: Suggestions()}
	}

	type scored struct {
		topic Topic
		score int
	}

	var matches []scored

	for _, topic := range topics {
		score := 0

		for _, key := range topic.Keys {
			if !strings.Contains(q, key) {
				continue
			}

			// Longer keys are more specific, so weight by word count. Without
			// this, a topic keyed on "space" would beat one keyed on
			// "disk space" for the question "how much disk space".
			score += len(strings.Fields(key)) * 10

			// A key that is the whole question is as certain as this gets.
			if q == key {
				score += 50
			}
		}

		if score > 0 {
			matches = append(matches, scored{topic: topic, score: score})
		}
	}

	if len(matches) == 0 {
		return Reply{Answer: unmatched, Followup: Suggestions()}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	return Reply{Answer: matches[0].topic.Answer, Matched: true}
}
