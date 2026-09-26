package brain

import (
	"strings"
	"time"
)

/*
 * What is true when somebody opens the program, as facts rather than as
 * sentences.
 *
 * The greeting was already spoken in the assistant's own words — and said
 * almost the same thing every time, which is the fault this fixes. It was
 * handed *sentences*: "We have not met properly: ask me what I can do, and I
 * will tell you what to say for each of it." A model given a finished
 * sentence and asked to put it in its own words will hand back the same
 * sentence with the words moved about. It was not deciding anything; it was
 * paraphrasing a constant.
 *
 * So it gets the state instead — the hour, whether they have met, how much is
 * remembered, what is waiting, what was last talked about — and decides for
 * itself what is worth saying, including saying almost nothing when nothing
 * has happened. The composed sentences are still there as the fallback for a
 * machine with no model, which is what they were always for.
 */

// About is the state of things, for the model to make a greeting out of.
type About map[string]any

/*
 * about is what is true now.
 *
 * Only what is true: a key is left out rather than given an empty value,
 * because a model handed "waiting: 0" will find something to say about
 * nothing waiting, and a model handed nothing will not.
 */
func (b *Brain) about(last *LastTalk, backAlready bool, introducing bool) About {
	facts := About{
		"hour of the day": time.Now().Format("15:04"),
		"part of the day": partOfDay(time.Now().Hour()),
	}

	if owner := strings.TrimSpace(b.Cfg.Owner); owner != "" {
		facts["who you are greeting"] = owner
	}

	if backAlready {
		// They were here minutes ago. You do not reintroduce yourself to
		// somebody you were talking to five minutes ago.
		facts["they were talking to you minutes ago"] = true
	}

	if introducing {
		facts["you have never met them before"] = true

		if shape := strings.TrimSpace(b.WhatItCanDoInShort()); shape != "" {
			facts["what you can do here"] = shape
		}
	}

	if b.DB != nil {
		if n, err := b.DB.CountFacts(); err == nil {
			facts["things you remember about their work"] = n
		}

		if n, err := b.DB.CountPendingLessons(); err == nil && n > 0 {
			facts["things you would like to remember, waiting for them"] = n
		}

		if approvals, err := b.DB.PendingInvocations(); err == nil && len(approvals) > 0 {
			facts["actions waiting for their approval"] = len(approvals)
		}
	}

	if last != nil && strings.TrimSpace(last.Topic) != "" {
		about := About{
			"about":        last.Topic,
			"how long ago": howLongAgo(time.Since(last.When)),
		}

		if b.carryingOn(last) {
			about["still the same conversation"] = true
		}

		facts["the last thing you talked about"] = about
	}

	b.mu.Lock()
	cut := b.cutShort
	b.mu.Unlock()

	if cut > 0 {
		facts["conversations the last run was cut off in the middle of"] = cut
	}

	if moved := strings.TrimSpace(b.journeyLine()); moved != "" {
		// This one stays a sentence: it is the one condition where being
		// wrong about the detail matters more than the phrasing, and the
		// sentence is already carefully worded.
		facts["something that has just happened to this brain"] = moved
	}

	switch b.Storage().Level {
	case "critical":
		facts["the drive it lives on"] = "nearly full, which will stop it learning"
	case "warn":
		facts["the drive it lives on"] = "filling up"
	}

	return facts
}

// partOfDay is the hour said the way a person would say it.
func partOfDay(hour int) string {
	switch {
	case hour < 5:
		return "the middle of the night"
	case hour < 12:
		return "morning"
	case hour < 18:
		return "afternoon"
	default:
		return "evening"
	}
}
