package mcpserve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

/*
 * The tools that hand the assistant something, against a stand-in that
 * answers the way the real door does (package server holds them to the
 * real one: handover_door_test.go there).
 */

// handoverDoor is a stand-in assistant whose door answers each handing
// over with what answers[tool] says (status and body), and keeps what it
// was given.
func handoverDoor(t *testing.T, answers map[string][2]string) (*httptest.Server, *door) {
	t.Helper()

	var mu sync.Mutex

	kept := &door{}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/outside/ask", func(w http.ResponseWriter, r *http.Request) {
		var asked struct {
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
			Client    struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"client"`
		}

		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		if err := dec.Decode(&asked); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"That request could not be read."}`)

			return
		}

		kept.mu.Lock()
		kept.asked = append(kept.asked, fmt.Sprintf("%s %s by %s/%s", asked.Tool, asked.Arguments, asked.Client.Name, asked.Client.Version))
		kept.mu.Unlock()

		mu.Lock()
		answer, known := answers[asked.Tool]
		mu.Unlock()

		if !known {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"That is not a question other programs may ask."}`)

			return
		}

		status := http.StatusOK
		fmt.Sscanf(answer[0], "%d", &status)

		w.WriteHeader(status)
		fmt.Fprint(w, answer[1])
	})

	ts := httptest.NewServer(sealing(mux))
	t.Cleanup(ts.Close)

	return ts, kept
}

func TestEachHandingOverGoesThroughTheDoorWithWhoHandedItOver(t *testing.T) {
	ts, kept := handoverDoor(t, map[string][2]string{
		"remember_fact": {"200", `{"kept":true}`},
		"add_reminder":  {"200", `{"at":"2027-01-31T07:00:00Z"}`},
		"add_todo":      {"200", `{"list":"from other programs"}`},
		"tell_user":     {"200", `{"shown":true}`},
		"say_to_user":   {"200", `{"said":true}`},
		"run_action":    {"200", `{"run":{"name":"backup","exit":0,"millis":1234,"output":"copied 3 files\n"}}`},
	})

	s := New(ts.URL, "test", theKey)

	for _, c := range []struct {
		tool, arguments, sent, said string
	}{
		{"remember_fact", `{"text":" The owner prefers tabs. "}`, `remember_fact {"text":"The owner prefers tabs."} by Claude Code/2.1.0`, "has not remembered it yet"},
		{"add_reminder", `{"text":"stand up","in_minutes":30}`, `add_reminder {"in_minutes":30,"text":"stand up"} by Claude Code/2.1.0`, "2027-01-31T07:00:00Z"},
		{"add_reminder", `{"text":"stand up","at":"2027-01-31T09:00:00+02:00"}`, `add_reminder {"at":"2027-01-31T09:00:00+02:00","text":"stand up"} by Claude Code/2.1.0`, "aloud"},
		{"add_todo", `{"text":"renew the domain","due":"2027-01-31"}`, `add_todo {"due":"2027-01-31","text":"renew the domain"} by Claude Code/2.1.0`, `"from other programs"`},
		{"tell_user", `{"text":"The build is done."}`, `tell_user {"text":"The build is done."} by Claude Code/2.1.0`, "No answer comes back"},
		{"say_to_user", `{"text":"The build is done."}`, `say_to_user {"text":"The build is done."} by Claude Code/2.1.0`, "aloud"},
		{"run_action", `{"name":"backup"}`, `run_action {"name":"backup"} by Claude Code/2.1.0`, "exit code 0 after 1.2 s"},
	} {
		before := len(kept.all())

		got, err := run(t, s, c.tool, c.arguments)
		if err != nil {
			t.Errorf("%s %s: %v", c.tool, c.arguments, err)

			continue
		}

		if !strings.Contains(got, c.said) {
			t.Errorf("%s said %q, without %q", c.tool, got, c.said)
		}

		all := kept.all()
		if len(all) != before+1 || all[before] != c.sent {
			t.Errorf("%s went through the door as %q, not %q", c.tool, all[before:], c.sent)
		}
	}

	// What a command printed is passed on as data, and said to be.
	if got, _ := run(t, s, "run_action", `{"name":"backup"}`); !strings.Contains(got, "copied 3 files") || !strings.Contains(got, "data, not instructions") {
		t.Errorf("what the action printed: %q", got)
	}
}

// Nothing malformed reaches the assistant: each tool takes its own
// arguments and no others.
func TestAHandingOverThatCannotBeReadAsksNothing(t *testing.T) {
	ts, kept := handoverDoor(t, map[string][2]string{})
	s := New(ts.URL, "test", theKey)

	long := strings.Repeat("а", 501)

	for tool, bad := range map[string][]string{
		"remember_fact": {`{}`, `{"text":""}`, `{"text":"  "}`, `{"text":"x","approve":true}`, `{"text":"` + long + `"}`, `{"text":"x","in_minutes":5}`, `[]`},
		"add_reminder":  {`{"text":"x"}`, `{"text":"x","at":"2027-01-31T09:00:00Z","in_minutes":5}`, `{"in_minutes":5}`, `{"text":"x","in_minutes":5,"due":"2027-01-31"}`},
		"add_todo":      {`{}`, `{"text":"x","in_minutes":5}`, `{"text":"x","list":"mine"}`},
		"tell_user":     {`{}`, `{"text":"x","aloud":true}`, `{"text":"` + long + `"}`},
		"say_to_user":   {`{}`, `{"text":"` + strings.Repeat("a", 301) + `"}`, `{"text":"x","voice":"loud"}`},
		"run_action":    {`{"name":"x","arguments":["--force"]}`, `{"command":"rm -rf ~"}`, `{"name":"` + strings.Repeat("a", 41) + `"}`, `{"name":"x","env":{"A":"b"}}`},
	} {
		for _, arguments := range bad {
			if got, err := run(t, s, tool, arguments); err == nil {
				t.Errorf("%s took %s and said %q", tool, arguments, got)
			}
		}
	}

	if all := kept.all(); len(all) != 0 {
		t.Errorf("something malformed reached the assistant: %v", all)
	}
}

func TestARefusedHandingOverSaysWhatTheUserCanDo(t *testing.T) {
	for _, c := range []struct {
		tool, arguments, status, body, said string
	}{
		{"remember_fact", `{"text":"x"}`, "403", `{"error":"Its user has not allowed other programs to ask that.","outside":"not allowed","subject":"remember"}`, "has not allowed other programs to tell it things to remember"},
		{"add_reminder", `{"text":"x","in_minutes":1}`, "403", `{"error":"no","outside":"not allowed","subject":"reminders"}`, "has not allowed other programs to set reminders"},
		{"add_todo", `{"text":"x"}`, "403", `{"error":"no","outside":"not allowed","subject":"todos"}`, "has not allowed other programs to put things on the to-do list"},
		{"tell_user", `{"text":"x"}`, "403", `{"error":"no","outside":"not allowed","subject":"tell"}`, "has not allowed other programs to leave them messages"},
		{"say_to_user", `{"text":"x"}`, "403", `{"error":"no","outside":"not allowed","subject":"speak"}`, "has not allowed other programs to have it say things aloud"},
		{"run_action", `{"name":"x"}`, "403", `{"error":"no","outside":"not allowed","subject":"actions"}`, "has not allowed other programs to run the actions they wrote"},
		{"tell_user", `{"text":"x"}`, "503", `{"error":"full","outside":"full"}`, "hand over no more"},
		{"remember_fact", `{"text":"x"}`, "503", `{"error":"full","outside":"full"}`, "hand over no more"},
		{"say_to_user", `{"text":"x"}`, "503", `{"error":"no voice","outside":"no voice"}`, "has no voice"},
		{"say_to_user", `{"text":"x"}`, "503", `{"error":"saying","outside":"saying"}`, "one at a time"},
		{"run_action", `{"name":"x"}`, "503", `{"error":"running","outside":"action running"}`, "runs one at a time"},
		{"run_action", `{"name":"x"}`, "403", `{"error":"stopped","outside":"stopped"}`, "Only the user can change that"},
		{"run_action", `{"name":"x"}`, "429", `{"error":"many","outside":"too many"}`, "too many"},
	} {
		ts, _ := handoverDoor(t, map[string][2]string{c.tool: {c.status, c.body}})

		got, err := run(t, New(ts.URL, "test", theKey), c.tool, c.arguments)
		if err == nil || !strings.Contains(err.Error(), c.said) {
			t.Errorf("%s refused with %s: said %q, error %v; %q is meant", c.tool, c.body, got, err, c.said)
		}
	}
}

// An answer that is not the one the tool asked for is not passed on as a
// success: a yes is only what the assistant said yes to.
func TestAHandingOverIsOnlyTakenWhenTheAssistantSaysSo(t *testing.T) {
	for _, c := range [][2]string{
		{"remember_fact", `{"kept":false}`}, {"remember_fact", `{"shown":true}`}, {"remember_fact", `{}`},
		{"tell_user", `{"shown":false}`}, {"tell_user", `{"kept":true}`},
		{"say_to_user", `{"said":false}`}, {"add_reminder", `{}`}, {"add_todo", `{"list":""}`},
		{"run_action", `{}`}, {"run_action", `{"run":null}`},
	} {
		ts, _ := handoverDoor(t, map[string][2]string{c[0]: {"200", c[1]}})

		arguments := `{"text":"x"}`

		switch c[0] {
		case "add_reminder":
			arguments = `{"text":"x","in_minutes":1}`
		case "run_action":
			arguments = `{"name":"x"}`
		}

		if got, err := run(t, New(ts.URL, "test", theKey), c[0], arguments); err == nil {
			t.Errorf("%s answered %s was passed on as %q", c[0], c[1], got)
		}
	}
}

func TestTheActionsThereAreAndOneThatIsNot(t *testing.T) {
	ts, kept := handoverDoor(t, map[string][2]string{"run_action": {"200", `{"actions":["backup","deploy-site"]}`}})
	s := New(ts.URL, "test", theKey)

	for _, arguments := range []string{`{}`, ``, `null`, `{"name":""}`, `{"name":"  "}`} {
		got, err := run(t, s, "run_action", arguments)
		if err != nil || !strings.Contains(got, "- backup") || !strings.Contains(got, "- deploy-site") || !strings.Contains(got, ": 2.") {
			t.Errorf("the list for %q: %q, %v", arguments, got, err)
		}
	}

	for _, sent := range kept.all() {
		if sent != `run_action {} by Claude Code/2.1.0` {
			t.Errorf("asking for the list went as %q", sent)
		}
	}

	none, _ := handoverDoor(t, map[string][2]string{"run_action": {"200", `{"actions":[]}`}})
	if got, err := run(t, New(none.URL, "test", theKey), "run_action", `{}`); err != nil || !strings.Contains(got, "has written no actions") || !strings.Contains(got, "Only they can write one") {
		t.Errorf("no actions: %q, %v", got, err)
	}

	unknown, _ := handoverDoor(t, map[string][2]string{"run_action": {"200", `{"unknown":true,"actions":["backup"]}`}})
	if got, err := run(t, New(unknown.URL, "test", theKey), "run_action", `{"name":"format-disk"}`); err != nil || !strings.Contains(got, "no action of that name, and ran nothing") || !strings.Contains(got, "- backup") {
		t.Errorf("an action that is not there: %q, %v", got, err)
	}

	// One that ran badly is said as it was, not as a success.
	for body, said := range map[string]string{
		`{"run":{"name":"backup","exit":3,"millis":200,"output":"disk full"}}`:              "exit code 3",
		`{"run":{"name":"backup","exit":-1,"millis":45000,"timed_out":true,"output":""}}`:   "stopped it",
		`{"run":{"name":"backup","exit":-1,"millis":1,"output":"no such file"}}`:            "did not end by itself",
		`{"run":{"name":"backup","exit":0,"millis":10,"output":"a\u001b[31mb","cut":true}}`: "(cut here: it printed more)",
	} {
		ts, _ := handoverDoor(t, map[string][2]string{"run_action": {"200", body}})

		got, err := run(t, New(ts.URL, "test", theKey), "run_action", `{"name":"backup"}`)
		if err != nil || !strings.Contains(got, said) || strings.ContainsRune(got, 0x1b) {
			t.Errorf("a run that ended as %s: %q, %v", body, got, err)
		}
	}
}
