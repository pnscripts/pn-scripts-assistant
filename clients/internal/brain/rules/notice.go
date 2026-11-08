package rules

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"pn-scripts-assistant/internal/brain/proof"
)

/*
 * Notice finds a standing instruction in something its user said, and
 * proposes it as a rule.
 *
 * Only standing ones: "always", "never", "from now on", "every time",
 * "ask me before", and the complaints that mean "stop doing that" ("it's
 * pointless to…", „тъпо е да…"). A one-off — "don't send it yet" — is an
 * instruction for this turn, and proposing it as a rule would teach its user
 * to decline cards without reading them. A question is never a rule, and
 * neither is "I always forget", which is about its speaker rather than the
 * assistant.
 *
 * The proposal is the sentence as it was said: the user is agreeing to their
 * own words, and can change them before keeping them.
 */
func Notice(message string) (Proposal, bool) {
	for _, s := range proof.Sentences(message) {
		if p, ok := noticeIn(s.Text); ok {
			p.Quote = strings.TrimSpace(message)
			if utf8.RuneCountInString(p.Quote) > MaxQuote {
				p.Quote = string([]rune(p.Quote)[:MaxQuote]) + "…"
			}

			return p, true
		}
	}

	return Proposal{}, false
}

// MaxQuote is how much of the message a rule came from is kept with it.
const MaxQuote = 600

func noticeIn(sentence string) (Proposal, bool) {
	sentence = strings.TrimSpace(sentence)

	if sentence == "" || strings.HasSuffix(sentence, "?") || utf8.RuneCountInString(sentence) > MaxText {
		return Proposal{}, false
	}

	low := lowered(sentence)

	if !standing(low) {
		return Proposal{}, false
	}

	text := Clean(sentence)
	actions := ActionsIn(text)

	// "Don't do that again": nothing in it says what, so there is nothing a
	// rule could hold on to. The model answers it; the card would only ask
	// its user to explain themselves.
	if len(actions) == 0 && len(leftovers(text)) == 0 && len(SpecificWords(text)) == 0 {
		return Proposal{}, false
	}

	return Proposal{
		Text:    text,
		Kind:    kindOf(text),
		Actions: actions,
		Words:   SpecificWords(text),
		Days:    DaysIn(text),
	}, true
}

// lowered is s in lower case with single spaces and the opening courtesy
// taken off, which is how every cue below is written.
func lowered(s string) string {
	low := strings.ToLower(strings.Join(strings.Fields(s), " "))
	low = strings.TrimLeft(low, " -–—•*\"'„“”«»")

	for _, p := range []string{"please, ", "please ", "моля те, ", "моля те ", "моля, ", "моля ", "и ", "and ", "ok, ", "ок, "} {
		low = strings.TrimPrefix(low, p)
	}

	return low
}

// Anywhere in the sentence: these only ever mean a standing instruction.
var standingPhrases = []string{
	"from now on", "from here on", "going forward", "in future,", "in the future,",
	"ask me before", "ask me first", "check with me before", "check with me first",
	"without asking me", "without asking first", "without my approval", "without my permission",
	"отсега нататък", "от сега нататък", "оттук нататък", "от тук нататък", "занапред", "за в бъдеще",
	"питай ме преди", "питай ме, преди", "питай ме първо", "първо ме питай", "първо питай", "попитай ме преди",
	"ме питай преди", "ме питай, преди", "ме попитай преди", "ме попитай, преди",
	"без да ме питаш", "без да ме попиташ", "без мое разрешение", "без моето разрешение", "без мое одобрение",
}

/*
 * "Every time…", "whenever…": a standing instruction only when it is about
 * something the assistant does — "whenever you send an invoice, attach the
 * PDF" — and not "whenever it rains, my head aches".
 */
var wheneverPhrases = []string{
	"every time you", "whenever you", "each time you", "every time ", "whenever ",
	"всеки път, когато", "всеки път когато", "винаги когато", "винаги, когато", "когато и да",
}

// Complaints that mean "stop doing that".
var complaints = []*regexp.Regexp{
	regexp.MustCompile(`^(it'?s|it is|that'?s|that is) (so |really |just )?(stupid|pointless|silly|annoying|dumb|useless|ridiculous|irritating) (to|when you) `),
	regexp.MustCompile(`^i (hate|can'?t stand) (it )?when you `),
	regexp.MustCompile(`^stop \p{L}+ing\b`),
	regexp.MustCompile(`^(много |просто |наистина )?(тъпо|глупаво|безсмислено|излишно|досадно|дразнещо) е,? (да|когато|че) `),
	regexp.MustCompile(`^(много )?(ме )?дразни,? (ме,? )?когато `),
	regexp.MustCompile(`^мразя,? когато `),
	regexp.MustCompile(`^(спри|престани|стига) да `),
}

// Words that make a "don't" a standing one rather than a one-off.
var standingMarkers = []string{" again", " anymore", " any more", " ever ", " ever.", " повече", " вече ", " никога"}

func standing(low string) bool {
	for _, p := range standingPhrases {
		if strings.Contains(low, p) {
			return true
		}
	}

	for _, re := range complaints {
		if re.MatchString(low) {
			return true
		}
	}

	for _, p := range wheneverPhrases {
		if strings.HasPrefix(low, p) || strings.Contains(low, " "+p) {
			return len(ActionsIn(low)) > 0
		}
	}

	first, rest := firstWord(low)

	switch first {
	case "always", "never":
		return aboutTheAssistantEN(first, rest)
	case "винаги", "никога":
		return aboutTheAssistantBG(rest)
	case "don't", "dont", "do", "не":
		if first == "do" && !strings.HasPrefix(rest, "not ") {
			return false
		}

		padded := " " + low + " "

		for _, m := range standingMarkers {
			if strings.Contains(padded, m) {
				return true
			}
		}
	}

	return false
}

func firstWord(s string) (string, string) {
	s = strings.TrimSpace(s)

	if i := strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == ',' }); i >= 0 {
		return s[:i], strings.TrimLeft(s[i:], " ,")
	}

	return s, ""
}

// "Always the same", "never mind", "never heard of it": a remark, not an
// instruction. An instruction goes on with what to do, so the word after
// "always" or "never" must be a verb said as an instruction.
func aboutTheAssistantEN(_ string, rest string) bool {
	next, _ := firstWord(rest)

	return imperativesEN[strings.Trim(next, ".,;:!")]
}

// The verbs people give instructions with, in the form they give them.
var imperativesEN = set(
	"accept", "add", "address", "agree", "answer", "approve", "ask", "assume", "attach", "avoid", "be",
	"bcc", "book", "bring", "buy", "call", "cc", "change", "charge", "check", "cite", "close", "come",
	"commit", "confirm", "contact", "copy", "create", "cut", "delete", "do", "double-check", "download",
	"drop", "edit", "email", "e-mail", "explain", "file", "fill", "follow", "forward", "give", "go",
	"include", "install", "keep", "leave", "let", "list", "log", "look", "make", "mark", "mention",
	"merge", "message", "move", "name", "offer", "open", "order", "pay", "pick", "post", "print",
	"promise", "proofread", "publish", "push", "put", "quote", "read", "record", "remember", "remove",
	"rename", "reply", "report", "respond", "round", "run", "save", "say", "schedule", "sell", "send",
	"set", "share", "show", "sign", "skip", "sort", "speak", "spend", "split", "start", "stop", "submit",
	"suggest", "summarise", "summarize", "switch", "take", "talk", "tell", "test", "text", "thank",
	"translate", "trust", "try", "turn", "type", "unsubscribe", "update", "upload", "use", "verify",
	"wait", "waste", "write", "work",
)

// Bulgarian clitics that stand between "винаги" and its verb.
var clitics = map[string]bool{
	"ме": true, "ми": true, "те": true, "ти": true, "го": true, "я": true, "ги": true, "им": true,
	"се": true, "си": true, "да": true, "не": true, "ще": true, "му": true, "ѝ": true, "ни": true, "ви": true,
}

/*
 * „Винаги проверявай" is an instruction; „винаги забравям", „никога не
 * съм бил" and „винаги имаше" are about somebody. The first word after the
 * clitics must be said as an instruction: the imperative ends in -й or -и
 * (-ете, -айте when polite), which no first-person or past form does, and an
 * auxiliary ("е", "сме", "беше") is never one.
 */
func aboutTheAssistantBG(rest string) bool {
	for _, w := range strings.Fields(rest) {
		w = strings.Trim(w, ".,;:!")

		if clitics[w] {
			continue
		}

		if w == "" || auxiliariesBG[w] {
			return false
		}

		for _, end := range []string{"й", "и", "ете", "йте"} {
			if strings.HasSuffix(w, end) {
				return true
			}
		}

		return false
	}

	return false
}

var auxiliariesBG = set("е", "са", "сме", "сте", "съм", "бях", "беше", "бяха", "бил", "била", "било", "били",
	"бъде", "бъдат", "има", "няма", "имаше", "нямаше", "ли", "би", "бихте", "бихме")

// kindOf reads which kind a rule's wording asks for.
func kindOf(text string) Kind {
	low := lowered(text)

	for _, p := range []string{
		"ask me", "check with me", "without asking", "without my approval", "without my permission",
		"питай ме", "попитай ме", "ме питай", "ме попитай", "първо питай", "без да ме питаш", "без да ме попиташ", "без да питаш",
		"без мое", "без моето",
	} {
		if strings.Contains(low, p) {
			return AskFirst
		}
	}

	for _, re := range complaints {
		if re.MatchString(low) {
			return Never
		}
	}

	padded := " " + low + " "

	for _, p := range []string{" never ", " don't ", " dont ", " do not ", " stop ", " никога ", " не ", " спри ", " престани "} {
		if strings.Contains(padded, p) {
			return Never
		}
	}

	return Always
}

/*
 * The words of each kind of action. Verbs first: "never delete files" is
 * about deleting, and "files" is only what is deleted. A kind is read from
 * its nouns only when the sentence has no verb of any kind ("no emails on
 * Sundays"). Every entry is the start of a word, so one entry covers the
 * forms of a Bulgarian verb.
 */
var actionVerbs = map[string][]string{
	Send:     {"send", "sent", "email", "e-mail", "mail", "text ", "изпрат", "изпращ", "прати", "праща", "пращай", "имейл", "мейл"},
	Post:     {"post", "publish", "share", "tweet", "comment", "публикув", "сподел", "постн", "поств", "коментир"},
	Commit:   {"commit", "push", "merge", "къмит", "комит", "пушв", "пушн", "мърдж"},
	Files:    {"save", "overwrite", "edit", "запис", "запаз", "презапис", "редакт"},
	Purchase: {"buy", "pay", "purchas", "order", "subscrib", "checkout", "купу", "купи", "плащ", "плати", "плат", "поръч", "абонир"},
	Delete:   {"delet", "remov", "eras", "wipe", "trash", "изтри", "трий", "трие", "махн", "премахн", "унищож"},
}

var actionNouns = map[string][]string{
	Send:     {"message", "letter", "reply", "replies", "писм", "съобщени", "отговор"},
	Post:     {"posts", "comments", "пост", "коментар", "публикаци"},
	Commit:   {"commits", "branch", "repo", "клон", "хранилищ"},
	Files:    {"file", "folder", "document", "файл", "папк", "документ"},
	Purchase: {"payment", "purchase", "subscription", "плащане", "покупк", "абонамент"},
}

// "Write to Anna" is sending, not a file.
var sendPhrases = []string{"write to ", "reply to ", "пиши на ", "пишеш на ", "пишете на "}

// ActionsIn is which kinds of action a rule's wording is about.
func ActionsIn(text string) []string {
	low := " " + lowered(text) + " "
	tokens := tokensOf(low)

	found := map[string]bool{}

	for _, a := range Actions() {
		if hasStem(tokens, low, actionVerbs[a]) {
			found[a] = true
		}
	}

	for _, p := range sendPhrases {
		if strings.Contains(low, " "+p) {
			found[Send] = true
		}
	}

	if len(found) == 0 {
		for _, a := range Actions() {
			if hasStem(tokens, low, actionNouns[a]) {
				found[a] = true
			}
		}
	}

	out := []string{}

	for _, a := range Actions() {
		if found[a] {
			out = append(out, a)
		}
	}

	return out
}

func hasStem(tokens []string, low string, stems []string) bool {
	for _, s := range stems {
		if strings.HasSuffix(s, " ") {
			if strings.Contains(low, " "+s) {
				return true
			}

			continue
		}

		for _, t := range tokens {
			if strings.HasPrefix(t, s) {
				return true
			}
		}
	}

	return false
}

func tokensOf(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '’' || r == '-')
	})
}

// The days of the week by the starts of their names; the weekend and the
// working week by theirs.
var dayStems = []struct {
	stem string
	days []time.Weekday
}{
	{"monday", []time.Weekday{time.Monday}}, {"tuesday", []time.Weekday{time.Tuesday}},
	{"wednesday", []time.Weekday{time.Wednesday}}, {"thursday", []time.Weekday{time.Thursday}},
	{"friday", []time.Weekday{time.Friday}}, {"saturday", []time.Weekday{time.Saturday}},
	{"sunday", []time.Weekday{time.Sunday}},
	{"weekend", []time.Weekday{time.Saturday, time.Sunday}},
	{"weekday", []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}},
	{"понеделни", []time.Weekday{time.Monday}}, {"вторни", []time.Weekday{time.Tuesday}},
	{"сряда", []time.Weekday{time.Wednesday}}, {"срядата", []time.Weekday{time.Wednesday}},
	{"сряди", []time.Weekday{time.Wednesday}}, {"четвъртъ", []time.Weekday{time.Thursday}},
	{"петък", []time.Weekday{time.Friday}}, {"петъц", []time.Weekday{time.Friday}},
	{"събот", []time.Weekday{time.Saturday}}, {"недел", []time.Weekday{time.Sunday}},
	{"уикенд", []time.Weekday{time.Saturday, time.Sunday}},
	{"почивни", []time.Weekday{time.Saturday, time.Sunday}},
	{"делни", []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}},
}

// DaysIn is the days of the week a rule's wording names.
func DaysIn(text string) []time.Weekday {
	var days []time.Weekday

	for _, t := range tokensOf(text) {
		for _, d := range dayStems {
			if strings.HasPrefix(t, d.stem) {
				days = append(days, d.days...)
			}
		}
	}

	return cleanDays(days)
}

var (
	emailLike  = regexp.MustCompile(`[\p{L}\p{N}._%+-]+@[\p{L}\p{N}-]+(\.[\p{L}\p{N}-]+)+`)
	handleLike = regexp.MustCompile(`(^|\s)@[\p{L}\p{N}_.]{2,}`)
	dottedLike = regexp.MustCompile(`(?i)\b[\p{L}\p{N}][\p{L}\p{N}_-]*(\.[\p{L}\p{N}_-]+)*\.[a-z]{2,6}\b`)
	quotedLike = regexp.MustCompile(`["„“”«]([^"„“”«»]{2,60})["“”»]`)
	wordLike   = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}'’-]*`)
)

/*
 * SpecificWords is what in a rule's wording names one thing: an address, a
 * link or file name, a @handle, a quoted phrase, a capitalised name in the
 * middle of a sentence. These narrow a rule to the actions that mention
 * them. Languages and days are not names here: "never write in English" is
 * not about actions that contain the word "English".
 */
func SpecificWords(text string) []string {
	var out []string

	taken := map[string]bool{}

	add := func(w string) {
		w = strings.TrimSpace(strings.Trim(w, ".,;:!?"))
		if w == "" || taken[strings.ToLower(w)] {
			return
		}

		taken[strings.ToLower(w)] = true
		out = append(out, w)
	}

	for _, m := range emailLike.FindAllString(text, -1) {
		add(m)
	}

	for _, m := range handleLike.FindAllString(text, -1) {
		add(m)
	}

	for _, m := range dottedLike.FindAllString(text, -1) {
		if !strings.Contains(strings.ToLower(text), "@"+strings.ToLower(m)) && !partOfTaken(m, taken) {
			add(m)
		}
	}

	for _, m := range quotedLike.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}

	for i, loc := range wordLike.FindAllStringIndex(text, -1) {
		w := text[loc[0]:loc[1]]

		first, _ := utf8.DecodeRuneInString(w)
		if i == 0 || !unicode.IsUpper(first) || w == "I" {
			continue
		}

		low := strings.ToLower(w)
		if languages[low] || len(DaysIn(w)) > 0 || months[low] || stopwords[low] || insideTaken(text, loc, taken) {
			continue
		}

		add(w)
	}

	return cleanWords(out)
}

func partOfTaken(m string, taken map[string]bool) bool {
	for t := range taken {
		if strings.Contains(t, strings.ToLower(m)) {
			return true
		}
	}

	return false
}

func insideTaken(text string, loc []int, taken map[string]bool) bool {
	w := strings.ToLower(text[loc[0]:loc[1]])

	for t := range taken {
		if strings.Contains(t, w) && t != w {
			return true
		}
	}

	return false
}

/*
 * leftovers is the words of a rule's wording that say more than a kind of
 * action, a day or a name: the conditions only a reader can weigh. Cue words,
 * courtesy, pronouns and words like "anything" carry none.
 */
func leftovers(text string) []string {
	specific := map[string]bool{}

	for _, w := range SpecificWords(text) {
		for _, t := range tokensOf(w) {
			specific[t] = true
		}
	}

	low := " " + lowered(text) + " "

	var out []string

	seen := map[string]bool{}

	for _, t := range tokensOf(low) {
		t = strings.Trim(t, "'’-")

		switch {
		case utf8.RuneCountInString(t) < 3, stopwords[t], specific[t], seen[t]:
			continue
		case len(DaysIn(t)) > 0:
			continue
		case isActionWord(t):
			continue
		}

		seen[t] = true
		out = append(out, t)
	}

	return out
}

func isActionWord(t string) bool {
	for _, a := range Actions() {
		for _, list := range [][]string{actionVerbs[a], actionNouns[a]} {
			for _, s := range list {
				if !strings.HasSuffix(s, " ") && strings.HasPrefix(t, s) {
					return true
				}
			}
		}
	}

	return false
}

var languages = set("english", "bulgarian", "german", "french", "spanish", "italian", "russian", "greek",
	"turkish", "romanian", "serbian", "polish", "dutch", "portuguese", "chinese", "japanese", "british", "american")

var months = set("january", "february", "march", "april", "may", "june", "july", "august", "september",
	"october", "november", "december", "януари", "февруари", "март", "април", "май", "юни", "юли", "август",
	"септември", "октомври", "ноември", "декември")

// Words that carry no condition of their own.
var stopwords = set(
	// English
	"the", "a", "an", "and", "or", "but", "to", "of", "in", "on", "at", "for", "with", "by", "from", "about",
	"as", "into", "onto", "than", "then", "that", "this", "these", "those", "it", "its", "it's", "is", "are",
	"be", "been", "was", "were", "do", "does", "did", "don't", "dont", "doesn't", "not", "no", "never",
	"always", "ever", "again", "anymore", "any", "more", "anything", "something", "everything", "nothing",
	"all", "every", "each", "one", "ones", "me", "my", "mine", "you", "your", "yours", "i", "we", "us", "our",
	"he", "she", "they", "them", "their", "please", "now", "from", "here", "going", "forward", "future",
	"time", "times", "whenever", "when", "before", "after", "first", "without", "ask", "asking", "asked",
	"check", "checking", "permission", "approval", "approve", "let", "know", "tell", "so", "just", "even",
	"stop", "stupid", "pointless", "silly", "annoying", "dumb", "useless", "ridiculous", "irritating",
	"hate", "can't", "cannot", "can", "will", "would", "should", "must", "have", "has", "had", "get",
	"make", "sure", "really", "very", "who", "what", "which", "where", "why", "how", "there", "on", "off",
	"way", "things", "thing", "stuff", "anyone", "anybody", "someone", "somebody", "write", "doing",
	// Bulgarian
	"и", "или", "но", "а", "да", "не", "се", "си", "ме", "ми", "те", "ти", "го", "я", "ги", "им", "ни",
	"ви", "на", "за", "в", "във", "с", "със", "от", "до", "по", "при", "без", "преди", "след", "когато",
	"като", "че", "това", "този", "тази", "тези", "той", "тя", "то", "те", "всичко", "нищо", "нещо",
	"някакво", "никакво", "всеки", "всяко", "всяка", "всички", "път", "пъти", "повече", "вече", "моля",
	"питай", "питаш", "попитай", "попиташ", "първо", "разрешение", "одобрение", "съгласие", "мое", "моето",
	"мен", "мой", "моя", "моите", "аз", "вие", "нас", "наш", "нашите", "винаги", "никога", "отсега",
	"сега", "нататък", "оттук", "тук", "занапред", "бъдеще", "кажи", "казвай", "знам", "искам", "трябва",
	"може", "можеш", "ще", "бъде", "е", "са", "съм", "си", "сме", "сте", "беше", "тъпо", "глупаво",
	"безсмислено", "излишно", "досадно", "дразнещо", "дразни", "мразя", "спри", "престани", "стига",
	"прави", "правиш", "правите", "направи", "пиши", "пишеш", "просто", "много", "наистина", "така",
	"кой", "какво", "кога", "къде", "защо", "как", "някой", "никой", "всеки", "мене", "тебе",
)

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}

	return m
}
