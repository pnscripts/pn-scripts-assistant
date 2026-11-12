package machinecare

import (
	"errors"
	"strings"
	"time"
)

/*
 * "Tell me when I can restart."
 *
 * Read answers whether a restart is fine now. When it is not (software
 * being installed, a driver not yet built for the waiting kernel, the
 * assistant's own work still running) the useful answer is a promise: look
 * again every few minutes and say so once it is. A Watch is that promise,
 * kept as plain state so the rules are tested without a clock: it ends the
 * moment the machine is ready, it gives up at a deadline it states at the
 * start (a waiter with no end is a leftover of its own), and one that ended
 * never changes again.
 *
 * It looks and says; restarting is always its user's.
 */

// How a watch stands.
const (
	Watching   = "watching"
	Safe       = "safe"
	GaveUp     = "gave_up"
	CannotTell = "cannot_tell"
	Stopped    = "stopped"
)

// DefaultWatch is how long a watch lasts when nobody says, MostWatch the
// longest one accepted, and HowOftenToLook how often it reads the machine.
const (
	DefaultWatch   = 4 * time.Hour
	MostWatch      = 12 * time.Hour
	HowOftenToLook = 2 * time.Minute
)

// ErrWatchTooLong refuses a watch past MostWatch, or one with no time at all.
var ErrWatchTooLong = errors.New("a restart is watched for up to 12 hours")

// Watch is one "tell me when I can restart".
type Watch struct {
	State string    `json:"state"`
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`

	// Ended is when it became safe, gave up or was stopped.
	Ended time.Time `json:"ended,omitempty"`

	// Looked is the last time the machine was read.
	Looked time.Time `json:"looked,omitempty"`

	// InTheWay is what stood in the way at the last look: empty once safe.
	InTheWay []Item `json:"in_the_way"`

	// Conversation is where it was asked for, to say the outcome there.
	Conversation int64 `json:"-"`
}

// NewWatch starts a watch at now for d (DefaultWatch when d is zero).
func NewWatch(now time.Time, d time.Duration) (Watch, error) {
	if d == 0 {
		d = DefaultWatch
	}

	if d < 0 || d > MostWatch {
		return Watch{}, ErrWatchTooLong
	}

	return Watch{State: Watching, Since: now, Until: now.Add(d), InTheWay: []Item{}}, nil
}

// Done is whether the watch has ended, one way or another.
func (w Watch) Done() bool { return w.State != Watching }

/*
 * Look applies one reading of the machine at now, and reports whether the
 * watch ended with it. Ready ends it as Safe; a machine that cannot be
 * read ends it as CannotTell; reaching Until while something is still in
 * the way ends it as GaveUp.
 */
func (w *Watch) Look(r Readiness, now time.Time) bool {
	if w.Done() {
		return false
	}

	w.Looked = now
	w.InTheWay = []Item{}

	switch {
	case !r.Known:
		w.State = CannotTell
	case r.Ready:
		w.State = Safe
	default:
		for _, i := range r.Items {
			if i.Blocking {
				w.InTheWay = append(w.InTheWay, i)
			}
		}

		if now.Before(w.Until) {
			return false
		}

		w.State = GaveUp
	}

	w.Ended = now

	return true
}

// Stop ends a watch its user no longer wants.
func (w *Watch) Stop(now time.Time) {
	if !w.Done() {
		w.State, w.Ended = Stopped, now
	}
}

// inTheWay says a blocking item short enough to list, in English and in
// Bulgarian; the page has the long forms in i18n.js (mcare.r.*).
var inTheWay = map[string]func(a map[string]string, bg bool) string{
	"dkms_missing": func(a map[string]string, bg bool) string {
		if bg {
			return "драйверът " + a["module"] + " не е компилиран за " + a["kernel"]
		}

		return "the driver " + a["module"] + " is not built for " + a["kernel"]
	},
	"installing": func(a map[string]string, bg bool) string {
		if bg {
			return a["what"] + " инсталира или обновява софтуер"
		}

		return a["what"] + " is installing or updating software"
	},
	"busy": func(a map[string]string, bg bool) string {
		if bg {
			return "асистентът все още работи по „" + a["what"] + "“"
		}

		return "the assistant is still working on “" + a["what"] + "”"
	},
}

// Briefly says one item short enough to list after others, in English or
// in Bulgarian.
func Briefly(i Item, bg bool) string {
	if say, ok := inTheWay[i.Key]; ok {
		return say(i.Args, bg)
	}

	return strings.TrimSuffix(i.Say, ".")
}

/*
 * Said is what the assistant says when the watch ends, in its user's
 * language ("bg" or anything else for English): out loud as a reminder
 * and in the conversation it was asked in. Empty while it is still going
 * or when its user stopped it.
 */
func (w Watch) Said(language string) string {
	bg := strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "bg")

	from, to := w.Since.Local().Format("15:04"), w.Until.Local().Format("15:04")

	var why []string

	for _, i := range w.InTheWay {
		why = append(why, Briefly(i, bg))
	}

	switch {
	case w.State == Safe && bg:
		return "Вече може да рестартирате: нищо няма да бъде прекъснато и нищо няма да липсва след това."
	case w.State == Safe:
		return "It is safe to restart now: nothing would be cut off, and nothing would come back missing."
	case w.State == GaveUp && bg:
		return "Следих между " + from + " и " + to + " ч., но рестартът още не е безопасен: " +
			strings.Join(why, "; ") + "."
	case w.State == GaveUp:
		return "I watched between " + from + " and " + to + ", and a restart is still not safe: " +
			strings.Join(why, "; ") + "."
	case w.State == CannotTell && bg:
		return "Не мога да проверя дали рестартът е безопасен на тази система."
	case w.State == CannotTell:
		return "I cannot tell whether a restart is safe on this system."
	}

	return ""
}
