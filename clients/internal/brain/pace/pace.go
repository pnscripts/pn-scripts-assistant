/*
 * Package pace measures how long it takes to answer, stage by stage.
 *
 * "Real time" is not a setting, and it is not a property of the model either.
 * It is a budget: somebody stops talking, and a fixed number of milliseconds
 * later a sound comes out of the speaker. Everything between those two moments
 * belongs to somebody — the microphone, the recogniser, the model, the voice —
 * and until each part is timed separately the only available answer to "why is
 * it slow" is the name of whichever part is easiest to blame.
 *
 * It matters more here than in most programs because this one is meant to run
 * on machines that differ by two orders of magnitude. On four processor cores
 * the model is the whole of the wait and nothing else is worth looking at; on a
 * graphics card the model drops to a fraction of a second and the program's own
 * fixed costs — the silence it waits out, the process it starts to speak with —
 * become the entire delay. The same code is slow for opposite reasons, and one
 * number cannot say which.
 *
 * So each stage is timed on every turn, kept in memory only, and shown. Kept in
 * memory because it is diagnostic rather than history: a hundred turns is more
 * than anybody looks at, and writing timings to the disk would mean writing
 * down what somebody said and when, forever, to answer a question nobody is
 * asking any more.
 */
package pace

import (
	"sync"
	"time"
)

// Kept is how many turns are remembered. Enough to see a pattern, few enough
// that it is obviously not a record of anybody's day.
const Kept = 40

/*
 * Turn is the timeline of one answer.
 *
 * The stages are marked as they pass rather than computed at the end, because
 * a turn can stop at any of them — nothing was said, the words were not
 * addressed to it, the model failed — and a half-finished timeline is still
 * worth reading. A turn that ends after Transcribed says something specific:
 * it heard and decided not to answer.
 */
type Turn struct {
	// When the person stopped speaking. Everything is measured from here,
	// because this is the moment they start waiting.
	Ended time.Time

	// Each stage, as it happened. Zero means it never got that far.
	Transcribed   time.Time
	Asked         time.Time
	FirstToken    time.Time
	FirstSentence time.Time
	FirstSound    time.Time
	Finished      time.Time

	// Spoken says whether this was a voice turn. A typed one has no silence to
	// wait out and no voice to synthesise, so its budget is a different shape.
	Spoken bool

	// Model is which model answered, and Tools whether it was offered any —
	// the two facts that move the numbers most.
	Model string
	Tools bool

	// Words is how many were said to it, which is what the recogniser and the
	// prefill both scale with.
	Words int
}

/*
 * Milestones is one turn's timings in milliseconds, ready to be shown.
 *
 * Separate from Turn because the stages are recorded as instants and read as
 * durations, and doing that conversion at the edge means every reader — the
 * page, a tool, a test — gets the same arithmetic rather than its own.
 */
type Milestones struct {
	Hearing    int `json:"hearing"`
	Thinking   int `json:"thinking"`
	Writing    int `json:"writing"`
	Speaking   int `json:"speaking"`
	ToFirst    int `json:"to_first_sound"`
	Altogether int `json:"altogether"`

	Spoken bool   `json:"spoken"`
	Model  string `json:"model"`
	Tools  bool   `json:"tools"`
	Words  int    `json:"words"`
	Ago    int    `json:"ago_seconds"`
}

/*
 * Milestones converts a timeline into the four waits somebody actually
 * experiences.
 *
 * Not the six stages that were recorded: "prefill" and "queueing behind
 * another turn" are the same wait from a chair, and splitting them here would
 * be describing the program rather than the delay.
 */
func (t Turn) Milestones() Milestones {
	m := Milestones{Spoken: t.Spoken, Model: t.Model, Tools: t.Tools, Words: t.Words}

	if t.Ended.IsZero() {
		return m
	}

	m.Ago = int(time.Since(t.Ended).Seconds())

	// Hearing: from the last word said to knowing what it was.
	m.Hearing = gap(t.Ended, t.Transcribed)

	// Thinking: from having the words to the model's first one back. On a
	// small machine this is nearly the whole turn.
	m.Thinking = gap(orElse(t.Transcribed, t.Ended), t.FirstToken)

	// Writing: the model's first word to a whole sentence, which is the
	// earliest anything can be said out loud.
	m.Writing = gap(t.FirstToken, t.FirstSentence)

	// Speaking: from having a sentence to sound in the room. This is the one
	// that is a fixed cost of the program rather than of the hardware.
	m.Speaking = gap(t.FirstSentence, t.FirstSound)

	m.ToFirst = gap(t.Ended, orElse(t.FirstSound, t.FirstToken))
	m.Altogether = gap(t.Ended, t.Finished)

	return m
}

func gap(from, to time.Time) int {
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return 0
	}

	return int(to.Sub(from) / time.Millisecond)
}

/*
 * orElse is the first instant, or the second when it never happened.
 *
 * Deliberately not "the earlier of the two", which is what this was and is
 * wrong in both places it is used. A spoken turn reaches its first sound a
 * second or two after the model's first word, so taking the earlier reported
 * every voice answer as ending when the text began — and the whole point of
 * measuring is that the gap between those two is a cost of this program.
 */
func orElse(first, fallback time.Time) time.Time {
	if first.IsZero() {
		return fallback
	}

	return first
}

var (
	mu      sync.Mutex
	turns   []Turn
	current *Turn
)

/*
 * Begin starts timing a turn at the moment somebody stopped talking.
 *
 * A turn already in progress is abandoned rather than finished. That is not
 * tidiness: it is what interrupting looks like from here, and an interrupted
 * turn's timings describe how long somebody was willing to wait, not how long
 * the machine took.
 */
func Begin(spoken bool, ended time.Time) {
	mu.Lock()
	defer mu.Unlock()

	if ended.IsZero() {
		ended = time.Now()
	}

	current = &Turn{Ended: ended, Spoken: spoken}
}

// Mark records a stage on the turn in progress, the first time it happens.
//
// First time only, because the stages repeat: a turn that calls two tools
// reaches "first token" three times, and the wait being measured is the one
// before the first of them.
func Mark(set func(*Turn)) {
	mu.Lock()
	defer mu.Unlock()

	if current == nil {
		return
	}

	set(current)
}

// Transcribed records that the words are known.
func Transcribed(words int) {
	Mark(func(t *Turn) {
		if t.Transcribed.IsZero() {
			t.Transcribed = time.Now()
			t.Words = words
		}
	})
}

// Asked records that the model has been given the turn, with which model and
// whether it was offered tools.
func Asked(model string, tools bool) {
	Mark(func(t *Turn) {
		if t.Asked.IsZero() {
			t.Asked = time.Now()
			t.Model = model
			t.Tools = tools
		}
	})
}

// FirstToken records the model's first word back.
func FirstToken() {
	Mark(func(t *Turn) {
		if t.FirstToken.IsZero() {
			t.FirstToken = time.Now()
		}
	})
}

// FirstSentence records that there is enough to say out loud.
func FirstSentence() {
	Mark(func(t *Turn) {
		if t.FirstSentence.IsZero() {
			t.FirstSentence = time.Now()
		}
	})
}

// FirstSound records sound reaching the room.
func FirstSound() {
	Mark(func(t *Turn) {
		if t.FirstSound.IsZero() {
			t.FirstSound = time.Now()
		}
	})
}

// Finish closes the turn and keeps it.
func Finish() {
	mu.Lock()
	defer mu.Unlock()

	if current == nil {
		return
	}

	current.Finished = time.Now()

	turns = append(turns, *current)
	if len(turns) > Kept {
		turns = turns[len(turns)-Kept:]
	}

	current = nil
}

// Recent is the turns that have finished, newest first.
func Recent() []Turn {
	mu.Lock()
	defer mu.Unlock()

	out := make([]Turn, 0, len(turns))

	for i := len(turns) - 1; i >= 0; i-- {
		out = append(out, turns[i])
	}

	return out
}

/*
 * Typical is the middle of the recent spoken turns, which is the number worth
 * quoting.
 *
 * The median rather than the mean, and only spoken turns. One turn that waited
 * on a tool, or on a model being loaded from the disk, moves an average by
 * enough to hide everything else; and a typed turn has no microphone and no
 * voice in it, so averaging the two together describes neither.
 */
func Typical() (Milestones, int) {
	all := Recent()

	var spoken []Milestones

	for _, t := range all {
		if !t.Spoken {
			continue
		}

		m := t.Milestones()

		if m.ToFirst > 0 {
			spoken = append(spoken, m)
		}
	}

	if len(spoken) == 0 {
		return Milestones{}, 0
	}

	return middle(spoken), len(spoken)
}

/*
 * middle takes the median of each stage separately.
 *
 * Which does not add up to the median total, and that is deliberate: the
 * question each stage answers is "how long does this part usually take", and
 * picking one representative turn would answer a different question with a
 * number that happened to belong to a real turn.
 */
func middle(of []Milestones) Milestones {
	pick := func(get func(Milestones) int) int {
		values := make([]int, 0, len(of))

		for _, m := range of {
			values = append(values, get(m))
		}

		return median(values)
	}

	return Milestones{
		Hearing:    pick(func(m Milestones) int { return m.Hearing }),
		Thinking:   pick(func(m Milestones) int { return m.Thinking }),
		Writing:    pick(func(m Milestones) int { return m.Writing }),
		Speaking:   pick(func(m Milestones) int { return m.Speaking }),
		ToFirst:    pick(func(m Milestones) int { return m.ToFirst }),
		Altogether: pick(func(m Milestones) int { return m.Altogether }),
		Spoken:     true,
		Words:      pick(func(m Milestones) int { return m.Words }),
	}
}

func median(values []int) int {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]int(nil), values...)

	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	return sorted[len(sorted)/2]
}

// Forget drops everything remembered. Used when a conversation is cleared, so
// the timings do not outlive the thing they describe.
func Forget() {
	mu.Lock()
	defer mu.Unlock()

	turns, current = nil, nil
}
