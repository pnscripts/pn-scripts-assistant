package brain

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/tools"
)

// machineInAFolder is a computer for the inventory to read: a snap, a menu
// entry and a dpkg that answers from a variable.
type machineInAFolder struct {
	root string

	mu   sync.Mutex
	dpkg string
}

func (m *machineInAFolder) write(t *testing.T, rel, text string) {
	t.Helper()

	path := filepath.Join(m.root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(path), 0o755)

	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (m *machineInAFolder) installSnap(t *testing.T, name, version, rev string) {
	t.Helper()

	m.write(t, "snap/"+name+"/"+rev+"/meta/snap.yaml", "name: "+name+"\nversion: "+version+"\napps:\n  "+name+":\n    command: x\n")
	os.Remove(filepath.Join(m.root, "snap", name, "current"))

	if err := os.Symlink(rev, filepath.Join(m.root, "snap", name, "current")); err != nil {
		t.Fatal(err)
	}

	m.write(t, "var/lib/snapd/snaps/"+name+"_"+rev+".snap", "squashfs")
}

func (m *machineInAFolder) removeSnap(t *testing.T, name, rev string) {
	t.Helper()

	os.RemoveAll(filepath.Join(m.root, "snap", name))
	os.Remove(filepath.Join(m.root, "var", "lib", "snapd", "snaps", name+"_"+rev+".snap"))
}

// setDpkg changes what dpkg-query says and rewrites the status file the
// way dpkg does (a new file renamed over the old one).
func (m *machineInAFolder) setDpkg(t *testing.T, lines string) {
	t.Helper()

	m.mu.Lock()
	m.dpkg = lines
	m.mu.Unlock()

	m.write(t, "var/lib/dpkg/status-new", lines)

	if err := os.Rename(filepath.Join(m.root, "var/lib/dpkg/status-new"), filepath.Join(m.root, "var/lib/dpkg/status")); err != nil {
		t.Fatal(err)
	}
}

func (m *machineInAFolder) scanner() programs.Scanner {
	return programs.Scanner{
		Root:     m.root,
		Home:     "/home/you",
		DataHome: "/home/you/.local/share",
		DataDirs: []string{"/usr/share"},
		Path:     []string{"/usr/bin"},
		Run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "apt-config" {
				return []byte("S='/var/lib/dpkg/status'\n"), nil
			}

			m.mu.Lock()
			defer m.mu.Unlock()

			return []byte(m.dpkg), nil
		},
	}
}

func inventoryOf(t *testing.T, b *Brain) map[string]programs.Record {
	t.Helper()

	rs, _, err := b.ProgramInventory()
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]programs.Record{}
	for _, r := range rs {
		out[r.Key()] = r
	}

	return out
}

func waitForPrograms(t *testing.T, what string, ok func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

/*
 * The whole path a real install takes: a machine in a folder, the brain
 * keeping its inventory with real file events, then a snap removed and put
 * back, a package upgraded and removed. Each is noticed by itself, without
 * anybody asking, and the tool says it.
 */
func TestTheInventoryNoticesRemovalAndReinstallByItself(t *testing.T) {
	b := quietBrain(t)
	m := &machineInAFolder{root: t.TempDir()}

	m.installSnap(t, "blender", "4.5.3", "60")
	m.write(t, "usr/share/applications/gimp.desktop", "[Desktop Entry]\nType=Application\nName=GIMP\nExec=gimp %U\n")
	m.setDpkg(t, "gimp\t2.10.36\tii \nffmpeg\t6.1\tii \n")

	b.programs.scanner = m.scanner()
	b.programs.opts = programs.Options{Debounce: 50 * time.Millisecond, Longest: time.Second, PollEvery: 200 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.KeepPrograms(ctx)
	defer b.StopPrograms()

	waitForPrograms(t, "the first look", func() bool { return len(inventoryOf(t, b)) == 4 })

	inv := inventoryOf(t, b)
	for _, key := range []string{"snap:blender", "app:gimp.desktop", "deb:gimp", "deb:ffmpeg"} {
		if !inv[key].Present {
			t.Errorf("%s not present after the first look: %+v", key, inv)
		}
	}

	// Removed.
	m.removeSnap(t, "blender", "60")
	waitForPrograms(t, "the removal", func() bool { return !inventoryOf(t, b)["snap:blender"].Present })

	out, err := tools.ListPrograms{Programs: b}.Execute(ctx, json.RawMessage(`{"name":"blender"}`))
	if err != nil || !strings.Contains(out, "was removed on") {
		t.Errorf("the tool says it went: %q %v", out, err)
	}

	// Back, newer.
	m.installSnap(t, "blender", "4.5.4", "61")
	waitForPrograms(t, "the reinstall", func() bool {
		r := inventoryOf(t, b)["snap:blender"]
		return r.Present && r.Version == "4.5.4"
	})

	if r := inventoryOf(t, b)["snap:blender"]; r.RemovedAt != nil || r.PreviousVersion != "4.5.3" {
		t.Errorf("returned %+v", r)
	}

	// A package upgraded, then one removed, through dpkg's own file.
	m.setDpkg(t, "gimp\t2.10.38\tii \nffmpeg\t6.1\tii \n")
	waitForPrograms(t, "the upgrade", func() bool { return inventoryOf(t, b)["deb:gimp"].Version == "2.10.38" })

	m.setDpkg(t, "gimp\t2.10.38\tii \nffmpeg\t6.1\trc \n")
	waitForPrograms(t, "the package removal", func() bool { return !inventoryOf(t, b)["deb:ffmpeg"].Present })

	// A menu entry removed.
	os.Remove(filepath.Join(m.root, "usr/share/applications/gimp.desktop"))
	waitForPrograms(t, "the menu entry removal", func() bool { return !inventoryOf(t, b)["app:gimp.desktop"].Present })

	if n := len(inventoryOf(t, b)); n != 4 {
		t.Errorf("one row per program however often it came and went: %d", n)
	}

	_, state, _ := b.ProgramInventory()
	if len(state) != 4 {
		t.Errorf("each source says how it is kept: %+v", state)
	}
}

// A source that cannot be read leaves what was known alone: a dpkg that is
// busy for a moment does not remove every package.
func TestAFailedLookRemovesNothing(t *testing.T) {
	b := quietBrain(t)
	m := &machineInAFolder{root: t.TempDir()}
	m.setDpkg(t, "gimp\t2.10\tii \n")

	b.programs.scanner = m.scanner()

	if _, err := b.lookAtPrograms(context.Background(), programs.Deb); err != nil {
		t.Fatal(err)
	}

	failing := m.scanner()
	failing.Run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, os.ErrDeadlineExceeded
	}

	b.programs.scanner = failing

	if _, err := b.lookAtPrograms(context.Background(), programs.Deb); err == nil {
		t.Error("the failure is reported")
	}

	if !inventoryOf(t, b)["deb:gimp"].Present {
		t.Error("a failed look removed a package")
	}
}

func TestStoppingTheInventoryTwiceOrNeverStartedIsHarmless(t *testing.T) {
	b := quietBrain(t)

	b.StopPrograms()

	if _, state, err := b.ProgramInventory(); err != nil || state != nil {
		t.Errorf("before any look: %v %v", state, err)
	}
}

// What the first look found was there before; what came after, even a
// second later or into a source that was empty then, was installed.
func TestOnlyWhatCameAfterTheFirstLookIsInstalled(t *testing.T) {
	b := quietBrain(t)
	m := &machineInAFolder{root: t.TempDir()}
	m.setDpkg(t, "gimp\t2.10\tii \n")

	b.programs.scanner = m.scanner()

	for _, k := range []programs.Kind{programs.Deb, programs.Snap} {
		if _, err := b.lookAtPrograms(context.Background(), k); err != nil {
			t.Fatal(err)
		}
	}

	m.setDpkg(t, "gimp\t2.10\tii \nkrita\t5.2\tii \n")
	m.installSnap(t, "blender", "4.5.3", "60")

	for _, k := range []programs.Kind{programs.Deb, programs.Snap} {
		changes, err := b.lookAtPrograms(context.Background(), k)
		if err != nil || len(changes) != 1 || changes[0].Kind != programs.Added {
			t.Fatalf("%s: %+v %v", k, changes, err)
		}
	}

	inv := inventoryOf(t, b)
	if !inv["deb:gimp"].Initial || inv["deb:krita"].Initial || inv["snap:blender"].Initial {
		t.Errorf("initial: gimp %v krita %v blender %v", inv["deb:gimp"].Initial, inv["deb:krita"].Initial, inv["snap:blender"].Initial)
	}

	out, _ := tools.ListPrograms{Programs: b}.Execute(context.Background(), nil)
	if !strings.Contains(out, "Installed: krita") || !strings.Contains(out, "Installed: blender") || strings.Contains(out, "Installed: gimp") {
		t.Errorf("%s", out)
	}
}
