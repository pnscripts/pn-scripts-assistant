/*
 * Package polish is the last look at a text before it goes out in its
 * user's name: an email, a message, a post, a comment.
 *
 * It reads the text the way the person receiving it will: they already see
 * who it is from, so "This is Anna writing" tells them nothing; they did not
 * ask how the sender hopes they are, and a sentence said twice reads as a
 * slip. It fixes what it can prove — a doubled sentence, a filler sentence,
 * an introduction the reader can already see, the slips the proof package is
 * sure of — and points at what only the writer can settle: a placeholder
 * left in ("[Name]"), a date whose weekday is wrong, a sentence that sounds
 * like a machine talking about itself.
 *
 * Nothing here needs a model, so the polished text is the same every time
 * and every change can be listed. Nothing here knows any particular person,
 * company or account: the sender's name is whatever the caller says it is.
 */
package polish

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"pn-scripts-assistant/internal/brain/proof"
)

// Draft is a text about to go out.
type Draft struct {
	Text string

	// Lang is "en", "bg" or empty to tell from the text.
	Lang string

	// From is the sender's name as the reader sees it beside the message,
	// or empty when it is not known.
	From string

	// Subject is the subject line the reader sees above the text, if any.
	Subject string
}

// Finding is one thing the last look found. Key names it for the page's
// words ('polish.f.<key>'); Found is the text it is about; Fixed says
// whether the polished text already has it put right.
type Finding struct {
	Key   string `json:"key"`
	Found string `json:"found"`
	Fixed bool   `json:"fixed"`

	// Hint is a correction the writer may want, when one is known — the
	// right weekday for a date.
	Hint string `json:"hint,omitempty"`
}

// The kinds of finding.
const (
	AlreadyVisible = "already_visible" // tells the reader what they already see
	Filler         = "filler"          // a sentence that says nothing
	Repeated       = "repeated"        // the same sentence twice
	Slip           = "slip"            // a proofreading slip (package proof)
	Placeholder    = "placeholder"     // "[Name]", "{{date}}", "TODO" left in
	MachineVoice   = "machine_voice"   // "as an AI…"
	WrongWeekday   = "wrong_weekday"   // "Monday, 10 November 2026" is a Tuesday
	Empty          = "empty"           // nothing to send
)

// FindingKeys is every key, for the page's strings.
func FindingKeys() []string {
	return []string{AlreadyVisible, Filler, Repeated, Slip, Placeholder, MachineVoice, WrongWeekday, Empty}
}

// Result is the text as it should go, and what changed or needs a look.
type Result struct {
	Text     string    `json:"text"`
	Findings []Finding `json:"findings"`

	// Changed is whether Text differs from what was given.
	Changed bool `json:"changed"`

	// Blocking is whether something is left that must not reach a reader:
	// a placeholder, or no text at all.
	Blocking bool `json:"blocking"`
}

// Open is the findings not yet put right.
func (r Result) Open() []Finding {
	var out []Finding

	for _, f := range r.Findings {
		if !f.Fixed {
			out = append(out, f)
		}
	}

	return out
}

// Polish gives the last look to a draft. now is today, for dates said
// without a year.
func Polish(d Draft, now time.Time) Result {
	original := d.Text
	text := strings.TrimSpace(normaliseNewlines(d.Text))

	if text == "" {
		return Result{Text: "", Findings: []Finding{{Key: Empty}}, Blocking: true}
	}

	lang := d.Lang
	if lang != "en" && lang != "bg" {
		lang = proof.Language(text)
	}

	var found []Finding

	// Whole sentences first, so the proofreading reads what is left.
	text, found = dropSentences(text, d, found)

	slips := proof.Check(text, lang)
	if fixed := proof.Apply(text, slips); fixed != text {
		for _, s := range slips {
			if s.Sure {
				found = append(found, Finding{Key: Slip, Found: s.Found, Hint: s.Suggest, Fixed: true})
			}
		}

		text = fixed
	}

	for _, s := range slips {
		if !s.Sure {
			found = append(found, Finding{Key: Slip, Found: s.Found, Hint: s.Suggest})
		}
	}

	found = append(found, placeholders(text)...)
	found = append(found, machineVoice(text)...)
	found = append(found, weekdays(text, now)...)

	text = tidyBlankLines(text)

	r := Result{Text: text, Findings: found, Changed: text != strings.TrimSpace(normaliseNewlines(original))}

	for _, f := range found {
		if f.Key == Placeholder {
			r.Blocking = true
		}
	}

	if r.Findings == nil {
		r.Findings = []Finding{}
	}

	return r
}

func normaliseNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

/*
 * dropSentences takes out the sentences that only cost the reader time: an
 * introduction of the sender they can already see, the subject said again
 * as the first line, the stock fillers, and a sentence said a second time.
 * Paragraphs and line breaks stay where they were.
 */
func dropSentences(text string, d Draft, found []Finding) (string, []Finding) {
	seen := map[string]bool{}

	var paragraphs []string

	for pi, para := range strings.Split(text, "\n") {
		var kept []string

		for _, s := range proof.Sentences(para) {
			sentence := s.Text
			key := normalised(sentence)

			switch {
			case pi == 0 && len(kept) == 0 && d.Subject != "" && key == normalised(d.Subject):
				found = append(found, Finding{Key: AlreadyVisible, Found: sentence, Fixed: true})

				continue
			case introducesSender(sentence, d.From):
				found = append(found, Finding{Key: AlreadyVisible, Found: sentence, Fixed: true})

				continue
			case isFiller(sentence):
				found = append(found, Finding{Key: Filler, Found: sentence, Fixed: true})

				continue
			case key != "" && seen[key] && utf8.RuneCountInString(key) > 12:
				found = append(found, Finding{Key: Repeated, Found: sentence, Fixed: true})

				continue
			}

			seen[key] = true
			kept = append(kept, sentence)
		}

		// A line that was a greeting and nothing else stays as it was; a
		// line emptied by the above goes, rather than leaving a gap.
		if len(kept) == 0 && strings.TrimSpace(para) != "" {
			continue
		}

		paragraphs = append(paragraphs, strings.Join(kept, " "))
	}

	return strings.Join(paragraphs, "\n"), found
}

// normalised is a sentence for comparing: lower case, letters and digits.
func normalised(s string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}

	return b.String()
}

var (
	// "This is Anna writing", "My name is Anna", „Пише ви Анна", „Казвам се
	// Анна" — said by the sender about the sender.
	// Short and whole: "Hi, this is Anna from the shop." A sentence that
	// goes on — "…, about your order" — says something and is left.
	introEN = regexp.MustCompile(`(?i)^(hi|hello|dear [^,]+)?,?\s*(this is|my name is|i am|i'm|it'?s)\s+([^,;:]{1,40}?)(\s+(writing|here|from [^,;:]{1,40}))?[.!]?$`)
	introBG = regexp.MustCompile(`(?i)^(здравейте|здравей|добър ден)?,?\s*(пише ви|казвам се|аз съм|обажда се)\s+([^,;:]{1,60}?)[.!]?$`)

	// Who is writing, said without a name: always visible beside the text.
	writingOnBehalf = regexp.MustCompile(`(?i)^(i am|i'm) writing (to you )?on behalf of .+[.!]?$|^пиша ви от (името на )?.+[.!]?$`)
)

// introducesSender is a sentence that tells the reader who the sender is,
// which the reader sees before they read a word.
func introducesSender(sentence, from string) bool {
	s := strings.TrimSpace(sentence)

	if writingOnBehalf.MatchString(s) {
		return true
	}

	for _, re := range []*regexp.Regexp{introEN, introBG} {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}

		who := strings.TrimSpace(m[3])

		// „Пише ви …" is always the sender: the phrase says nothing else.
		if re == introBG && strings.HasPrefix(strings.ToLower(m[2]), "пише ви") {
			return true
		}

		if from != "" && namesTheSame(who, from) {
			return true
		}
	}

	return false
}

// namesTheSame reports whether who is the sender's name or the start of it
// ("Anna" for "Anna Petrova", "Anna Petrova from the shop").
func namesTheSame(who, from string) bool {
	who, from = normalised(firstWords(who, 3)), normalised(from)
	if who == "" || from == "" {
		return false
	}

	return strings.HasPrefix(who, from) || strings.HasPrefix(from, who)
}

func firstWords(s string, n int) string {
	fields := strings.Fields(s)
	if len(fields) > n {
		fields = fields[:n]
	}

	return strings.Join(fields, " ")
}

// Sentences that say nothing to the reader, whole. A sentence that only
// starts the same way is left: it may go on to say something.
var fillers = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^i hope (this|my) (e-?mail|message|letter|note) finds you well[.!]?$`),
	regexp.MustCompile(`(?i)^i hope (you are|you're|all is) (well|doing well|good)[.!]?$`),
	regexp.MustCompile(`(?i)^(please )?(do not|don't) hesitate to (contact|reach out to|get in touch with) (me|us)( if you have any (questions|concerns))?[.!]?$`),
	regexp.MustCompile(`(?i)^(please )?feel free to (contact|reach out to) (me|us)( with any questions)?[.!]?$`),
	regexp.MustCompile(`(?i)^i am writing to you( today)?[.!]?$`),
	regexp.MustCompile(`(?i)^надявам се,? че (сте|си) (добре|добри)[.!]?$`),
	regexp.MustCompile(`(?i)^надявам се,? (че )?(това |моето )?(писмо|съобщение) (ви|те) (намира|заварва) (в добро здраве|добре)[.!]?$`),
	regexp.MustCompile(`(?i)^не се колебайте да (се свържете|ми пишете|ни пишете|се обърнете)( с (мен|нас))?( при (въпроси|нужда))?[.!]?$`),
	regexp.MustCompile(`(?i)^пиша ви( днес)?[.!]?$`),
}

func isFiller(sentence string) bool {
	s := strings.TrimSpace(sentence)

	for _, re := range fillers {
		if re.MatchString(s) {
			return true
		}
	}

	return false
}

var placeholderLike = regexp.MustCompile(`(?i)\[(your |the )?(name|company|date|address|phone|email|title|име|фирма|дата|адрес|телефон)[^\]]{0,30}\]|\{\{[^}]{1,40}\}\}|<(your |the )?(name|company|date)>|\bTODO\b|\bTBD\b|\bXXX\b|lorem ipsum`)

func placeholders(text string) []Finding {
	var out []Finding

	for _, m := range placeholderLike.FindAllString(text, -1) {
		out = append(out, Finding{Key: Placeholder, Found: m})
	}

	return out
}

var machineLike = regexp.MustCompile(`(?i)\bas an ai\b[^.!?\n]*|\bas a (large )?language model\b[^.!?\n]*|(?:^|[^\p{L}])като (изкуствен интелект|езиков модел)[^.!?\n]*`)

func machineVoice(text string) []Finding {
	var out []Finding

	for _, m := range machineLike.FindAllString(text, -1) {
		out = append(out, Finding{Key: MachineVoice, Found: strings.TrimLeftFunc(m, func(r rune) bool { return !unicode.IsLetter(r) })})
	}

	return out
}

var (
	dayNamesEN = map[string]time.Weekday{"monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
		"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday, "sunday": time.Sunday}
	dayNamesBG = map[string]time.Weekday{"понеделник": time.Monday, "вторник": time.Tuesday, "сряда": time.Wednesday,
		"четвъртък": time.Thursday, "петък": time.Friday, "събота": time.Saturday, "неделя": time.Sunday}
	monthsEN = map[string]time.Month{"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
		"july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12}
	monthsBG = map[string]time.Month{"януари": 1, "февруари": 2, "март": 3, "април": 4, "май": 5, "юни": 6,
		"юли": 7, "август": 8, "септември": 9, "октомври": 10, "ноември": 11, "декември": 12}

	// "Monday, 10 November 2026", "Monday 10 November", "понеделник, 10 ноември 2026 г."
	dateDayFirst = regexp.MustCompile(`(?i)(\p{L}+),?\s+(\d{1,2})(?:st|nd|rd|th)?\s+(\p{L}+)(?:\s+(\d{4}))?`)
	// "Monday, November 10, 2026"
	dateMonthFirst = regexp.MustCompile(`(?i)(\p{L}+),?\s+(\p{L}+)\s+(\d{1,2})(?:st|nd|rd|th)?(?:,?\s+(\d{4}))?`)
)

// weekdays finds a date said with its weekday where the two disagree. A date
// without a year is taken as the next one from now (or today's).
func weekdays(text string, now time.Time) []Finding {
	var out []Finding

	check := func(whole, dayWord, dayNum, monthWord, year string) {
		day, ok := dayNamesEN[strings.ToLower(dayWord)]
		if !ok {
			if day, ok = dayNamesBG[strings.ToLower(dayWord)]; !ok {
				return
			}
		}

		month, ok := monthsEN[strings.ToLower(monthWord)]
		if !ok {
			if month, ok = monthsBG[strings.ToLower(monthWord)]; !ok {
				return
			}
		}

		n, err := strconv.Atoi(dayNum)
		if err != nil || n < 1 || n > 31 {
			return
		}

		y := now.Year()
		if year != "" {
			y, _ = strconv.Atoi(year)
		}

		date := time.Date(y, month, n, 12, 0, 0, 0, time.UTC)
		if date.Day() != n {
			return // 31 February: not a date at all, and not this check's business
		}

		if year == "" && date.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)) {
			date = date.AddDate(1, 0, 0)
		}

		if date.Weekday() != day {
			out = append(out, Finding{Key: WrongWeekday, Found: strings.TrimSpace(whole),
				Hint: weekdayName(date.Weekday(), dayWord)})
		}
	}

	for _, m := range dateDayFirst.FindAllStringSubmatch(text, -1) {
		check(m[0], m[1], m[2], m[3], m[4])
	}

	for _, m := range dateMonthFirst.FindAllStringSubmatch(text, -1) {
		check(m[0], m[1], m[3], m[2], m[4])
	}

	return out
}

// weekdayName is d in the language the wrong one was written in.
func weekdayName(d time.Weekday, like string) string {
	if _, bg := dayNamesBG[strings.ToLower(like)]; bg {
		for name, w := range dayNamesBG {
			if w == d {
				return name
			}
		}
	}

	return d.String()
}

// tidyBlankLines leaves at most one empty line between paragraphs.
func tidyBlankLines(text string) string {
	lines := strings.Split(text, "\n")

	var out []string

	blank := 0

	for _, l := range lines {
		l = strings.TrimRight(l, " \t")

		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}

		out = append(out, l)
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}
