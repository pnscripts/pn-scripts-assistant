package proof

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

/*
 * The Bulgarian rules: the ones a careful reader notices first and a spell
 * checker does not, because every word in them is spelled correctly.
 */
func bulgarian(text string) []Finding {
	var out []Finding

	list := words(text)

	for i, w := range list {
		lower := strings.ToLower(w.text)

		var next, prev *word
		if i+1 < len(list) && onlySpace(between(text, w, list[i+1])) {
			next = &list[i+1]
		}

		if i > 0 && onlySpace(between(text, list[i-1], w)) {
			prev = &list[i-1]
		}

		// "в" and "във", "с" and "със": the longer one before a word that
		// starts with the same sound, the shorter one everywhere else.
		if next != nil {
			if f, ok := vavSas(w, lower, *next); ok {
				out = append(out, f)
			}
		}

		// The short pronoun is written ѝ, with the accent, to tell it from
		// the conjunction-like й inside words.
		if lower == "й" {
			out = append(out, Finding{
				Rule: "short_pronoun", At: w.at, Length: w.end - w.at, Found: w.text, Suggest: keepCase(w.text, "ѝ"),
				Why:   "The short pronoun is written ѝ.",
				WhyBG: "Кратката форма на местоимението се пише „ѝ“.",
				Sure:  true,
			})
		}

		// "не" is written apart from a verb.
		if fixed, ok := joinedNe[lower]; ok {
			out = append(out, Finding{
				Rule: "ne_apart", At: w.at, Length: w.end - w.at, Found: w.text, Suggest: keepCase(w.text, fixed),
				Why:   "\"не\" is written apart from the verb.",
				WhyBG: "Отрицателната частица „не“ се пише отделно от глагола.",
				Sure:  true,
			})
		}

		// "най" and "по" for comparison are joined with a hyphen.
		if next != nil && (lower == "най" || (lower == "по" && comparable[strings.ToLower(next.text)])) {
			span := text[w.at:next.end]

			out = append(out, Finding{
				Rule: "comparison_hyphen", At: w.at, Length: next.end - w.at, Found: span,
				Suggest: w.text + "-" + next.text,
				Why:     "\"по\" and \"най\" in a comparison are joined to the word with a hyphen.",
				WhyBG:   "„По“ и „най“ за степенуване се пишат слято с думата чрез малко тире.",
				Sure:    true,
			})
		}

		// The full article after a preposition: the noun is not the subject,
		// so it takes the short article.
		if prev != nil && prepositions[strings.ToLower(prev.text)] {
			if f, ok := fullArticle(w, lower); ok {
				out = append(out, f)
			}
		}

		// A comma before "че", "защото", "но", "който" and the like.
		if f, ok := commaBefore(text, list, i, lower); ok {
			out = append(out, f)
		}
	}

	out = append(out, bulgarianQuotes(text)...)

	return out
}

func vavSas(w word, lower string, next word) (Finding, bool) {
	first, _ := utf8.DecodeRuneInString(strings.ToLower(next.text))
	if !unicode.Is(unicode.Cyrillic, first) {
		return Finding{}, false
	}

	var want string

	switch lower {
	case "в":
		if first == 'в' || first == 'ф' {
			want = "във"
		}
	case "във":
		if first != 'в' && first != 'ф' {
			want = "в"
		}
	case "с":
		if first == 'с' || first == 'з' {
			want = "със"
		}
	case "със":
		if first != 'с' && first != 'з' {
			want = "с"
		}
	}

	if want == "" {
		return Finding{}, false
	}

	return Finding{
		Rule: "v_vav_s_sas", At: w.at, Length: w.end - w.at, Found: w.text, Suggest: keepCase(w.text, want),
		Why:   "\"във\" and \"със\" go before a word starting with в/ф and с/з; \"в\" and \"с\" go everywhere else.",
		WhyBG: "„Във“ и „със“ се пишат пред дума, започваща съответно с „в“/„ф“ и „с“/„з“; в останалите случаи — „в“ и „с“.",
		Sure:  true,
	}, true
}

var joinedNe = map[string]string{
	"незнам": "не знам", "незная": "не зная", "немога": "не мога", "неможе": "не може",
	"неможеш": "не можеш", "неможем": "не можем", "неможете": "не можете", "немогат": "не могат",
	"неискам": "не искам", "неискаш": "не искаш", "неиска": "не иска", "нетрябва": "не трябва",
	"незнае": "не знае", "незнаеш": "не знаеш", "незнаем": "не знаем", "незнаят": "не знаят",
	"небих": "не бих", "небеше": "не беше", "небяха": "не бяха", "нестава": "не става",
	"неставаше": "не ставаше", "немогъл": "не могъл", "немогла": "не могла",
}

// Words that "по" makes comparative and which nothing else after "по"
// could mean.
var comparable = map[string]bool{
	"добре": true, "лошо": true, "скоро": true, "рано": true, "късно": true, "често": true,
	"бързо": true, "бавно": true, "лесно": true, "трудно": true, "важно": true, "добър": true,
	"добра": true, "добро": true, "добри": true, "добрият": true, "добрата": true, "доброто": true,
	"добрите": true, "голям": true, "голяма": true, "голямо": true, "големи": true, "малък": true,
	"малка": true, "малки": true, "лесен": true, "лесна": true, "лесни": true, "висок": true,
	"висока": true, "високо": true, "високи": true, "нисък": true, "ниска": true, "ниско": true,
	"ниски": true, "силен": true, "силна": true, "силно": true, "слаб": true, "слаба": true, "слабо": true,
	"евтин": true, "евтина": true, "евтино": true, "скъп": true, "скъпа": true, "скъпо": true,
	"красив": true, "красива": true, "красиво": true, "интересен": true, "интересно": true,
	"удобен": true, "удобна": true, "удобно": true, "надолу": true, "нагоре": true, "напред": true,
	"назад": true, "далече": true, "далеч": true, "близо": true, "надълго": true, "нашироко": true,
	"вероятно": true, "подробно": true, "точно": true,
}

var prepositions = map[string]bool{
	"в": true, "във": true, "на": true, "за": true, "от": true, "до": true, "при": true, "по": true,
	"под": true, "над": true, "пред": true, "без": true, "към": true, "през": true, "с": true,
	"със": true, "у": true, "след": true, "около": true, "срещу": true, "чрез": true, "покрай": true,
	"заради": true, "сред": true, "въпреки": true, "спрямо": true, "според": true, "зад": true,
}

// Words ending in -ът or -ят whose ending is not an article.
var notArticle = map[string]bool{
	"свят": true, "път": true, "кът": true, "прът": true, "лят": true, "отнят": true, "поят": true,
	"смеят": true, "стоят": true, "пеят": true, "траят": true, "спят": true, "крият": true, "мият": true,
	"лаят": true, "вият": true, "шият": true, "пият": true, "сеят": true, "бият": true, "грят": true,
	"умрят": true, "оят": true, "дотрят": true, "запят": true, "изпят": true, "разпят": true,
	"клеят": true, "таят": true, "веят": true, "реят": true,
}

func fullArticle(w word, lower string) (Finding, bool) {
	if utf8.RuneCountInString(lower) < 5 || notArticle[lower] {
		return Finding{}, false
	}

	var short string

	switch {
	case strings.HasSuffix(lower, "ът"):
		short = w.text[:len(w.text)-len("ът")] + keepTail(w.text, "а")
	case strings.HasSuffix(lower, "ят"):
		short = w.text[:len(w.text)-len("ят")] + keepTail(w.text, "я")
	default:
		return Finding{}, false
	}

	return Finding{
		Rule: "full_article", At: w.at, Length: w.end - w.at, Found: w.text, Suggest: short,
		Why:   "After a preposition the word is not the subject, so it takes the short article (-а/-я).",
		WhyBG: "След предлог думата не е подлог, затова се пише с кратък член (-а/-я).",
		Sure:  true,
	}, true
}

// keepTail writes ending in capitals when the word is written in capitals.
func keepTail(word, ending string) string {
	if strings.ToUpper(word) == word {
		return strings.ToUpper(ending)
	}

	return ending
}

/*
 * Commas before the words that start a clause. The rule is firm for these;
 * the exceptions are when the word follows another that already joins the
 * clause ("така че", "въпреки че", "и който") or a preposition ("на който",
 * "от когато"), or starts the sentence.
 */
var clauseWords = map[string]bool{
	"че": true, "защото": true, "но": true, "който": true, "която": true, "което": true, "които": true,
	"чийто": true, "чиято": true, "чието": true, "чиито": true, "когато": true, "където": true,
	"докато": true, "понеже": true, "откъдето": true, "накъдето": true,
}

var joinsAlready = map[string]bool{
	"така": true, "макар": true, "въпреки": true, "освен": true, "само": true, "след": true,
	"без": true, "и": true, "или": true, "ако": true, "но": true, "а": true, "дори": true, "щом": true,
	"понеже": true, "защото": true, "докато": true, "тъй": true, "отколкото": true, "че": true,
	"когато": true, "където": true, "нито": true, "та": true, "пък": true, "именно": true, "сякаш": true,
	"като": true, "да": true, "все": true, "едва": true, "вместо": true, "не": true, "чак": true,
	"още": true, "точно": true, "тъкмо": true, "поне": true, "даже": true, "особено": true,
}

func commaBefore(text string, list []word, i int, lower string) (Finding, bool) {
	if !clauseWords[lower] || i == 0 {
		return Finding{}, false
	}

	prev := list[i-1]
	gap := between(text, prev, list[i])

	// Anything but a space between them — a comma, a dash, a full stop, a
	// line — is not a missing comma.
	if !onlySpace(gap) {
		return Finding{}, false
	}

	p := strings.ToLower(prev.text)
	if joinsAlready[p] || prepositions[p] {
		return Finding{}, false
	}

	// "Не знам кой/кога" are other words; "всичко което" still takes the
	// comma, so nothing more is excused.
	return Finding{
		Rule: "comma_before_clause", At: prev.end, Length: len(gap), Found: gap, Suggest: ", ",
		Why:   "A comma goes before \"" + list[i].text + "\" when it starts a clause.",
		WhyBG: "Пред „" + list[i].text + "“ се пише запетая, когато въвежда подчинено или противопоставително изречение.",
		Sure:  true,
	}, true
}

// Straight quotation marks in Bulgarian text are written „ and “.
var straightQuoted = regexp.MustCompile(`"([^"\n]+)"`)

func bulgarianQuotes(text string) []Finding {
	var out []Finding

	for _, m := range straightQuoted.FindAllStringSubmatchIndex(text, -1) {
		inner := text[m[2]:m[3]]
		if Language(inner) != "bg" {
			continue
		}

		out = append(out, Finding{
			Rule: "bulgarian_quotes", At: m[0], Length: m[1] - m[0], Found: text[m[0]:m[1]],
			Suggest: "„" + inner + "“",
			Why:     "Bulgarian quotation marks are „ and “.",
			WhyBG:   "Българските кавички са „ и “.",
			Sure:    true,
		})
	}

	return out
}

/*
 * The English rules: the slips that pass every spell checker because each
 * word in them is a real word.
 */
func english(text string) []Finding {
	var out []Finding

	list := words(text)

	for i, w := range list {
		lower := strings.ToLower(w.text)

		var next *word
		if i+1 < len(list) && onlySpace(between(text, w, list[i+1])) {
			next = &list[i+1]
		}

		if (lower == "a" || lower == "an") && next != nil {
			if want, ok := article(next.text); ok && want != lower {
				out = append(out, Finding{
					Rule: "a_an", At: w.at, Length: w.end - w.at, Found: w.text, Suggest: keepCase(w.text, want),
					Why:   "\"an\" goes before a vowel sound and \"a\" before a consonant sound.",
					WhyBG: "„An“ се пише пред гласен звук, а „a“ — пред съгласен.",
					Sure:  true,
				})
			}
		}

		if next != nil && strings.ToLower(next.text) == "of" && modals[lower] {
			span := text[next.at:next.end]

			out = append(out, Finding{
				Rule: "of_have", At: next.at, Length: len(span), Found: span, Suggest: keepCase(span, "have"),
				Why:   "\"" + w.text + " have\" is what is meant; \"of\" only sounds like it.",
				WhyBG: "Правилно е „" + w.text + " have“; „of“ само звучи така.",
				Sure:  true,
			})
		}

		if next != nil && strings.ToLower(next.text) == "then" && comparatives[lower] {
			out = append(out, Finding{
				Rule: "then_than", At: next.at, Length: next.end - next.at, Found: next.text,
				Suggest: keepCase(next.text, "than"),
				Why:     "A comparison takes \"than\"; \"then\" is about time.",
				WhyBG:   "При сравнение се пише „than“; „then“ означава „след това“.",
				Sure:    true,
			})
		}

		if w.text == "i" && (i == 0 || text[list[i-1].end:w.at] != ".") && !strings.HasPrefix(text[w.end:], ".") {
			out = append(out, Finding{
				Rule: "capital_i", At: w.at, Length: 1, Found: "i", Suggest: "I",
				Why: "The pronoun \"I\" is always a capital.", WhyBG: "Местоимението „I“ винаги е с главна буква.",
				Sure: true,
			})
		}

		if fixed, ok := englishSlips[lower]; ok {
			out = append(out, Finding{
				Rule: "misspelling", At: w.at, Length: w.end - w.at, Found: w.text, Suggest: keepCase(w.text, fixed),
				Why:   "Usually written \"" + fixed + "\".",
				WhyBG: "Обикновено се пише „" + fixed + "“.",
				Sure:  true,
			})
		}

		if lower == "your" && next != nil && strings.ToLower(next.text) == "welcome" {
			out = append(out, Finding{
				Rule: "your_youre", At: w.at, Length: w.end - w.at, Found: w.text, Suggest: keepCase(w.text, "you're"),
				Why:   "\"You're welcome\" is \"you are welcome\".",
				WhyBG: "„You're welcome“ означава „you are welcome“.",
			})
		}
	}

	return out
}

var modals = map[string]bool{
	"could": true, "would": true, "should": true, "must": true, "might": true,
}

var comparatives = map[string]bool{
	"more": true, "less": true, "better": true, "worse": true, "rather": true, "greater": true,
	"fewer": true, "bigger": true, "smaller": true, "faster": true, "slower": true, "higher": true,
	"lower": true, "larger": true, "older": true, "younger": true, "longer": true, "shorter": true,
	"easier": true, "harder": true, "cheaper": true,
}

var englishSlips = map[string]string{
	"alot": "a lot", "irregardless": "regardless", "definately": "definitely", "seperate": "separate",
	"recieve": "receive", "occured": "occurred", "untill": "until", "wich": "which", "accomodate": "accommodate",
	"neccessary": "necessary", "occurence": "occurrence", "tommorow": "tomorrow", "begining": "beginning",
	"beleive": "believe", "goverment": "government", "enviroment": "environment", "existance": "existence",
}

/*
 * article is "a" or "an" for the word that follows, by its sound rather
 * than its first letter; ok is false when the word is not one this can
 * decide, such as a number.
 */
func article(next string) (string, bool) {
	lower := strings.ToLower(next)
	first, _ := utf8.DecodeRuneInString(lower)

	if !unicode.Is(unicode.Latin, first) {
		return "", false
	}

	// An initialism is said letter by letter: an MBA, a URL, an FBI agent.
	if len(next) >= 2 && len(next) <= 5 && strings.ToUpper(next) == next {
		if strings.ContainsRune("aefhilmnorsx", first) {
			return "an", true
		}

		return "a", true
	}

	for _, silent := range []string{"hour", "honest", "honour", "honor", "heir"} {
		if strings.HasPrefix(lower, silent) {
			return "an", true
		}
	}

	for _, sounded := range []string{"one", "once", "eu", "ewe", "use", "usu", "usa", "uti", "uto", "ure", "uni", "ubiq", "uk"} {
		if strings.HasPrefix(lower, sounded) {
			// "an unimportant", "an uninstall": the un- prefix is a vowel.
			if sounded == "uni" && (strings.HasPrefix(lower, "unin") || strings.HasPrefix(lower, "unim")) {
				return "an", true
			}

			return "a", true
		}
	}

	if strings.ContainsRune("aeiou", first) {
		return "an", true
	}

	return "a", true
}
