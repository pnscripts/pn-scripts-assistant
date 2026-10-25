package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

func TestToDosAndThePlanFromThePage(t *testing.T) {
	ts, _, _ := newServer(t)

	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	var item store.Todo

	if code := sendJSON(t, ts.URL+"/api/todos", map[string]any{"text": "Send the offer", "list": "Work", "due": tomorrow}, &item); code != http.StatusOK || item.List != "work" {
		t.Fatalf("%d %+v", code, item)
	}

	if code := sendJSON(t, ts.URL+"/api/todos", map[string]any{"text": "x", "due": "next week"}, nil); code != http.StatusBadRequest {
		t.Errorf("a bad date answered %d", code)
	}

	var plan struct {
		Agenda struct {
			Coming []struct{ Kind, What string } `json:"coming"`
		} `json:"agenda"`
		Calendar struct {
			Configured bool `json:"configured"`
		} `json:"calendar"`
	}

	getJSON(t, ts.URL+"/api/plan?days=3", &plan)

	if len(plan.Agenda.Coming) != 1 || plan.Agenda.Coming[0].Kind != "todo" || plan.Calendar.Configured {
		t.Errorf("%+v", plan)
	}

	id := strconv.FormatInt(item.ID, 10)

	if code := sendJSON(t, ts.URL+"/api/todos/"+id+"/tick", map[string]any{"done": true}, &item); code != http.StatusOK || item.DoneAt == nil {
		t.Errorf("%d %+v", code, item)
	}

	if code := sendJSON(t, ts.URL+"/api/todos/"+id+"/remove", nil, nil); code != http.StatusOK {
		t.Errorf("removing answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/todos/"+id+"/tick", map[string]any{"done": true}, nil); code != http.StatusNotFound {
		t.Errorf("ticking a removed item answered %d", code)
	}
}

/*
 * Setting up the online calendar asks the server first, keeps the address,
 * and offers the calendar tools from then on; removing it takes them away.
 */
func TestTheOnlineCalendarIsSetUpAndRemoved(t *testing.T) {
	ts, _, b := newServer(t)

	calendar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)

		if _, pass, _ := r.BasicAuth(); pass != "app-password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:propstat><d:prop>`+
			`<d:displayname>Home</d:displayname><d:resourcetype><d:collection/><c:calendar/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`)
	}))
	defer calendar.Close()

	if code := sendJSON(t, ts.URL+"/api/settings/calendar", map[string]any{"url": calendar.URL + "/cal/", "user": "anna", "password": "wrong"}, nil); code != http.StatusBadRequest {
		t.Errorf("a wrong password answered %d", code)
	}

	if b.Cfg.CalendarURL != "" {
		t.Fatal("a calendar that refused was kept")
	}

	var set struct {
		Configured bool   `json:"configured"`
		Name       string `json:"name"`
	}

	if code := sendJSON(t, ts.URL+"/api/settings/calendar", map[string]any{"url": calendar.URL + "/cal/", "user": "anna", "password": "app-password"}, &set); code != http.StatusOK || set.Name != "Home" {
		t.Fatalf("%d %+v", code, set)
	}

	if _, there := b.Agent.Registry.Get("read_calendar"); !there {
		t.Error("the calendar tools were not offered once it was set up")
	}

	// An empty password keeps the saved one.
	if code := sendJSON(t, ts.URL+"/api/settings/calendar", map[string]any{"url": calendar.URL + "/cal/", "user": "anna"}, nil); code != http.StatusOK {
		t.Errorf("saving again without retyping the password answered %d", code)
	}

	sendJSON(t, ts.URL+"/api/settings/calendar", map[string]any{"remove": true}, nil)

	if _, there := b.Agent.Registry.Get("write_calendar"); there || b.Cfg.CalendarPassword != "" {
		t.Error("removing the calendar left its tools or its password")
	}
}
