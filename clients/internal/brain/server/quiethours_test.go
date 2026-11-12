package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Quiet hours are set from Settings: saved as one window, shown back on the
 * status and the settings answer, cleared with an empty value, and a window
 * that cannot be meant is refused with why and changes nothing.
 */
func TestQuietHoursAreSetFromSettings(t *testing.T) {
	ts, _, b := newServer(t)

	read := func(path string) map[string]any {
		t.Helper()

		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		out := map[string]any{}
		_ = json.NewDecoder(res.Body).Decode(&out)

		return out
	}

	code, saved := postBody(t, ts.URL+"/api/settings", `{"quiet_hours":" 22:00 - 7:00 "}`)
	if code != http.StatusOK {
		t.Fatalf("setting them returned %d", code)
	}

	if got, _ := saved["quiet_hours"].(string); got != "22:00-07:00" {
		t.Errorf("the save answered %q", got)
	}

	if b.Cfg.QuietHours != "22:00-07:00" {
		t.Fatalf("saved as %q", b.Cfg.QuietHours)
	}

	if file, _ := config.Load(b.Root); file.QuietHours != "22:00-07:00" {
		t.Errorf("not in the file: %q", file.QuietHours)
	}

	if got, _ := read("/api/status")["quiet_hours"].(string); got != "22:00-07:00" {
		t.Errorf("the status shows %q", got)
	}

	code, out := postBody(t, ts.URL+"/api/settings", `{"quiet_hours":"22:00-22:00"}`)
	if msg, _ := out["error"].(string); code != http.StatusBadRequest || !strings.Contains(msg, "two different times") {
		t.Errorf("a window that cannot be meant: %d %v", code, out)
	}

	if b.Cfg.QuietHours != "22:00-07:00" {
		t.Errorf("a refused window changed the setting to %q", b.Cfg.QuietHours)
	}

	if code, _ := postBody(t, ts.URL+"/api/settings", `{"quiet_hours":""}`); code != http.StatusOK || b.Cfg.QuietHours != "" {
		t.Errorf("clearing them: %d, still %q", code, b.Cfg.QuietHours)
	}
}

func quietJSON(t *testing.T, url string) map[string]any {
	t.Helper()

	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	out := map[string]any{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	return out
}

/*
 * The goals and places pages say when unasked work waits for the quiet hours:
 * the window and when it opens, for the page to say in its language, and on a
 * goal that would otherwise start itself now, that it waits. Nothing while the
 * window is open or when there is none.
 */
func TestThePagesSayWhenUnaskedWorkWaitsForTheQuietHours(t *testing.T) {
	ts, _, b := newServer(t)

	now := time.Now()
	b.SetQuietHours(now.Add(2*time.Hour).Format("15:04") + "-" + now.Add(4*time.Hour).Format("15:04"))

	starts, err := b.DB.NewGoal(store.Goal{Name: "Tidy the notes", EveryDays: 7, StartsItself: true})
	if err != nil {
		t.Fatal(err)
	}

	asked, err := b.DB.NewGoal(store.Goal{Name: "Read the manual", EveryDays: 7})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/goals", "/api/places"} {
		quiet, ok := quietJSON(t, ts.URL+path)["quiet"].(map[string]any)
		if !ok {
			t.Fatalf("%s does not say the work waits", path)
		}

		opens, _ := time.Parse(time.RFC3339, quiet["opens"].(string))

		if tomorrow, _ := quiet["tomorrow"].(bool); tomorrow != (opens.In(now.Location()).Day() != now.Day()) {
			t.Errorf("%s says tomorrow=%v for an opening at %v", path, tomorrow, opens)
		}
		if quiet["from"] != now.Add(2*time.Hour).Format("15:04") || quiet["to"] != now.Add(4*time.Hour).Format("15:04") ||
			opens.Sub(now) < time.Hour+58*time.Minute || opens.Sub(now) > 2*time.Hour {
			t.Errorf("%s says %v", path, quiet)
		}
	}

	waits := map[float64]bool{}

	for _, g := range quietJSON(t, ts.URL+"/api/goals")["goals"].([]any) {
		goal := g.(map[string]any)
		waits[goal["id"].(float64)], _ = goal["waits_for_quiet"].(bool)
	}

	if !waits[float64(starts)] {
		t.Error("the due goal that starts itself does not say it waits")
	}

	if waits[float64(asked)] {
		t.Error("a goal somebody starts says it waits for the quiet hours")
	}

	b.SetQuietHours(now.Add(-time.Hour).Format("15:04") + "-" + now.Add(time.Hour).Format("15:04"))

	for _, path := range []string{"/api/goals", "/api/places"} {
		if quiet, ok := quietJSON(t, ts.URL+path)["quiet"]; ok {
			t.Errorf("%s inside the window says %v", path, quiet)
		}
	}

	for _, g := range quietJSON(t, ts.URL+"/api/goals")["goals"].([]any) {
		if w, _ := g.(map[string]any)["waits_for_quiet"].(bool); w {
			t.Errorf("inside the window a goal says it waits: %v", g)
		}
	}
}

// What the pages say about the wait, in both languages, with the window and
// the time in each, and no dash as punctuation.
func TestTheWaitIsSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	needs := map[string][]string{
		"room.waitsQuietToday":    {"{at}"},
		"room.waitsQuietTomorrow": {"{at}"},
		"sys.placesQuietToday":    {"{from}", "{to}", "{at}"},
		"sys.placesQuietTomorrow": {"{from}", "{to}", "{at}"},
	}

	for key, holes := range needs {
		en, bg := strings2(t, js, key)

		if !bulgarianLetters.MatchString(bg) {
			t.Errorf("%s has no Bulgarian: %q", key, bg)
		}

		for _, hole := range holes {
			if !strings.Contains(en, hole) || !strings.Contains(bg, hole) {
				t.Errorf("%s does not carry %s in both: %q / %q", key, hole, en, bg)
			}
		}

		if strings.ContainsAny(en+bg, "–—") {
			t.Errorf("%s uses a dash: %q / %q", key, en, bg)
		}

		if strings.Contains(key, "Tomorrow") != (strings.Contains(en, "tomorrow") && strings.Contains(bg, "утре")) {
			t.Errorf("%s says the wrong day: %q / %q", key, en, bg)
		}
	}
}
