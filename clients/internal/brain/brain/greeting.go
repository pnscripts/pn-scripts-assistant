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
func (b *Brain) Greet() Greeting {
	var parts []string

	parts = append(parts, b.timeOfDay())

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

	if owner != "" && rand.Intn(2) == 0 {
		line = strings.TrimSuffix(line, ".") + ", " + owner + "."
	}

	return line
}

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
