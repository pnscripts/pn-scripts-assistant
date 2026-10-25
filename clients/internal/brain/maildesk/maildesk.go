/*
 * Package maildesk sorts new mail the way a careful assistant would before
 * anybody reads it: what is suspicious, what needs an answer, what is from a
 * person, and what is a newsletter or a machine.
 *
 * It decides from the headers and nothing else, in code, so the same inbox
 * sorts the same way every morning and every line can be explained: the
 * scam check warned; the sender asked a question; the message carries an
 * unsubscribe link. A model reads a message only when somebody wants it
 * read, or a reply drafted.
 */
package maildesk

import (
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/scam"
)

// Item is one message as the desk sees it.
type Item struct {
	// Ref is how the message is found again: "imap:INBOX:123" or
	// "local:Inbox:45".
	Ref string `json:"ref"`

	From    string    `json:"from"`
	ReplyTo string    `json:"reply_to,omitempty"`
	Subject string    `json:"subject"`
	Date    string    `json:"date"`
	When    time.Time `json:"when"`
	Seen    bool      `json:"seen"`
	Bulk    bool      `json:"bulk"`

	// Why is the reason it is in its group, in English and Bulgarian.
	Why   string `json:"why,omitempty"`
	WhyBG string `json:"why_bg,omitempty"`

	// Scam is the check's verdict, when it warned.
	Scam string `json:"scam,omitempty"`

	verdict scam.Verdict
}

// Digest is new mail, sorted.
type Digest struct {
	Since      time.Time `json:"since"`
	Total      int       `json:"total"`
	Unseen     int       `json:"unseen"`
	Suspicious []Item    `json:"suspicious"`
	Reply      []Item    `json:"needs_reply"`
	People     []Item    `json:"from_people"`
	Bulk       []Item    `json:"bulk"`
}

// When reads a message's Date header; the zero time when it cannot.
func When(date string) time.Time {
	at, err := mail.ParseDate(strings.TrimSpace(date))
	if err != nil {
		return time.Time{}
	}

	return at
}

var automatic = regexp.MustCompile(`(?i)(^|[<\s"._+-])(no-?reply|do-?not-?reply|donotreply|mailer-daemon|postmaster|notifications?|notices?|notify|alerts?|newsletters?|news|digest|bounce[s]?|automated|system)([._+-][^@]*)?@`)

// Automatic reports whether an address is a machine's: noreply@,
// notifications@ and the like.
func Automatic(from string) bool { return automatic.MatchString(from) }

/*
 * asks are the words that mean somebody wants something back. In the
 * subject only: the desk sorts from headers, and a subject that asks is the
 * clearest signal a sender gives.
 */
var asks = []string{
	"?", "please", "could you", "can you", "would you", "request", "question", "urgent", "asap",
	"deadline", "confirm", "reminder", "invoice", "quote", "offer", "meeting", "call", "help",
	"моля", "можете ли", "бихте ли", "може ли", "въпрос", "запитване", "спешно", "срок",
	"потвърд", "напомняне", "фактура", "оферта", "среща", "помощ", "заявка",
}

// Sort sorts items newer than since; a message with no readable date counts
// as new. Within each group the newest is first. look may be nil: the
// digest checks from the headers alone and asks no server about anybody,
// as a list of mail always has; reading or answering one checks it fully.
func Sort(items []Item, since time.Time, look scam.Lookup) Digest {
	d := Digest{Since: since, Suspicious: []Item{}, Reply: []Item{}, People: []Item{}, Bulk: []Item{}}

	for _, it := range items {
		if it.When.IsZero() {
			it.When = When(it.Date)
		}

		if !it.When.IsZero() && it.When.Before(since) {
			continue
		}

		d.Total++

		if !it.Seen {
			d.Unseen++
		}

		it.verdict = scam.Check(scam.Message{From: it.From, ReplyTo: it.ReplyTo, Subject: it.Subject}, look)

		subject := strings.ToLower(it.Subject)

		switch {
		case it.verdict.Warns():
			it.Scam = it.verdict.String()
			level := "подозрително"
			if it.verdict.Level == scam.LikelyScam {
				level = "вероятна измама"
			}

			it.Why = "The scam check warned: " + strings.Join(it.verdict.Flags, "; ") + "."
			it.WhyBG = "Проверката за измама предупреди — " + level + ". Не отваряйте връзки, " +
				"не плащайте и не отговаряйте, преди да проверите подателя по друг път."
			d.Suspicious = append(d.Suspicious, it)
		case it.Bulk:
			it.Why, it.WhyBG = "Sent to a list: it has an unsubscribe link or says it is bulk mail.",
				"Изпратено до списък: има връзка за отписване или е обявено като масова поща."
			d.Bulk = append(d.Bulk, it)
		case Automatic(it.From):
			it.Why, it.WhyBG = "Sent by a machine (a no-reply or notification address).",
				"Изпратено автоматично (адрес без отговор или за известия)."
			d.Bulk = append(d.Bulk, it)
		case strings.HasPrefix(subject, "re:") || strings.HasPrefix(subject, "отг:") || containsAny(subject, asks):
			it.Why, it.WhyBG = "From a person, and the subject asks for something or continues a conversation.",
				"От човек, а темата иска нещо или продължава разговор."
			d.Reply = append(d.Reply, it)
		default:
			it.Why, it.WhyBG = "From a person.", "От човек."
			d.People = append(d.People, it)
		}
	}

	for _, group := range [][]Item{d.Suspicious, d.Reply, d.People, d.Bulk} {
		sort.SliceStable(group, func(i, j int) bool { return group[i].When.After(group[j].When) })
	}

	return d
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}

	return false
}

// Senders is the bulk mail's senders, most messages first: "News (4)".
func Senders(items []Item, most int) []string {
	count := map[string]int{}

	var order []string

	for _, it := range items {
		name := Name(it.From)
		if count[name] == 0 {
			order = append(order, name)
		}

		count[name]++
	}

	sort.SliceStable(order, func(i, j int) bool { return count[order[i]] > count[order[j]] })

	if most > 0 && len(order) > most {
		order = order[:most]
	}

	out := make([]string, len(order))

	for i, name := range order {
		out[i] = name

		if count[name] > 1 {
			out[i] += " (" + strconv.Itoa(count[name]) + ")"
		}
	}

	return out
}

// Name is the display name of an address, or the address.
func Name(from string) string {
	if a, err := mail.ParseAddress(from); err == nil {
		if a.Name != "" {
			return a.Name
		}

		return a.Address
	}

	return strings.TrimSpace(from)
}

// Address is the bare address in a From or Reply-To header.
func Address(from string) string {
	if a, err := mail.ParseAddress(from); err == nil {
		return a.Address
	}

	return strings.Trim(strings.TrimSpace(from), "<>")
}

// ReplySubject is the subject of an answer: "Re: " once, never "Re: Re:".
func ReplySubject(subject string) string {
	s := strings.TrimSpace(subject)
	lower := strings.ToLower(s)

	for _, prefix := range []string{"re:", "отг:", "aw:"} {
		if strings.HasPrefix(lower, prefix) {
			return s
		}
	}

	return "Re: " + s
}
