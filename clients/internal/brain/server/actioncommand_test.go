package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The command behind an action stays in the window (ASSISTANT-ACTION-
 * COMMAND-RECORD-1127, from the whole-candidate review of 1.3.0).
 *
 * The list of actions with their commands is read with the window's key
 * only (readInTheWindowOnly), and the door tells a program an action's
 * name and never its command. 1.3.0 still wrote "Command: <the line>"
 * into "What I did", which any program on this computer and every paired
 * device reads at GET /api/deeds: the one way round the window's key.
 *
 * Here as a program would do it: an action is written in the window, a
 * program runs it through the real door, then a program with no window
 * reads the whole record, every filter of it, and finds the name and not
 * the command; nor is the command anywhere in the stored record.
 */
func TestAProgramReadingTheRecordNeverSeesAnActionsCommand(t *testing.T) {
	ts, db, b := newServer(t)

	// Words that appear nowhere else in what the assistant writes.
	const command = "echo plover-quartz-7731"

	code, body := post(t, ts.URL+"/api/outside/actions", `{"name":"say-hello","command":"`+command+`"}`)
	if code != http.StatusOK {
		t.Fatalf("writing the action in the window: %d %s", code, body)
	}

	if err := b.AllowOutside(otherprograms.Actions, true); err != nil {
		t.Fatal(err)
	}

	if text, err := asked(t, ts.URL, "run_action", `{"name":"say-hello"}`); err != nil || !strings.Contains(text, "exit code 0") {
		t.Fatalf("running it for a program: %q, %v", text, err)
	}

	// What it printed went to the program that ran it; that is the point
	// of an action. The line that printed it did not.
	readAll := func(when string) {
		t.Helper()

		for _, query := range []string{"?since=all", "?since=all&changes=1", "?since=day"} {
			res, err := bare.Get(ts.URL + "/api/deeds" + query)
			if err != nil {
				t.Fatal(err)
			}

			raw, _ := io.ReadAll(res.Body)
			res.Body.Close()

			if res.StatusCode != http.StatusOK {
				t.Fatalf("%s, GET /api/deeds%s by a program: %d %s", when, query, res.StatusCode, raw)
			}

			if strings.Contains(string(raw), "plover-quartz") {
				t.Errorf("%s, a program reading /api/deeds%s sees the action's command: %s", when, query, raw)
			}

			var page struct {
				Deeds []store.Deed `json:"deeds"`
			}

			if err := json.Unmarshal(raw, &page); err != nil {
				t.Fatalf("%s, /api/deeds%s: %v", when, query, err)
			}

			deeds := page.Deeds

			// The record still says which action, by its name.
			named := false
			for _, d := range deeds {
				named = named || strings.Contains(d.Summary+d.Asked, "say-hello")
			}

			if query == "?since=all" && !named {
				t.Errorf("%s, the record no longer names the action: %s", when, raw)
			}
		}

		stored, err := db.Deeds(store.DeedFilter{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}

		for _, d := range stored {
			if strings.Contains(d.Summary+d.SummaryBG+d.Asked+d.Request+d.Result, "plover-quartz") {
				t.Errorf("%s, the stored record holds the command: %+v", when, d)
			}
		}
	}

	readAll("after it ran")

	// Taken away in the window: still only the name.
	var state struct {
		Actions []store.OutsideAction `json:"actions"`
	}

	windowGet(t, ts.URL+"/api/outside/actions", &state)

	if len(state.Actions) != 1 || state.Actions[0].Command != command {
		t.Fatalf("the window reads its action with the command: %+v", state)
	}

	if code, body := post(t, ts.URL+"/api/outside/actions/remove", `{"id":`+itoa(state.Actions[0].ID)+`}`); code != http.StatusOK {
		t.Fatalf("taking it away: %d %s", code, body)
	}

	readAll("after it was taken away")
}
