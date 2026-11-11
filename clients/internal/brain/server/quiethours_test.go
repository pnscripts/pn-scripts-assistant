package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
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
