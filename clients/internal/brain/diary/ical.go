/*
 * Package diary reads and writes calendar files.
 *
 * The interop that stands in for an account. A calendar was avoided here for a
 * long time and the reason was sound — it is the kind of thing that quietly
 * ends up on somebody else's server — so this one is a table in the brain's
 * own database, and .ics is how things get in and out when somebody decides
 * they should. Nothing here talks to a network.
 *
 * Not a complete iCalendar implementation, and not trying to be. It reads the
 * events out of what calendars actually export and writes something they will
 * read back. Recurrence is the honest gap: an event that repeats is imported
 * as the one occurrence it names, because expanding an RRULE wrongly is worse
 * than not claiming to.
 */
package diary

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

// Read pulls the events out of an iCalendar file.
func Read(text string, cameFrom string) ([]store.Event, error) {
	var (
		out     []store.Event
		current *store.Event
		inside  bool
	)

	for _, line := range unfold(text) {
		name, value := field(line)

		switch {
		case name == "BEGIN" && value == "VEVENT":
			inside = true
			current = &store.Event{CameFrom: cameFrom}

			continue

		case name == "END" && value == "VEVENT":
			if current != nil && strings.TrimSpace(current.Title) != "" {
				finish(current)
				out = append(out, *current)
			}

			inside, current = false, nil

			continue
		}

		if !inside || current == nil {
			continue
		}

		key, params := split(name)

		switch key {
		case "UID":
			current.UID = value
		case "SUMMARY":
			current.Title = unescape(value)
		case "LOCATION":
			current.Place = unescape(value)
		case "DESCRIPTION":
			current.Notes = unescape(value)
		case "DTSTART":
			at, allDay, ok := moment(value, params)
			if ok {
				current.Starts, current.AllDay = at, allDay
			}
		case "DTEND":
			if at, _, ok := moment(value, params); ok {
				current.Ends = at
			}
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no events were found in that file")
	}

	return out, nil
}

/*
 * finish fills in what the file left out.
 *
 * A VEVENT with no DTEND is legal and common — it means an hour for a timed
 * event and the whole of it for a day. Leaving it at zero would put every such
 * event at the start of 1970 and make the diary useless in a way that looks
 * like a parsing bug rather than a missing field.
 */
func finish(e *store.Event) {
	if !e.Ends.IsZero() {
		return
	}

	if e.AllDay {
		e.Ends = e.Starts.AddDate(0, 0, 1)

		return
	}

	e.Ends = e.Starts.Add(time.Hour)
}

/*
 * unfold puts continued lines back together.
 *
 * iCalendar wraps at 75 characters and continues with a leading space, so a
 * description of any length arrives in pieces. Reading them as separate lines
 * loses most of every long field, quietly.
 */
func unfold(text string) []string {
	var out []string

	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		if len(out) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			out[len(out)-1] += line[1:]

			continue
		}

		out = append(out, line)
	}

	return out
}

func field(line string) (string, string) {
	name, value, found := strings.Cut(line, ":")

	if !found {
		return strings.ToUpper(strings.TrimSpace(line)), ""
	}

	return strings.ToUpper(strings.TrimSpace(name)), strings.TrimSpace(value)
}

// split separates a field name from its parameters: DTSTART;VALUE=DATE.
func split(name string) (string, string) {
	key, params, _ := strings.Cut(name, ";")

	return key, params
}

/*
 * moment reads a date or a date and time.
 *
 * Three shapes in practice: a bare date for a whole day, a local time, and a
 * UTC time ending in Z. A time zone named in a TZID parameter is read as local
 * — this machine's, not the one it was written in — which is wrong for a
 * calendar carried across a continent and right for every other case. Saying
 * so here rather than pretending it is handled.
 */
func moment(value, params string) (time.Time, bool, bool) {
	value = strings.TrimSpace(value)

	if strings.Contains(strings.ToUpper(params), "VALUE=DATE") || len(value) == 8 {
		at, err := time.ParseInLocation("20060102", value, time.Local)

		return at, true, err == nil
	}

	if strings.HasSuffix(value, "Z") {
		at, err := time.Parse("20060102T150405Z", value)

		return at.Local(), false, err == nil
	}

	at, err := time.ParseInLocation("20060102T150405", value, time.Local)

	return at, false, err == nil
}

func unescape(value string) string {
	return strings.NewReplacer(
		`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`,
	).Replace(value)
}

func escape(value string) string {
	return strings.NewReplacer(
		`\`, `\\`, "\n", `\n`, ",", `\,`, ";", `\;`,
	).Replace(value)
}

// Write turns events into a file any calendar will read.
func Write(events []store.Event) string {
	var b strings.Builder

	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//PN Scripts Assistant//EN\r\n")

	for _, e := range events {
		b.WriteString("BEGIN:VEVENT\r\n")

		uid := e.UID
		if uid == "" {
			// Its own identity, so exporting and re-importing updates what is
			// there rather than making a second copy of everything.
			uid = fmt.Sprintf("%d@pn-scripts-assistant", e.ID)
		}

		b.WriteString("UID:" + uid + "\r\n")
		b.WriteString("SUMMARY:" + escape(e.Title) + "\r\n")

		if e.AllDay {
			b.WriteString("DTSTART;VALUE=DATE:" + e.Starts.Format("20060102") + "\r\n")
			b.WriteString("DTEND;VALUE=DATE:" + e.Ends.Format("20060102") + "\r\n")
		} else {
			b.WriteString("DTSTART:" + e.Starts.UTC().Format("20060102T150405Z") + "\r\n")
			b.WriteString("DTEND:" + e.Ends.UTC().Format("20060102T150405Z") + "\r\n")
		}

		if e.Place != "" {
			b.WriteString("LOCATION:" + escape(e.Place) + "\r\n")
		}

		if e.Notes != "" {
			b.WriteString("DESCRIPTION:" + escape(e.Notes) + "\r\n")
		}

		b.WriteString("END:VEVENT\r\n")
	}

	b.WriteString("END:VCALENDAR\r\n")

	return b.String()
}
