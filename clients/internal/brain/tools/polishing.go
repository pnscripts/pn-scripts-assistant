package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/polish"
)

/*
 * Polishing is a tool whose call carries words that go out in its user's
 * name, and that gives them the last look (package polish) before anybody is
 * asked about them.
 *
 * Called by the loop before the rules are checked and before the question is
 * shown, so the person approving reads the text as it will go — polished —
 * with what was changed listed above it, and the rules are judged on those
 * same words. What it returns replaces the call's arguments.
 */
type Polishing interface {
	Polish(args json.RawMessage) (json.RawMessage, polish.Result)
}

// PolishNote is what the question shows above a polished text: what was put
// right, and what is left for the writer.
func PolishNote(r polish.Result) string {
	var fixed, open []string

	for _, f := range r.Findings {
		line := polishLine(f)

		if f.Fixed {
			fixed = append(fixed, line)
		} else {
			open = append(open, line)
		}
	}

	var b strings.Builder

	if len(fixed) > 0 {
		b.WriteString("Polished before you see it:\n")

		for _, l := range fixed {
			b.WriteString("  - " + l + "\n")
		}
	}

	if len(open) > 0 {
		b.WriteString("Worth a look before it goes:\n")

		for _, l := range open {
			b.WriteString("  - " + l + "\n")
		}
	}

	return b.String()
}

func polishLine(f polish.Finding) string {
	var what string

	switch f.Key {
	case polish.AlreadyVisible:
		what = "taken out, the reader already sees it"
	case polish.Filler:
		what = "taken out, it says nothing to the reader"
	case polish.Repeated:
		what = "taken out, it was said twice"
	case polish.Slip:
		what = "a slip"
	case polish.Placeholder:
		what = "left to fill in; it cannot go out like this"
	case polish.MachineVoice:
		what = "sounds like a machine talking about itself"
	case polish.WrongWeekday:
		what = "the weekday does not match the date"
	case polish.Empty:
		return "there is no text"
	}

	line := fmt.Sprintf("%q: %s", f.Found, what)

	if f.Hint != "" && f.Key != polish.Slip {
		line += " (" + f.Hint + "?)"
	} else if f.Hint != "" {
		line += fmt.Sprintf(" → %q", f.Hint)
	}

	return line
}

// senderName is the name the reader sees beside a message: the display
// name of the From address ("Anna Petrova <anna@…>"), or else the name of
// the person the assistant works for.
func senderName(from string, owner func() string) string {
	from = strings.TrimSpace(from)

	if i := strings.Index(from, "<"); i > 0 {
		if name := strings.Trim(strings.TrimSpace(from[:i]), `"`); name != "" {
			return name
		}
	}

	if owner != nil {
		return strings.TrimSpace(owner())
	}

	return ""
}

// Polish gives the message the last look and puts the polished words in
// the call.
func (t SendEmail) Polish(raw json.RawMessage) (json.RawMessage, polish.Result) {
	var a sendArgs

	if json.Unmarshal(raw, &a) != nil {
		return raw, polish.Result{}
	}

	address := ""
	if t.Account != nil {
		address = t.Account().From
	}

	from := senderName(address, t.Owner)

	r := polish.Polish(polish.Draft{Text: a.Body, Subject: a.Subject, From: from}, time.Now())

	if !r.Changed {
		return raw, r
	}

	a.Body = r.Text

	out, err := json.Marshal(a)
	if err != nil {
		return raw, r
	}

	return out, r
}

/*
 * Polish gives a waiting draft the last look. The polished words travel in
 * the call — the question shows them, the pin keeps them — and are written
 * into the draft only when it is sent after the yes; a draft that is not
 * sent keeps its own words.
 */
func (t SendDraft) Polish(raw json.RawMessage) (json.RawMessage, polish.Result) {
	var a draftArgs

	if json.Unmarshal(raw, &a) != nil || t.Desk.Drafts == nil {
		return raw, polish.Result{}
	}

	dr, err := t.Desk.Drafts.Draft(a.ID)
	if err != nil {
		return raw, polish.Result{}
	}

	address := ""
	if t.Desk.Account != nil {
		address = t.Desk.Account().From
	}

	r := polish.Polish(polish.Draft{Text: dr.Body, Subject: dr.Subject, From: senderName(address, t.Desk.Owner)}, time.Now())

	if !r.Changed || r.Text == "" {
		return raw, r
	}

	out, err := json.Marshal(map[string]any{"id": a.ID, "polished": r.Text})
	if err != nil {
		return raw, r
	}

	return out, r
}

// SentAs is what a sent message is shown as afterwards: where it went, from
// which address, when, and the words exactly as they went.
func SentAs(said, from string, to []string, subject, body string, at time.Time) string {
	var b strings.Builder

	b.WriteString(said + "\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\n")

	if from != "" {
		b.WriteString("From: " + from + "\n")
	}

	b.WriteString("At: " + at.Local().Format("2 Jan 2006 15:04") + "\n")
	b.WriteString("Subject: " + subject + "\n\n")
	b.WriteString(body)

	return b.String()
}

/*
 * PolishText gives any text the last look before it goes out — a post, a
 * comment, a message typed into another program — and returns it polished
 * with what changed. Safe: it only reads and writes words back to the
 * conversation; whatever then sends them is asked about as always.
 */
type PolishText struct{}

func (PolishText) Name() string { return "polish_text" }

func (PolishText) Description() string {
	return "Give a text the last look before it goes out in the user's name (a post, a comment, a message, " +
		"a reply): takes out what the reader already sees (who is writing), filler and repetition, fixes sure " +
		"spelling slips in English or Bulgarian, and points at placeholders, wrong weekdays and a machine " +
		"talking about itself. Use before posting or sending any text that the send tools do not polish."
}

func (PolishText) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "The text, in full."},
			"from": {"type": "string", "description": "The name the reader sees beside it, if known."},
			"subject": {"type": "string", "description": "A title or subject the reader sees above it, if any."}
		},
		"required": ["text"]
	}`)
}

func (PolishText) Risk() Risk { return Safe }

func (PolishText) Cues() []string {
	return []string{"polish", "изглади", "публикация", "коментар"}
}

func (PolishText) Summarize(json.RawMessage) string { return "Polish a text before it goes out" }

func (PolishText) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Text    string `json:"text"`
		From    string `json:"from"`
		Subject string `json:"subject"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	r := polish.Polish(polish.Draft{Text: a.Text, From: a.From, Subject: a.Subject}, time.Now())

	var b strings.Builder

	if note := PolishNote(r); note != "" {
		b.WriteString(note + "\n")
	} else {
		b.WriteString("Nothing to change.\n\n")
	}

	if r.Blocking {
		b.WriteString("Do not send or post it until what is listed above is filled in.\n\n")
	}

	b.WriteString("The text as it should go:\n" + r.Text)

	return b.String(), nil
}

// blockingWords says what stops a text going out.
func blockingWords(r polish.Result) string {
	var found []string

	for _, f := range r.Findings {
		switch f.Key {
		case polish.Placeholder:
			found = append(found, fmt.Sprintf("%q", f.Found))
		case polish.Empty:
			return "there is no text"
		}
	}

	return "it still has " + strings.Join(found, ", ") + " to fill in"
}
