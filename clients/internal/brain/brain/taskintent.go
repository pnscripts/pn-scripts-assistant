package brain

import (
	"strings"

	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * Being asked for a job rather than an answer.
 *
 * Explicit, and only explicit, for now. The planner will eventually recognise
 * a job from its shape, and when it does this route stays — because the shape
 * of a job is a guess and this is not one, and somebody who wants a piece of
 * work done needs a way to say so that cannot be talked out of it.
 *
 * The phrases are matched at either end of the sentence. "As a task, go
 * through my projects" and "go through my projects as a task" are the same
 * request, and being told the second form does not work is the kind of thing
 * that teaches somebody the feature is unreliable.
 */
var taskPhrases = []string{
	"as a task",
	"as a job",
	"do this as a task",
	"като задача",
	"като работа",
}

/*
 * askedForATask returns the request with the phrase taken out.
 *
 * Taken out because it becomes the step's instruction and the task's name, and
 * a task called "As a task, go through my projects" reads like the program
 * repeating itself back.
 */
func askedForATask(message string) (string, bool) {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)

	for _, phrase := range taskPhrases {
		switch {
		case strings.HasPrefix(lower, phrase):
			rest := strings.TrimSpace(trimmed[len(phrase):])
			rest = strings.TrimSpace(strings.TrimLeft(rest, ",:—-"))

			if rest != "" {
				return rest, true
			}

		case strings.HasSuffix(lower, phrase):
			rest := strings.TrimSpace(trimmed[:len(trimmed)-len(phrase)])
			rest = strings.TrimSpace(strings.TrimRight(rest, ",:—-"))

			if rest != "" {
				return rest, true
			}
		}
	}

	return trimmed, false
}

/*
 * worthPlanning decides whether to spend a planning call on this turn.
 *
 * Two routes in, and the difference between them is a guess and an
 * instruction. Somebody who said "as a task" gets one whatever the planner
 * comes back with; a request that merely reads like work gets a plan and
 * becomes a task only if the plan has more than one step in it — otherwise the
 * turn is answered exactly as it always was, and the whole attempt cost one
 * short call.
 *
 * The guess is not made about spoken turns. A planning call is most of a
 * minute of silence on this processor, and it would be spent on a sentence
 * that a recogniser has already had one guess at. Saying "as a task" out loud
 * still works, because that is not a guess.
 */
func (b *Brain) worthPlanning(req ChatRequest) (request string, forced, worth bool) {
	if b.Tasks == nil {
		return "", false, false
	}

	request, forced = askedForATask(req.Message)

	if forced {
		return request, true, true
	}

	if req.Spoken {
		return request, false, false
	}

	_, looks := llm.LooksLikeAJob(req.Message)

	return request, false, looks
}
