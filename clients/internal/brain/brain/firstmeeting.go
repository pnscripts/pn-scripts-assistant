package brain

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/wording"
)

/*
 * The first meeting is a conversation, not a form.
 *
 * It used to be a card over the window — "Before we start", two fields and a
 * Begin button — followed by a composed greeting ("Good evening.") and a list
 * of what the program can do. Its owner's word for it was cliché: a
 * questionnaire, then a recital. What he asked for is a real conversation,
 * so the card is gone and so is every fixed line on the way in.
 *
 * What setup needs from a first meeting — what to call him, what he calls the
 * assistant, which language he speaks — is taken from what he says, turn by
 * turn, and written to the settings the moment it is said. The model is told
 * it has just met him and is asked to talk, not to interview: react to what
 * he says, ask one thing only when it needs it. The opener, when there is a
 * model to write one, is the model's own; when there is none, nothing is said
 * until he speaks, rather than something canned.
 */

// FirstMeeting is whether this is somebody the assistant has not met: no
// settings written yet, or no name for its owner and no conversation on record.
func (b *Brain) FirstMeeting() bool {
	b.mu.Lock()
	owner, isNew := strings.TrimSpace(b.Cfg.Owner), b.Cfg.New
	b.mu.Unlock()

	return isNew || (owner == "" && b.lastConversation() == nil)
}

// stillGettingAcquainted is the wider question the conversation asks on every
// turn: is there anything setup still needs to hear?
func (b *Brain) stillGettingAcquainted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return strings.TrimSpace(b.Cfg.Owner) == "" || b.Cfg.New
}

// firstMeetingNote is what the model is told while it does not know who it
// is talking to. Instructions about how to talk, not lines to say.
const firstMeetingNote = `You have only just met the person you are talking to, and you do not
know their name yet. Talk with them the way a person would on meeting
someone: react to what they actually said, and carry on from it. No
greeting formula, no "how can I help you", no list of what you can do, no
exclamation marks. If you need something from them — what to call them,
or what they would like to call you — ask for that one thing, once, when
it fits; never a list of questions. Answer in the language they write in.
When they tell you their name, use it.`

/*
 * Learned is what one turn of the first meeting settled.
 */
type Learned struct {
	Owner    string
	Name     string
	Language string
}

func (l Learned) empty() bool { return l.Owner == "" && l.Name == "" && l.Language == "" }

/*
 * What people say when they give a name, in the two languages its owner
 * speaks. Deliberately narrow: a name is taken only from a sentence that is
 * plainly giving one, because writing a wrong name into the settings is worse
 * than asking again.
 */
var (
	ownerSaid = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bmy name(?:'s| is)\s+(\p{L}[\p{L}'-]*)`),
		regexp.MustCompile(`(?i)\bcall me\s+(\p{L}[\p{L}'-]*)`),
		regexp.MustCompile(`(?:^|[\s,.])[Ii](?:'m| am)\s+(\p{Lu}[\p{L}'-]*)`),
		regexp.MustCompile(`(?i)(?:^|[\s,.])(?:казвам се|името ми е|викай ми|наричай ме|казвай ми)\s+(\p{L}[\p{L}'-]*)`),
		regexp.MustCompile(`(?:^|[\s,.])(?:[Аа]з съм|[Тт]ук е)\s+(\p{Lu}[\p{L}'-]*)`),
	}

	nameGiven = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:your name (?:is|will be)|call yourself|i'll call you|i will call you|you(?:'re| are) called)\s+(\p{L}[\p{L}'-]*)`),
		regexp.MustCompile(`(?i)(?:^|[\s,.])(?:ще се казваш|ще те наричам|ще те викам|казвай се|наречи се|името ти е|името ти ще е)\s+(\p{L}[\p{L}'-]*)`),
	}

	// Words the patterns above can catch that are not names.
	notAName = map[string]bool{
		"a": true, "an": true, "the": true, "not": true, "just": true, "here": true, "fine": true,
		"good": true, "ok": true, "okay": true, "sorry": true, "back": true, "busy": true, "tired": true,
		"trying": true, "going": true, "looking": true, "working": true, "sure": true, "done": true,
		"тук": true, "добре": true, "зает": true, "заета": true, "готов": true, "готова": true,
		"като": true, "просто": true, "човек": true,
	}
)

// LearnFromWords reads what one message says about who is who and which
// language is spoken. Nothing is guessed: what is not plainly said is empty.
func LearnFromWords(message string) Learned {
	var l Learned

	if name := firstMatch(nameGiven, message); name != "" {
		l.Name = name
	}

	// Both can be said in one breath — "I'm Alex, and you're Ariel" — but
	// never as the same word.
	if owner := firstMatch(ownerSaid, message); owner != l.Name {
		l.Owner = owner
	}

	letters, cyrillic := 0, 0

	for _, r := range message {
		if unicode.IsLetter(r) {
			letters++

			if unicode.Is(unicode.Cyrillic, r) {
				cyrillic++
			}
		}
	}

	if letters >= 3 && cyrillic*2 > letters {
		l.Language = "bg"
	}

	return l
}

func firstMatch(patterns []*regexp.Regexp, message string) string {
	for _, re := range patterns {
		m := re.FindStringSubmatch(message)
		if len(m) < 2 {
			continue
		}

		name := strings.Trim(m[1], "'-")

		if name == "" || notAName[strings.ToLower(name)] {
			continue
		}

		runes := []rune(name)

		return string(unicode.ToUpper(runes[0])) + string(runes[1:])
	}

	return ""
}

/*
 * learnFromFirstMeeting writes down what a message settled, while there is
 * still something to settle. A name already given is not replaced by a later
 * sentence that happens to look like one; a language already chosen is left.
 */
func (b *Brain) learnFromFirstMeeting(message string) Learned {
	if !b.stillGettingAcquainted() {
		return Learned{}
	}

	heard := LearnFromWords(message)

	b.mu.Lock()

	var took Learned

	if heard.Owner != "" && strings.TrimSpace(b.Cfg.Owner) == "" {
		b.Cfg.Owner, took.Owner = heard.Owner, heard.Owner
	}

	if heard.Name != "" {
		b.Cfg.Name, took.Name = heard.Name, heard.Name
	}

	if heard.Language != "" && strings.TrimSpace(b.Cfg.Language) == "" {
		b.Cfg.Language, took.Language = heard.Language, heard.Language
	}

	if took.empty() {
		b.mu.Unlock()

		return took
	}

	b.Cfg.New = false
	cfg := b.Cfg
	b.mu.Unlock()

	if err := cfg.Save(b.Root); err != nil {
		b.Log.Warn("what the first conversation settled could not be saved", "error", err)
	} else {
		b.Log.Info("learned from the first conversation",
			"owner", took.Owner != "", "name", took.Name, "language", took.Language)
	}

	return took
}

/*
 * firstWords is the opener on a first meeting, in the model's own words, or
 * nothing at all. There is no fallback sentence on purpose: when no model can
 * answer, the window waits for its owner to speak first, which is a better
 * start than a line nobody wrote for him.
 */
func (b *Brain) firstWords(ctx context.Context) string {
	provider, err := b.Router.Provider(b.Cfg.DefaultProvider)
	if err != nil {
		return ""
	}

	language := "the language of this machine's owner; English if nothing says otherwise"
	if strings.EqualFold(strings.TrimSpace(b.Cfg.Language), "bg") {
		language = "Bulgarian"
	}

	return oneLine(wording.Say(ctx, provider, b.modelRoles().Talk, wording.Want{
		Brief: "the first thing you say to somebody who has just opened you on their own " +
			"computer and whom you have never met. Not a greeting formula, not an offer " +
			"of help, no list of what you can do, no exclamation marks. One or two plain " +
			"sentences, and at most one question — the thing you actually need, which " +
			"is what to call them",
		Facts: map[string]any{
			"you have never met them":    true,
			"you do not know their name": true,
			"what you are":               "an assistant that lives on this computer and remembers what it is told",
			"language":                   language,
		},
		Plain:  "",
		Most:   240,
		Within: wording.AtLeisure,
	}))
}
