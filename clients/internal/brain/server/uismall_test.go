package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

/*
 * Small things a Bulgarian reader met in English, or met run together
 * (ASSISTANT-UI-SMALL-1115, seen in the real window and the harness).
 */

var cyrillic = regexp.MustCompile(`\p{Cyrillic}`)

/*
 * Every title and every label the markup gives a control is filled in the
 * page's language. The buttons that step back to every panel and turn the
 * ring, and the bell, carried English titles and labels with nothing to say
 * them in Bulgarian: a pointer resting on them, or a screen reader, met
 * English on a Bulgarian page.
 */
func TestEveryTitleAndLabelInTheMarkupIsSaidInBothLanguages(t *testing.T) {
	page := regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(readAsset(t, "index.html"), "")
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	key := regexp.MustCompile(`data-i18n-(?:title|label)="([^"]+)"`)

	for _, tag := range regexp.MustCompile(`(?s)<[a-z][^>]*>`).FindAllString(page, -1) {
		tag = strings.Join(strings.Fields(tag), " ")

		if strings.Contains(tag, ` title="`) && !strings.Contains(tag, "data-i18n-title=") {
			t.Errorf("a title with nothing to say it in Bulgarian: %s", tag)
		}

		if strings.Contains(tag, `aria-label="`) && !strings.Contains(tag, "data-i18n-label=") {
			t.Errorf("a label with nothing to say it in Bulgarian: %s", tag)
		}

		for _, m := range key.FindAllStringSubmatch(tag, -1) {
			if en, bg := strings2(t, js, m[1]); en == "" || !cyrillic.MatchString(bg) {
				t.Errorf("%s: %q / %q", m[1], en, bg)
			}
		}
	}
}

/*
 * The line under the panels when stepped back ("Every panel, where it
 * stands...") is the page's, in its language. The stage has no words of
 * the page (package stage is shared and knows nothing of i18n.js), so the
 * page hands it the line and the stage says it again whenever the page's
 * language changes, as it does the panel's name on the bar.
 */
func TestTheStageSaysItsHintInThePagesLanguage(t *testing.T) {
	en, bg := strings2(t, readAsset(t, filepath.Join("js", "i18n.js")), "stage.hint")
	if !strings.HasPrefix(en, "Every panel, where it stands.") || !cyrillic.MatchString(bg) {
		t.Fatalf("stage.hint: %q / %q", en, bg)
	}

	scenes := readAsset(t, filepath.Join("js", "scenes.js"))
	if !regexp.MustCompile(`hint:\s*\(\)\s*=>[^\n]*'stage\.hint'`).MatchString(scenes) {
		t.Error("scenes.js does not hand the stage its hint in the page's language")
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "stage", "stage.js"))
	if err != nil {
		t.Fatal(err)
	}

	stage := string(raw)

	// Said when the stage starts and again on every retitle (pn-language,
	// DOMContentLoaded in scenes.js).
	if !strings.Contains(stage, "hint.textContent = hintText();") {
		t.Error("stage.js does not write the hint it was handed")
	}

	if !regexp.MustCompile(`(?s)retitle\(\)\s*\{[^}]*sayWhere\(\);[^}]*hint\.textContent = hintText\(\);`).MatchString(stage) {
		t.Error("stage.js does not say the hint again when the page's words change")
	}

	// The English of the page and of the stage's own fallback are one line.
	if !strings.Contains(stage, "'"+en+"'") {
		t.Errorf("stage.js's own English differs from the page's: %q", en)
	}
}

/*
 * "Where I look: Claude Desktop: found 1 · VS Code: not set up Kept current
 * by file events" ran two sentences together: the list ends its sentence
 * before the next begins, in both languages.
 */
func TestWhereILookEndsItsSentenceBeforeTheNext(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	for _, k := range []string{"org.imp.files"} {
		en, bg := strings2(t, js, k)
		if !strings.HasSuffix(en, ".") || !strings.HasSuffix(bg, ".") {
			t.Errorf("%s does not end its sentence: %q / %q", k, en, bg)
		}
	}

	en, bg := strings2(t, js, "org.imp.watched")
	if !regexp.MustCompile(`^\p{Lu}`).MatchString(en) || !regexp.MustCompile(`^\p{Lu}`).MatchString(bg) {
		t.Errorf("org.imp.watched does not begin a sentence: %q / %q", en, bg)
	}

	if !strings.Contains(readAsset(t, filepath.Join("js", "equip.js")), "T('org.imp.files', { list: said.join(' · ') }) + ' ' + T('org.imp.watched')") {
		t.Error("equip.js no longer puts the two sentences side by side the way this test reads them")
	}
}

/*
 * The hint fits the window: one line that does not wrap ran past both
 * edges of a phone (390 px) in Bulgarian, the longer language. It may
 * wrap, never wider than the window less a gutter.
 */
func TestTheStageHintFitsAPhone(t *testing.T) {
	css := readAsset(t, filepath.Join("css", "console.css"))

	at := strings.Index(css, "\n.stage-hint {")
	if at < 0 {
		t.Fatal("console.css has no .stage-hint rule")
	}

	rule := css[at : at+strings.Index(css[at:], "}")]

	if strings.Contains(rule, "white-space: nowrap") {
		t.Error(".stage-hint never wraps, so a long line runs past a phone's edges")
	}

	if !strings.Contains(rule, "max-width: min(") || !strings.Contains(rule, "100vw - 32px") || !strings.Contains(rule, "width: max-content") {
		t.Errorf(".stage-hint is not held inside the window: %s", rule)
	}
}
