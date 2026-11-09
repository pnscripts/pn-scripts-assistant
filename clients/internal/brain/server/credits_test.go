package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/preflight"
)

/*
 * The credits the downloaded parts' licences ask for are given.
 *
 * The three voices were trained from scratch by Bryce Beattie on recordings
 * in the public domain (LibriVox, and LJ Speech, which is LibriVox too):
 * nothing is owed, and they are credited anyway, with who made them and from
 * what. Piper, whisper.cpp, the Whisper model and Ollama are MIT. Each is
 * credited in the program, in English and in Bulgarian, and in
 * THIRD-PARTY-NOTICES.md.
 */

// The credits every place must carry, by what they are about.
var credits = map[string][]string{
	"cori":     {"Cori", "en_GB-cori-medium", "Bryce Beattie", "LibriVox", "public domain"},
	"ljspeech": {"LJ Speech", "en_US-ljspeech-medium", "Bryce Beattie", "Keith Ito", "LibriVox", "public domain"},
	"norman":   {"Norman", "en_US-norman-medium", "Bryce Beattie", "LibriVox", "public domain"},
	"programs": {"Piper", "whisper.cpp", "Whisper", "OpenAI", "Ollama", "MIT"},
}

func TestTheCreditsAreInThePageInBothLanguages(t *testing.T) {
	page := readAsset(t, "index.html")
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	for _, key := range []string{"title", "intro", "cori", "ljspeech", "norman", "programs", "more"} {
		name := "credits." + key

		if !strings.Contains(page, `data-i18n="`+name+`"`) {
			t.Errorf("the Updates view has no %s", name)
		}

		en, bg := strings2(t, js, name)

		if en == "" || bg == "" {
			continue
		}

		if !regexp.MustCompile(`[а-яА-Я]`).MatchString(bg) {
			t.Errorf("%s in Bulgarian has no Bulgarian in it: %q", name, bg)
		}

		// The names, licences and authors are the same words in both.
		for _, want := range credits[key] {
			if !strings.Contains(en, want) {
				t.Errorf("%s in English does not say %q: %q", name, want, en)
			}

			// Names and licences stay as they are; "public domain" is
			// said in Bulgarian.
			if want == "public domain" {
				want = "обществено достояние"
			}

			if !strings.Contains(bg, want) {
				t.Errorf("%s in Bulgarian does not say %q: %q", name, want, bg)
			}
		}

		for _, text := range []string{en, bg} {
			if strings.ContainsAny(text, "–—") {
				t.Errorf("%s uses a dash: %q", name, text)
			}
		}
	}

	if !strings.Contains(page, `id="credits-card"`) {
		t.Error("the credits card is not on the page")
	}

	// The voices 0.13 installed are no longer spoken with, so the page does
	// not credit them as if they were.
	for _, gone := range []string{"jenny_dioco", "en_GB-alba-medium", "northern_english_male"} {
		if strings.Contains(page, gone) || strings.Contains(js, gone) {
			t.Errorf("the page still credits %s, which is no longer used", gone)
		}
	}
}

func TestTheNoticesFileCreditsEveryDownloadedPart(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "THIRD-PARTY-NOTICES.md"))
	if err != nil {
		t.Fatal(err)
	}

	notices := string(raw)

	for _, words := range credits {
		for _, want := range words {
			if !strings.Contains(notices, want) {
				t.Errorf("THIRD-PARTY-NOTICES.md does not say %q", want)
			}
		}
	}

	// The releases it names are the ones the program pins.
	for _, want := range []string{
		strings.TrimPrefix(preflight.OllamaVersion, "v"),
		preflight.PiperRelease,
		preflight.PiperVoicesRevision,
		preflight.WhisperTag,
		preflight.SpeechModelRevision,
	} {
		if !strings.Contains(notices, want) {
			t.Errorf("THIRD-PARTY-NOTICES.md does not name the pinned %s", want)
		}
	}
}

func readAsset(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("assets", name))
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

/*
 * strings2 reads one entry of i18n.js: its English and its Bulgarian.
 *
 * An entry is "'key': [" followed by two quoted strings, each possibly in
 * pieces joined with "+". Read here rather than run, which is enough for
 * text that has no code in it.
 */
func strings2(t *testing.T, js, key string) (string, string) {
	t.Helper()

	at := strings.Index(js, "'"+key+"': [")
	if at < 0 {
		t.Errorf("i18n.js has no %s", key)

		return "", ""
	}

	rest := js[at+len(key)+5:]

	end := strings.Index(rest, "],\n")
	if end < 0 {
		t.Errorf("%s does not end", key)

		return "", ""
	}

	var parts []string

	var piece strings.Builder

	inside, escaped := false, false

	for _, r := range rest[:end] {
		switch {
		case escaped:
			piece.WriteRune(r)
			escaped = false
		case inside && r == '\\':
			escaped = true
		case r == '\'':
			inside = !inside
		case inside:
			piece.WriteRune(r)
		case r == ',':
			parts = append(parts, piece.String())
			piece.Reset()
		}
	}

	parts = append(parts, piece.String())

	var out []string

	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}

	if len(out) != 2 {
		t.Errorf("%s has %d strings, want English and Bulgarian", key, len(out))

		return "", ""
	}

	return out[0], out[1]
}
