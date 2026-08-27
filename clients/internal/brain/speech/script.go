package speech

import (
	"strings"
	"unicode"
)

// Choosing a voice by what is actually being said.
//
// An English voice handed Cyrillic does not refuse — it reads the letters, so a
// Bulgarian sentence comes out as spelling rather than speech. The voice has to
// follow the language of the reply, not the language of the owner, because a
// brain that answers in Bulgarian and speaks in English is worse than one that
// does neither.

// scriptOf reports which writing system a text is mostly in.
func scriptOf(text string) string {
	var cyrillic, latin int

	for _, r := range text {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}

	// A few borrowed words should not change the voice; a majority should.
	if cyrillic > latin {
		return "cyrillic"
	}

	if latin > 0 {
		return "latin"
	}

	return ""
}

// voiceForText picks the best installed voice for what is about to be said.
//
// The owner's choice is honoured whenever it can carry the text. It is only
// overridden when the script plainly does not match — which is the difference
// between respecting a preference and reading Cyrillic letter by letter.
func voiceForText(text string) Voice {
	chosen := CurrentVoice()
	script := scriptOf(text)

	if script == "" || voiceMatchesScript(chosen, script) {
		return chosen
	}

	for _, v := range Voices() {
		if voiceMatchesScript(v, script) {
			return v
		}
	}

	// Nothing installed can say it properly. The chosen voice mangling the
	// words is still better than silence, and the interface says which voices
	// are installed.
	return chosen
}

// cyrillicLocales are the voice prefixes that read Cyrillic.
var cyrillicLocales = []string{"bg_", "ru_", "uk_", "sr_", "mk_", "be_", "kk_"}

func voiceMatchesScript(v Voice, script string) bool {
	// The system voice follows the desktop's language rather than a model, so
	// it is treated as able to attempt anything.
	if v.Engine != "piper" {
		return true
	}

	isCyrillic := false

	for _, prefix := range cyrillicLocales {
		if strings.HasPrefix(v.ID, prefix) {
			isCyrillic = true

			break
		}
	}

	if script == "cyrillic" {
		return isCyrillic
	}

	return !isCyrillic
}
