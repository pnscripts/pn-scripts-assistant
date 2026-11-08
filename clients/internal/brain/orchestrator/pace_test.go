package orchestrator

import (
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

func TestAnAllowanceSpentTooFastIsSaid(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	left := func(f float64) *float64 { return &f }

	// A week, 30 % gone (renews in 4.9 days), 60 % used: ahead.
	p := PaceOf(&store.Usage{Remaining: left(40), Window: "seven_day", ResetsAt: now.Add(117*time.Hour + 36*time.Minute)}, now)
	if !p.Known || !p.Ahead || int(p.Elapsed+0.5) != 30 || !strings.Contains(p.Say(), "ahead of pace") || !strings.Contains(p.Say(), "7-day") {
		t.Errorf("%+v %q", p, p.Say())
	}

	// Five hours, half gone, 40 % used: on pace.
	p = PaceOf(&store.Usage{Remaining: left(60), Window: "five_hour", ResetsAt: now.Add(150 * time.Minute)}, now)
	if !p.Known || p.Ahead || strings.Contains(p.Say(), "ahead") {
		t.Errorf("%+v", p)
	}

	if p := PaceOf(&store.Usage{Remaining: left(60), Window: "300 minutes", ResetsAt: now.Add(time.Hour)}, now); !p.Known || p.Window != 5*time.Hour {
		t.Errorf("%+v", p)
	}

	for _, u := range []*store.Usage{nil, {Window: "seven_day", ResetsAt: now.Add(time.Hour)},
		{Remaining: left(10), Window: "fortnight", ResetsAt: now.Add(time.Hour)},
		{Remaining: left(10), Window: "seven_day", ResetsAt: now.Add(-time.Hour)}} {
		if PaceOf(u, now).Known {
			t.Errorf("%+v was read as known", u)
		}
	}
}
