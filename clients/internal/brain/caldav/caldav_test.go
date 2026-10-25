package caldav

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

// server is a small CalDAV calendar at /cal/home/ for user anna.
type server struct {
	mu     sync.Mutex
	stored map[string]string
	asked  string
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, pass, _ := r.BasicAuth()
	if user != "anna" || pass != "app-password" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/cal/home/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	body, _ := io.ReadAll(r.Body)

	s.mu.Lock()
	defer s.mu.Unlock()

	switch r.Method {
	case "PROPFIND":
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+
			`<d:response><d:href>/cal/home/</d:href><d:propstat><d:prop><d:displayname>Семейство</d:displayname>`+
			`<d:resourcetype><d:collection/><c:calendar/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`)
	case "REPORT":
		s.asked = string(body)
		w.WriteHeader(http.StatusMultiStatus)

		var b strings.Builder

		b.WriteString(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`)

		for href, ics := range s.stored {
			b.WriteString(`<d:response><d:href>` + href + `</d:href><d:propstat><d:prop><c:calendar-data>`)
			xmlEscape(&b, ics)
			b.WriteString(`</c:calendar-data></d:prop></d:propstat></d:response>`)
		}

		b.WriteString(`</d:multistatus>`)
		_, _ = io.WriteString(w, b.String())
	case http.MethodPut:
		if _, there := s.stored[r.URL.Path]; there && r.Header.Get("If-None-Match") == "*" {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		if !strings.Contains(string(body), "DTSTAMP:") || r.Header.Get("Content-Type") != "text/calendar; charset=utf-8" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		s.stored[r.URL.Path] = string(body)
		w.WriteHeader(http.StatusCreated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func xmlEscape(b *strings.Builder, s string) {
	b.WriteString(strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s))
}

func TestACalendarIsCheckedReadAndAddedTo(t *testing.T) {
	srv := &server{stored: map[string]string{
		"/cal/home/old.ics": "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:old\r\nSUMMARY:Dentist & check\r\n" +
			"DTSTART:20261027T080000Z\r\nDTEND:20261027T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	}}

	ts := httptest.NewServer(srv)
	defer ts.Close()

	cal := Calendar{URL: ts.URL + "/cal/home", User: "anna", Password: "app-password"}
	ctx := context.Background()

	name, err := cal.Check(ctx)
	if err != nil || name != "Семейство" {
		t.Fatalf("%q %v", name, err)
	}

	from := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)

	events, err := cal.Events(ctx, from, from.AddDate(0, 0, 7))
	if err != nil || len(events) != 1 || events[0].Title != "Dentist & check" || events[0].CameFrom != "online calendar" {
		t.Fatalf("%+v %v", events, err)
	}

	if !strings.Contains(srv.asked, `start="20261026T000000Z"`) {
		t.Errorf("the question did not carry the dates: %s", srv.asked)
	}

	uid, err := cal.Add(ctx, store.Event{Title: "Родителска среща", Starts: from.Add(30 * time.Hour), Place: "Училище"})
	if err != nil || !strings.HasSuffix(uid, "@pn-scripts-assistant") {
		t.Fatal(uid, err)
	}

	events, _ = cal.Events(ctx, from, from.AddDate(0, 0, 7))
	if len(events) != 2 || events[0].Title != "Родителска среща" || events[0].Ends.Sub(events[0].Starts) != time.Hour {
		t.Errorf("%+v", events)
	}

	if _, err := cal.Add(ctx, store.Event{UID: uid, Title: "Again", Starts: from}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("an appointment was replaced: %v", err)
	}
}

func TestTheCalendarSaysWhatIsWrong(t *testing.T) {
	ts := httptest.NewServer(&server{stored: map[string]string{}})
	defer ts.Close()

	ctx := context.Background()

	cases := []struct {
		cal  Calendar
		want error
	}{
		{Calendar{}, ErrNotSetUp},
		{Calendar{URL: ts.URL + "/cal/home/", User: "anna", Password: "wrong"}, ErrRefused},
		{Calendar{URL: ts.URL + "/elsewhere/", User: "anna", Password: "app-password"}, ErrNotFound},
		{Calendar{URL: "http://calendar.example.com/cal/", User: "anna", Password: "x"}, ErrNotHTTPS},
		{Calendar{URL: "ftp://x/", User: "a"}, ErrNotHTTPS},
	}

	for _, c := range cases {
		if _, err := c.cal.Check(ctx); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.cal.URL, err, c.want)
		}
	}
}
