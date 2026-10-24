package autonomy

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/workspace"
)

const testGB = uint64(1 << 30)

func TestTheLargestModelThatFitsIsChosen(t *testing.T) {
	options := []Option{{Name: "m:3b", GB: 3}, {Name: "m:8b", GB: 6}, {Name: "m:32b", GB: 20}}

	got, why, err := Choose(options, Room{Memory: 12 * testGB, Disk: 100 * testGB})
	if err != nil || got.Name != "m:8b" || !strings.Contains(why, "fits") {
		t.Errorf("memory: %v %q %v", got, why, err)
	}

	// The disk keeps its margin: 6 GB + 4 GB does not fit in 9 GB.
	if got, _, _ := Choose(options, Room{Memory: 64 * testGB, Disk: 9 * testGB}); got.Name != "m:3b" {
		t.Errorf("disk: %v", got)
	}

	if _, _, err := Choose(options, Room{Memory: 1 * testGB, Disk: 100 * testGB}); err == nil || !strings.Contains(err.Error(), "no model fits") {
		t.Errorf("nothing fits: %v", err)
	}
}

type fakeOllama struct {
	installed map[string]string
	// say is what each pull reports; fail fails it.
	say  map[string][]string
	fail map[string]bool
	// digest is what a pull installs.
	digest map[string]string
	pulled []string
}

func (f *fakeOllama) Pull(_ context.Context, name string, progress func(string)) error {
	f.pulled = append(f.pulled, name)

	if f.fail[name] {
		return errors.New("connection reset")
	}

	for _, line := range f.say[name] {
		progress(name + " — " + line)
	}

	f.installed[name] = f.digest[name]

	return nil
}

func (f *fakeOllama) DigestOf(_ context.Context, name string) string { return f.installed[name] }

func TestAModelIsDownloadedCheckedRecordedOrTheNextThatFitsIsTried(t *testing.T) {
	root := t.TempDir()
	ledger := LedgerAt(root)

	ok := []string{"pulling manifest", "verifying sha256 digest", "success"}
	f := &fakeOllama{
		installed: map[string]string{},
		say:       map[string][]string{"big": ok, "mid": {"pulling manifest", "success"}, "small": ok},
		fail:      map[string]bool{},
		digest:    map[string]string{"big": "sha256:aaaaaaaaaaaaaaaa", "mid": "sha256:bbbbbbbbbbbbbbbb", "small": "sha256:cccccccccccccccc"},
	}

	options := []Option{
		{Name: "big", GB: 8, Digest: "sha256:ffffffffffffffff"}, // the wrong digest arrives
		{Name: "mid", GB: 5}, // never says it verified
		{Name: "small", GB: 2},
	}

	e, err := PullModel(context.Background(), f, options, Room{Memory: 16 * testGB, Disk: 100 * testGB}, ledger, "game_developer", nil)
	if err != nil {
		t.Fatal(err)
	}

	if e.Name != "small" || strings.Join(f.pulled, ",") != "big,mid,small" {
		t.Errorf("chose %s after %v", e.Name, f.pulled)
	}

	if !ledger.Mine("model", "small") || ledger.Mine("model", "big") {
		t.Errorf("ledger: %+v", ledger.Entries())
	}

	// Already here: nothing is downloaded or recorded as ours.
	f.pulled = nil
	if _, err := PullModel(context.Background(), f, []Option{{Name: "small", GB: 2}}, Room{Memory: 16 * testGB, Disk: 100 * testGB}, ledger, "x", nil); err != nil || len(f.pulled) != 0 {
		t.Errorf("pulled what was here: %v %v", f.pulled, err)
	}

	// Offline: every pull fails and it says so, with what it tried.
	off := &fakeOllama{installed: map[string]string{}, fail: map[string]bool{"a": true}}
	if _, err := PullModel(context.Background(), off, []Option{{Name: "a", GB: 1}}, Room{Memory: 16 * testGB, Disk: 100 * testGB}, nil, "x", nil); err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("offline: %v", err)
	}
}

func TestOnlyWhatGoalModePutHereIsEverOfferedForRemoval(t *testing.T) {
	ledger := LedgerAt(t.TempDir())
	now := time.Now()

	_ = ledger.Add(Entry{Kind: "model", Name: "old", When: now.Add(-60 * 24 * time.Hour)})
	_ = ledger.Add(Entry{Kind: "model", Name: "busy", When: now.Add(-60 * 24 * time.Hour)})
	_ = ledger.Add(Entry{Kind: "tool", Name: "Godot", When: now.Add(-time.Hour), Source: "github.com", Bytes: 80 << 20})

	unused := ledger.Unused(30*24*time.Hour, func(e Entry) bool { return e.Name == "busy" }, now)
	if len(unused) != 1 || unused[0].Name != "old" {
		t.Errorf("unused: %+v", unused)
	}

	if ledger.Mine("model", "llama3.2:3b") {
		t.Error("a model it did not download is called its own")
	}

	lines := Lines(ledger.Since(now.Add(-2 * time.Hour)))
	if len(lines) != 1 || !strings.Contains(lines[0], "Godot") || !strings.Contains(lines[0], "from github.com") {
		t.Errorf("since: %v", lines)
	}

	_ = ledger.Forget("model", "old")
	if ledger.Mine("model", "old") {
		t.Error("not forgotten")
	}
}

func TestAToolIsInstalledAloneOnlyWhenItIsUserLevelAndThereIsRoom(t *testing.T) {
	installed := false
	r := provision.Recipe{
		ID: "thing", Title: "Thing", Kind: provision.Tool, Size: "50MB", Source: "example.org, checked against its SHA-256",
		Find: func() (string, string, bool) { return "/bin/true", "1.0", installed },
		Install: func(context.Context, io.Writer) error {
			installed = true

			return nil
		},
	}
	root := r
	root.ID, root.NeedsRoot = "rooted", true

	book := provision.NewBook(r, root)
	ledger := LedgerAt(t.TempDir())

	if _, err := ProvideTool(context.Background(), book, "rooted", "", 100*testGB, ledger, "x", nil); !errors.Is(err, ErrNeedsOwner) {
		t.Errorf("administrator install went ahead: %v", err)
	}

	if _, err := ProvideTool(context.Background(), book, "thing", "", 1*testGB, ledger, "x", nil); err == nil || installed {
		t.Errorf("installed with no room: %v", err)
	}

	e, err := ProvideTool(context.Background(), book, "thing", "", 100*testGB, ledger, "game_developer", nil)
	if err != nil || !installed || e.Version != "1.0" || !ledger.Mine("tool", "Thing") {
		t.Errorf("install: %+v %v", e, err)
	}
}

func TestEachGoalWorksInAFolderOfItsOwn(t *testing.T) {
	root := t.TempDir()

	a, err := GoalFolder(root, 7, "Super space game!")
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Base(a) != "7-super-space-game" || filepath.Dir(filepath.Dir(a)) != root {
		t.Errorf("folder: %s", a)
	}

	cfg, err := workspace.Load(a)
	if err != nil {
		t.Fatal(err)
	}

	scope := workspace.ScopeOf(a, cfg)
	if !scope.Allows(filepath.Join(a, "build", "game.x86_64")) || scope.Allows(filepath.Join(root, "brain.sqlite")) {
		t.Error("the folder does not keep the work inside it")
	}

	b, _ := GoalFolder(root, 8, "Бизнес стратегия")
	if a == b || !strings.HasPrefix(filepath.Base(b), "8-") {
		t.Errorf("two goals share %s", b)
	}

	if _, err := os.Stat(workspace.Path(b)); err != nil {
		t.Error(err)
	}
}

// On a machine without graphics acceleration the largest model that fits in
// memory is not chosen: one that answers in reasonable time is.
func TestAProcessorOnlyMachineGetsAModelThatAnswersInTime(t *testing.T) {
	options := []Option{{Name: "small", GB: 2}, {Name: "medium", GB: 5}, {Name: "large", GB: 20}}

	got, why, err := Choose(options, Room{Disk: 100 * gb, Memory: 27 * gb, Processor: true})
	if err != nil || got.Name != "medium" || !strings.Contains(why, "processor") {
		t.Errorf("chose %s (%s, %v)", got.Name, why, err)
	}

	if got, _, _ := Choose(options, Room{Disk: 100 * gb, Memory: 27 * gb}); got.Name != "large" {
		t.Errorf("with a graphics card: %s", got.Name)
	}
}
