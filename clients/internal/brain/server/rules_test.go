package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/polish"
	"pn-scripts-assistant/internal/brain/rules"
)

type rulesPage struct {
	Rules []struct {
		rules.Rule
		Mode  string      `json:"mode"`
		Scope rules.Scope `json:"scope"`
	} `json:"rules"`
	Kinds   []string `json:"kinds"`
	Actions []string `json:"actions"`
}

/*
 * The Rules page from end to end: a proposal waits at the top until it is
 * kept with its words edited; a rule written on the page is kept at once;
 * a kept rule is switched off, changed and deleted; and only live rules are
 * what the assistant follows.
 */
func TestTheRulesPage(t *testing.T) {
	ts, db, b := newServer(t)

	p, _ := rules.Notice("Never send emails to clients.")

	proposed, err := db.AddRule(p.Rule(0, time.Now()))
	if err != nil {
		t.Fatal(err)
	}

	var page rulesPage

	getJSON(t, ts.URL+"/api/rules", &page)

	if len(page.Rules) != 1 || page.Rules[0].Status != rules.Proposed || len(page.Kinds) != 3 || len(page.Actions) != 6 {
		t.Fatalf("%+v", page)
	}

	if page.Rules[0].Mode != "ask" || len(page.Rules[0].Scope.Unsure) != 1 {
		t.Errorf("the page does not say it asks because of %q: %+v", "clients", page.Rules[0])
	}

	if len(b.LiveRules()) != 0 {
		t.Fatal("a proposal is followed before it is kept")
	}

	// Kept with its words narrowed first: what is kept is what was looked at.
	var kept struct {
		rules.Rule
		Mode string `json:"mode"`
	}

	code := sendJSON(t, ts.URL+"/api/rules/"+itoa(proposed.ID)+"/decide",
		map[string]any{"keep": true, "words": []string{"client.example"}}, &kept)
	if code != http.StatusOK || kept.Status != rules.Kept || !kept.On || kept.Mode != "stop" ||
		len(kept.Words) != 1 || kept.Words[0] != "client.example" {
		t.Fatalf("%d %+v", code, kept)
	}

	if code := sendJSON(t, ts.URL+"/api/rules/"+itoa(proposed.ID)+"/decide", map[string]any{"keep": false}, nil); code != http.StatusConflict {
		t.Errorf("decided twice: %d", code)
	}

	if live := b.LiveRules(); len(live) != 1 {
		t.Fatalf("live %v", live)
	}

	// Written on the page: kept at once, with what it is about read from it.
	var written struct{ rules.Rule }

	if code := sendJSON(t, ts.URL+"/api/rules", map[string]any{"text": "Никога не публикувай в неделя"}, &written); code != http.StatusOK {
		t.Fatalf("adding answered %d", code)
	}

	if written.Status != rules.Kept || !written.On || written.Text != "Никога не публикувай в неделя." ||
		len(written.Actions) != 1 || written.Actions[0] != rules.Post || len(written.Days) != 1 || written.Days[0] != time.Sunday {
		t.Errorf("%+v", written)
	}

	// Switched off, changed, deleted.
	if code := sendJSON(t, ts.URL+"/api/rules/"+itoa(written.ID)+"/switch", map[string]any{"on": false}, nil); code != http.StatusOK {
		t.Errorf("switch answered %d", code)
	}

	if live := b.LiveRules(); len(live) != 1 {
		t.Errorf("a switched-off rule is live: %v", live)
	}

	var changed struct{ rules.Rule }

	if code := sendJSON(t, ts.URL+"/api/rules/"+itoa(written.ID),
		map[string]any{"kind": "ask", "days": []int{6, 0}}, &changed); code != http.StatusOK || changed.Kind != rules.AskFirst ||
		len(changed.Days) != 2 || changed.On {
		t.Errorf("%d %+v", code, changed)
	}

	for _, bad := range []map[string]any{
		{"kind": "sometimes"}, {"actions": []string{"launch"}}, {"days": []int{7}}, {"text": "  "},
	} {
		if code := sendJSON(t, ts.URL+"/api/rules/"+itoa(written.ID), bad, nil); code != http.StatusBadRequest {
			t.Errorf("%v answered %d", bad, code)
		}
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/rules/"+itoa(written.ID), nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %v %v", err, resp)
	}

	resp.Body.Close()

	if _, err := db.Rule(written.ID); err == nil {
		t.Error("still there")
	}

	if code := sendJSON(t, ts.URL+"/api/rules/99/switch", map[string]any{"on": true}, nil); code != http.StatusNotFound {
		t.Errorf("a rule that is not there answered %d", code)
	}
}

// The Writing page's last look: the text as it should go and why.
func TestTheLastLookFromTheWritingPage(t *testing.T) {
	ts, _, _ := newServer(t)

	var got polish.Result

	code := sendJSON(t, ts.URL+"/api/writing/polish", map[string]any{
		"text": "Hi,\n\nI hope this email finds you well. Dear [Name], the parcel left today.",
	}, &got)

	if code != http.StatusOK || !got.Blocking || !got.Changed {
		t.Fatalf("%d %+v", code, got)
	}
}

// Every kind, kind of action, mode, day and finding the pages show is said
// in both languages.
func TestRulesAndPolishAreSaidInBothLanguages(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("assets", "js", "i18n.js"))
	if err != nil {
		t.Fatal(err)
	}

	var keys []string

	for _, k := range rules.Kinds() {
		keys = append(keys, "rules.kind."+string(k))
	}

	for _, a := range rules.Actions() {
		keys = append(keys, "rules.act."+a)
	}

	for _, m := range rules.Modes() {
		keys = append(keys, "rules.mode."+m)
	}

	for d := 0; d < 7; d++ {
		keys = append(keys, "rules.day."+strconv.Itoa(d))
	}

	for _, k := range polish.FindingKeys() {
		keys = append(keys, "polish.f."+k)
	}

	for _, key := range keys {
		if !strings.Contains(string(js), "'"+key+"': [") {
			t.Errorf("%s is not in i18n.js", key)
		}
	}
}
