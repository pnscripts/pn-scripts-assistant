package brain

import (
	"context"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/progress"
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

	// All is the whole queue rather than what happens to be in front of you.
	// "Remember everything in the waiting list" is a different instruction from
	// "remember them", and answering the first with a hundred is not an answer.
	All bool
}

/*
 * Naming the list is what makes a long sentence an instruction.
 *
 * The short forms below have to be short, because "remember that" is only an
 * instruction about the queue when there is nothing else it could be about.
 * But "I want you to start to remember one by one everything that is in the
 * waiting list" is not ambiguous at any length — it says which list. That
 * sentence was asked three times and went to the model every time, which
 * answered that it had processed a large number of lessons, having processed
 * none.
 */
var queueNames = []string{
	"waiting list", "waiting for you", "review queue", "the queue",
	"waiting queue", "list of waiting", "things waiting", "what is waiting",
	"what's waiting", "waiting to be remembered",
}

// keepVerbs and dropVerbs are the two things that can be done to it.
var (
	keepVerbs = []string{"remember", "keep", "accept", "save", "learn", "confirm", "approve"}
	dropVerbs = []string{"forget", "discard", "reject", "delete", "throw away", "get rid"}
)

// wholeQueue means all of it, not the part on the screen.
var wholeQueue = []string{
	"everything", "every one", "all of", "all the", "one by one",
	"each one", "the whole", "the lot", "them all",
}

/*
 * asking is a question about the queue, not an instruction to change it.
 *
 * "Do you remember the waiting list?" contains a verb and names the list and
 * means neither. Cheap to check and the cost of getting it wrong is nine
 * thousand judgements made on somebody's behalf.
 */
func asking(text string) bool {
	if strings.HasSuffix(text, "?") {
		return true
	}

	for _, opener := range []string{
		"do you", "did you", "can you", "could you", "would you", "will you",
		"what ", "what's", "how ", "why ", "when ", "which ", "is there",
		"are there", "how many", "tell me what", "show me",
	} {
		if strings.HasPrefix(text, opener) {
			return true
		}
	}

	return false
}

func anyOf(text string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(text, p) {
			return true
		}
	}

	return false
}

// readLessonInstruction decides whether a message is an instruction about the
// pending queue.
//
// Deliberately narrow. It matches only short, unambiguous sentences, because a
// false positive silently rewrites what the brain believes about its owner —
// and "remind me to forget about the meeting" must not empty the review queue.
func readLessonInstruction(message string) LessonInstruction {
	text := strings.ToLower(strings.TrimSpace(message))

	/*
	 * A sentence that names the list, first, at any length.
	 *
	 * The length rule below exists because "remember that" could be about
	 * anything; it does not apply to a sentence that says which list it means.
	 */
	if !asking(text) && anyOf(text, queueNames) {
		all := anyOf(text, wholeQueue)

		if anyOf(text, dropVerbs) {
			return LessonInstruction{Accept: false, Found: true, All: all}
		}

		if anyOf(text, keepVerbs) {
			return LessonInstruction{Accept: true, Found: true, All: all}
		}
	}

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

	// The whole list is a job, not a turn. See workThroughTheQueue.
	if instruction.All {
		return b.startOnTheQueue(instruction.Accept)
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

/*
 * startOnTheQueue works through all of it, behind the conversation.
 *
 * Nine thousand lessons cannot be answered in a turn. Accepting one embeds it,
 * which is a fifth of a second on this machine, so the whole queue is between
 * half an hour and an hour — long past the point where anybody is still
 * sitting there, and long past any sensible request timeout. Discarding is
 * cheap, but it goes the same way so that the answer to the same sentence does
 * not depend on which verb was used.
 *
 * Through the job runner rather than a bare goroutine, so it appears under
 * "Everything it is doing", can be stopped, and counts against what this
 * machine is willing to run at once.
 */
func (b *Brain) startOnTheQueue(accept bool) (string, bool) {
	total, err := b.DB.CountPendingLessons()
	if err != nil || total == 0 {
		return "", false
	}

	verb := map[bool]string{true: "Remembering", false: "Discarding"}[accept]
	what := fmt.Sprintf("%s everything in the waiting list (%d)", verb, total)

	if b.Jobs == nil {
		// No runner: do what can be done in a turn and say so, rather than
		// promising the rest and dropping it.
		return "", false
	}

	_, err = b.Jobs.Start(what, func(ctx context.Context) (string, error) {
		return b.workThroughTheQueue(ctx, accept, total)
	})
	if err != nil {
		return fmt.Sprintf(
			"I would have to do that in the background and there is already "+
				"something running there — %v. Ask me again when it has finished, "+
				"or stop it under \"Everything it is doing\".", err), true
	}

	how := map[bool]string{
		true:  "remembering",
		false: "discarding",
	}[accept]

	return fmt.Sprintf(
		"Started %s all %d, one at a time, in the background. It takes a while — "+
			"each one has to be compared against everything I already know — so the "+
			"count under \"Waiting for you\" will come down slowly rather than at once. "+
			"You can carry on talking to me while it runs, and stop it under "+
			"\"Everything it is doing\".", how, total), true
}

/*
 * workThroughTheQueue walks the queue once, by id.
 *
 * By id and never backwards, because accepting a lesson takes it out of the
 * results: asking repeatedly for "the first hundred proposed" would re-read
 * whatever moved up, and would meet anything that failed to store on every
 * single pass without ever getting past it.
 *
 * Stopping is not failing. Somebody who stops this after four thousand has had
 * four thousand of them dealt with, and the answer says so.
 */
func (b *Brain) workThroughTheQueue(ctx context.Context, accept bool, total int) (string, error) {
	var kept, duplicates, dropped, failed, seen int

	var after int64

	for {
		if ctx.Err() != nil {
			break
		}

		batch, err := b.DB.LessonsAfter(learning.StatusProposed, after, queuePage)
		if err != nil {
			return "", err
		}

		if len(batch) == 0 {
			break
		}

		for _, l := range batch {
			if ctx.Err() != nil {
				break
			}

			after = l.ID
			seen++

			if !accept {
				if err := b.DB.SetLessonStatus(l.ID, learning.StatusRejected); err != nil {
					failed++

					continue
				}

				dropped++

				continue
			}

			decision, err := b.DecideLesson(ctx, l.ID, true)
			if err != nil {
				failed++

				continue
			}

			if decision.Duplicate {
				duplicates++

				continue
			}

			kept++
		}

		progress.SetBackground("learning", fmt.Sprintf(
			"Going through the waiting list — %d of %d", seen, total))
	}

	progress.Done()

	return theQueueReport(kept, duplicates, dropped, failed, ctx.Err() != nil), nil
}

// theQueueReport says what became of them, in numbers rather than adjectives.
func theQueueReport(kept, duplicates, dropped, failed int, stopped bool) string {
	var parts []string

	if kept > 0 {
		parts = append(parts, fmt.Sprintf("%d remembered", kept))
	}

	if duplicates > 0 {
		parts = append(parts, fmt.Sprintf("%d I already knew", duplicates))
	}

	if dropped > 0 {
		parts = append(parts, fmt.Sprintf("%d discarded", dropped))
	}

	/*
	 * What could not be stored is said, not swallowed.
	 *
	 * The usual cause is the embedding model not running, and it fails the
	 * same quiet way for every one of nine thousand — so a report that leaves
	 * it out reads as a job well done on a queue that has not moved.
	 */
	if failed > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d I could not store — the embedding model may not be running", failed))
	}

	if len(parts) == 0 {
		return "There was nothing in the waiting list to go through."
	}

	opening := "Finished the waiting list"
	if stopped {
		opening = "Stopped part way through the waiting list"
	}

	return fmt.Sprintf("%s: %s.", opening, strings.Join(parts, ", "))
}
