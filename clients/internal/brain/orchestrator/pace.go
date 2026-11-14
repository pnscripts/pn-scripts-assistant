package orchestrator

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Pacing an allowance: is a subscription being used faster than its window
 * runs? Read from what the agent itself last said about its allowance — how
 * much is left and when it renews — never from a guess. A week's allowance
 * 60 % used with 30 % of the week gone will run out early; the person
 * asked to start more work should see that before saying yes.
 */

// Pace is how an allowance is being spent.
type Pace struct {
	Known   bool
	Used    float64 // percent of the allowance used
	Elapsed float64 // percent of its window gone
	Window  time.Duration
	Ahead   bool // used faster than the window runs, by more than PaceMargin
}

// PaceMargin is how far ahead of the window spending may be before it is
// called ahead: a little early use is how any week starts.
const PaceMargin = 10

// PaceOf reads u at now.
func PaceOf(u *store.Usage, now time.Time) Pace {
	if u == nil || u.Remaining == nil || u.ResetsAt.IsZero() || !u.ResetsAt.After(now) {
		return Pace{}
	}

	length := windowLength(u.Window)
	if length <= 0 {
		return Pace{}
	}

	left := u.ResetsAt.Sub(now)
	if left > length {
		left = length
	}

	p := Pace{Known: true, Used: 100 - *u.Remaining, Window: length}
	p.Elapsed = 100 * (1 - float64(left)/float64(length))
	p.Ahead = p.Used > p.Elapsed+PaceMargin

	return p
}

// windowLength reads the windows the agents name: Claude Code's
// "five_hour" and "seven_day", Codex's "300 minutes".
func windowLength(window string) time.Duration {
	w := strings.ToLower(strings.TrimSpace(window))

	switch {
	case strings.HasPrefix(w, "five_hour"):
		return 5 * time.Hour
	case strings.HasPrefix(w, "seven_day"), w == "weekly", w == "week":
		return 7 * 24 * time.Hour
	case strings.HasSuffix(w, " minutes"):
		if n, err := strconv.Atoi(strings.TrimSuffix(w, " minutes")); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}

	return 0
}

// Say is the pace in a sentence, or empty when it is not known.
func (p Pace) Say() string { return p.SayIn("en") }

// SayIn is the pace in a sentence in a language ("en" or "bg").
func (p Pace) SayIn(lang string) string {
	if !p.Known {
		return ""
	}

	if lang == "bg" {
		window := fmt.Sprintf("%.0f ч.", p.Window.Hours())
		if days := p.Window.Hours() / 24; p.Window >= 24*time.Hour && fmt.Sprintf("%.0f", days) == "1" {
			window = "1 ден"
		} else if p.Window >= 24*time.Hour {
			window = fmt.Sprintf("%.0f дни", days)
		}

		s := fmt.Sprintf("изразходвани %.0f %% от лимита при изминали %.0f %% от периода му (%s)", p.Used, p.Elapsed, window)

		if p.Ahead {
			s += "; по-бързо от темпото: с тази скорост ще свърши, преди да се поднови"
		}

		return s
	}

	window := fmt.Sprintf("%.0f-hour", p.Window.Hours())
	if p.Window >= 24*time.Hour {
		window = fmt.Sprintf("%.0f-day", p.Window.Hours()/24)
	}

	s := fmt.Sprintf("%.0f %% of its allowance used with %.0f %% of its %s window gone", p.Used, p.Elapsed, window)

	if p.Ahead {
		s += "; ahead of pace: at this rate it runs out before it renews"
	}

	return s
}
