/*
 * Package wording is the assistant saying something in its own words.
 *
 * The division is the point: this program decides what is true, and the model
 * decides how to say it. Facts come from here — a task finished, four things
 * are waiting, Godot is not installed — and the model is given those and
 * nothing else, with instructions to use them and add nothing. It is not
 * asked what happened, because it does not know; it is asked to speak.
 *
 * Everything it says was a sentence written in Go before this, and that was
 * defensible on a machine where a model costs most of a minute to answer. It
 * is still the wrong thing: an assistant that greets its owner with a string
 * formatted in a function is a program pretending to be one. So the sentences
 * stay, as what is said when there is no model to ask — offline, or before
 * one is installed, or when the answer takes too long to be worth waiting
 * for. Nothing here is ever the only thing standing between somebody and an
 * answer.
 *
 * What it must never do is invent. A greeting that mentions a file nobody has
 * is worse than a plain one, so the facts are given as data, the brief says
 * to use only those, and anything that comes back empty or absurdly long is
 * dropped in favour of the plain sentence.
 */
package wording

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * How long a phrasing is worth waiting for, which depends on who is waiting.
 *
 * On this machine a model reads about seven tokens a second, so even a short
 * brief and a handful of facts cost twenty to forty seconds. Somebody who has
 * just typed a sentence and is watching for an answer will not spend that on
 * wording; somebody who has just opened the program, or whose hour-long job
 * has finished, is not waiting on the sentence at all.
 */
const (
	// Typing is the budget for an answer somebody is sitting in front of.
	Typing = 25 * time.Second

	// HowLong is the budget for everything else: a greeting, a report on a
	// finished job, a line said while work carries on.
	HowLong = 90 * time.Second
)

/*
 * Quiet reports that nothing is to be phrased, so a caller can skip looking
 * for a model at all.
 *
 * True in a test binary that did not ask for phrasing. Not only for speed,
 * though half a minute a sentence would be reason enough: a test that asserts
 * what the assistant said has to be asserting something decided in code, and
 * a phrasing is by design not that. The tests of this package set the
 * variable and pass a model of their own.
 */
func Quiet() bool { return testing.Testing() && os.Getenv("PN_TEST_WORDING") == "" }

// Want is one thing to say: what kind of thing it is, the facts it may use,
// and what to say if there is no model to ask.
type Want struct {
	// Brief is what this is, in a line: "a greeting", "an answer to somebody
	// who said yes to a proposal that cannot start".
	Brief string

	// Facts is the only material allowed. Anything the sentence may mention
	// has to be in here.
	Facts any

	// Plain is what the program would have said by itself. Used when no model
	// answers, and never shown alongside the model's version.
	Plain string

	// Most is roughly how long the answer may be, in characters. Zero is two
	// sentences' worth.
	Most int

	// Within is how long to wait for it. Zero is HowLong.
	Within time.Duration
}

/*
 * Say puts the facts into the assistant's own words, or gives back the plain
 * sentence.
 *
 * The provider is passed in rather than chosen here so that privacy stays
 * where it is decided: whoever calls this has already been told which model
 * may see what.
 */
func Say(ctx context.Context, model llm.Provider, name string, w Want) string {
	if model == nil || strings.TrimSpace(w.Brief) == "" {
		return w.Plain
	}

	if Quiet() {
		return w.Plain
	}

	most := w.Most
	if most <= 0 {
		most = 320
	}

	facts, err := json.Marshal(w.Facts)
	if err != nil {
		return w.Plain
	}

	within := w.Within
	if within <= 0 {
		within = HowLong
	}

	ctx, stop := context.WithTimeout(ctx, within)
	defer stop()

	reply, err := model.Chat(ctx, llm.Request{
		Model:     name,
		MaxTokens: most / 3,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: brief(w.Brief, most)},
			{Role: llm.RoleUser, Content: string(facts)},
		},
	})
	if err != nil {
		return w.Plain
	}

	said := tidy(reply.Content)
	if said == "" || len(said) > most*3 {
		return w.Plain
	}

	return said
}

/*
 * brief is what the model is told before the facts.
 *
 * Every line of it is paid for twice: once in time on a processor, and once
 * in the model's attention. On this machine the prompt is the wait almost
 * exactly — about seven tokens a second — and a first draft of this ran to
 * two hundred tokens, which is half a minute of reading instructions before
 * saying ten words, and the answer came back too late to use. So: four
 * lines, each one earning its place.
 */
func brief(what string, most int) string {
	return "You are an assistant. Say " + what + ", in " + length(most) + ".\n" +
		"Use only the facts you are given: every name, number and path must appear in them.\n" +
		"Do not say anything is finished, working or ready unless a fact says so — a subject is not a result.\n" +
		"Plain words. No flattery, no offer of more help, no lists, no headings. Reply with the sentence only."
}

func length(most int) string {
	switch {
	case most <= 160:
		return "one sentence"
	case most <= 400:
		return "two or three sentences"
	default:
		return "a short paragraph"
	}
}

/*
 * tidy is the model's answer with the usual decorations taken off.
 *
 * Small models wrap an answer in quotes, prefix it with "Assistant:", or hand
 * back a fenced block because the facts arrived as JSON. None of that is the
 * sentence, and none of it is worth failing over.
 */
func tidy(said string) string {
	said = strings.TrimSpace(said)

	if fence := strings.Index(said, "```"); fence >= 0 {
		rest := said[fence+3:]
		if end := strings.Index(rest, "```"); end >= 0 {
			rest = rest[:end]
		}

		if line := strings.Index(rest, "\n"); line >= 0 {
			rest = rest[line+1:]
		}

		said = strings.TrimSpace(rest)
	}

	for _, prefix := range []string{"Assistant:", "assistant:", "Answer:", "Reply:"} {
		said = strings.TrimSpace(strings.TrimPrefix(said, prefix))
	}

	if len(said) > 1 && strings.HasPrefix(said, `"`) && strings.HasSuffix(said, `"`) {
		said = strings.TrimSpace(said[1 : len(said)-1])
	}

	return said
}
