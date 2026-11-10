package server

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

/*
 * Every name in the menu is said in the page's language.
 *
 * 1.0.0 left twelve of the menu's names in English on a Bulgarian page
 * (Command centre, Models, System, Operations, The room, Teaching,
 * Organisation, Tasks, Reaching it, Privacy, The house, Memory map), and the
 * two buttons at the top that open a view by name had English tooltips. Each
 * label now names its string in i18n.js, and each string has Bulgarian in it.
 */
func TestEveryMenuNameIsSaidInBothLanguages(t *testing.T) {
	page := readAsset(t, "index.html")
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	item := regexp.MustCompile(`(?s)<button type="button" class="nav-item[^"]*" data-view="([^"]+)">(.*?)</button>`)
	label := regexp.MustCompile(`<span([^>]*)>([^<]*)</span>`)
	key := regexp.MustCompile(`data-i18n="([^"]+)"`)
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	items := item.FindAllStringSubmatch(page, -1)
	if len(items) < 20 {
		t.Fatalf("found %d menu entries; has the menu moved?", len(items))
	}

	for _, m := range items {
		view, named := m[1], false

		for _, span := range label.FindAllStringSubmatch(m[2], -1) {
			if strings.Contains(span[1], "nav-count") {
				continue
			}

			k := key.FindStringSubmatch(span[1])
			if k == nil {
				t.Errorf("the menu entry for %s (%q) is not translated", view, strings.TrimSpace(span[2]))

				continue
			}

			named = true

			en, bg := strings2(t, js, k[1])
			if en != strings.TrimSpace(strings.ReplaceAll(span[2], "&amp;", "&")) {
				t.Errorf("%s: the page says %q and i18n.js %q", k[1], span[2], en)
			}

			if !cyrillic.MatchString(bg) {
				t.Errorf("%s in Bulgarian has no Bulgarian in it: %q", k[1], bg)
			}
		}

		if !named {
			t.Errorf("the menu entry for %s has no name", view)
		}
	}

	// The buttons at the top that open a view by its name.
	top := regexp.MustCompile(`<button type="button" class="icon-button" data-view="[^"]+"([^>]*)>`)
	for _, b := range top.FindAllStringSubmatch(page, -1) {
		k := regexp.MustCompile(`data-i18n-title="([^"]+)"`).FindStringSubmatch(b[1])
		if k == nil {
			t.Errorf("a button at the top has an English-only tooltip: %s", b[0])

			continue
		}

		if _, bg := strings2(t, js, k[1]); !cyrillic.MatchString(bg) {
			t.Errorf("%s in Bulgarian has no Bulgarian in it: %q", k[1], bg)
		}
	}
}
