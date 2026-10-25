/*
 * Package caldav reads and adds appointments in an online calendar.
 *
 * Most people's calendar is not a file on their computer; it is on a server
 * — Nextcloud, Fastmail, iCloud, a hosting company's — and every one of those
 * speaks CalDAV: HTTP with an XML question in it, and iCalendar coming back.
 * This is the small part of it an assistant needs: is the address right,
 * what is on between two dates, and put this appointment in. Nothing is ever
 * changed or deleted on the server; a new appointment is written beside the
 * others, and refused if one with its identity is already there.
 *
 * The address, the user and the password are typed by the owner, like the
 * mail account's, and go only to that address. Over plain http the password
 * would travel readable, so only https is accepted — or http to this machine,
 * for a calendar server of one's own.
 */
package caldav

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/diary"
	"pn-scripts-assistant/internal/brain/store"
)

// Calendar is one calendar on a server: the address of the collection.
type Calendar struct {
	URL      string
	User     string
	Password string

	// Client is the HTTP client; nil means one with a 20-second timeout.
	Client *http.Client
}

// Configured reports whether there is enough to try.
func (c Calendar) Configured() bool { return strings.TrimSpace(c.URL) != "" }

// Errors worth telling apart.
var (
	ErrNotSetUp  = errors.New("no online calendar is set up: give its CalDAV address, user and password under Plan")
	ErrRefused   = errors.New("the calendar server refused the user name or password")
	ErrNotHTTPS  = errors.New("the calendar address must start with https:// — over http the password would travel readable")
	ErrNotFound  = errors.New("there is no calendar at that address; it should be the calendar's own CalDAV address")
	ErrDuplicate = errors.New("an appointment with that identity is already in the calendar")
)

func (c Calendar) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}

	return &http.Client{Timeout: 20 * time.Second}
}

// base is the collection address, checked, ending in a slash.
func (c Calendar) base() (*url.URL, error) {
	if !c.Configured() {
		return nil, ErrNotSetUp
	}

	u, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%q is not a web address", c.URL)
	}

	switch u.Scheme {
	case "https":
	case "http":
		if !local(u.Hostname()) {
			return nil, ErrNotHTTPS
		}
	default:
		return nil, ErrNotHTTPS
	}

	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}

	return u, nil
}

func local(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

func (c Calendar) do(ctx context.Context, method, target string, body string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	if c.User != "" || c.Password != "" {
		req.SetBasicAuth(c.User, c.Password)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("the calendar server could not be reached: %w", err)
	}

	switch res.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		res.Body.Close()

		return nil, ErrRefused
	case http.StatusNotFound:
		res.Body.Close()

		return nil, ErrNotFound
	}

	return res, nil
}

// Check asks the server whether the address is a calendar this user can
// read, and returns its name when it says one.
func (c Calendar) Check(ctx context.Context) (string, error) {
	u, err := c.base()
	if err != nil {
		return "", err
	}

	res, err := c.do(ctx, "PROPFIND", u.String(), `<?xml version="1.0" encoding="utf-8"?>`+
		`<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+
		`<d:prop><d:displayname/><d:resourcetype/></d:prop></d:propfind>`,
		map[string]string{"Depth": "0", "Content-Type": "application/xml; charset=utf-8"})
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusMultiStatus {
		return "", fmt.Errorf("the server answered %s, not as a calendar", res.Status)
	}

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))

	var ms multistatus

	if err := xml.Unmarshal(raw, &ms); err != nil {
		return "", fmt.Errorf("the server's answer could not be read: %w", err)
	}

	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if ps.Prop.ResourceType.Calendar != nil {
				name := strings.TrimSpace(ps.Prop.DisplayName)
				if name == "" {
					name = "calendar"
				}

				return name, nil
			}
		}
	}

	return "", errors.New("that address is a folder on the server but not a calendar; open the calendar's settings for its own CalDAV address")
}

type multistatus struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Prop struct {
				DisplayName  string `xml:"displayname"`
				CalendarData string `xml:"calendar-data"`
				ResourceType struct {
					Calendar *struct{} `xml:"calendar"`
				} `xml:"resourcetype"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

// Events is what is on between from and to, soonest first.
func (c Calendar) Events(ctx context.Context, from, to time.Time) ([]store.Event, error) {
	u, err := c.base()
	if err != nil {
		return nil, err
	}

	query := `<?xml version="1.0" encoding="utf-8"?>` +
		`<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">` +
		`<d:prop><d:getetag/><c:calendar-data/></d:prop>` +
		`<c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT">` +
		`<c:time-range start="` + from.UTC().Format("20060102T150405Z") + `" end="` + to.UTC().Format("20060102T150405Z") + `"/>` +
		`</c:comp-filter></c:comp-filter></c:filter></c:calendar-query>`

	res, err := c.do(ctx, "REPORT", u.String(), query,
		map[string]string{"Depth": "1", "Content-Type": "application/xml; charset=utf-8"})
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("the calendar server answered %s", res.Status)
	}

	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}

	var ms multistatus

	if err := xml.Unmarshal(raw, &ms); err != nil {
		return nil, fmt.Errorf("the calendar's answer could not be read: %w", err)
	}

	var out []store.Event

	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if strings.TrimSpace(ps.Prop.CalendarData) == "" {
				continue
			}

			events, err := diary.Read(ps.Prop.CalendarData, "online calendar")
			if err != nil {
				continue
			}

			for _, e := range events {
				// A repeating event comes back as its first occurrence (see
				// the diary package); one that started before the window and
				// does not reach into it is not "on" in it.
				if e.Ends.Before(from) || e.Starts.After(to) {
					continue
				}

				out = append(out, e)
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Starts.Before(out[j].Starts) })

	return out, nil
}

// Add puts a new appointment in the calendar and returns its identity. It
// never replaces one that is there.
func (c Calendar) Add(ctx context.Context, e store.Event) (string, error) {
	u, err := c.base()
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(e.Title) == "" || e.Starts.IsZero() {
		return "", errors.New("an appointment needs a title and a time")
	}

	if e.Ends.IsZero() || !e.Ends.After(e.Starts) {
		if e.AllDay {
			e.Ends = e.Starts.AddDate(0, 0, 1)
		} else {
			e.Ends = e.Starts.Add(time.Hour)
		}
	}

	if e.UID == "" {
		var b [12]byte

		_, _ = rand.Read(b[:])
		e.UID = hex.EncodeToString(b[:]) + "@pn-scripts-assistant"
	}

	target := u.ResolveReference(&url.URL{Path: url.PathEscape(strings.TrimSuffix(e.UID, "@pn-scripts-assistant")) + ".ics"})

	res, err := c.do(ctx, http.MethodPut, target.String(), diary.Write([]store.Event{e}),
		map[string]string{"Content-Type": "text/calendar; charset=utf-8", "If-None-Match": "*"})
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusCreated, http.StatusNoContent, http.StatusOK:
		return e.UID, nil
	case http.StatusPreconditionFailed:
		return "", ErrDuplicate
	}

	detail, _ := io.ReadAll(io.LimitReader(res.Body, 500))

	return "", fmt.Errorf("the calendar server did not take the appointment: %s %s", res.Status, strings.TrimSpace(string(detail)))
}
