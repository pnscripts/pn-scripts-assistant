package server

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

/*
 * The page says that answers come from an AI (EU AI Act article 50(1)), in
 * both languages, in two places that are always on screen: beside the
 * conversation, where answers appear, and as a small AI mark beside the
 * assistant's name in the top bar, which every page shares.
 */
func TestThePageSaysAnswersComeFromAnAI(t *testing.T) {
	page := readAsset(t, "index.html")
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	chat := regexp.MustCompile(`(?s)<div class="card card-chat" data-area="chat">(?:\s*<!--.*?-->)?\s*<h2>(.*?)</h2>`).FindStringSubmatch(page)
	if chat == nil {
		t.Fatal("the conversation card has no heading of its own")
	}

	if !strings.Contains(chat[1], `data-i18n="cc.conversation"`) {
		t.Errorf("the heading lost its name: %s", chat[1])
	}

	if !strings.Contains(chat[1], `class="ai-notice"`) || !strings.Contains(chat[1], `data-i18n="cc.aiNotice"`) ||
		!strings.Contains(chat[1], `data-i18n-title="cc.aiNoticeTitle"`) {
		t.Errorf("the conversation heading does not carry the AI notice: %s", chat[1])
	}

	brand := regexp.MustCompile(`(?s)<div class="brand">(.*?)</header>`).FindStringSubmatch(page)
	if brand == nil || !strings.Contains(brand[1], `class="ai-badge"`) ||
		!strings.Contains(brand[1], `data-i18n-title="cc.aiNoticeTitle"`) ||
		!strings.Contains(brand[1], `data-i18n-label="cc.aiNoticeTitle"`) {
		t.Fatal("the top bar has no AI mark beside the name")
	}

	en, bg := strings2(t, js, "cc.aiNotice")
	if !strings.Contains(en, "AI") || !strings.Contains(en, "wrong") {
		t.Errorf("English notice: %q", en)
	}

	if !strings.Contains(bg, "изкуствен интелект") || !strings.Contains(bg, "грешат") {
		t.Errorf("Bulgarian notice: %q", bg)
	}

	en, bg = strings2(t, js, "cc.aiNoticeTitle")
	for _, want := range []string{"AI system", "language model", "marked"} {
		if !strings.Contains(en, want) {
			t.Errorf("English explanation has no %q: %q", want, en)
		}
	}

	for _, want := range []string{"изкуствен интелект", "езиков модел", "отбелязани"} {
		if !strings.Contains(bg, want) {
			t.Errorf("Bulgarian explanation has no %q: %q", want, bg)
		}
	}

	for _, s := range []string{en, bg} {
		if strings.ContainsAny(s, "–—") {
			t.Errorf("a dash in %q", s)
		}
	}

	// Never hidden: not by [hidden], not by the rules that drop the hint on
	// small screens.
	css := readAsset(t, filepath.Join("css", "console.css"))
	for _, rule := range regexp.MustCompile(`(?s)([^{}]*)\{([^{}]*)\}`).FindAllStringSubmatch(css, -1) {
		if (strings.Contains(rule[1], ".ai-notice") || strings.Contains(rule[1], ".ai-badge")) &&
			regexp.MustCompile(`display:\s*none|visibility:\s*hidden|opacity:\s*0[;\s]`).MatchString(rule[2]) {
			t.Errorf("a rule hides the AI notice: %s{%s}", rule[1], rule[2])
		}
	}

	if !strings.Contains(css, ".ai-notice") || !strings.Contains(css, ".ai-badge") {
		t.Error("the AI notice has no style of its own")
	}
}
