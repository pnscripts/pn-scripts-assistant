package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

/*
 * Asking how quick it is, and what would make it quicker.
 *
 * "You are slow" is the complaint, and it is one sentence covering four
 * separate waits with four different answers — the microphone waiting out a
 * silence, the recogniser, the model, the voice. Which of them it is depends
 * on the machine: on four processor cores the model is nearly all of it, and
 * on a machine with a card the model drops under a second and what is left is
 * this program's own doing.
 *
 * A tool rather than only a panel, because the complaint is made out loud and
 * usually while waiting. Being told to open a page is not an answer to it.
 */
type HowFast struct {
	// Reading hands back the measured timings and what they mean here.
	Reading func() Speed
}

// Speed is what the timings came to and what would change them.
type Speed struct {
	// Over is how many turns were measured. Nothing else is worth reading
	// when this is zero.
	Over int

	// Verdict is the one-line answer.
	Verdict string

	// The four waits, in milliseconds.
	Hearing  int
	Thinking int
	Writing  int
	Speaking int
	ToFirst  int

	// Model is what answered, and Quick the small model for conversation,
	// empty when none is installed.
	Model string
	Quick string

	// Findings are what is holding it back, most important first.
	Findings []Finding
}

// Finding is one thing that is slow and the one thing that would change it.
type Finding struct {
	Stage   string
	Costs   int
	Because string
	Change  string
}

func (HowFast) Name() string { return "how_fast_can_you_answer" }

func (HowFast) Description() string {
	return "Report how long answering actually takes on this machine, broken into the four " +
		"waits — making out the words, the model thinking, writing the first sentence, and " +
		"producing the first sound — and say what would make it quicker here. Use this for " +
		"\"why are you so slow\", \"how fast are you\", \"is this real time\", \"what is taking " +
		"so long\", or any question about speed or performance."
}

func (HowFast) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

// Safe: it reports measurements and changes nothing.
func (HowFast) Risk() Risk { return Safe }

func (HowFast) Summarize(json.RawMessage) string { return "Check how quickly it is answering" }

func (t HowFast) Execute(context.Context, json.RawMessage) (string, error) {
	if t.Reading == nil {
		return "", fmt.Errorf("nothing here is being timed")
	}

	s := t.Reading()

	if s.Over == 0 {
		return "Nothing has been timed yet. Say something to me and ask again — the " +
			"measurement starts at your last word, so it needs a turn to have happened.", nil
	}

	var b strings.Builder

	b.WriteString(s.Verdict)

	/*
	 * The stages, but only the ones worth a sentence.
	 *
	 * A stage costing forty milliseconds is not information, it is padding,
	 * and reciting all four every time buries the one that matters on this
	 * machine underneath three that do not.
	 */
	b.WriteString("\n\nWhere it goes:")

	for _, stage := range []struct {
		what string
		cost int
	}{
		{"making out the words", s.Hearing},
		{"the model thinking", s.Thinking},
		{"writing enough to say", s.Writing},
		{"producing the first sound", s.Speaking},
	} {
		if stage.cost < 50 {
			continue
		}

		fmt.Fprintf(&b, "\n· %s — %s", stage.what, milliseconds(stage.cost))
	}

	if s.Model != "" {
		fmt.Fprintf(&b, "\n\nAnswered by %s.", s.Model)

		if s.Quick == "" {
			b.WriteString(" There is no small model installed, so every greeting " +
				"is answered by that one too.")
		} else {
			fmt.Fprintf(&b, " Conversation goes to %s.", s.Quick)
		}
	}

	for _, f := range s.Findings {
		fmt.Fprintf(&b, "\n\n%s (%s): %s. %s.",
			f.Stage, milliseconds(f.Costs), f.Because, f.Change)
	}

	return b.String(), nil
}

// milliseconds reads a duration the way somebody would say it.
func milliseconds(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}

	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
