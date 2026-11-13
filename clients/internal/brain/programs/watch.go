package programs

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

/*
 * Watching for installs, updates and removals.
 *
 * Events, not polling: on Linux every target folder is an inotify watch, so
 * an idle computer costs nothing (the reader is blocked in the kernel).
 * Events are gathered per source and debounced: a source is looked at once
 * its folder has been quiet for Debounce, and at the latest Longest after
 * the first event, so an upgrade of four hundred packages is a handful of
 * looks, not thousands, and a stream that never stops is still looked at.
 *
 * What cannot be watched is polled every PollEvery by its modification
 * times: a folder that is not there yet (it may appear when the first
 * Flatpak is installed), one that went away, one the kernel refused (the
 * per-user watch limit), and a source with nothing to watch at all. A
 * polled folder that appears is watched from then on.
 */

// Options are the timings of a watch.
type Options struct {
	Debounce  time.Duration
	Longest   time.Duration
	PollEvery time.Duration
}

// DefaultOptions are the timings the assistant uses.
func DefaultOptions() Options {
	return Options{Debounce: 2 * time.Second, Longest: time.Minute, PollEvery: 10 * time.Minute}
}

// Why a target is polled.
const (
	WhyMissing  = "not there"
	WhyLimit    = "the system refused a file watch"
	WhyNoEvents = "this system gives no file events"
	WhyNoPath   = "nothing to watch"
)

// reasons is each Why as a short code the page says in its own language.
var reasons = map[string]string{
	WhyMissing:  "missing",
	WhyLimit:    "limit",
	WhyNoEvents: "no-events",
	WhyNoPath:   "no-path",
}

// Reasons is every code a Polled's Reason can be, sorted.
func Reasons() []string {
	out := make([]string, 0, len(reasons))
	for _, code := range reasons {
		out = append(out, code)
	}

	sort.Strings(out)

	return out
}

// Polled is a target looked at by the poll, and why: Why in English, Reason
// as one of Reasons for the page.
type Polled struct {
	Dir    string `json:"dir"`
	Only   string `json:"only,omitempty"`
	Why    string `json:"why"`
	Reason string `json:"reason"`
}

// SourceState is how one source is kept current.
type SourceState struct {
	Kind    Kind     `json:"kind"`
	Watched int      `json:"watched"`
	Polled  []Polled `json:"polled,omitempty"`
}

type target struct {
	Target
	kind  Kind
	wd    int    // the watch, or -1 when polled
	why   string // why polled
	print string // the poll's fingerprint
}

// Watching is a watch going on. Stop it, or cancel its context.
type Watching struct {
	opts Options
	fire func(Kind)

	mu      sync.Mutex
	targets []*target
	byWatch map[int]*target
	noPath  []Kind
	dirty   map[Kind]bool
	events  events

	signal chan struct{}
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

// events is the platform's events: inotify on Linux, ReadDirectoryChangesW
// and RegNotifyChangeKeyValue on Windows, none elsewhere.
type events interface {
	add(t Target) (int, error)
	remove(wd int)
	close()
}

/*
 * Watch starts watching each source through its targets and calls fire
 * (one call at a time, from one goroutine) when a source should be looked
 * at again. It never calls fire after Stop returns.
 */
func Watch(ctx context.Context, targets map[Kind][]Target, opts Options, fire func(Kind)) *Watching {
	ctx, cancel := context.WithCancel(ctx)

	w := &Watching{
		opts:    opts,
		fire:    fire,
		byWatch: map[int]*target{},
		dirty:   map[Kind]bool{},
		signal:  make(chan struct{}, 1),
		cancel:  cancel,
	}

	// Without file events everything is polled.
	if ev, err := newEvents(w); err == nil {
		w.events = ev
	}

	kinds := make([]Kind, 0, len(targets))
	for k := range targets {
		kinds = append(kinds, k)
	}

	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })

	w.mu.Lock()

	for _, k := range kinds {
		if len(targets[k]) == 0 {
			w.noPath = append(w.noPath, k)

			continue
		}

		for _, tg := range targets[k] {
			t := &target{Target: tg, kind: k, wd: -1, print: fingerprint(tg)}
			w.tryWatch(t)
			w.targets = append(w.targets, t)
		}
	}

	ev := w.events
	w.mu.Unlock()

	w.wg.Add(2)

	go w.loop(ctx)
	go w.poll(ctx)

	if ev != nil {
		w.wg.Add(1)

		go func() {
			defer w.wg.Done()

			readEvents(ctx, w, ev)
		}()
	}

	return w
}

// tryWatch adds a watch for a target; on failure it is polled. Under mu.
func (w *Watching) tryWatch(t *target) bool {
	if !there(t.Target) {
		t.why = WhyMissing

		return false
	}

	if w.events == nil {
		t.why = WhyNoEvents

		return false
	}

	wd, err := w.events.add(t.Target)
	if err != nil {
		t.why = WhyLimit

		return false
	}

	t.wd, t.why = wd, ""
	w.byWatch[wd] = t

	return true
}

// Stop ends the watch and waits until nothing of it runs.
func (w *Watching) Stop() {
	w.once.Do(func() {
		w.cancel()

		w.mu.Lock()
		ev := w.events
		w.mu.Unlock()

		if ev != nil {
			ev.close()
		}
	})

	w.wg.Wait()
}

// State is how each source is kept current now.
func (w *Watching) State() []SourceState {
	w.mu.Lock()
	defer w.mu.Unlock()

	by := map[Kind]*SourceState{}

	var order []Kind

	get := func(k Kind) *SourceState {
		if s, ok := by[k]; ok {
			return s
		}

		by[k] = &SourceState{Kind: k}
		order = append(order, k)

		return by[k]
	}

	for _, t := range w.targets {
		s := get(t.kind)

		if t.wd >= 0 {
			s.Watched++
		} else {
			s.Polled = append(s.Polled, Polled{Dir: t.Dir, Only: t.Only, Why: t.why, Reason: reasons[t.why]})
		}
	}

	for _, k := range w.noPath {
		get(k).Polled = append(get(k).Polled, Polled{Why: WhyNoPath, Reason: reasons[WhyNoPath]})
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	out := make([]SourceState, 0, len(order))
	for _, k := range order {
		out = append(out, *by[k])
	}

	return out
}

// changed marks a source to be looked at again.
func (w *Watching) changed(k Kind) {
	w.mu.Lock()
	w.dirty[k] = true
	w.mu.Unlock()

	select {
	case w.signal <- struct{}{}:
	default:
	}
}

// everything marks every source: the kernel dropped events.
func (w *Watching) everything() {
	w.mu.Lock()
	for _, t := range w.targets {
		w.dirty[t.kind] = true
	}
	w.mu.Unlock()

	select {
	case w.signal <- struct{}{}:
	default:
	}
}

// event is one file event: a watch, the name in its folder, and whether the
// folder itself went away.
func (w *Watching) event(wd int, name string, gone, overflow bool) {
	if overflow {
		w.everything()

		return
	}

	w.mu.Lock()

	t, ok := w.byWatch[wd]
	if !ok {
		w.mu.Unlock()

		return
	}

	if gone {
		delete(w.byWatch, wd)

		if w.events != nil {
			w.events.remove(wd)
		}

		t.wd, t.why, t.print = -1, WhyMissing, fingerprint(t.Target)
	}

	matters := gone || t.Only == "" || name == t.Only
	kind := t.kind
	w.mu.Unlock()

	if matters {
		w.changed(kind)
	}
}

func (w *Watching) loop(ctx context.Context) {
	defer w.wg.Done()

	type pending struct{ first, last time.Time }

	waiting := map[Kind]pending{}

	timer := time.NewTimer(time.Hour)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.signal:
			now := time.Now()

			w.mu.Lock()
			for k := range w.dirty {
				p, ok := waiting[k]
				if !ok {
					p.first = now
				}

				p.last = now
				waiting[k] = p
			}

			w.dirty = map[Kind]bool{}
			w.mu.Unlock()
		case <-timer.C:
		}

		now := time.Now()
		next := time.Hour

		var due []Kind

		for k, p := range waiting {
			at := p.last.Add(w.opts.Debounce)
			if latest := p.first.Add(w.opts.Longest); latest.Before(at) {
				at = latest
			}

			if !now.Before(at) {
				due = append(due, k)

				continue
			}

			if d := at.Sub(now); d < next {
				next = d
			}
		}

		sort.Slice(due, func(i, j int) bool { return due[i] < due[j] })

		for _, k := range due {
			delete(waiting, k)

			if ctx.Err() != nil {
				return
			}

			w.fire(k)
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		timer.Reset(next)
	}
}

func (w *Watching) poll(ctx context.Context) {
	defer w.wg.Done()

	every := w.opts.PollEvery
	if every <= 0 {
		every = DefaultOptions().PollEvery
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		var changed []Kind

		w.mu.Lock()

		for _, t := range w.targets {
			if t.wd >= 0 {
				continue
			}

			now := fingerprint(t.Target)
			if now != t.print {
				t.print = now
				changed = append(changed, t.kind)
			}

			// A folder that appeared is watched from now on.
			if t.why == WhyMissing {
				w.tryWatch(t)
			}
		}

		changed = append(changed, w.noPath...)
		w.mu.Unlock()

		for _, k := range changed {
			w.changed(k)
		}
	}
}

// fingerprint is what the poll compares: whether the folder (and its one
// file) is there, and when each last changed.
func fingerprint(t Target) string {
	if t.Registry {
		print, _ := keyStamp(t.Dir)

		return print
	}

	print := stamp(t.Dir)
	if t.Only != "" {
		print += "|" + stamp(filepath.Join(t.Dir, t.Only))
	}

	return print
}

func stamp(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return "-"
	}

	return st.ModTime().UTC().Format(time.RFC3339Nano) + "/" + strconv.FormatInt(st.Size(), 10)
}

// there is whether a target can be watched now: its folder, or its key.
func there(t Target) bool {
	if t.Registry {
		_, ok := keyStamp(t.Dir)

		return ok
	}

	st, err := os.Stat(t.Dir)

	return err == nil && st.IsDir()
}
