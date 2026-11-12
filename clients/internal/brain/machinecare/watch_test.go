package machinecare

import (
	"strings"
	"testing"
	"time"
)

func blocked(what string) Readiness {
	return Judge(Inputs{Installing: []string{what}})
}

/*
 * "Tell me when I can restart": watched until the machine is ready, said
 * once, and never watched for ever.
 */
func TestARestartWatchEndsWhenTheMachineIsReady(t *testing.T) {
	t0 := time.Date(2026, 11, 12, 22, 0, 0, 0, time.Local)

	w, err := NewWatch(t0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if w.State != Watching || !w.Until.Equal(t0.Add(DefaultWatch)) {
		t.Fatalf("a new watch: %+v", w)
	}

	// Still installing: it keeps watching and says what is in the way.
	if w.Look(blocked("apt"), t0.Add(2*time.Minute)) {
		t.Fatalf("finished while apt was installing: %+v", w)
	}

	if w.State != Watching || len(w.InTheWay) != 1 || w.InTheWay[0].Key != "installing" {
		t.Fatalf("does not say what is in the way: %+v", w)
	}

	// Ready: it ends, once, and says since when.
	ready := t0.Add(9 * time.Minute)
	if !w.Look(Judge(Inputs{}), ready) {
		t.Fatal("the ready machine did not end the watch")
	}

	if w.State != Safe || !w.Ended.Equal(ready) || len(w.InTheWay) != 0 {
		t.Fatalf("%+v", w)
	}

	if w.Look(blocked("dpkg"), ready.Add(time.Minute)) || w.State != Safe {
		t.Fatalf("an ended watch changed: %+v", w)
	}
}

// Not ready by its deadline, it gives up and says what was still in the
// way; a machine it cannot read is not watched at all.
func TestARestartWatchGivesUpAtItsDeadline(t *testing.T) {
	t0 := time.Date(2026, 11, 12, 22, 0, 0, 0, time.Local)

	w, _ := NewWatch(t0, time.Hour)

	if w.Look(blocked("apt"), t0.Add(59*time.Minute)) {
		t.Fatal("gave up early")
	}

	if !w.Look(blocked("apt"), t0.Add(time.Hour)) || w.State != GaveUp || len(w.InTheWay) != 1 {
		t.Fatalf("did not give up at its deadline: %+v", w)
	}

	w2, _ := NewWatch(t0, time.Hour)
	if !w2.Look(Readiness{}, t0) || w2.State != CannotTell {
		t.Fatalf("an unreadable machine is watched: %+v", w2)
	}
}

func TestARestartWatchHasALimit(t *testing.T) {
	t0 := time.Now()

	for _, d := range []time.Duration{-time.Hour, MostWatch + time.Minute} {
		if _, err := NewWatch(t0, d); err == nil {
			t.Errorf("a watch of %s was accepted", d)
		}
	}

	if w, err := NewWatch(t0, MostWatch); err != nil || !w.Until.Equal(t0.Add(MostWatch)) {
		t.Errorf("the longest watch was refused: %v", err)
	}
}

// What it says when a watch ends, in English and Bulgarian.
func TestAnEndedWatchIsSaidInItsUsersLanguage(t *testing.T) {
	t0 := time.Date(2026, 11, 12, 22, 0, 0, 0, time.Local)

	w, _ := NewWatch(t0, time.Hour)
	w.Look(Judge(Inputs{}), t0.Add(5*time.Minute))

	if s := w.Said("en"); s != "It is safe to restart now: nothing would be cut off, and nothing would come back missing." {
		t.Errorf("en: %q", s)
	}

	if s := w.Said("bg"); !strings.HasPrefix(s, "Вече може да рестартирате") {
		t.Errorf("bg: %q", s)
	}

	w2, _ := NewWatch(t0, time.Hour)
	w2.Look(blocked("apt"), t0.Add(time.Hour))

	if s := w2.Said("en"); !strings.Contains(s, "22:00 and 23:00") || !strings.Contains(s, "apt is installing") {
		t.Errorf("en gave up: %q", s)
	}

	if s := w2.Said("bg"); !strings.Contains(s, "apt") || !strings.Contains(s, "23:00") {
		t.Errorf("bg gave up: %q", s)
	}

	if s := (Watch{State: Watching}).Said("en"); s != "" {
		t.Errorf("a watch still going said: %q", s)
	}
}

// Every blocking item Judge can give has its short form in both languages.
func TestEveryBlockingItemCanBeListed(t *testing.T) {
	r := Judge(Inputs{Running: "6.8.0-1", Installed: []string{"6.8.0-1", "6.8.0-2"},
		DKMS: "rtl8821au/5.12, 6.8.0-1, x86_64: installed", Installing: []string{"apt"}, Busy: []string{"Tidy"}})

	for _, i := range r.Items {
		if !i.Blocking {
			continue
		}

		say, ok := inTheWay[i.Key]
		if !ok {
			t.Errorf("%s has no short form", i.Key)

			continue
		}

		for _, bg := range []bool{false, true} {
			if s := say(i.Args, bg); strings.Contains(s, "{") || strings.HasSuffix(s, " ") || s == "" {
				t.Errorf("%s (bg %v): %q", i.Key, bg, s)
			}
		}
	}
}
