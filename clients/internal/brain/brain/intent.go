package brain

import (
	"context"
	"fmt"
	"strings"

	"pn-brain/internal/brain/learning"
)

// Answering a direct instruction about the review queue, without the model.
//
// The greeting says four things are waiting for review, so "remember them" is
// the obvious next sentence — and until now the model would answer "I will
// remember these instructions" and do nothing at all, because it has no way to
// act on its own queue. An assistant that says it did something it did not do
// is worse than one that admits it cannot.
//
// Handled here rather than as a tool for two reasons. Tools are not offered on
// spoken turns, which is exactly when somebody is most likely to say this out
// loud. And the answer is deterministic — accept or reject what is already
// listed — so putting a language model between the instruction and the action
// adds a minute of latency and a chance of getting it wrong.

// acceptPhrases mean "keep what you are asking about".
var acceptPhrases = []string{
	"remember them", "remember those", "remember it", "remember that",
	"yes remember", "keep them", "keep those", "accept them", "accept those",
	"save them", "save those", "yes save", "learn them", "go ahead and remember",
}

// rejectPhrases mean "throw them away".
var rejectPhrases = []string{
	"forget them", "forget those", "forget it", "discard them", "discard those",
	"reject them", "reject those", "delete them", "do not remember",
	"don't remember", "throw them away", "no forget",
}

// queuePage is how much of the queue one instruction works through.
//
// Accepting a lesson embeds it, so the whole of a nine-thousand-item queue
// would hold the turn open far past the point where anybody is still listening.
// A hundred is roughly a minute, and the answer says what is left.
const queuePage = 100

// LessonInstruction is a recognised instruction about the review queue.
type LessonInstruction struct {
	Accept bool
	Found  bool
}

// readLessonInstruction decides whether a message is an instruction about the
// pending queue.
//
// Deliberately narrow. It matches only short, unambiguous sentences, because a
// false positive silently rewrites what the brain believes about its owner —
// and "remind me to forget about the meeting" must not empty the review queue.
func readLessonInstruction(message string) LessonInstruction {
	text := strings.ToLower(strings.TrimSpace(message))
	text = strings.Trim(text, ".!?")

	// A long sentence is a thought, not a command. Anything with a subject and
	// clauses is discussing the queue rather than instructing about it.
	if len(strings.Fields(text)) > 6 {
		return LessonInstruction{}
	}

	// Strip a leading acknowledgement: "okay, remember them" is the same
	// instruction as "remember them".
	for _, opener := range []string{"okay ", "ok ", "yes ", "sure ", "right ", "please ", "alright "} {
		text = strings.TrimPrefix(text, opener)
		text = strings.TrimPrefix(text, strings.TrimSpace(opener)+", ")
	}

	text = strings.TrimSpace(strings.Trim(text, ","))

	for _, phrase := range rejectPhrases {
		if strings.Contains(text, phrase) {
			return LessonInstruction{Accept: false, Found: true}
		}
	}

	for _, phrase := range acceptPhrases {
		if strings.Contains(text, phrase) {
			return LessonInstruction{Accept: true, Found: true}
		}
	}

	return LessonInstruction{}
}

// handleLessonInstruction acts on the queue and says plainly what happened.
//
// Returns whether it handled the message. When nothing is waiting it does not:
// "remember that" with an empty queue is somebody talking about something else.
func (b *Brain) handleLessonInstruction(ctx context.Context, message string) (string, bool) {
	instruction := readLessonInstruction(message)
	if !instruction.Found {
		return "", false
	}

	pending, err := b.DB.LessonsByStatus(learning.StatusProposed, queuePage)
	if err != nil || len(pending) == 0 {
		return "", false
	}

	if !instruction.Accept {
		for _, l := range pending {
			b.DB.SetLessonStatus(l.ID, learning.StatusRejected)
		}

		rest := andTheRest(b)
		many := "all " + fmt.Sprint(len(pending))

		if rest != "" {
			many = fmt.Sprint(len(pending))
		}

		return fmt.Sprintf("Discarded %s. I will not remember %s.%s",
			count(len(pending), "one", many),
			map[bool]string{true: "it", false: "them"}[len(pending) == 1],
			rest), true
	}

	var kept, duplicates int

	for _, l := range pending {
		decision, err := b.DecideLesson(ctx, l.ID, true)
		if err != nil {
			continue
		}

		if decision.Duplicate {
			duplicates++

			continue
		}

		kept++
	}

	// "All" only when there is nothing behind them. "Remembered all 100" above
	// "9,097 more are waiting" is a sentence arguing with the sentence after it.
	rest := andTheRest(b)
	all := "all "

	if rest != "" {
		all = ""
	}

	switch {
	case kept == 0 && duplicates > 0:
		return "I already knew all of that, so nothing was stored twice." + rest, true
	case kept == 0:
		return "I could not store those — the embedding model may not be running.", true
	case duplicates > 0:
		return fmt.Sprintf("Remembered %d. The other %d I already knew.%s",
			kept, duplicates, rest), true
	case kept == 1:
		return "Remembered it." + rest, true
	default:
		return fmt.Sprintf("Remembered %s%d.%s", all, kept, rest), true
	}
}

/*
 * What is still in the queue, said out loud.
 *
 * The queue is worked a hundred at a time — accepting a lesson embeds it, and
 * nine thousand of those would hold the turn open for an hour — so "remembered
 * all 100" was true of the batch and false of the queue. Somebody who says
 * "remember them", hears "all", and then sees the same badge is entitled to
 * think nothing happened.
 *
 * Empty when the queue is clear, so the ordinary case of four lessons still
 * ends on "Remembered all 4." and nothing more.
 */
func andTheRest(b *Brain) string {
	left, err := b.DB.CountPendingLessons()
	if err != nil || left == 0 {
		return ""
	}

	if left == 1 {
		return " One more is still waiting — say it again for that one."
	}

	// Only promise a hundred when there are a hundred to promise.
	next := "the rest"
	if left > queuePage {
		next = "the next hundred"
	}

	return fmt.Sprintf(" %d more are still waiting — say it again for %s.", left, next)
}
