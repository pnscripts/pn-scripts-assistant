package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/environs"
	"pn-scripts-assistant/internal/brain/storage"
)

/*
 * What the server says on the System page, said in both languages.
 *
 * Until 1.0.1 the System page drew the server's English as it came: why a
 * turn was not acted on, what a program on the machine needs, what is wrong
 * with a copy or a place, how much room is left. Each now comes with a key
 * the page says in its own language; these check every key the server can
 * send is in i18n.js with Bulgarian in it.
 */
func TestTheSystemPageSaysWhatTheServerSendsInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	said := func(key, from string) {
		t.Helper()

		if key == "" {
			t.Errorf("%s has no key", from)

			return
		}

		if !strings.Contains(js, "'"+key+"': [") {
			t.Errorf("%s sends %s, which i18n.js does not have", from, key)

			return
		}

		if _, bg := strings2(t, js, key); !cyrillic.MatchString(bg) {
			t.Errorf("%s (from %s) has no Bulgarian in it: %q", key, from, bg)
		}
	}

	// What became of a turn the microphone heard.
	turns := []struct {
		turn      Overheard
		addressed bool
		wake      string
	}{
		{Overheard{}, false, "brain"},
		{Overheard{HeardSpeech: true}, false, "brain"},
		{Overheard{HeardSpeech: true, Text: "brain, hello"}, true, "brain"},
		{Overheard{HeardSpeech: true, Text: "hello"}, false, ""},
		{Overheard{HeardSpeech: true, Text: "hello there"}, false, "brain"},
		{Overheard{HeardSpeech: true, Text: "brian what time is it"}, false, "brain"},
	}

	near := false

	for _, c := range turns {
		text, key, vars := why(c.turn, c.addressed, c.wake)
		said(key, "why ("+text+")")

		if key == "sys.heardWhy.near" {
			near = vars["near"] != ""
		}
	}

	if !near {
		t.Error("a near miss of the name was not said with what was heard")
	}

	// How much room is left.
	for _, free := range []uint64{10 << 30, 10 << 20, 1 << 15} {
		text, key, _ := roomOutlook(storage.Report{DatabaseBytes: 1 << 20, FreeBytes: free}, 1000)
		said(key, "roomOutlook ("+text+")")
	}

	// What is wrong with a copy or a place: the words the packages use.
	for _, dir := range []string{"copies", "places"} {
		var source strings.Builder

		files, _ := filepath.Glob(filepath.Join("..", dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}

			raw, _ := os.ReadFile(f)
			source.Write(raw)
		}

		for _, m := range regexp.MustCompile(`Trouble = "([^"]+)"`).FindAllStringSubmatch(source.String(), -1) {
			said(troubleKeys[m[1]], dir+" trouble "+m[1])
		}
	}

	for english, key := range troubleKeys {
		if en, _ := strings2(t, js, key); en != english {
			t.Errorf("%s says %q in English, and the server %q", key, en, english)
		}
	}

	// What a program on the machine needs, and why it cannot be used.
	for _, p := range environs.Known() {
		thing := environs.Thing{ID: p.ID, Kind: p.Kind, Title: p.Title, Needs: p.Needs}

		if w := envWords(thing); w.NeedsKey != "" {
			said(w.NeedsKey, p.ID)

			if en, _ := strings2(t, js, w.NeedsKey); en != p.Needs {
				t.Errorf("%s says %q in English, and the program %q", w.NeedsKey, en, p.Needs)
			}
		} else if !envCommand.MatchString(p.Needs) {
			t.Errorf("what %s needs is not said in Bulgarian: %q", p.ID, p.Needs)
		}
	}

	for _, why := range []string{
		"it is at /opt/x/godot and is not executable: chmod +x /opt/x/godot",
		"it may be at /opt/x/godot, which this user may not read",
	} {
		w := envWords(environs.Thing{ID: "godot", Why: why})
		said(w.WhyKey, why)

		if w.Vars["at"] != "/opt/x/godot" {
			t.Errorf("%q names %v, not the place", why, w.Vars["at"])
		}
	}

	// The services and abilities the brain adds, read from where it adds them.
	brainSource, err := os.ReadFile(filepath.Join("..", "brain", "environment.go"))
	if err != nil {
		t.Fatal(err)
	}

	abilities := regexp.MustCompile(`ID: "(ability:[^"]+)"`).FindAllStringSubmatch(string(brainSource), -1)
	if len(abilities) < 4 {
		t.Fatalf("found %d abilities in brain/environment.go; have they moved?", len(abilities))
	}

	for _, a := range abilities {
		w := envWords(environs.Thing{ID: a[1], Needs: "something", Why: "something"})
		said(w.TitleKey, a[1])
		said(w.NeedsKey, a[1])
	}

	said(envWords(environs.Thing{ID: "ability:clear-voice", Why: "x"}).WhyKey, "ability:clear-voice")

	for _, needs := range []string{"a key for Claude, in the System tab", "privacy is set to keep everything on this machine"} {
		said(envWords(environs.Thing{ID: "service:anthropic", Title: "Claude", Needs: needs}).NeedsKey, needs)
	}
}
