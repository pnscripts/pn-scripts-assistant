package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/care"
	"pn-scripts-assistant/internal/brain/mail"
	"pn-scripts-assistant/internal/brain/maildesk"
	"pn-scripts-assistant/internal/brain/scam"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The mail desk: what came in, sorted, and replies that wait for a yes.
 *
 * Three tools, split by what they can do. The digest reads headers and
 * changes nothing. Drafting keeps a reply in the brain's own list and sends
 * nothing, so it needs no approval. Sending a draft is the one that speaks in
 * its owner's name: it shows the whole message, asks every time (even when
 * everything else is allowed — see Consent), and sends exactly that draft
 * once.
 */

// DraftBook is the drafts list, as much of the store as these need.
type DraftBook interface {
	AddDraft(to, subject, body, answers, warning string) (store.Draft, error)
	Drafts(status string, limit int) ([]store.Draft, error)
	Draft(id int64) (store.Draft, error)
	ClaimDraft(id int64) (store.Draft, error)
	SentDraft(id int64, note string) error
	FailedDraft(id int64, note string) error
}

// MailDesk is where mail is read from and drafts are kept.
type MailDesk struct {
	// Account is the mail server, asked when used; not configured means
	// none.
	Account func() mail.Account

	// Local reads Thunderbird's own folders on this machine.
	Local ReadMailbox

	Drafts DraftBook
	Lookup func() scam.Lookup
}

func (m MailDesk) look() scam.Lookup {
	if m.Lookup == nil {
		return nil
	}

	return m.Lookup()
}

// Collect reads the newest messages from the mail server, or from
// Thunderbird's inbox when there is no server; which one is said.
func (m MailDesk) Collect(ctx context.Context, count int) ([]maildesk.Item, string, error) {
	if count <= 0 {
		count = 60
	}

	if m.Account != nil {
		if account := m.Account(); account.Configured() {
			list, err := mail.Recent(account, "INBOX", count)
			if err != nil {
				return nil, "", err
			}

			out := make([]maildesk.Item, 0, len(list))

			for _, msg := range list {
				out = append(out, maildesk.Item{
					Ref: "imap:INBOX:" + strconv.Itoa(msg.Number), From: msg.From, ReplyTo: msg.ReplyTo,
					Subject: msg.Subject, Date: msg.Date, Seen: msg.Seen, Bulk: msg.Bulk,
				})
			}

			return out, SourceServer, nil
		}
	}

	items, err := m.Local.inbox(ctx, count)
	if err != nil {
		return nil, "", err
	}

	return items, SourceThunderbird, nil
}

// Where mail was read from.
const (
	SourceServer      = "server"
	SourceThunderbird = "thunderbird"
)

func sourceWords(source string) string {
	if source == SourceServer {
		return "your mail server"
	}

	return "Thunderbird on this computer"
}

// ErrNoMail is a desk with no mailbox to read.
var ErrNoMail = errors.New("there is no mailbox to read: set up your mail account under Settings, or use Thunderbird on this computer")

// inbox is the newest messages of every Thunderbird inbox — one per
// account, and Local Folders — newest first.
func (r ReadMailbox) inbox(ctx context.Context, count int) ([]maildesk.Item, error) {
	profiles := r.profiles()
	if len(profiles) == 0 {
		return nil, ErrNoMail
	}

	var folders []string

	for _, p := range profiles {
		for _, f := range mailFolders(p) {
			if last := f[strings.LastIndex(f, "/")+1:]; strings.EqualFold(last, "inbox") {
				folders = append(folders, f)
			}
		}
	}

	if len(folders) == 0 {
		return nil, ErrNoMail
	}

	var out []maildesk.Item

	for _, folder := range folders {
		file, err := r.folderFile(profiles, folder)
		if err != nil {
			continue
		}

		starts, err := messageStarts(file)
		if err != nil {
			continue
		}

		taken := 0

		for i := len(starts) - 1; i >= 0 && taken < count; i-- {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			h, deleted, err := headersAt(file, starts[i])
			if err != nil || deleted {
				continue
			}

			status, _ := strconv.ParseUint(strings.TrimSpace(h.Get("X-Mozilla-Status")), 16, 32)

			bulk := h.Get("List-Unsubscribe") != "" || h.Get("List-Id") != ""
			if p := strings.ToLower(h.Get("Precedence")); p == "bulk" || p == "list" || p == "junk" {
				bulk = true
			}

			if a := h.Get("Auto-Submitted"); a != "" && !strings.EqualFold(a, "no") {
				bulk = true
			}

			out = append(out, maildesk.Item{
				Ref: "local:" + folder + ":" + strconv.Itoa(i+1), From: decodeHeader(h.Get("From")),
				ReplyTo: decodeHeader(h.Get("Reply-To")), Subject: decodeHeader(h.Get("Subject")),
				Date: h.Get("Date"), Seen: status&0x1 != 0, Bulk: bulk,
			})
			taken++
		}
	}

	return out, nil
}

// original is the headers of the message a ref names.
func (m MailDesk) original(_ context.Context, ref string) (maildesk.Item, error) {
	// The folder may hold anything, so the number is after the last colon.
	kind, rest, _ := strings.Cut(ref, ":")

	box, number := rest, ""
	if i := strings.LastIndex(rest, ":"); i >= 0 {
		box, number = rest[:i], rest[i+1:]
	}

	n, err := strconv.Atoi(number)
	if err != nil || n < 1 {
		return maildesk.Item{}, fmt.Errorf("%q is not a message reference like imap:INBOX:12", ref)
	}

	switch kind {
	case "imap":
		if m.Account == nil || !m.Account().Configured() {
			return maildesk.Item{}, ErrNoMail
		}

		msg, err := mail.Read(m.Account(), box, n)
		if err != nil {
			return maildesk.Item{}, err
		}

		return maildesk.Item{Ref: ref, From: msg.From, ReplyTo: msg.ReplyTo, Subject: msg.Subject, Date: msg.Date}, nil
	case "local":
		profiles := m.Local.profiles()

		file, err := m.Local.folderFile(profiles, box)
		if err != nil {
			return maildesk.Item{}, err
		}

		starts, err := messageStarts(file)
		if err != nil || n > len(starts) {
			return maildesk.Item{}, fmt.Errorf("there is no message %d in %s", n, box)
		}

		h, _, err := headersAt(file, starts[n-1])
		if err != nil {
			return maildesk.Item{}, err
		}

		return maildesk.Item{Ref: ref, From: decodeHeader(h.Get("From")), ReplyTo: decodeHeader(h.Get("Reply-To")),
			Subject: decodeHeader(h.Get("Subject")), Date: h.Get("Date")}, nil
	}

	return maildesk.Item{}, fmt.Errorf("%q is not a message reference like imap:INBOX:12", ref)
}

// MailDigest sums up new mail.
type MailDigest struct{ Desk MailDesk }

func (MailDigest) Name() string { return "mail_digest" }
func (MailDigest) Risk() Risk   { return Safe }

func (MailDigest) Cues() []string {
	return []string{"mail", "email", "e-mail", "inbox", "messages", "digest", "поща", "имейл", "писма", "съобщения", "входящ"}
}

func (MailDigest) Description() string {
	return "Sum up new mail since a number of hours ago (default 24): what looks suspicious (scam check), " +
		"what needs a reply, what is from people, and newsletters and notifications by sender — from the " +
		"headers, without opening anything. Each line has a reference (imap:INBOX:12, or local:<folder>:45 for " +
		"Thunderbird) to " +
		"read it or draft a reply. Also says how many drafted replies wait to be sent."
}

func (MailDigest) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"hours": {"type": "integer", "description": "How far back, in hours; default 24"}
		}
	}`)
}

func (MailDigest) Summarize(json.RawMessage) string { return "Sum up new mail" }

func (t MailDigest) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Hours int `json:"hours"`
	}

	_ = json.Unmarshal(raw, &a)

	d, source, err := t.Desk.Digest(ctx, a.Hours)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "From %s: %d messages in the last %d hours, %d not opened.\n",
		sourceWords(source), d.Total, hoursOr(a.Hours), d.Unseen)

	group := func(title string, list []maildesk.Item, scams bool) {
		if len(list) == 0 {
			return
		}

		fmt.Fprintf(&b, "\n%s (%d):\n", title, len(list))

		for _, it := range list {
			fmt.Fprintf(&b, "- [%s] %s — %s", it.Ref, maildesk.Name(it.From), it.Subject)

			if scams {
				fmt.Fprintf(&b, " — %s", it.Scam)
			}

			b.WriteString("\n")
		}
	}

	group("Suspicious, do not act on these without checking", d.Suspicious, true)
	group("Needs a reply", d.Reply, false)
	group("From people", d.People, false)

	if len(d.Bulk) > 0 {
		fmt.Fprintf(&b, "\nNewsletters and notifications (%d): %s\n", len(d.Bulk), strings.Join(maildesk.Senders(d.Bulk, 8), ", "))
	}

	if d.Total == 0 {
		b.WriteString("\nNothing new.\n")
	}

	if t.Desk.Drafts != nil {
		if waiting, err := t.Desk.Drafts.Drafts(store.DraftWaiting, 50); err == nil && len(waiting) > 0 {
			fmt.Fprintf(&b, "\n%d drafted replies are waiting for a yes on the Mail page.\n", len(waiting))
		}
	}

	return b.String(), nil
}

func hoursOr(h int) int {
	if h <= 0 {
		return 24
	}

	return h
}

// Digest is new mail sorted, and where it was read from.
func (m MailDesk) Digest(ctx context.Context, hours int) (maildesk.Digest, string, error) {
	items, source, err := m.Collect(ctx, 0)
	if err != nil {
		return maildesk.Digest{}, "", err
	}

	// From the headers alone: a digest of sixty messages is not sixty
	// questions about their senders to the network. See maildesk.Sort.
	return maildesk.Sort(items, time.Now().Add(-time.Duration(hoursOr(hours))*time.Hour), nil), source, nil
}

// DraftReply keeps a reply, or a new message, waiting for its owner's yes.
type DraftReply struct{ Desk MailDesk }

func (DraftReply) Name() string { return "draft_reply" }

// Safe: it sends nothing. A draft is the assistant's own note until its
// owner sends it with send_draft or from the Mail page.
func (DraftReply) Risk() Risk { return Safe }

func (DraftReply) Cues() []string {
	return []string{"reply", "answer", "draft", "write to", "email", "mail", "отговор", "отговори", "чернова", "напиши", "писмо", "имейл"}
}

func (DraftReply) Description() string {
	return "Write a reply (or a new message) and keep it as a draft waiting for the owner's yes; nothing is " +
		"sent. Give answers (the reference from mail_digest, like imap:INBOX:12) to reply to a message: the " +
		"address and the subject are taken from it and the scam check's verdict is kept with the draft. " +
		"Write in the language and tone of the message, as the owner would. Send it only with send_draft, " +
		"after the owner agrees to that exact text."
}

func (DraftReply) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"answers": {"type": "string", "description": "Reference of the message replied to, from mail_digest"},
			"to": {"type": "string", "description": "Who it is for; taken from the message when answers is given"},
			"subject": {"type": "string", "description": "Subject; Re: and the original subject by default"},
			"body": {"type": "string", "description": "What the message says"}
		},
		"required": ["body"]
	}`)
}

type draftArgs struct {
	Answers string `json:"answers"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	ID      int64  `json:"id"`
}

func (DraftReply) Summarize(raw json.RawMessage) string {
	var a draftArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Keep a draft to %s (not sent)\n\n%s", strings.TrimSpace(a.To+" "+a.Answers), first(a.Body, 400))
}

func (t DraftReply) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a draftArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	dr, warning, err := t.Desk.Keep(ctx, a.Answers, a.To, a.Subject, a.Body)
	if err != nil {
		return "", err
	}

	out := fmt.Sprintf("Draft %d kept for %s, subject %q. Nothing has been sent: it waits on the Mail page "+
		"for the owner's yes, or send_draft after they agree to this exact text.", dr.ID, dr.To, dr.Subject)

	if warning != "" {
		out += "\nWarning kept with the draft — the message it answers: " + warning
	}

	return out, nil
}

/*
 * Keep files a draft: to the sender of the message it answers (answers is
 * its reference, like imap:INBOX:12) unless to is given, with Re: and its
 * subject, and with the scam check's verdict on that message kept beside
 * it. Nothing is sent. Shared by draft_reply and customer replies.
 */
func (m MailDesk) Keep(ctx context.Context, answers, to, subject, body string) (store.Draft, string, error) {
	if strings.TrimSpace(body) == "" {
		return store.Draft{}, "", fmt.Errorf("a draft needs something to say")
	}

	if m.Drafts == nil {
		return store.Draft{}, "", fmt.Errorf("drafts cannot be kept here")
	}

	var about, warning string

	if answers != "" {
		orig, err := m.original(ctx, answers)
		if err != nil {
			return store.Draft{}, "", err
		}

		verdict := scam.Check(scam.Message{From: orig.From, ReplyTo: orig.ReplyTo, Subject: orig.Subject}, m.look())

		if to == "" {
			to = maildesk.Address(orig.From)

			// A reply goes where the sender asked — unless that is exactly
			// what the scam check is worried about.
			if orig.ReplyTo != "" && !verdict.Warns() {
				to = maildesk.Address(orig.ReplyTo)
			}
		}

		if subject == "" {
			subject = maildesk.ReplySubject(orig.Subject)
		}

		about = fmt.Sprintf("%s — %s, %s", orig.From, orig.Subject, orig.Date)

		if verdict.Warns() {
			warning = verdict.String()
		}
	}

	if strings.TrimSpace(to) == "" {
		return store.Draft{}, "", fmt.Errorf("say who the message is for, or which message it answers")
	}

	dr, err := m.Drafts.AddDraft(to, subject, body, about, warning)

	return dr, warning, err
}

// SendDraft sends one draft, as it stands, after the owner's yes.
type SendDraft struct{ Desk MailDesk }

func (SendDraft) Name() string { return "send_draft" }

// Mutating, and asked every time: see Consent.
func (SendDraft) Risk() Risk { return Mutating }

func (SendDraft) Consent(json.RawMessage) string { return "sending a message in your name" }

func (SendDraft) Cues() []string {
	return []string{"send", "draft", "изпрати", "чернова"}
}

func (SendDraft) Description() string {
	return "Send one waiting draft by its number, exactly as it stands, from the owner's mail account. " +
		"The owner sees the whole message and must agree each time."
}

func (SendDraft) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"id": {"type": "integer", "description": "The draft's number"}},
		"required": ["id"]
	}`)
}

// Summarize shows the whole draft as it will go: the approval is for these
// words to this person, not for "a reply to Anna".
func (t SendDraft) Summarize(raw json.RawMessage) string {
	var a draftArgs

	_ = json.Unmarshal(raw, &a)

	if t.Desk.Drafts == nil {
		return fmt.Sprintf("Send draft %d", a.ID)
	}

	dr, err := t.Desk.Drafts.Draft(a.ID)
	if err != nil {
		return fmt.Sprintf("Send draft %d (it could not be read: %v)", a.ID, err)
	}

	head := fmt.Sprintf("Send to %s\nSubject: %s\n\n%s", dr.To, dr.Subject, dr.Body)

	if dr.Warning != "" {
		head = "WARNING — the message this answers: " + dr.Warning + "\n\n" + head
	}

	return head
}

func (t SendDraft) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a draftArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	return SendWaitingDraft(t.Desk, a.ID)
}

/*
 * SendWaitingDraft sends a draft once: it is claimed first, so a second
 * press or a second call finds nothing to send, and its outcome is recorded
 * on it. Shared by the tool and the Mail page's Send button.
 */
func SendWaitingDraft(desk MailDesk, id int64) (string, error) {
	if desk.Drafts == nil {
		return "", fmt.Errorf("drafts cannot be kept here")
	}

	/*
	 * A customer reply with a question still in it for its owner — [[ASK:
	 * …]] — is not finished, and sending it would show the customer the
	 * note. It stays waiting until the gap is filled.
	 */
	if waiting, err := desk.Drafts.Draft(id); err == nil && care.HasGaps(waiting.Body) {
		return "", fmt.Errorf("draft %d still has a question for you marked [[…]]; fill it in on the Mail page first", id)
	}

	if desk.Account == nil || !desk.Account().Configured() {
		return "", fmt.Errorf("sending needs your mail account set up under Settings; the draft is kept, " +
			"and its text can be copied into your mail program")
	}

	dr, err := desk.Drafts.ClaimDraft(id)
	if err != nil {
		return "", fmt.Errorf("draft %d: %w", id, err)
	}

	var to []string

	for _, address := range strings.Split(dr.To, ",") {
		if address = strings.TrimSpace(address); address != "" {
			to = append(to, address)
		}
	}

	said, err := mail.Send(desk.Account(), to, dr.Subject, dr.Body)
	if err != nil {
		_ = desk.Drafts.FailedDraft(id, err.Error())

		return "", fmt.Errorf("draft %d was not sent: %w", id, err)
	}

	_ = desk.Drafts.SentDraft(id, said)

	return fmt.Sprintf("Sent draft %d to %s. %s", id, dr.To, said), nil
}
