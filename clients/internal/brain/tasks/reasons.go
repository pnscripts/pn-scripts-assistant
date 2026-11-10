package tasks

import "strings"

/*
 * Why a task waits or stopped, in words the page can say in either language.
 *
 * The reason is written on the task in English, as a sentence, and stays
 * that way: it is read back by the conversation and by whoever looks at the
 * database. The ones the program writes in fixed words are named here once,
 * used by the code that writes them, and each has a key in the page's
 * i18n.js (task.why.*), so a Bulgarian page says them in Bulgarian. A reason
 * that carries an error from somewhere else keeps that error's words.
 */
const (
	ReasonAsked      = "it asked you something"
	ReasonDecision   = "it needs your decision"
	ReasonStopped    = "you stopped it"
	ReasonNotRunning = "the model it was using is not running"
	ReasonSlow       = "the model took longer to answer than it was given"
	ReasonMostSteps  = "it reached the most steps it was allowed for one job"
	ReasonNoBuild    = "this build cannot run tasks"
	ReasonHalfHour   = "it reached the half hour it was given and was not finished"
	ReasonThinking   = "it used up the thinking it was given for this"

	// Followed by the error, and by the specialist's title in lower case.
	reasonError     = "it stopped on an error: "
	reasonWaitingOn = "waiting on the "
)

var reasonKeys = map[string]string{
	ReasonAsked:      "task.why.asked",
	ReasonDecision:   "task.why.decision",
	ReasonStopped:    "task.why.stopped",
	ReasonNotRunning: "task.why.notRunning",
	ReasonSlow:       "task.why.slow",
	ReasonMostSteps:  "task.why.mostSteps",
	ReasonNoBuild:    "task.why.noBuild",
	ReasonHalfHour:   "task.why.halfHour",
	ReasonThinking:   "task.why.thinking",
}

/*
 * ReasonKey is the page's key for a reason and what goes into it, or an
 * empty key when the reason is not one the program words itself.
 *
 * "why" is the error after "it stopped on an error:", with "why_key" when
 * that error is itself one of the fixed reasons; "who" is the specialist's
 * title after "waiting on the".
 */
func ReasonKey(because string) (string, map[string]string) {
	if key, ok := reasonKeys[because]; ok {
		return key, nil
	}

	if rest, ok := strings.CutPrefix(because, reasonError); ok {
		args := map[string]string{"why": rest}

		if inner, ok := reasonKeys[rest]; ok {
			args["why_key"] = inner
		}

		return "task.why.error", args
	}

	if who, ok := strings.CutPrefix(because, reasonWaitingOn); ok {
		return "task.why.waitingOn", map[string]string{"who": who}
	}

	return "", nil
}

// ReasonKeys is every key ReasonKey can give, for the test that i18n.js has
// each of them.
func ReasonKeys() []string {
	out := []string{"task.why.error", "task.why.waitingOn"}

	for _, key := range reasonKeys {
		out = append(out, key)
	}

	return out
}
