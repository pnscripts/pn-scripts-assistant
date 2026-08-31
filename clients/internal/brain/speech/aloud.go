package speech

import (
	"context"
	"pn-brain/internal/brain/progress"
	"strings"
	"sync"
	"unicode"
)

/*
 * Speaking an answer while it is still being written.
 *
 * A reply here is produced at about ten tokens a second, so three sentences is
 * most of a minute of silence followed by all of it at once. That is not a
 * conversation; it is a form submission with a voice on the end.
 *
 * Said sentence by sentence as they arrive, the first words come a few seconds
 * after the question — which is roughly how long a person takes to start
 * answering, and is the whole difference between talking to something and
 * waiting for it.
 */

// Aloud speaks text as it arrives, one sentence at a time and in order.
type Aloud struct {
	ctx context.Context

	mu      sync.Mutex
	pending strings.Builder
	queue   chan string
	done    chan struct{}
	started bool

	// spoken is everything actually said, for the record afterwards.
	spoken strings.Builder

	// saying marks that the step has already been set to Speaking, so a
	// multi-sentence answer is one entry in the panel rather than one per
	// sentence alternating with Answering.
	saying bool
}

// NewAloud starts a speaker. Close must be called.
func NewAloud(ctx context.Context) *Aloud {
	a := &Aloud{
		ctx: ctx,
		// Small: the point is to stay close behind the writing, not to build a
		// backlog of sentences nobody has heard yet.
		queue: make(chan string, 8),
		done:  make(chan struct{}),
	}

	go a.run()

	return a
}

/*
 * Write takes the next piece of the answer.
 *
 * Pieces arrive a few characters at a time, so they are gathered until there is
 * a whole sentence. Speaking a fragment is worse than waiting for the rest of
 * it: the synthesiser puts the wrong tune on half a clause, and the pause
 * afterwards lands in the middle of the thought.
 */
func (a *Aloud) Write(text string) {
	if text == "" {
		return
	}

	/*
	 * Nothing more is queued once its owner has cut in.
	 *
	 * Stopping the sound and then saying the remaining four sentences anyway
	 * would be worse than never stopping: the person interrupted because they
	 * wanted to say something, and being answered by the rest of the previous
	 * answer is the machine insisting on finishing its point.
	 */
	if Interrupted() {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.pending.WriteString(text)

	/*
	 * A reasoning model's working is not the answer.
	 *
	 * Models like deepseek-r1 narrate their deliberation inside <think> tags
	 * before answering. Read aloud, that is several minutes of the model
	 * talking to itself, and the person waiting has no way to know the answer
	 * has not started.
	 */
	if a.skipThinking() {
		return
	}

	for {
		sentence, rest, found := cutSentence(a.pending.String())
		if !found {
			break
		}

		a.pending.Reset()
		a.pending.WriteString(rest)

		if strings.TrimSpace(sentence) == "" {
			continue
		}

		a.started = true

		select {
		case a.queue <- sentence:
		case <-a.ctx.Done():
			return
		}
	}
}

/*
 * skipThinking holds everything back while a <think> block is open.
 *
 * Reports whether there is nothing speakable yet. Once the block closes it is
 * removed and what follows is spoken normally.
 */
func (a *Aloud) skipThinking() bool {
	text := a.pending.String()

	open := strings.Index(text, "<think>")
	if open < 0 {
		return false
	}

	close := strings.Index(text[open:], "</think>")
	if close < 0 {
		// Still inside it. Keep only what came before, which is usually
		// nothing, and wait.
		a.pending.Reset()
		a.pending.WriteString(text[:open])

		return true
	}

	a.pending.Reset()
	a.pending.WriteString(text[:open] + text[open+close+len("</think>"):])

	return false
}

// Close says whatever is left and waits for the voice to finish.
func (a *Aloud) Close() {
	a.mu.Lock()

	last := strings.TrimSpace(a.pending.String())

	a.pending.Reset()

	if last != "" {
		a.started = true

		select {
		case a.queue <- last:
		case <-a.ctx.Done():
		}
	}

	a.mu.Unlock()

	close(a.queue)
	<-a.done
}

// Started reports whether anything was said, so a caller knows not to speak the
// whole answer again afterwards.
func (a *Aloud) Started() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.started
}

// run speaks the queue in order.
//
// Strictly one at a time: two sentences spoken at once is not a faster answer,
// it is two voices.
func (a *Aloud) run() {
	defer close(a.done)

	for sentence := range a.queue {
		if a.ctx.Err() != nil || Interrupted() {
			return
		}

		/*
		 * Said out loud is a different thing from written.
		 *
		 * Without this the line reads "Answering" from the first token until
		 * the last word is spoken, which on this machine is minutes of one
		 * word covering two quite different waits.
		 */
		/*
		 * One "Speaking" for the whole answer, not one per sentence.
		 *
		 * A spoken answer is several sentences, and setting the step around
		 * each of them made the panel a wall of alternating Speaking and
		 * Answering — a dozen entries with no durations, burying the steps
		 * that actually said something. The distinction between writing and
		 * saying is still worth drawing; it just belongs to the answer, not to
		 * every sentence within it.
		 */
		if !a.saying {
			progress.Set("speaking", "Speaking")

			a.saying = true
		}

		if err := SpeakAndWait(a.ctx, sentence); err != nil {
			// A voice that failed is not a reason to lose the rest of the
			// answer; the text is on screen either way.
			return
		}

		a.mu.Lock()
		a.spoken.WriteString(sentence)
		a.mu.Unlock()

	}
}

/*
 * cutSentence takes one complete sentence off the front.
 *
 * A full stop is only the end of a sentence when something follows it that
 * looks like a new one — otherwise every decimal point and every "e.g." starts
 * the voice off mid-number. Waiting for the following space costs nothing,
 * because the next characters are already on their way.
 */
func cutSentence(text string) (sentence, rest string, found bool) {
	for i, r := range text {
		if r != '.' && r != '!' && r != '?' && r != '\n' {
			continue
		}

		after := text[i+len(string(r)):]

		if r != '\n' {
			// Needs a space or a newline after it to be an ending.
			if after == "" {
				continue
			}

			next := []rune(after)[0]

			if !unicode.IsSpace(next) {
				continue
			}

			// A single letter before a full stop is an initial, not an end.
			if i > 0 && isInitial(text[:i]) {
				continue
			}
		}

		sentence = strings.TrimSpace(text[:i+len(string(r))])
		rest = strings.TrimLeft(after, " \t")

		// Too short to be worth saying on its own; wait for more.
		if len([]rune(sentence)) < 2 {
			continue
		}

		return sentence, rest, true
	}

	return "", text, false
}

// isInitial reports that the character before a full stop is a lone letter,
// as in "J. Smith" — which is not the end of a sentence.
func isInitial(before string) bool {
	r := []rune(before)

	if len(r) == 0 || !unicode.IsLetter(r[len(r)-1]) {
		return false
	}

	if len(r) == 1 {
		return true
	}

	return unicode.IsSpace(r[len(r)-2])
}
