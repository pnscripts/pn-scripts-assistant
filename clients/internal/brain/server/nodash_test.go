package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

/*
 * No dash stands in for a missing value on the page (ASSISTANT-UI-SMALL-1115).
 *
 * The page put an em dash wherever a value was not there yet or not known:
 * "Who is good at" before its list arrived, the owner's name, the figures of
 * the command centre, the Operations tables, the cost of a cloud model that
 * is not set up. People read those as text, and the owner's rule is that no
 * text people read carries the dashes "—" or "–". The placeholder is "…"
 * (pnNone in i18n.js).
 *
 * Read the way a person meets them: every string literal of every page
 * script and of the stage, the markup outside its comments, and the values
 * the server sends that the page shows as they are.
 */
var dashes = regexp.MustCompile("[–—]")

func TestNoDashStandsInForAMissingValue(t *testing.T) {
	literal := regexp.MustCompile("'((?:[^'\\\\\n]|\\\\.)*)'|\"((?:[^\"\\\\\n]|\\\\.)*)\"|`((?:[^`\\\\]|\\\\.)*)`")
	blockComment := regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComment := regexp.MustCompile(`(?m)^\s*//.*$|\s//[^'"` + "`" + `\n]*$`)
	htmlComment := regexp.MustCompile(`(?s)<!--.*?-->`)

	scripts, err := filepath.Glob(filepath.Join("assets", "js", "*.js"))
	if err != nil || len(scripts) < 20 {
		t.Fatalf("the page scripts: %v, %v", scripts, err)
	}

	scripts = append(scripts, filepath.Join("..", "..", "stage", "stage.js"))

	for _, file := range scripts {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		src := blockComment.ReplaceAllStringFunc(string(raw), func(s string) string {
			return strings.Repeat("\n", strings.Count(s, "\n"))
		})
		src = lineComment.ReplaceAllString(src, "")

		for _, m := range literal.FindAllStringIndex(src, -1) {
			if s := src[m[0]:m[1]]; dashes.MatchString(s) {
				t.Errorf("%s:%d: a dash in what the page says: %s", filepath.Base(file), strings.Count(src[:m[0]], "\n")+1, s)
			}
		}
	}

	page := htmlComment.ReplaceAllStringFunc(readAsset(t, "index.html"), func(s string) string {
		return strings.Repeat("\n", strings.Count(s, "\n"))
	})

	for i, line := range strings.Split(page, "\n") {
		if dashes.MatchString(line) {
			t.Errorf("index.html:%d: a dash in the page: %s", i+1, strings.TrimSpace(line))
		}
	}

	// What the server sends for the page to show as it is.
	for _, part := range []Part{cloudPart(false, ""), cloudPart(true, "a model")} {
		if dashes.MatchString(part.Name + part.Status + part.Cost + part.Note) {
			t.Errorf("a part the Updates page lists: %+v", part)
		}
	}
}
