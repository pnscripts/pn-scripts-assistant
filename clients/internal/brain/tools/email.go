package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/mail"
)

/*
 * The owner's mail.
 *
 * Registered only when a mailbox has been set up, like the smart-home tools:
 * a tool the model can see is a tool it will try, and an assistant that keeps
 * offering to read mail it cannot reach is worse than one that does not offer.
 *
 * Reading is Safe and sending is Mutating, which is the same line drawn
 * everywhere else here. Reading looks at something that already exists.
 * Sending puts words in its owner's name in front of another person, and it
 * cannot be taken back — that is the clearest case for the queue there is.
 */

// ReadEmail lists recent messages.
type ReadEmail struct{ Account func() mail.Account }

func (ReadEmail) Name() string { return "read_email" }

func (ReadEmail) Description() string {
	return "List the most recent emails in the owner's mailbox, newest first, " +
		"with who they are from and what they are about. Give a number to read " +
		"one of them in full."
}

func (ReadEmail) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"count": {"type": "integer", "description": "How many to list, default 10"},
			"number": {"type": "integer", "description": "Read this one in full, from a previous listing"},
			"folder": {"type": "string", "description": "Which mailbox, default INBOX"}
		}
	}`)
}

func (ReadEmail) Risk() Risk { return Safe }

func (ReadEmail) Summarize(raw json.RawMessage) string {
	var a emailArgs

	_ = json.Unmarshal(raw, &a)

	if a.Number > 0 {
		return fmt.Sprintf("Read email %d", a.Number)
	}

	return "List your recent email"
}

type emailArgs struct {
	Count  int    `json:"count"`
	Number int    `json:"number"`
	Folder string `json:"folder"`
}

func (t ReadEmail) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	if t.Account == nil {
		return "", fmt.Errorf("no mailbox is set up")
	}

	var a emailArgs

	_ = json.Unmarshal(raw, &a)

	account := t.Account()

	if a.Number > 0 {
		msg, err := mail.Read(account, a.Folder, a.Number)
		if err != nil {
			return "", err
		}

		body := msg.Body
		if len(body) > MaxReadBytes {
			body = body[:MaxReadBytes] + "\n\n[truncated]"
		}

		return fmt.Sprintf("From: %s\nDate: %s\nSubject: %s\n\n%s",
			msg.From, msg.Date, msg.Subject, body), nil
	}

	count := a.Count
	if count <= 0 || count > 50 {
		count = 10
	}

	messages, err := mail.Recent(account, a.Folder, count)
	if err != nil {
		return "", err
	}

	if len(messages) == 0 {
		return "There is nothing in the mailbox.", nil
	}

	var b strings.Builder

	for _, m := range messages {
		fmt.Fprintf(&b, "%d. %s — %s (%s)\n", m.Number, m.From, m.Subject, m.Date)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// SendEmail sends one.
type SendEmail struct{ Account func() mail.Account }

func (SendEmail) Name() string { return "send_email" }

func (SendEmail) Description() string {
	return "Send an email from the owner's address. Always says who it is to, " +
		"what the subject is and what it says, and waits for the owner to agree."
}

func (SendEmail) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"to": {"type": "string", "description": "Who it is for; several separated by commas"},
			"subject": {"type": "string", "description": "The subject line"},
			"body": {"type": "string", "description": "What the message says"}
		},
		"required": ["to", "subject", "body"]
	}`)
}

// Mutating, and of everything here this is the one that most deserves it:
// a sent message is in front of another person and cannot be recalled.
func (SendEmail) Risk() Risk { return Mutating }

/*
 * Summarize shows the whole message, not a description of it.
 *
 * The person approving this is agreeing to words going out in their name. A
 * summary that says "send an email to Anna" hides the only part that matters,
 * and an approval given to a summary like that is worth nothing.
 */
func (SendEmail) Summarize(raw json.RawMessage) string {
	var a sendArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Send to %s\nSubject: %s\n\n%s", a.To, a.Subject, a.Body)
}

type sendArgs struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (t SendEmail) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	if t.Account == nil {
		return "", fmt.Errorf("no mailbox is set up")
	}

	var a sendArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	var to []string

	for _, address := range strings.Split(a.To, ",") {
		if address = strings.TrimSpace(address); address != "" {
			to = append(to, address)
		}
	}

	return mail.Send(t.Account(), to, a.Subject, a.Body)
}
