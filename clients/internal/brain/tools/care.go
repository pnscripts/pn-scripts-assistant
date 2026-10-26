package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/care"
	"pn-scripts-assistant/internal/brain/proof"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Customer replies for a small business, written from what the business
 * itself has written down — its answer book and the documents in its
 * folder — and checked before its owner sees them (package care). Writing
 * a reply sends nothing: it is shown, or kept as a mail draft that waits for
 * its owner's yes like every other, and a draft with a question still in it
 * for the owner cannot be sent at all.
 */

// CareBook is where the answer book and the business's profile are kept.
type CareBook interface {
	CareAnswers() ([]store.CareAnswer, error)
	AddCareAnswer(question, answer string) (store.CareAnswer, error)
	ChangeCareAnswer(id int64, question, answer string) (store.CareAnswer, error)
	RemoveCareAnswer(id int64) error
	CareProfile() (map[string]string, error)
	SetCareProfile(values map[string]string) error
}

// CustomerCare is what the customer tools and the Customers page share.
type CustomerCare struct {
	Book CareBook

	// Ask has the model write something. Nil is no model.
	Ask func(ctx context.Context, system, user string) (string, string, error)

	// Desk keeps drafts, as for every other reply.
	Desk MailDesk

	// Root is the brain's folder, which a documents folder written as
	// "@brain/…" is inside.
	Root string
}

// ErrNoCareModel is a reply asked for with no model to write it.
var ErrNoCareModel = errors.New("no language model is available to write the reply")

// Profile is how the business wants to sound.
func (c CustomerCare) Profile() care.Profile {
	if c.Book == nil {
		return care.Profile{}
	}

	m, _ := c.Book.CareProfile()

	return care.Profile{Name: m["name"], Signature: m["signature"], Tone: m["tone"], Folder: m["folder"], Always: m["always"]}
}

// SetProfile keeps how the business wants to sound.
func (c CustomerCare) SetProfile(p care.Profile) error {
	if p.Tone != "" && p.Tone != "friendly" && p.Tone != "formal" {
		return errors.New("the tone must be friendly or formal")
	}

	if f := strings.TrimSpace(p.Folder); f != "" && !strings.HasPrefix(f, "@brain") && !filepath.IsAbs(f) {
		return errors.New("give the documents folder as a full path")
	}

	return c.Book.SetCareProfile(map[string]string{
		"name": p.Name, "signature": p.Signature, "tone": p.Tone, "folder": p.Folder, "always": p.Always,
	})
}

// Folder is the documents folder as a path on this machine.
func (c CustomerCare) Folder(p care.Profile) string {
	f := strings.TrimSpace(p.Folder)
	if rest, ok := strings.CutPrefix(f, "@brain"); ok && c.Root != "" {
		return filepath.Join(c.Root, strings.TrimLeft(rest, "/\\"))
	}

	return f
}

// docCache keeps what each document said, by its path and when it changed,
// so a folder of PDFs is not read again for every reply.
var docCache sync.Map // path → cachedDoc

type cachedDoc struct {
	mod  time.Time
	text string
}

// Docs reads the business's documents. One that cannot be read is left out.
func (c CustomerCare) Docs(ctx context.Context) []care.Doc {
	var out []care.Doc

	for _, path := range care.Files(c.Folder(c.Profile())) {
		if err := GuardSensitive(ctx, path); err != nil {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if v, ok := docCache.Load(path); ok && v.(cachedDoc).mod.Equal(info.ModTime()) {
			out = append(out, care.Doc{Path: path, Text: v.(cachedDoc).text})

			continue
		}

		text := ""

		if plainKinds[strings.ToLower(filepath.Ext(path))] {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			text = string(data)
			if ext := strings.ToLower(filepath.Ext(path)); ext == ".html" || ext == ".htm" {
				text = ReadableText(text)
			}
		} else {
			args, _ := json.Marshal(map[string]string{"path": path})

			got, err := ReadDocument{}.Execute(ctx, args)
			if err != nil {
				continue
			}

			text = got
		}

		docCache.Store(path, cachedDoc{info.ModTime(), text})
		out = append(out, care.Doc{Path: path, Text: text})
	}

	return out
}

func (c CustomerCare) answers() []care.Answer {
	if c.Book == nil {
		return nil
	}

	list, _ := c.Book.CareAnswers()

	out := make([]care.Answer, 0, len(list))
	for _, a := range list {
		out = append(out, care.Answer{ID: a.ID, Question: a.Question, Answer: a.Answer})
	}

	return out
}

// Written is a reply written and checked, and the model that wrote it.
type Written struct {
	care.Reply

	Model string       `json:"model,omitempty"`
	Draft *store.Draft `json:"draft,omitempty"`
}

// Reply writes a checked reply to a customer's message.
func (c CustomerCare) Reply(ctx context.Context, message, lang string) (Written, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return Written{}, errors.New("give the customer's message")
	}

	if len([]rune(message)) > 20000 {
		return Written{}, errors.New("the message is too long; give the part that needs an answer")
	}

	if c.Ask == nil {
		return Written{}, ErrNoCareModel
	}

	if lang != "en" && lang != "bg" {
		lang = proof.Language(message)
	}

	p := c.Profile()
	kind := care.KindOf(message)
	passages := care.Find(message, c.answers(), c.Docs(ctx), 6)

	text, model, err := c.Ask(ctx, care.ReplyBrief(p, kind, lang), care.ReplyInput(message, passages))
	if err != nil {
		return Written{}, err
	}

	return Written{Reply: care.Finish(text, message, passages, p, kind, lang), Model: model}, nil
}

// Keep files a written reply as a mail draft, waiting for its owner's yes.
func (c CustomerCare) Keep(ctx context.Context, answers, to, subject, body string) (store.Draft, string, error) {
	return c.Desk.Keep(ctx, answers, to, subject, body)
}

// careCues are the words of a business's customers. Not "reply" or "mail":
// those are the mail desk's, and most mail is not from a customer.
var careCues = []string{
	"customer", "client", "complain", "enquir", "inquir", "refund", "faq", "answer book", "my shop", "my business",
	"клиент", "оплак", "запитван", "рекламац", "магазин", "фирмат", "бизнес", "често задавани",
}

// CustomerReply writes a checked reply to a customer.
type CustomerReply struct{ With CustomerCare }

func (CustomerReply) Name() string   { return "customer_reply" }
func (CustomerReply) Cues() []string { return careCues }

// Safe: it sends nothing. With answers or to, the reply is kept as a draft
// that waits for its owner's yes like any other.
func (CustomerReply) Risk() Risk { return Safe }

func (CustomerReply) Description() string {
	return "Write a reply to a customer of the owner's business from the business's own answer book and " +
		"documents, in the customer's language, then check it: says what kind of message it is, which of the " +
		"business's words it used, any question the business's words cannot answer (left as [[ASK: …]] for the " +
		"owner), any promise of money or time nobody has made, and any figure the business never gave. Sends " +
		"nothing. With answers (a mail reference like imap:INBOX:12) or to, keeps it as a mail draft for the " +
		"owner's yes. Show the owner every point it raises."
}

func (CustomerReply) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"message": {"type": "string", "description": "The customer's message, as they wrote it"},
			"answers": {"type": "string", "description": "Reference of the mail it answers, from mail_digest, to keep the reply as a draft"},
			"to": {"type": "string", "description": "The customer's address, to keep the reply as a draft"},
			"language": {"type": "string", "enum": ["auto", "en", "bg"]}
		},
		"required": ["message"]
	}`)
}

type customerArgs struct {
	Message  string `json:"message"`
	Answers  string `json:"answers"`
	To       string `json:"to"`
	Language string `json:"language"`
}

func (CustomerReply) Summarize(raw json.RawMessage) string {
	var a customerArgs

	_ = json.Unmarshal(raw, &a)

	return "Write a reply to a customer: " + shortText(a.Message, 60)
}

func shortText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}

	return s
}

func (t CustomerReply) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a customerArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	w, err := t.With.Reply(ctx, a.Message, a.Language)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "A %s. ", strings.ToLower(w.Kind.EN))

	if len(w.BasedOn) == 0 {
		b.WriteString("Nothing in the answer book or the business's documents matches it, so the reply has gaps for the owner. ")
	} else {
		var from []string
		for _, p := range w.BasedOn {
			from = append(from, p.Title)
		}

		b.WriteString("Written from: " + strings.Join(from, "; ") + ". ")
	}

	for _, g := range w.Gaps {
		b.WriteString("\nThe owner must answer: " + g)
	}

	for _, p := range w.Promises {
		b.WriteString("\nPromise nobody has made yet — the owner decides: " + p)
	}

	for _, u := range w.Unsupported {
		b.WriteString("\nNot in the business's words — check it: " + u)
	}

	for _, x := range w.Always {
		b.WriteString("\nThe owner always decides messages about: " + x)
	}

	if a.Answers != "" || a.To != "" {
		dr, warning, err := t.With.Keep(ctx, a.Answers, a.To, "", w.Text)
		if err != nil {
			return "", err
		}

		fmt.Fprintf(&b, "\nKept as draft %d for %s; nothing is sent until the owner says yes on the Mail page.", dr.ID, dr.To)

		if len(w.Gaps) > 0 {
			b.WriteString(" It cannot be sent until the [[ASK]] gaps are filled in.")
		}

		if warning != "" {
			b.WriteString("\nWarning about the message it answers: " + warning)
		}
	}

	b.WriteString("\n\nThe reply:\n" + w.Text)

	return b.String(), nil
}

// BusinessAnswers shows the answer book.
type BusinessAnswers struct{ With CustomerCare }

func (BusinessAnswers) Name() string   { return "business_answers" }
func (BusinessAnswers) Risk() Risk     { return Safe }
func (BusinessAnswers) Cues() []string { return careCues }

func (BusinessAnswers) Description() string {
	return "Show the business's answer book — the questions it has answered, as it wants them answered — and " +
		"how it wants to sound (name, tone, signature, documents folder). With query, only the entries that match."
}

func (BusinessAnswers) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {"query": {"type": "string", "description": "Words to look for"}}}`)
}

func (BusinessAnswers) Summarize(json.RawMessage) string { return "The business's answer book" }

func (t BusinessAnswers) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Query string `json:"query"`
	}

	_ = json.Unmarshal(raw, &a)

	p := t.With.Profile()

	var b strings.Builder

	if p.Name != "" {
		fmt.Fprintf(&b, "Business: %s. ", p.Name)
	}

	if p.Tone != "" {
		fmt.Fprintf(&b, "Tone: %s. ", p.Tone)
	}

	if p.Folder != "" {
		fmt.Fprintf(&b, "Documents: %s (%d files read). ", p.Folder, len(care.Files(t.With.Folder(p))))
	}

	if p.Always != "" {
		fmt.Fprintf(&b, "Always asks the owner about: %s. ", p.Always)
	}

	list := t.With.answers()

	if q := strings.TrimSpace(a.Query); q != "" {
		var hit []care.Answer

		for _, p := range care.Find(q, list, nil, 20) {
			for _, x := range list {
				if fmt.Sprintf("answer:%d", x.ID) == p.Where {
					hit = append(hit, x)
				}
			}
		}

		list = hit
	}

	if len(list) == 0 {
		b.WriteString("\nThe answer book has no entries" + map[bool]string{true: " that match.", false: " yet."}[a.Query != ""])

		return b.String(), nil
	}

	for _, x := range list {
		fmt.Fprintf(&b, "\n%d. Q: %s\n   A: %s", x.ID, x.Question, x.Answer)
	}

	return b.String(), nil
}

// ChangeBusinessAnswers adds, changes or removes an answer.
type ChangeBusinessAnswers struct{ With CustomerCare }

func (ChangeBusinessAnswers) Name() string   { return "change_business_answers" }
func (ChangeBusinessAnswers) Risk() Risk     { return Mutating }
func (ChangeBusinessAnswers) Cues() []string { return careCues }

func (ChangeBusinessAnswers) Description() string {
	return "Add an entry to the business's answer book, change one, or remove one by its number. Only what " +
		"the owner said the answer is — never an answer you made up."
}

func (ChangeBusinessAnswers) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["add", "change", "remove"]},
			"id": {"type": "integer", "description": "The entry's number, to change or remove it"},
			"question": {"type": "string"},
			"answer": {"type": "string"}
		},
		"required": ["action"]
	}`)
}

type careChangeArgs struct {
	Action   string `json:"action"`
	ID       int64  `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func (ChangeBusinessAnswers) Summarize(raw json.RawMessage) string {
	var a careChangeArgs

	_ = json.Unmarshal(raw, &a)

	switch a.Action {
	case "remove":
		return fmt.Sprintf("Remove answer %d from the answer book", a.ID)
	case "change":
		return fmt.Sprintf("Change answer %d in the answer book", a.ID)
	}

	return "Add to the answer book: " + shortText(a.Question, 60)
}

func (t ChangeBusinessAnswers) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a careChangeArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	book := t.With.Book

	switch a.Action {
	case "add":
		x, err := book.AddCareAnswer(a.Question, a.Answer)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("Added to the answer book as %d.", x.ID), nil
	case "change":
		if _, err := book.ChangeCareAnswer(a.ID, a.Question, a.Answer); err != nil {
			return "", err
		}

		return fmt.Sprintf("Answer %d changed.", a.ID), nil
	case "remove":
		if err := book.RemoveCareAnswer(a.ID); err != nil {
			return "", err
		}

		return fmt.Sprintf("Answer %d removed from the answer book.", a.ID), nil
	}

	return "", errors.New("the action must be add, change or remove")
}
