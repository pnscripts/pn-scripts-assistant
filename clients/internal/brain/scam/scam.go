/*
 * Package scam is the assistant's first look at a message for fraud.
 *
 * Phishing, fake invoices, "your account will be suspended", a buyer who wants
 * to pay through a link — the same few tricks, told in endless new words. A
 * person reading mail in a hurry misses them; an assistant reading mail for
 * that person must not hand the trick on as if it were a real request. So
 * every message the mail tools read is checked here first, and what is found
 * is written beside the message, where the model and the person both see it.
 *
 * It flags, it never decides. A flag means "look twice", and the playbook
 * that goes with it (scam_guard) says what looking twice is. Nothing here
 * knows any particular person, company or mailbox: the brands are the ones
 * phishing everywhere pretends to be.
 */
package scam

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Message is what a check needs from one piece of mail.
type Message struct {
	From    string
	ReplyTo string
	Subject string
	Body    string
}

// Lookup answers whether a domain exists. Known is false when the question
// could not be answered (offline), which is never treated as "does not exist".
type Lookup interface {
	Resolves(domain string) (exists, known bool)
}

// Levels, from nothing found to most likely a scam.
const (
	Clean      = "no warning signs"
	Suspicious = "suspicious"
	LikelyScam = "likely a scam"
)

// Verdict is what one check found.
type Verdict struct {
	Level string
	Score int
	Flags []string
}

// Warns reports whether anything is worth saying.
func (v Verdict) Warns() bool { return v.Level != Clean }

// String is the verdict as it is written beside a message.
func (v Verdict) String() string {
	if !v.Warns() {
		return "Scam check: " + Clean + "."
	}

	return fmt.Sprintf("Scam check: %s — %s. Do not open its links, reply, pay, "+
		"or give any password or code because of it; tell the owner what was found "+
		"and follow the scam_guard playbook.", strings.ToUpper(v.Level), strings.Join(v.Flags, "; "))
}

/*
 * brands are the names phishing borrows most, with the domains that really
 * are them. A link or sender that wears the name on any other domain is a
 * lookalike.
 */
var brands = map[string][]string{
	"paypal":    {"paypal.com", "paypal.me"},
	"apple":     {"apple.com", "icloud.com"},
	"icloud":    {"icloud.com", "apple.com"},
	"microsoft": {"microsoft.com", "live.com", "outlook.com", "office.com", "microsoftonline.com"},
	"outlook":   {"outlook.com", "microsoft.com", "live.com"},
	"office365": {"microsoft.com", "office.com"},
	"google":    {"google.com", "gmail.com", "youtube.com", "googleusercontent.com"},
	"gmail":     {"gmail.com", "google.com"},
	"amazon":    {"amazon.com", "amazon.co.uk", "amazon.de", "amazon.fr", "amazon.it", "amazon.es", "amazonaws.com"},
	"netflix":   {"netflix.com"},
	"facebook":  {"facebook.com", "facebookmail.com", "meta.com", "fb.com"},
	"instagram": {"instagram.com", "facebookmail.com"},
	"whatsapp":  {"whatsapp.com"},
	"dhl":       {"dhl.com", "dhl.de"},
	"fedex":     {"fedex.com"},
	"ups":       {"ups.com"},
	"github":    {"github.com", "githubusercontent.com"},
	"stripe":    {"stripe.com"},
	"wise":      {"wise.com"},
	"revolut":   {"revolut.com"},
	"binance":   {"binance.com"},
	"coinbase":  {"coinbase.com"},
	"steam":     {"steampowered.com", "steamcommunity.com"},
	"dropbox":   {"dropbox.com"},
	"docusign":  {"docusign.com", "docusign.net"},
	"linkedin":  {"linkedin.com"},
	"mozilla":   {"mozilla.org", "mozilla.com"},
	"npmjs":     {"npmjs.com", "npmjs.org"},
	"wordpress": {"wordpress.org", "wordpress.com"},
}

// freeMail are providers anybody can sign up to, so a "company" writing from
// one has not shown it is a company.
var freeMail = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "hotmail.com": true,
	"live.com": true, "yahoo.com": true, "aol.com": true, "mail.ru": true, "yandex.ru": true,
	"gmx.com": true, "gmx.net": true, "proton.me": true, "protonmail.com": true, "qq.com": true,
	"163.com": true, "126.com": true, "icloud.com": true, "zoho.com": true, "abv.bg": true,
}

var shorteners = map[string]bool{
	"bit.ly": true, "tinyurl.com": true, "t.co": true, "goo.gl": true, "ow.ly": true,
	"is.gd": true, "buff.ly": true, "cutt.ly": true, "rebrand.ly": true, "t.me": true, "wa.me": true,
}

// secondLevel are the country suffixes under which the registrable name is
// three labels long, not two.
var secondLevel = map[string]bool{
	"co.uk": true, "org.uk": true, "ac.uk": true, "com.au": true, "net.au": true, "co.jp": true,
	"co.nz": true, "com.br": true, "co.in": true, "co.za": true, "com.cn": true, "com.tr": true,
	"com.mx": true, "co.kr": true,
}

type rule struct {
	name    string
	pattern *regexp.Regexp
}

var (
	urgency = rule{"urgency", regexp.MustCompile(`(?i)\b(urgent|immediately|within 24 hours|within 48 hours|final notice|last warning|act now|suspend(ed)?|locked|deactivat|expire[sd]?|limited time)\b|спешно|незабавно|блокиран|спрян`)}
	asksFor = []rule{
		{"asks for a password or code", regexp.MustCompile(`(?i)\b(verify your (account|identity|email)|confirm your (account|password|identity)|password|passcode|one[- ]time code|2fa|login details|sign[- ]in details|security code|seed phrase|recovery phrase|private key)\b|парола|код за потвърждение`)},
		{"asks for payment or bank details", regexp.MustCompile(`(?i)\b(wire transfer|bank details|iban|swift|gift ?cards?|bitcoin|crypto|usdt|western union|payment (is )?(required|overdue|pending)|unpaid invoice|outstanding (balance|invoice)|customs fee|delivery fee|processing fee|release fee|update (your )?(billing|payment))\b|доплат|такса за доставка|банкова сметка`)},
		{"asks to install or authorise something", regexp.MustCompile(`(?i)\b(install (this|the) (app|extension|software)|remote access|anydesk|teamviewer|authori[sz]e (the |this |our )?app|grant access|run (this|the) (script|command)|npm install)\b`)},
		{"moves the talk to another channel", regexp.MustCompile(`(?i)\b(telegram|whatsapp|signal me|text me|contact me on)\b`)},
	}
	companyWords = regexp.MustCompile(`(?i)\b(ltd|llc|inc|gmbh|company|corporation|support team|billing department|legal department|security team)\b`)
	links        = regexp.MustCompile(`(?i)https?://([^/\s"'<>)\]]+)`)
)

/*
 * Check looks at one message.
 *
 * look may be nil, and then no domain is asked about — the private setting,
 * or a list of headers being checked quickly. Everything else is the words
 * and the addresses themselves.
 */
func Check(m Message, look Lookup) Verdict {
	var v Verdict

	add := func(score int, flag string) {
		v.Score += score
		v.Flags = append(v.Flags, flag)
	}

	sender, name := address(m.From)
	domain := domainOf(sender)
	base := Registrable(domain)
	text := m.Subject + "\n" + m.Body

	if look != nil && base != "" {
		if exists, known := look.Resolves(base); known && !exists {
			add(4, "the sender's domain "+base+" does not exist")
		}
	}

	if reply, _ := address(m.ReplyTo); reply != "" {
		if rb := Registrable(domainOf(reply)); rb != "" && base != "" && rb != base {
			add(2, "replies go to "+rb+", not to the sender's "+base)
		}
	}

	if brand := wears(name+" "+localPart(sender), base); brand != "" {
		add(3, fmt.Sprintf("the sender calls itself %s but writes from %s", brand, base))
	} else if brand := lookalike(base); brand != "" {
		add(3, fmt.Sprintf("the sender's domain %s imitates %s", base, brand))
	}

	if freeMail[base] && companyWords.MatchString(name+" "+text) {
		add(2, "a company writing from a free mail address ("+base+")")
	}

	seen := map[string]bool{}

	var hosts []string

	for _, match := range links.FindAllStringSubmatch(m.Body, -1) {
		host := strings.ToLower(strings.TrimSuffix(strings.Split(match[1], ":")[0], "."))
		if at := strings.LastIndex(host, "@"); at >= 0 {
			// user@host in a link is a classic way to show one name and go
			// to another.
			add(3, "a link hides its real address behind "+host[:at]+"@")
			host = host[at+1:]
		}

		if !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}

	sort.Strings(hosts)

	for _, host := range hosts {
		hb := Registrable(host)

		switch {
		case net.ParseIP(host) != nil:
			add(2, "a link goes to a bare address, "+host)
		case strings.Contains(host, "xn--"):
			add(3, "a link uses look-alike letters ("+host+")")
		case shorteners[hb]:
			add(1, "a link hides where it goes ("+hb+")")
		}

		if brand := lookalike(hb); brand != "" {
			add(3, fmt.Sprintf("a link to %s imitates %s", hb, brand))
		} else if brand := wears(host, hb); brand != "" && net.ParseIP(host) == nil {
			add(3, fmt.Sprintf("a link puts %s in front of %s", brand, hb))
		}

		if look != nil && hb != "" {
			if exists, known := look.Resolves(hb); known && !exists {
				add(1, "a link goes to "+hb+", which does not exist")
			}
		}
	}

	// What the addresses and links showed, before the words are weighed.
	structural := v.Score

	// Every link on the sender's own domain or a brand's real one.
	ownLinks := true

	for _, host := range hosts {
		hb := Registrable(host)
		if hb != base && !anyGenuine(hb) {
			ownLinks = false
		}
	}

	urgent := urgency.pattern.MatchString(text)

	asks := 0

	for _, r := range asksFor {
		if r.pattern.MatchString(text) {
			asks++

			add(1, r.name)
		}
	}

	if urgent {
		add(1, "pressure to act quickly")
	}

	if urgent && asks > 0 {
		add(2, "pressure combined with a request for credentials, money or access")
	}

	/*
	 * Words alone are a reason to look twice, not a verdict. A password
	 * reset from a real service says "password" and "expires" too; what
	 * makes a scam is those words arriving from somewhere that is not who
	 * it claims to be. So without anything wrong with the addresses, the
	 * words can make a message suspicious and no more — and from a brand's
	 * own domain, linking only to itself, they are not counted at all.
	 */
	if structural == 0 {
		switch {
		case anyGenuine(base) && ownLinks:
			v.Score = min(v.Score, 1)
		default:
			v.Score = min(v.Score, 3)
		}
	}

	switch {
	case v.Score >= 4:
		v.Level = LikelyScam
	case v.Score >= 2:
		v.Level = Suspicious
	default:
		v.Level = Clean
	}

	return v
}

// address splits "Name <a@b>" into the address and the name.
func address(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	if a, err := mail.ParseAddress(raw); err == nil {
		return strings.ToLower(a.Address), a.Name
	}

	if m := regexp.MustCompile(`[^\s<>"]+@[^\s<>"]+`).FindString(raw); m != "" {
		return strings.ToLower(m), strings.TrimSpace(strings.Replace(raw, m, "", 1))
	}

	return "", raw
}

func domainOf(addr string) string {
	if at := strings.LastIndex(addr, "@"); at >= 0 {
		return strings.Trim(strings.ToLower(addr[at+1:]), ". >")
	}

	return ""
}

func localPart(addr string) string {
	if at := strings.LastIndex(addr, "@"); at >= 0 {
		return addr[:at]
	}

	return ""
}

// Registrable is the part of a host somebody registered: mail.example.co.uk
// is example.co.uk.
func Registrable(host string) string {
	host = strings.Trim(strings.ToLower(host), ".")
	parts := strings.Split(host, ".")

	if len(parts) < 2 {
		return host
	}

	if len(parts) >= 3 && secondLevel[strings.Join(parts[len(parts)-2:], ".")] {
		return strings.Join(parts[len(parts)-3:], ".")
	}

	return strings.Join(parts[len(parts)-2:], ".")
}

// genuine reports whether a registrable domain really is a brand's.
func genuine(brand, base string) bool {
	for _, d := range brands[brand] {
		if base == d {
			return true
		}
	}

	return false
}

// anyGenuine is whether a registrable domain is one of the brands' own.
func anyGenuine(base string) bool {
	for brand := range brands {
		if genuine(brand, base) {
			return true
		}
	}

	return false
}

// wears is a brand that text names while base is not that brand's domain.
func wears(text, base string) string {
	if base == "" {
		return ""
	}

	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})

	for _, w := range words {
		// Three-letter names are words too often ("pop-ups") to count here.
		if _, ok := brands[w]; ok && len(w) > 3 && !genuine(w, base) && !ownedBy(w, base) {
			return w
		}
	}

	return ""
}

// ownedBy is true for any of a brand's other country domains, amazon.de
// alongside amazon.com, which the short list cannot all name.
func ownedBy(brand, base string) bool {
	label, _, _ := strings.Cut(base, ".")

	return label == brand && brand != "ups" && brand != "wise" && brand != "steam"
}

/*
 * lookalike is the brand a registrable domain imitates: the brand with one
 * letter changed, added or dropped (paypa1, mozila), or the brand with
 * something stuck to it (paypal-secure, appleid-support).
 */
func lookalike(base string) string {
	if base == "" {
		return ""
	}

	label, _, _ := strings.Cut(base, ".")

	for brand := range brands {
		if genuine(brand, base) || label == brand {
			continue
		}

		if len(brand) >= 5 && strings.Contains(label, brand) {
			return brand
		}

		if len(brand) >= 5 {
			for _, part := range append([]string{label}, strings.Split(label, "-")...) {
				if oneEditApart(part, brand) {
					return brand
				}
			}
		}
	}

	return ""
}

func oneEditApart(a, b string) bool {
	if a == b {
		return false
	}

	ra, rb := []rune(a), []rune(b)

	if len(ra) < len(rb) {
		ra, rb = rb, ra
	}

	if len(ra)-len(rb) > 1 {
		return false
	}

	i, j, edits := 0, 0, 0

	for i < len(ra) && j < len(rb) {
		if ra[i] == rb[j] {
			i++
			j++

			continue
		}

		edits++
		if edits > 1 {
			return false
		}

		if len(ra) == len(rb) {
			i++
			j++
		} else {
			i++
		}
	}

	return edits+(len(ra)-i)+(len(rb)-j) <= 1
}

/*
 * DNS asks the system's resolver, briefly, and remembers the answer for the
 * life of the program. A failure that is not "no such host" — no network,
 * a timeout — is unknown, never "does not exist".
 */
type DNS struct {
	mu    sync.Mutex
	known map[string]bool
}

// Resolves implements Lookup.
func (d *DNS) Resolves(domain string) (bool, bool) {
	d.mu.Lock()
	if v, ok := d.known[domain]; ok {
		d.mu.Unlock()

		return v, true
	}
	d.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := net.DefaultResolver.LookupHost(ctx, domain)

	exists := err == nil

	if err != nil {
		var dnsErr *net.DNSError
		if !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
			return false, false
		}
	}

	d.mu.Lock()
	if d.known == nil {
		d.known = map[string]bool{}
	}
	d.known[domain] = exists
	d.mu.Unlock()

	return exists, true
}
