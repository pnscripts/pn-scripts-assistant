package speech

import (
	"regexp"
	"strings"
)

/*
 * Waiting for somebody to finish, rather than for a stopwatch.
 *
 * A turn used to end after a fixed gap of quiet, and no single gap is right.
 * Set it short and it cuts in while somebody is still assembling a sentence;
 * set it long and every quick question is followed by an awkward wait. Both
 * were tried and both are wrong, because the question a gap cannot answer is
 * whether the person had finished — and that is a question about the words,
 * not about the silence.
 *
 * So the words are consulted. Somebody who stops after "and the other thing
 * is" has plainly not finished, however long the pause; somebody who stops
 * after "what is the weather today" has, immediately. Once the transcript
 * exists this is nearly free to check, and it turns a fixed timeout into
 * something that behaves like listening.
 */

// stillGoing are words nobody ends a thought on.
//
// Conjunctions, prepositions and the noises people make while thinking. A
// sentence that stops on one of these has stopped mid-way, and waiting is
// obviously right.
var stillGoing = map[string]bool{
	"and": true, "but": true, "so": true, "or": true, "because": true,
	"which": true, "that": true, "who": true, "if": true, "when": true,
	"while": true, "with": true, "for": true, "to": true, "of": true,
	"in": true, "on": true, "at": true, "from": true, "about": true,
	"like": true, "than": true, "then": true, "also": true, "plus": true,
	"the": true, "a": true, "an": true, "my": true, "your": true, "its": true,

	// Thinking noises. Whisper transcribes these, and they are the clearest
	// possible signal that somebody is still composing.
	"um": true, "uh": true, "erm": true, "hmm": true, "eh": true, "well": true,
}

// endsProperly matches a transcript that closes on a full stop, question mark
// or exclamation — whisper's own judgement that the thought completed.
var endsProperly = regexp.MustCompile(`[.!?…]["')\]]*\s*$`)

/*
 * SoundsUnfinished reports that somebody was still mid-sentence.
 *
 * Deliberately conservative: it says yes only when there is positive evidence
 * of an unfinished thought, never merely because it could not tell. Waiting
 * when somebody has finished is the more annoying mistake of the two — they
 * are sitting there having asked a question, watching nothing happen — so
 * anything ambiguous is treated as complete.
 */
/*
 * NeedsMore decides whether to keep listening, from the words and from how
 * long somebody was speaking.
 *
 * The duration is what makes this work. A transcript of "." is not somebody
 * saying nothing; it is what the recogniser returns for a fragment it could
 * not parse, and five seconds of speech coming back as a full stop means the
 * recording was cut somewhere in the middle. Judged on the text alone that
 * looks finished — it ends in a full stop, which is exactly the signal a
 * finished sentence gives — so the turn was thrown away and nothing ever
 * reached the conversation.
 */
func NeedsMore(text string, spokeForMS int) bool {
	trimmed := strings.TrimSpace(text)

	// Stripped of punctuation, so "." and "..." and "Thank you." are compared
	// on what they actually carry.
	bare := strings.TrimSpace(strings.Trim(trimmed, `.,!?…"'-— `))

	/*
	 * Far fewer words than the recording could have held means it was cut.
	 *
	 * Whisper answers a fragment with a full stop, or with whichever phrase it
	 * has seen most often — "Thank you." — and both of those read as complete
	 * sentences. Against the length of the recording they plainly are not.
	 *
	 * One word per second is the test, and it is deliberately far below how
	 * anybody actually talks: ordinary speech runs at two to three. Anything
	 * that slow is not a person speaking slowly, it is most of the audio
	 * having gone missing — five seconds returning "." or four returning
	 * "Thank you." Setting the bar at a realistic rate would start chasing
	 * people who pause, which is the failure this was meant to fix.
	 */
	if spokeForMS > 1500 {
		seconds := float64(spokeForMS) / 1000

		if float64(len(strings.Fields(bare))) < seconds {
			return true
		}
	}

	return SoundsUnfinished(text)
}

func SoundsUnfinished(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}

	// A closing punctuation mark is whisper saying the thought landed. It is
	// not infallible, but it is the best single signal available and it agrees
	// with the speaker most of the time.
	if endsProperly.MatchString(trimmed) {
		return false
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}

	last := strings.ToLower(strings.Trim(fields[len(fields)-1], `",;:-—`))

	if stillGoing[last] {
		return true
	}

	/*
	 * Trailing comma or dash: an explicit mark of more to come.
	 *
	 * Whisper writes these where somebody paused without stopping, which is
	 * exactly the case a fixed timeout gets wrong.
	 */
	if strings.HasSuffix(trimmed, ",") || strings.HasSuffix(trimmed, "-") ||
		strings.HasSuffix(trimmed, "—") {
		return true
	}

	return false
}

/*
 * MostContinuations caps how many times a turn may be extended.
 *
 * Somebody genuinely talking at length is served by two or three extensions;
 * beyond that the more likely explanation is a transcript that never ends
 * tidily — a language whisper punctuates poorly, or a room it keeps hearing
 * fragments in — and continuing forever would mean never answering at all.
 */
const MostContinuations = 3

// JoinTurns glues a continued turn back into one piece of speech.
func JoinTurns(parts []string) string {
	var kept []string

	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}

	return strings.Join(kept, " ")
}
