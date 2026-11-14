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
 * Work nobody asked for at that moment (a goal that starts itself, reading
 * the places it looks after) is different: there is nobody to say yes, and
 * it is the heaviest work there is on a processor somebody is using. With a
 * window set, it waits for the window whatever the allowance, and starts in
 * it by itself; the page says so, and its own button starts it at once.
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
func Say(w Window, opens, now time.Time) string { return SayIn(w, opens, now, "en") }

// SayIn is the hold in a sentence in a language ("en" or "bg").
func SayIn(w Window, opens, now time.Time, lang string) string {
	if lang == "bg" {
		return fmt.Sprintf("Задържано за тихите ви часове (от %s до %s): лимитът му се изразходва по-бързо, "+
			"отколкото се подновява, затова тежката работа изчаква до %s %s. Отговорете с „да“, за да започне веднага.",
			w.From(), w.To(), hhmm(opens.Hour()*60+opens.Minute()), dayOfIn(opens, now, lang))
	}

	return fmt.Sprintf("Held for your quiet hours (%s to %s): its allowance is being spent faster than it renews, "+
		"so heavy work waits until %s %s. Say yes to start it now anyway.",
		w.From(), w.To(), hhmm(opens.Hour()*60+opens.Minute()), dayOf(opens, now))
}

/*
 * What becomes of a held question when the window opens.
 *
 * A call held for the quiet hours and for nothing else would have run
 * without a question at any other time: its user's settings already said
 * yes to it. So when the window opens it answers itself, unless its user
 * answered first. A call that would have been asked anyway (full access, a
 * permission set to ask, a rule, a protected file) waits for its user
 * whenever that is, and says so.
 */
const (
	StartsItself = "If you have not answered by then, it starts by itself when they open."
	WaitsForYou  = "It does not start by itself: answer it when you want it to run."

	// The same two in Bulgarian, for the approval card on a Bulgarian page.
	StartsItselfBG = "Ако дотогава не сте отговорили, започва автоматично, когато настъпят тихите часове."
	WaitsForYouBG  = "Не започва автоматично: отговорете, когато искате да се изпълни."
)

// OpenNow says whether the saved setting is a window and it is open at now:
// the moment held questions that answer themselves are answered. No window,
// or one that cannot be read, is never open, so nothing answers itself.
func OpenNow(setting string, now time.Time) bool {
	w, err := Parse(setting)

	return err == nil && w.Contains(now)
}

// Held is the sentence for work on an allowance at now under the saved
// setting, or empty when it is not held (no window, a window that cannot be
// read, an allowance on pace, or the window open).
func Held(setting string, ahead bool, now time.Time) string {
	return HeldIn(setting, ahead, now, "en")
}

// HeldIn is Held in a language ("en" or "bg").
func HeldIn(setting string, ahead bool, now time.Time, lang string) string {
	w, err := Parse(setting)
	if err != nil {
		return ""
	}

	held, opens := Hold(w, ahead, now)
	if !held {
		return ""
	}

	return SayIn(w, opens, now, lang)
}

// Waits says whether work nobody asked for at that moment waits at now under
// the saved setting, and when the window opens. A setting that cannot be read
// holds nothing back.
func Waits(setting string, now time.Time) (bool, time.Time) {
	w, err := Parse(setting)
	if err != nil || !w.Set() || w.Contains(now) {
		return false, time.Time{}
	}

	return true, w.Opens(now)
}

// Sleep is how long a loop that looks every round should wait from now: a
// whole round, or less when the window opens sooner, so work waiting for it
// starts at the opening rather than up to a round later. Never under a
// second, so a loop woken a moment early does not spin.
func Sleep(setting string, now time.Time, round time.Duration) time.Duration {
	waits, opens := Waits(setting, now)
	if !waits {
		return round
	}

	until := opens.Sub(now)

	switch {
	case until < time.Second:
		return time.Second
	case until < round:
		return until
	}

	return round
}

// Waiting is the sentence for work nobody asked for that waits at now, or
// empty when it does not.
func Waiting(setting string, now time.Time) string {
	waits, opens := Waits(setting, now)
	if !waits {
		return ""
	}

	w, _ := Parse(setting)

	return fmt.Sprintf("Waiting for your quiet hours (%s to %s): it starts by itself at %s %s.",
		w.From(), w.To(), hhmm(opens.Hour()*60+opens.Minute()), dayOf(opens, now))
}

// dayOf is "today" or "tomorrow" for when, said at now.
func dayOf(when, now time.Time) string { return dayOfIn(when, now, "en") }

func dayOfIn(when, now time.Time, lang string) string {
	ny, nm, nd := now.Date()
	if wy, wm, wd := when.Date(); wy != ny || wm != nm || wd != nd {
		if lang == "bg" {
			return "утре"
		}

		return "tomorrow"
	}

	if lang == "bg" {
		return "днес"
	}

	return "today"
}
