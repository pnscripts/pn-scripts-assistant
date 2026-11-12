package aidisclosure

import (
	"strings"
	"testing"
)

// The notice says, in plain words, that this is an AI and can be wrong, in
// the language the person reads.
func TestTheNoticeSaysItIsAnAI(t *testing.T) {
	en, bg := Notice("en"), Notice("bg")

	if !strings.Contains(en, "AI assistant") || !strings.Contains(en, "wrong") {
		t.Errorf("English notice does not say it is an AI that can be wrong: %q", en)
	}

	if !strings.Contains(bg, "изкуствен интелект") || !strings.Contains(bg, "грешат") {
		t.Errorf("Bulgarian notice does not say it is an AI that can be wrong: %q", bg)
	}

	for _, lang := range []string{"", "de", "EN", "en-GB"} {
		if Notice(lang) != en {
			t.Errorf("Notice(%q) = %q, want the English one", lang, Notice(lang))
		}
	}

	for _, lang := range []string{"BG", " bg ", "bg-BG"} {
		if Notice(lang) != bg {
			t.Errorf("Notice(%q) = %q, want the Bulgarian one", lang, Notice(lang))
		}
	}

	for _, n := range []string{en, bg} {
		if !Discloses(n) {
			t.Errorf("the notice itself is not recognised as a disclosure: %q", n)
		}

		if strings.ContainsAny(n, "–—!") {
			t.Errorf("the notice has a dash or an exclamation mark: %q", n)
		}
	}
}

func TestDisclosesRecognisesSayingItIsAnAI(t *testing.T) {
	yes := []string{
		"I am an AI assistant that lives on this computer. What should I call you?",
		"I'm an AI; what is your name?",
		"I am artificial intelligence running here.",
		"Аз съм асистент с изкуствен интелект. Как да ви наричам?",
		"Аз съм AI асистент. Как се казвате?",
		"Аз съм ИИ асистент на този компютър.",
		"Изкуственият интелект на този компютър, това съм аз.",
	}

	no := []string{
		"",
		"I live on this computer and remember what I am told. What should I call you?",
		"Said is the past of say.",    // "ai" inside a word
		"MAIL arrived; PAID invoice.", // "AI" inside capitals
		"Живея на този компютър. Как да ви наричам?",
		"Взаимно, приятелю.", // "ИИ" never stands alone here
		"Хаир и Кайо.",
	}

	for _, s := range yes {
		if !Discloses(s) {
			t.Errorf("Discloses(%q) = false, want true", s)
		}
	}

	for _, s := range no {
		if Discloses(s) {
			t.Errorf("Discloses(%q) = true, want false", s)
		}
	}
}

/*
 * The opener of a first meeting always says what it is. The model is asked
 * to, and when its words do not, the notice goes in front of them: the
 * question to the person stays last, where an answer is expected.
 */
func TestIntroducedPutsTheNoticeFirstOnlyWhenMissing(t *testing.T) {
	said := "I live on this computer. What should I call you?"

	got := Introduced(said, "en")
	if got != Notice("en")+" "+said {
		t.Errorf("Introduced = %q", got)
	}

	if !strings.HasSuffix(got, "What should I call you?") {
		t.Errorf("the question is no longer last: %q", got)
	}

	already := "I am an AI assistant on this computer. What should I call you?"
	if got := Introduced(already, "en"); got != already {
		t.Errorf("a disclosure was said twice: %q", got)
	}

	// Words in Cyrillic get the Bulgarian notice, whatever the setting says.
	bg := "Живея на този компютър. Как да ви наричам?"
	if got := Introduced(bg, ""); got != Notice("bg")+" "+bg {
		t.Errorf("Bulgarian words got %q", got)
	}

	if got := Introduced("I live here. Your name?", "bg"); !strings.HasPrefix(got, Notice("bg")) {
		t.Errorf("a Bulgarian owner got %q", got)
	}

	// Nothing said, nothing added: with no model there is no AI talking.
	if got := Introduced("  ", "en"); got != "" {
		t.Errorf("an empty opener became %q", got)
	}
}

// The mail header follows the AI-Disclosure field of draft-abaris-aicdh:
// a structured dictionary whose mode says the AI wrote it and a person
// approved it.
func TestTheMailHeaderIsAStructuredAIDisclosure(t *testing.T) {
	if MailHeader != "AI-Disclosure" {
		t.Errorf("MailHeader = %q", MailHeader)
	}

	if !strings.HasPrefix(MailValue, "mode=ai-originated") {
		t.Errorf("MailValue = %q", MailValue)
	}

	if strings.ContainsAny(MailValue, "\r\n") {
		t.Errorf("MailValue breaks the header: %q", MailValue)
	}

	if !strings.Contains(MailValue, `provider="PN Scripts Assistant"`) {
		t.Errorf("MailValue does not name what wrote it: %q", MailValue)
	}
}

// Document properties say the same in words a person reads in File, Properties.
func TestTheDocumentMarkSaysItWasWrittenByAnAI(t *testing.T) {
	if !strings.Contains(DocumentNote, "AI") || !strings.Contains(DocumentNote, "PN Scripts Assistant") {
		t.Errorf("DocumentNote = %q", DocumentNote)
	}

	if DocumentKeywords != "AI-generated" {
		t.Errorf("DocumentKeywords = %q", DocumentKeywords)
	}

	for _, s := range []string{DocumentNote, DocumentKeywords, Producer} {
		if strings.ContainsAny(s, `()\<>&"`) {
			t.Errorf("%q needs escaping in PDF or XML; keep the marks plain", s)
		}
	}
}
