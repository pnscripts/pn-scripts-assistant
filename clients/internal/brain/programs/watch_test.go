package programs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fires counts what a watch reported, by source.
type fires struct {
	mu    sync.Mutex
	count map[Kind]int
	ch    chan Kind
}

func newFires() *fires { return &fires{count: map[Kind]int{}, ch: make(chan Kind, 100)} }

func (f *fires) fire(k Kind) {
	f.mu.Lock()
	f.count[k]++
	f.mu.Unlock()
	f.ch <- k
}

func (f *fires) of(k Kind) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.count[k]
}

func (f *fires) wait(t *testing.T, k Kind, within time.Duration) {
	t.Helper()

	deadline := time.After(within)

	for {
		select {
		case got := <-f.ch:
			if got == k {
				return
			}
		case <-deadline:
			t.Fatalf("no change reported for %s within %v", k, within)
		}
	}
}

func (f *fires) none(t *testing.T, within time.Duration) {
	t.Helper()

	select {
	case got := <-f.ch:
		t.Fatalf("unexpected change reported for %s", got)
	case <-time.After(within):
	}
}

func quick() Options {
	return Options{Debounce: 80 * time.Millisecond, Longest: 2 * time.Second, PollEvery: time.Hour}
}

func needsEvents(t *testing.T) {
	t.Helper()

	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("file events are read with inotify on Linux and ReadDirectoryChangesW on Windows; other systems poll (TestAWatchThatCannotBeSetIsPolled)")
	}
}

func TestEveryKindOfChangeInAWatchedFolderIsNoticed(t *testing.T) {
	needsEvents(t)

	dir := t.TempDir()
	put(t, dir, "existing.desktop", "x")
	put(t, dir, "moved.desktop", "x")

	f := newFires()
	w := Watch(context.Background(), map[Kind][]Target{App: {{Dir: dir}}}, quick(), f.fire)
	defer w.Stop()

	steps := map[string]func(){
		"create": func() { put(t, dir, "new.desktop", "x") },
		"write":  func() { put(t, dir, "existing.desktop", "y") },
		"delete": func() { os.Remove(filepath.Join(dir, "new.desktop")) },
		"rename over": func() {
			put(t, dir, ".tmp", "z")
			os.Rename(filepath.Join(dir, ".tmp"), filepath.Join(dir, "existing.desktop"))
		},
		"move away": func() { os.Rename(filepath.Join(dir, "moved.desktop"), filepath.Join(t.TempDir(), "m")) },
	}

	for _, name := range []string{"create", "write", "delete", "rename over", "move away"} {
		steps[name]()
		f.wait(t, App, 2*time.Second)
	}

	if s := w.State(); len(s) != 1 || s[0].Kind != App || s[0].Watched != 1 || len(s[0].Polled) != 0 {
		t.Errorf("state %+v", s)
	}
}

func TestABurstOfChangesIsOneLook(t *testing.T) {
	needsEvents(t)

	dir := t.TempDir()

	f := newFires()
	opts := quick()
	opts.Debounce = 300 * time.Millisecond

	w := Watch(context.Background(), map[Kind][]Target{Deb: {{Dir: dir}}}, opts, f.fire)
	defer w.Stop()

	// A thousand events, as an upgrade of many packages makes.
	for i := 0; i < 1000; i++ {
		put(t, dir, "f", string(rune('a'+i%26)))
	}

	f.wait(t, Deb, 3*time.Second)
	f.none(t, 600*time.Millisecond)

	if n := f.of(Deb); n != 1 {
		t.Errorf("%d looks for one burst", n)
	}
}

func TestAChangeThatNeverStopsIsStillLookedAt(t *testing.T) {
	needsEvents(t)

	dir := t.TempDir()

	f := newFires()
	opts := Options{Debounce: 200 * time.Millisecond, Longest: 500 * time.Millisecond, PollEvery: time.Hour}

	w := Watch(context.Background(), map[Kind][]Target{Deb: {{Dir: dir}}}, opts, f.fire)
	defer w.Stop()

	stop := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(stop) {
		put(t, dir, "f", time.Now().String())
		time.Sleep(50 * time.Millisecond)
	}

	if n := f.of(Deb); n < 2 {
		t.Errorf("a stream of changes longer than Longest is looked at during it: %d looks", n)
	}
}

func TestOnlyTheNamedFileCounts(t *testing.T) {
	needsEvents(t)

	dir := t.TempDir()
	put(t, dir, "status", "a")

	f := newFires()
	w := Watch(context.Background(), map[Kind][]Target{Deb: {{Dir: dir, Only: "status"}}}, quick(), f.fire)
	defer w.Stop()

	put(t, dir, "lock", "x")
	put(t, dir, "status-old", "x")
	f.none(t, 400*time.Millisecond)

	// dpkg writes status-new and renames it over status.
	put(t, dir, "status-new", "b")
	os.Rename(filepath.Join(dir, "status-new"), filepath.Join(dir, "status"))
	f.wait(t, Deb, 2*time.Second)
}

func TestSourcesAreToldApart(t *testing.T) {
	needsEvents(t)

	a, b := t.TempDir(), t.TempDir()

	f := newFires()
	w := Watch(context.Background(), map[Kind][]Target{Snap: {{Dir: a}}, Flatpak: {{Dir: b}}}, quick(), f.fire)
	defer w.Stop()

	put(t, b, "x", "1")
	f.wait(t, Flatpak, 2*time.Second)
	f.none(t, 300*time.Millisecond)

	if f.of(Snap) != 0 {
		t.Error("a change in one source made another look")
	}
}

func TestAWatchThatCannotBeSetIsPolled(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "not-yet")

	events := runtime.GOOS == "linux" || runtime.GOOS == "windows"

	if events {
		was := addWatch
		addWatch = func(fd int, path string, mask uint32) (int, error) {
			if path == dir {
				return -1, syscall.ENOSPC
			}

			return was(fd, path, mask)
		}

		t.Cleanup(func() { addWatch = was })
	}

	f := newFires()
	opts := Options{Debounce: 50 * time.Millisecond, Longest: time.Second, PollEvery: 150 * time.Millisecond}

	w := Watch(context.Background(), map[Kind][]Target{App: {{Dir: dir}, {Dir: missing}}}, opts, f.fire)
	defer w.Stop()

	s := w.State()
	if len(s) != 1 || len(s[0].Polled) != 2 || s[0].Watched != 0 {
		t.Fatalf("both targets polled: %+v", s)
	}

	reasons, codes := map[string]string{}, map[string]string{}
	for _, p := range s[0].Polled {
		reasons[p.Dir], codes[p.Dir] = p.Why, p.Reason
	}

	// The page says the reason by its code, in the page's language.
	if codes[missing] != "missing" || (events && codes[dir] != "limit") {
		t.Errorf("the reasons' codes %v", codes)
	}

	if events && reasons[dir] != WhyLimit {
		t.Errorf("why %s is polled: %q", dir, reasons[dir])
	}

	if reasons[missing] != WhyMissing {
		t.Errorf("why %s is polled: %q", missing, reasons[missing])
	}

	f.none(t, 400*time.Millisecond)

	// A change the poll sees: a file added (the folder's time moves).
	time.Sleep(20 * time.Millisecond)
	put(t, dir, "new.desktop", "x")
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(dir, future, future)
	f.wait(t, App, 2*time.Second)

	// And the folder that was not there appears.
	if err := os.MkdirAll(missing, 0o755); err != nil {
		t.Fatal(err)
	}

	f.wait(t, App, 2*time.Second)
}

func TestAWatchedFolderThatGoesAwayIsPolledAfterwards(t *testing.T) {
	needsEvents(t)

	parent := t.TempDir()
	dir := filepath.Join(parent, "applications")
	os.MkdirAll(dir, 0o755)

	f := newFires()
	opts := Options{Debounce: 50 * time.Millisecond, Longest: time.Second, PollEvery: 150 * time.Millisecond}

	w := Watch(context.Background(), map[Kind][]Target{App: {{Dir: dir}}}, opts, f.fire)
	defer w.Stop()

	os.RemoveAll(dir)
	f.wait(t, App, 2*time.Second)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := w.State(); len(s[0].Polled) == 1 && s[0].Polled[0].Why == WhyMissing {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if s := w.State(); len(s[0].Polled) != 1 {
		t.Fatalf("a removed folder is polled: %+v", s)
	}

	// Back again: noticed by the poll.
	os.MkdirAll(dir, 0o755)
	f.wait(t, App, 2*time.Second)
}

func TestStoppingEndsEverything(t *testing.T) {
	dir := t.TempDir()

	f := newFires()
	ctx, cancel := context.WithCancel(context.Background())

	w := Watch(ctx, map[Kind][]Target{App: {{Dir: dir}}}, quick(), f.fire)
	cancel()
	w.Stop()
	w.Stop() // twice is harmless

	put(t, dir, "after.desktop", "x")
	f.none(t, 300*time.Millisecond)
}

func TestASourceWithNothingToWatchIsLookedAtByThePoll(t *testing.T) {
	f := newFires()
	opts := Options{Debounce: 20 * time.Millisecond, Longest: time.Second, PollEvery: 100 * time.Millisecond}

	// dpkg without a status file to watch (no apt-config): looked at every
	// PollEvery, because nothing else would ever notice.
	w := Watch(context.Background(), map[Kind][]Target{Deb: nil}, opts, f.fire)
	defer w.Stop()

	f.wait(t, Deb, time.Second)

	if s := w.State(); len(s) != 1 || len(s[0].Polled) != 1 || s[0].Polled[0].Why != WhyNoPath ||
		s[0].Polled[0].Reason != "no-path" {
		t.Errorf("state %+v", s)
	}
}

// Every reason a folder can be polled for has its code, and no two share one.
func TestEveryReasonForPollingHasACode(t *testing.T) {
	got := Reasons()
	if strings.Join(got, ",") != "limit,missing,no-events,no-path" {
		t.Errorf("reasons %v", got)
	}

	for _, why := range []string{WhyMissing, WhyLimit, WhyNoEvents, WhyNoPath} {
		if reasons[why] == "" {
			t.Errorf("%q has no code", why)
		}
	}
}
