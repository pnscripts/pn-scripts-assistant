package proof

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

/*
 * Tone presets: the handful of ways people ask for something to be written.
 *
 * Each has two halves. Brief is the instruction the writer's model is given
 * to rewrite in that tone, in the language of the text. Checks are what can
 * be counted without a model — a contraction in a formal letter, a sentence
 * of forty words in a "concise" one — so a rewrite is checked against the
 * tone it was asked for, and somebody writing by hand gets the same notes.
 */
type Tone struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	LabelBG string `json:"label_bg"`
	Brief   string `json:"brief"`
	BriefBG string `json:"brief_bg"`

	// Longest is the most words a sentence should have in this tone; zero
	// for no limit.
	Longest int `json:"longest,omitempty"`

	// Avoid is words and phrases that do not belong in this tone, by
	// language.
	Avoid   []string `json:"-"`
	AvoidBG []string `json:"-"`

	// NoContractions, NoExclamations and NoShouting are the formal checks.
	NoContractions bool `json:"-"`
	NoExclamations bool `json:"-"`
	NoShouting     bool `json:"-"`
}

// Tones are the presets, in the order they are offered.
var Tones = []Tone{
	{
		Name: "formal", Label: "Formal", LabelBG: "Официален",
		Brief:   "Rewrite in a formal, professional register: complete sentences, no contractions, no slang, no exclamation marks, polite and precise.",
		BriefBG: "Пренапиши в официален, делови стил: пълни изречения, без жаргон и разговорни думи, без удивителни, учтиво и точно, с учтивата форма „Вие“.",
		Longest: 30, NoContractions: true, NoExclamations: true, NoShouting: true,
		Avoid:   []string{"hey", "hi guys", "guys", "gonna", "wanna", "gotta", "okay", "ok", "cool", "awesome", "stuff", "kinda", "yeah", "btw", "asap"},
		AvoidBG: []string{"здрасти", "здрасне", "мерси", "ок", "окей", "супер", "яко", "готино", "бе", "ма", "чао", "абе", "пич"},
	},
	{
		Name: "friendly", Label: "Friendly", LabelBG: "Приятелски",
		Brief:   "Rewrite in a warm, friendly and natural tone, as one person writing to another, keeping every fact.",
		BriefBG: "Пренапиши в топъл, приятелски и естествен тон, като от човек към човек, без да губиш нито един факт.",
		Longest: 30, NoShouting: true,
	},
	{
		Name: "concise", Label: "Concise", LabelBG: "Кратък",
		Brief:   "Rewrite as briefly as the meaning allows: short sentences, no filler, no repetition, every fact kept.",
		BriefBG: "Пренапиши възможно най-кратко, без да се губи смисълът: кратки изречения, без излишни думи и повторения, с всички факти.",
		Longest: 20,
		Avoid:   []string{"very", "really", "actually", "basically", "just", "quite", "in order to", "at this point in time", "due to the fact that", "it is important to note that", "in terms of"},
		AvoidBG: []string{"всъщност", "наистина", "просто", "доста", "абсолютно", "с цел да", "във връзка с това", "на този етап", "трябва да се отбележи, че", "по отношение на", "в общи линии"},
	},
	{
		Name: "persuasive", Label: "Persuasive", LabelBG: "Убедителен",
		Brief:   "Rewrite to persuade: lead with the benefit to the reader, give one concrete reason or example for each claim, end with a clear next step. Claim nothing the text does not support.",
		BriefBG: "Пренапиши убедително: започни с ползата за читателя, дай конкретна причина или пример за всяко твърдение и завърши с ясна следваща стъпка. Не твърди нищо, което текстът не подкрепя.",
		Longest: 25,
		Avoid:   []string{"maybe", "perhaps", "i think", "i guess", "sort of", "kind of", "might be"},
		AvoidBG: []string{"може би", "мисля, че", "евентуално", "един вид", "горе-долу", "като че ли"},
	},
	{
		Name: "simple", Label: "Simple", LabelBG: "Опростен",
		Brief:   "Rewrite in plain, simple language anyone can read: everyday words, short sentences, one idea per sentence, any necessary term explained.",
		BriefBG: "Пренапиши на ясен и прост език, разбираем за всеки: всекидневни думи, кратки изречения, по една мисъл в изречение, с обяснение на нужните термини.",
		Longest: 18,
	},
	{
		Name: "polite", Label: "Polite", LabelBG: "Учтив",
		Brief:   "Rewrite politely and tactfully: thank where it fits, ask rather than demand, soften criticism without changing what is said.",
		BriefBG: "Пренапиши учтиво и тактично: благодари, където е уместно, моли вместо да изискваш, смекчи критиката, без да променяш казаното.",
		Longest: 30, NoExclamations: true, NoShouting: true,
		Avoid:   []string{"you must", "you need to", "obviously", "as i already said", "asap"},
		AvoidBG: []string{"трябва да", "очевидно", "както вече казах", "веднага"},
	},
}

// ToneNamed is the preset called name, if there is one.
func ToneNamed(name string) (Tone, bool) {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, t := range Tones {
		if t.Name == name || strings.EqualFold(t.Label, name) || strings.EqualFold(t.LabelBG, name) {
			return t, true
		}
	}

	return Tone{}, false
}

// BriefIn is the preset's instruction in lang.
func (t Tone) BriefIn(lang string) string {
	if lang == "bg" {
		return t.BriefBG
	}

	return t.Brief
}

var contraction = regexp.MustCompile(`(?i)\b\w+(n't|'re|'ll|'ve|'m|'d)\b|\b(it|that|there|what|he|she|let)'s\b`)

/*
 * CheckTone is the countable half of a tone: sentences longer than it
 * allows, words that do not belong in it, contractions and exclamation
 * marks in a formal text, words in capitals. And the length asked for, when
 * there is one — most words, zero for none.
 */
func CheckTone(text, lang string, tone Tone, most int) []Finding {
	if lang != "bg" && lang != "en" {
		lang = Language(text)
	}

	var out []Finding

	if tone.Longest > 0 {
		for _, s := range Sentences(text) {
			n := len(strings.Fields(s.Text))
			if n <= tone.Longest {
				continue
			}

			out = append(out, Finding{
				Rule: "long_sentence", At: s.At, Length: len(s.Text), Found: s.Text,
				Why:   fmt.Sprintf("%d words in one sentence; a %s text keeps to about %d.", n, strings.ToLower(tone.Label), tone.Longest),
				WhyBG: fmt.Sprintf("%d думи в едно изречение; за %s текст — до около %d.", n, adjectiveBG(tone), tone.Longest),
			})
		}
	}

	avoid := tone.Avoid
	if lang == "bg" {
		avoid = tone.AvoidBG
	}

	lower := strings.ToLower(text)

	for _, phrase := range avoid {
		for at := 0; ; {
			i := strings.Index(lower[at:], phrase)
			if i < 0 {
				break
			}

			start, end := at+i, at+i+len(phrase)
			at = end

			if !wholeWord(lower, start, end) {
				continue
			}

			out = append(out, Finding{
				Rule: "tone_word", At: start, Length: end - start, Found: text[start:end],
				Why:   fmt.Sprintf("\"%s\" does not fit a %s text.", text[start:end], strings.ToLower(tone.Label)),
				WhyBG: fmt.Sprintf("„%s“ не подхожда за %s текст.", text[start:end], adjectiveBG(tone)),
			})
		}
	}

	if tone.NoContractions && lang == "en" {
		for _, m := range contraction.FindAllStringIndex(text, -1) {
			out = append(out, Finding{
				Rule: "contraction", At: m[0], Length: m[1] - m[0], Found: text[m[0]:m[1]],
				Why:   "A formal text writes the words out in full.",
				WhyBG: "В официален текст думите се изписват изцяло.",
			})
		}
	}

	if tone.NoExclamations {
		for i := strings.Index(text, "!"); i >= 0; {
			out = append(out, Finding{
				Rule: "exclamation", At: i, Length: 1, Found: "!",
				Why:   fmt.Sprintf("A %s text does not use exclamation marks.", strings.ToLower(tone.Label)),
				WhyBG: fmt.Sprintf("В %s текст не се използват удивителни.", adjectiveBG(tone)),
			})

			next := strings.Index(text[i+1:], "!")
			if next < 0 {
				break
			}

			i += 1 + next
		}
	}

	if tone.NoShouting {
		for _, w := range words(text) {
			if utf8.RuneCountInString(w.text) < 4 || strings.ToUpper(w.text) != w.text || !hasLetter(w.text) {
				continue
			}

			out = append(out, Finding{
				Rule: "capitals", At: w.at, Length: w.end - w.at, Found: w.text,
				Why:   "A word in capitals reads as shouting.",
				WhyBG: "Дума с главни букви се възприема като викане.",
			})
		}
	}

	if most > 0 {
		if n := len(strings.Fields(text)); n > most {
			out = append(out, Finding{
				Rule: "too_long", At: 0, Length: 0,
				Why:   fmt.Sprintf("%d words, %d more than the %d asked for.", n, n-most, most),
				WhyBG: fmt.Sprintf("%d думи — с %d повече от поисканите %d.", n, n-most, most),
			})
		}
	}

	return tidy(out)
}

func adjectiveBG(t Tone) string {
	switch t.Name {
	case "formal":
		return "официален"
	case "friendly":
		return "приятелски"
	case "concise":
		return "кратък"
	case "persuasive":
		return "убедителен"
	case "simple":
		return "опростен"
	case "polite":
		return "учтив"
	}

	return strings.ToLower(t.LabelBG)
}

func wholeWord(text string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}

	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}

	return false
}

// Sentence is one sentence of a text and where it starts.
type Sentence struct {
	Text string
	At   int
}

var sentenceEnd = regexp.MustCompile(`[.!?…]+["“”»)]*(\s+|$)|\n\s*\n`)

// Sentences splits text into sentences, roughly: at a full stop, question
// or exclamation mark followed by a space, and at a blank line.
func Sentences(text string) []Sentence {
	var out []Sentence

	start := 0

	for _, m := range sentenceEnd.FindAllStringIndex(text, -1) {
		// Not after an abbreviation or a single letter.
		before := text[start:m[0]]
		last := before
		if i := strings.LastIndexAny(before, " \n\t"); i >= 0 {
			last = before[i+1:]
		}

		if text[m[0]] == '.' && (abbreviations[strings.ToLower(last)] || utf8.RuneCountInString(last) == 1) {
			continue
		}

		piece := strings.TrimSpace(text[start:m[1]])
		if piece != "" {
			out = append(out, Sentence{Text: piece, At: start + strings.Index(text[start:m[1]], piece)})
		}

		start = m[1]
	}

	if rest := strings.TrimSpace(text[start:]); rest != "" {
		out = append(out, Sentence{Text: rest, At: start + strings.Index(text[start:], rest)})
	}

	return out
}

// Stats is how long a text is and how hard it reads.
type Stats struct {
	Words      int     `json:"words"`
	Characters int     `json:"characters"`
	Sentences  int     `json:"sentences"`
	Paragraphs int     `json:"paragraphs"`
	Average    float64 `json:"average_sentence"`
	Longest    int     `json:"longest_sentence"`

	// Minutes is the reading time at about 200 words a minute, at least one
	// for any text at all.
	Minutes int `json:"reading_minutes"`
}

// Measure counts a text.
func Measure(text string) Stats {
	s := Stats{Words: len(strings.Fields(text)), Characters: utf8.RuneCountInString(text)}

	for _, p := range strings.Split(text, "\n\n") {
		if strings.TrimSpace(p) != "" {
			s.Paragraphs++
		}
	}

	for _, sentence := range Sentences(text) {
		s.Sentences++

		if n := len(strings.Fields(sentence.Text)); n > s.Longest {
			s.Longest = n
		}
	}

	if s.Sentences > 0 {
		s.Average = float64(int(float64(s.Words)/float64(s.Sentences)*10+0.5)) / 10
	}

	if s.Words > 0 {
		s.Minutes = (s.Words + 199) / 200
	}

	return s
}
