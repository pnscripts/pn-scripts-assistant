/*
 * Package quiethours is the window its user sets aside for heavy work.
 *
 * Coding agents, long goals and learning spend paid allowances that renew on
 * their own clock (five hours, a week). Spent faster than they renew, they run
 * out before the week does, and the work that needed them most waits anyway,
 * at a worse time. A window of quiet hours ("22:00-07:00") is where that work
 * goes instead: outside it, work on an allowance that is ahead of its pace
 * is held and says until when, and its user can start it at once with one
 * yes. Work on an allowance that is on pace is never held, and with no window
 * set nothing is.
 *
 * Read by the wall clock of the machine, so a summer time change moves the
 * window with the clock on the wall rather than by a fixed number of hours.
 */
package quiethours

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Window is a span of the day, from Start to End in minutes after midnight.
// End before Start means it runs across midnight.
type Window struct {
	start, end int
	set        bool
}

// ErrNotAWindow is a window that could not be meant.
var ErrNotAWindow = errors.New(`quiet hours are written as a start and an end, like "22:00-07:00"`)

// Parse reads "22:00-07:00" (spaces and a single digit hour allowed). Empty
// is no window.
func Parse(s string) (Window, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "–", "-"))
	if s == "" {
		return Window{}, nil
	}

	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return Window{}, ErrNotAWindow
	}

	start, ok1 := clock(parts[0])
	end, ok2 := clock(parts[1])

	if !ok1 || !ok2 {
		return Window{}, ErrNotAWindow
	}

	if start == end {
		return Window{}, errors.New("quiet hours that start when they end are either always or never: give two different times")
	}

	return Window{start: start, end: end, set: true}, nil
}

// clock reads "7:00" or "07:00" as minutes after midnight.
func clock(s string) (int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || len(h) < 1 || len(h) > 2 || len(m) != 2 {
		return 0, false
	}

	hour, err1 := strconv.Atoi(h)
	minute, err2 := strconv.Atoi(m)

	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 ||
		strings.ContainsAny(h+m, "+-") {
		return 0, false
	}

	return hour*60 + minute, true
}

// Set says whether there is a window at all.
func (w Window) Set() bool { return w.set }

// String is the window as it is saved: "22:00-07:00", or empty.
func (w Window) String() string {
	if !w.set {
		return ""
	}

	return hhmm(w.start) + "-" + hhmm(w.end)
}

// From and To are the two ends as a person reads them.
func (w Window) From() string { return hhmm(w.start) }
func (w Window) To() string   { return hhmm(w.end) }

func hhmm(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// Contains says whether t's wall clock is inside the window.
func (w Window) Contains(t time.Time) bool {
	if !w.set {
		return false
	}

	m := t.Hour()*60 + t.Minute()

	if w.start < w.end {
		return m >= w.start && m < w.end
	}

	return m >= w.start || m < w.end
}

// Opens is when the window next opens: t itself when it is open now.
func (w Window) Opens(t time.Time) time.Time {
	if !w.set || w.Contains(t) {
		return t
	}

	y, mo, d := t.Date()
	today := time.Date(y, mo, d, w.start/60, w.start%60, 0, 0, t.Location())

	if today.After(t) {
		return today
	}

	return time.Date(y, mo, d+1, w.start/60, w.start%60, 0, 0, t.Location())
}

// Hold says whether work on an allowance is held at now, and until when.
// Only work whose allowance is ahead of its pace, only outside the window.
func Hold(w Window, ahead bool, now time.Time) (bool, time.Time) {
	if !w.set || !ahead || w.Contains(now) {
		return false, time.Time{}
	}

	return true, w.Opens(now)
}

// Say is the hold in a sentence for the person asked, said at now.
func Say(w Window, opens, now time.Time) string {
	day := "today"

	ny, nm, nd := now.Date()
	if oy, om, od := opens.Date(); oy != ny || om != nm || od != nd {
		day = "tomorrow"
	}

	return fmt.Sprintf("Held for your quiet hours (%s to %s): its allowance is being spent faster than it renews, "+
		"so heavy work waits until %s %s. Ask again then, or say yes to start it now anyway.",
		w.From(), w.To(), hhmm(opens.Hour()*60+opens.Minute()), day)
}

// Held is the sentence for work on an allowance at now under the saved
// setting, or empty when it is not held (no window, a window that cannot be
// read, an allowance on pace, or the window open).
func Held(setting string, ahead bool, now time.Time) string {
	w, err := Parse(setting)
	if err != nil {
		return ""
	}

	held, opens := Hold(w, ahead, now)
	if !held {
		return ""
	}

	return Say(w, opens, now)
}
