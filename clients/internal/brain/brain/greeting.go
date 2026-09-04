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

	/*
	 * And which conversation this is.
	 *
	 * Opening the program used to drop you back into the last conversation
	 * whatever its age, silently — so a sentence typed on Tuesday morning
	 * joined a thread from Friday night, and nothing on the screen said which
	 * one you were in. Either is a reasonable thing to do; doing it without
	 * saying so is not.
	 */
	CarryOn bool `json:"carry_on"`

	// Last is the conversation before this one, whether it is being carried on
	// or left where it is, so the interface can name it either way.
	Last *LastTalk `json:"last,omitempty"`
}

// LastTalk is enough of the previous conversation to say which one it was.
type LastTalk struct {
	ID    int64     `json:"id"`
	Topic string    `json:"topic"`
	When  time.Time `json:"when"`
	Turns int       `json:"turns"`
}

/*
 * CarryOnWithin is how long a conversation stays the conversation.
 *
 * Inside it, restarting the program is an interruption — the model reloading,
 * a crash, a rebuild — and carrying on is what somebody means to happen.
 * Outside it, they have been away and come back to something else, and
 * appending to Friday's thread makes the history useless as a record of what
 * was discussed when.
 */
const CarryOnWithin = 3 * time.Hour

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

	last := b.lastConversation()

	/*
	 * Coming back after a few minutes is not an arrival.
	 *
	 * The greeting is written for opening the program after a while: good
	 * evening, here is where we were, here is what is waiting, I know 1042
	 * things about your work, ask me anything. Delivered every launch, it
	 * turns a conversation into a machine rebooting at somebody — and during
	 * an afternoon of restarts it was said eight times in an hour, word for
	 * word, to a person who had never left the room.
	 *
	 * So when the last thing said was minutes ago, only what has actually
	 * changed is worth saying, and often nothing has. You do not reintroduce
	 * yourself to somebody you were talking to five minutes ago.
	 */
	backAlready := last != nil && time.Since(last.When) < JustCameBack

	if !backAlready {
		parts = append(parts, b.timeOfDay())
	}

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
	 * And whether it has been carried somewhere new, which comes before
	 * everything else about the day.
	 *
	 * It is the one condition where the brain looks completely normal and a
	 * large part of what it knows cannot be acted on: every path it learned
	 * names a place that does not exist on this machine.
	 */
	if moved := b.journeyLine(); moved != "" {
		parts = append(parts, moved)
	}

	/*
	 * Where the conversation stands, before what is queued.
	 *
	 * The greeting opened with the review queue — "there is one thing I would
	 * like to remember" — which is housekeeping, and housekeeping is not what
	 * somebody wants first on coming back to a conversation they were part way
	 * through. What they were talking about is.
	 */
	if line := lastTopicLine(last, b.carryingOn(last)); line != "" {
		parts = append(parts, line)
	}

	if waiting := b.waitingLine(); waiting != "" {
		parts = append(parts, waiting)
	} else if known := b.knowledgeLine(); known != "" && !backAlready {
		/*
		 * "I know 1042 things about your work. Ask me anything."
		 *
		 * The most robotic sentence in the program, and the one that repeats
		 * unchanged forever. It is worth saying to somebody arriving; said to
		 * somebody who has been here all along it is a machine reciting its
		 * own specification.
		 */
		parts = append(parts, known)
	}

	if warning := b.storageLine(); warning != "" {
		parts = append(parts, warning)
	}

	return Greeting{
		Text:    strings.Join(parts, " "),
		CarryOn: b.carryingOn(last),
		Last:    last,
	}
}

// lastConversation is what was being talked about before this run, or nothing
// on a brain that has never been talked to.
func (b *Brain) lastConversation() *LastTalk {
	recent, err := b.DB.RecentConversations(1)
	if err != nil || len(recent) == 0 {
		return nil
	}

	last := recent[0]

	topic := strings.TrimSpace(last.Title)
	if topic == "" {
		topic = strings.TrimSpace(last.Opening)
	}

	return &LastTalk{ID: last.ID, Topic: topic, When: last.When, Turns: last.Turns}
}

// carryingOn decides whether this is the same conversation or the next one.
func (b *Brain) carryingOn(last *LastTalk) bool {
	return last != nil && time.Since(last.When) < CarryOnWithin
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
 * lastTopicLine says what the last conversation was, and which one this is.
 *
 * Named by its opening line, which is what a conversation is actually
 * remembered by. The two cases are worded differently on purpose, because they
 * are different situations: carrying on where you were is being handed back
 * something you already have, and starting fresh is being told what the
 * previous thing was before it goes out of sight.
 *
 * Skipped when it was only just said — coming straight back to a window that
 * is still open does not need to be told what is on the screen.
 */
func lastTopicLine(last *LastTalk, carryOn bool) string {
	if last == nil {
		return ""
	}

	age := time.Since(last.When)

	// Still on screen: saying it back is noise, not orientation.
	if age < TooRecentToMention {
		return ""
	}

	topic := strings.TrimSpace(last.Topic)
	if topic == "" {
		return ""
	}

	const mostOfIt = 70

	if len(topic) > mostOfIt {
		topic = strings.TrimSpace(topic[:mostOfIt]) + "…"
	}

	if carryOn {
		return fmt.Sprintf("Carrying on from where we were, %s: %s.", howLongAgo(age), topic)
	}

	return fmt.Sprintf("This is a new conversation. The last one was %s: %s.",
		howLongAgo(age), topic)
}

/*
 * howLongAgo is a gap in the words a person would use.
 *
 * Rounded, and deliberately so. "Fourteen minutes ago" is what somebody wants
 * to hear; a duration printed to the second is a thing to decode, and this
 * sentence is usually being listened to rather than read.
 */
func howLongAgo(age time.Duration) string {
	switch {
	case age < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(age.Minutes()))

	case age < 2*time.Hour:
		return "an hour ago"

	case age < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(age.Hours()))

	case age < 48*time.Hour:
		return "yesterday"
	}

	return fmt.Sprintf("%d days ago", int(age.Hours()/24))
}

/*
 * TooRecentToMention is how fresh a conversation has to be for repeating it
 * back to be pointless rather than useful.
 */
const TooRecentToMention = 2 * time.Minute

/*
 * JustCameBack is how recently somebody has to have been here for the greeting
 * to stop being a greeting.
 *
 * Half an hour. Long enough to cover stepping away for a coffee or the program
 * being restarted while it is being worked on; short enough that coming back
 * after an evening still gets a proper hello.
 */
const JustCameBack = 30 * time.Minute

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
