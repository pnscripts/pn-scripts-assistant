package brain

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// Greeting is what the brain says when it opens, before being asked anything.
//
// Composed from what it already knows rather than generated. Asking the model
// for a greeting would cost the better part of a minute on this hardware before
// it said hello, which is the opposite of the point — and it would produce
// "Hello! How can I assist you today?", which says nothing a person could not
// have guessed.
//
// What is worth saying on opening is what has changed and what is waiting. A
// greeting that mentions three lessons needing review is doing a job; one that
// announces itself is furniture.
type Greeting struct {
	Text  string `json:"text"`
	Spoke bool   `json:"-"`
}

// Greet composes an opening line.
/*
 * NoteCutShort records how many turns the last run left unfinished.
 *
 * Told to the brain rather than worked out by it, because only the startup
 * repair knows: by the time anything else looks, the conversations have been
 * closed off and are indistinguishable from ones that ended properly.
 */
func (b *Brain) NoteCutShort(n int) {
	b.mu.Lock()
	b.cutShort = n
	b.mu.Unlock()
}

func (b *Brain) Greet() Greeting {
	var parts []string

	parts = append(parts, b.timeOfDay())

	/*
	 * What the last run was in the middle of, if it was in the middle of
	 * something.
	 *
	 * Being told the program stopped during a question is the difference
	 * between an assistant that lost your answer and one that ignored it. It
	 * is also the first thing worth knowing on opening it again, which is why
	 * it comes before what is waiting.
	 */
	if cut := b.cutShortLine(); cut != "" {
		parts = append(parts, cut)
	}

	/*
	 * Where the conversation stands, before what is queued.
	 *
	 * The greeting opened with the review queue — "there is one thing I would
	 * like to remember" — which is housekeeping, and housekeeping is not what
	 * somebody wants first on coming back to a conversation they were part way
	 * through. What they were talking about is.
	 */
	if last := b.lastTopicLine(); last != "" {
		parts = append(parts, last)
	}

	if waiting := b.waitingLine(); waiting != "" {
		parts = append(parts, waiting)
	} else if known := b.knowledgeLine(); known != "" {
		parts = append(parts, known)
	}

	if warning := b.storageLine(); warning != "" {
		parts = append(parts, warning)
	}

	return Greeting{Text: strings.Join(parts, " ")}
}

/*
 * cutShortLine says the last run ended in the middle of something.
 *
 * Plural handled properly: "1 questions" in the first sentence somebody hears
 * is a small thing that makes everything after it sound automatic.
 */
func (b *Brain) cutShortLine() string {
	b.mu.Lock()
	n := b.cutShort
	b.mu.Unlock()

	switch {
	case n <= 0:
		return ""
	case n == 1:
		return "Last time I stopped part way through a question, so it went unanswered."
	default:
		return fmt.Sprintf(
			"Last time I stopped part way through %d questions, so they went unanswered.", n)
	}
}

// timeOfDay opens the way a person would.
//
// Varied deliberately: the same sentence every single time stops being a
// greeting and becomes a startup message.
func (b *Brain) timeOfDay() string {
	owner := b.Cfg.Owner

	hour := time.Now().Hour()

	var greetings []string

	switch {
	case hour < 5:
		greetings = []string{"Still up?", "Late one."}
	case hour < 12:
		greetings = []string{"Morning.", "Good morning."}
	case hour < 18:
		greetings = []string{"Afternoon.", "Good afternoon."}
	default:
		greetings = []string{"Evening.", "Good evening."}
	}

	line := greetings[rand.Intn(len(greetings))]

	/*
	 * The name goes inside the sentence, before whatever ends it.
	 *
	 * Only the full stop was stripped, so the late-night greetings kept their
	 * question mark and came out as "Still up?, Petar." — punctuation in the
	 * middle of a question, in the first three words anybody hears, which
	 * makes everything after it sound machine-generated.
	 */
	if owner != "" && rand.Intn(2) == 0 {
		ends := "."

		if last := line[len(line)-1:]; last == "?" || last == "!" || last == "." {
			ends = last
			line = line[:len(line)-1]
		}

		line += ", " + owner + ends
	}

	return line
}

/*
 * lastTopicLine says what the last conversation was about.
 *
 * Named by its opening line, which is what a conversation is actually
 * remembered by. Skipped when it was only just said — coming straight back to
 * a window that is still open does not need to be told what is on the screen.
 */
func (b *Brain) lastTopicLine() string {
	recent, err := b.DB.RecentConversations(1)
	if err != nil || len(recent) == 0 {
		return ""
	}

	last := recent[0]

	topic := last.Title
	if strings.TrimSpace(topic) == "" {
		topic = last.Opening
	}

	return topicLine(topic, time.Since(last.When))
}

/*
 * topicLine decides whether to mention a conversation, and how to say it.
 *
 * Separated from reading the database so both halves can be checked: whether a
 * conversation from a moment ago is worth repeating back, and whether a long
 * opening line is cut somewhere a person can still read.
 */
func topicLine(topic string, age time.Duration) string {
	// Still on screen: saying it back is noise, not orientation.
	if age < TooRecentToMention {
		return ""
	}

	topic = strings.TrimSpace(topic)
	if topic == "" {
		return ""
	}

	const mostOfIt = 70

	if len(topic) > mostOfIt {
		topic = strings.TrimSpace(topic[:mostOfIt]) + "…"
	}

	return fmt.Sprintf("Last time we were on: %s.", topic)
}

/*
 * TooRecentToMention is how fresh a conversation has to be for repeating it
 * back to be pointless rather than useful.
 */
const TooRecentToMention = 2 * time.Minute

// waitingLine mentions what needs the owner, which is the most useful thing to
// open with when there is any.
func (b *Brain) waitingLine() string {
	pending, err := b.DB.CountPendingLessons()
	if err != nil || pending == 0 {
		return ""
	}

	approvals, _ := b.DB.PendingInvocations()

	switch {
	case len(approvals) > 0 && pending > 0:
		return fmt.Sprintf("%s and %s are waiting for you.",
			count(pending, "thing I want to remember", "things I want to remember"),
			count(len(approvals), "action", "actions"))
	case pending == 1:
		return "There is one thing I would like to remember, when you have a moment."
	default:
		return fmt.Sprintf("There are %d things I would like to remember, when you have a moment.", pending)
	}
}

// knowledgeLine is the fallback when nothing needs attention: how much the
// brain has, which is the honest answer to what it is for.
func (b *Brain) knowledgeLine() string {
	facts, err := b.DB.CountFacts()
	if err != nil || facts == 0 {
		return "I do not know anything about your work yet — point me at a folder and I will learn it."
	}

	return fmt.Sprintf("I know %d things about your work. Ask me anything.", facts)
}

// storageLine warns while there is still room to act on it.
func (b *Brain) storageLine() string {
	report := b.Storage()

	switch report.Level {
	case "critical":
		return "This drive is nearly full, which will stop me learning."
	case "warn":
		return "This drive is filling up."
	default:
		return ""
	}
}

func count(n int, singular, plural string) string {
	if n == 1 {
		return "one " + singular
	}

	return fmt.Sprintf("%d %s", n, plural)
}
