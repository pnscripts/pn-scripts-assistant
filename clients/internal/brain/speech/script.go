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
// Three things decide it, in order of how reliable they are.
//
// The script is decisive when it disagrees: an English voice reading Cyrillic
// spells the letters out, and no preference is worth that.
//
// The configured spoken language is the next best signal. Somebody who has said
// they speak German is being answered in German, and telling German apart from
// Dutch by looking at the letters is not something worth attempting.
//
// Otherwise the owner's choice stands. Overriding a preference on a guess is
// worse than occasionally using an English voice for a French sentence.
func voiceForText(text string) Voice {
	chosen := CurrentVoice()
	script := scriptOf(text)
	installed := Voices()

	// The script is wrong for the chosen voice: it would spell rather than
	// speak, so something else has to say it.
	if script != "" && !voiceMatchesScript(chosen, script) {
		if v, found := voiceForLanguage(installed, Language(), script); found {
			return v
		}

		for _, v := range installed {
			// Only a voice whose language is actually known, so the system
			// voice is not picked for a script nobody has checked it against.
			if v.Engine == "piper" && voiceMatchesScript(v, script) {
				return v
			}
		}

		// Nothing installed can say it properly. The chosen voice mangling the
		// words is still better than silence.
		return chosen
	}

	// The script is fine, but the owner speaks a language this voice is not
	// for — and there is a voice that is.
	if code := Language(); code != "" && !voiceIsForLanguage(chosen, code) {
		if v, found := voiceForLanguage(installed, code, script); found {
			return v
		}
	}

	return chosen
}

// voiceForLanguage finds an installed voice for an ISO code.
func voiceForLanguage(installed []Voice, code, script string) (Voice, bool) {
	if code == "" {
		return Voice{}, false
	}

	for _, v := range installed {
		if !voiceIsForLanguage(v, code) {
			continue
		}

		// A voice for the right language but the wrong script would still
		// spell; that can happen when a language is written both ways.
		if script != "" && !voiceMatchesScript(v, script) {
			continue
		}

		return v, true
	}

	return Voice{}, false
}

// voiceIsForLanguage reports whether a voice is known to speak a given ISO code.
//
// Only piper voices can answer this, because their language is part of their
// identity. The system voice follows the desktop's settings, which this cannot
// see — and assuming it could handle anything let it be chosen for Cyrillic,
// where it would spell rather than speak. Unknown is treated as no.
func voiceIsForLanguage(v Voice, code string) bool {
	if v.Engine != "piper" {
		return false
	}

	return strings.HasPrefix(strings.ToLower(v.ID), strings.ToLower(code)+"_")
}

// cyrillicLocales are the voice prefixes that read Cyrillic.
var cyrillicLocales = []string{"bg_", "ru_", "uk_", "sr_", "mk_", "be_", "kk_"}

func voiceMatchesScript(v Voice, script string) bool {
	// The system voice follows the desktop's language rather than a model, so
	// what it can read is unknown. It is left alone when it is the deliberate
	// choice, but never selected over a voice whose language is known.
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
