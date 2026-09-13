package llm

import "strings"

/*
 * LooksLikeAJob reports a request that is work rather than a question.
 *
 * An allowlist, and conservative in the same direction as ChooseModel and for
 * the same reason: the two mistakes are not symmetric. Missing one costs
 * nothing — the request is answered the way it always was, and its owner can
 * say "do that as a task" — where taking a greeting for a job costs a planning
 * call on a machine where that is most of a minute, and produces a panel of
 * work with "good morning" in it.
 *
 * It matches shapes rather than subjects. A job is several things joined, or a
 * sweep over a set, or something with a stated end — and none of those is a
 * topic, which is what makes this list short enough to be worth keeping.
 */
func LooksLikeAJob(message string) (string, bool) {
	text := normalise(message)

	if text == "" {
		return "", false
	}

	// A short sentence is a question. "list my drives" is not a job, whatever
	// verb it starts with, and wrapping it in a plan helps nobody.
	if len(strings.Fields(text)) < 6 {
		return "", false
	}

	for _, shape := range jobShapes {
		if strings.Contains(text, shape) {
			return shape, true
		}
	}

	return "", false
}

/*
 * jobShapes are the forms a piece of work takes when somebody describes one.
 *
 * English and Bulgarian, as everywhere else here. "and then" is the strongest
 * of them by a distance: two things joined in one sentence is the whole
 * definition of something that cannot be done in a single turn.
 */
var jobShapes = []string{
	// Several things, one after another.
	"and then", "after that", "then tell me", "then write", "then send",
	"и след това", "после", "и накрая",

	// A sweep over a set.
	"go through", "work through", "go over", "each of", "every file",
	"every project", "all of my", "all my", "one by one",
	"мини през", "прегледай", "всеки файл", "всички мои",

	// A stated end rather than a stated question.
	"until it", "until they", "so that", "make sure that",
	"докато", "така че",

	// Asked for outright.
	"work on this", "take care of this", "sort out", "tidy up",
	"заеми се", "погрижи се",
}
