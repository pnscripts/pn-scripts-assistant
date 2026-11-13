//go:build windows

package programs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

/*
 * The Windows sources read for real (CI job "The Windows window opens"):
 * an uninstall entry written for this user, a shortcut Windows itself
 * makes, the user's app packages, and the events that notice each change.
 * Everything made here is the test's own (a key named PNSA-Test-…, a
 * temporary folder) and removed afterwards.
 */

const userUninstall = `Software\Microsoft\Windows\CurrentVersion\Uninstall`

func testKey(t *testing.T) string {
	t.Helper()

	name := fmt.Sprintf("PNSA-Test-%d", time.Now().UnixNano())

	t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, userUninstall+`\`+name) })

	return name
}

func writeEntry(t *testing.T, name, version string) {
	t.Helper()

	k, _, err := registry.CreateKey(registry.CURRENT_USER, userUninstall+`\`+name, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}

	defer k.Close()

	for v, s := range map[string]string{
		"DisplayName":     "PN Scripts test program " + name,
		"DisplayVersion":  version,
		"UninstallString": `C:\nowhere\uninstall.exe`,
		"InstallLocation": `C:\nowhere`,
	} {
		if err := k.SetStringValue(v, s); err != nil {
			t.Fatal(err)
		}
	}
}

func findIn(ps []Program, id string) (Program, bool) {
	for _, p := range ps {
		if p.ID == id {
			return p, true
		}
	}

	return Program{}, false
}

func TestTheRealRegistryIsReadAndWatched(t *testing.T) {
	s := FromEnvironment()
	if !s.Windows || s.Registry == nil || len(s.StartMenus) != 2 {
		t.Fatalf("this computer's scanner: %+v", s)
	}

	before, err := s.Scan(context.Background(), Win)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("installed programs here: %d", len(before))

	// A user who never installed a program for themselves has no key of
	// their own yet (it is then polled, like a folder not there yet):
	// made here as the first such install would, removed afterwards.
	if _, there := keyStamp(`HKCU\` + userUninstall); !there {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, userUninstall, registry.SET_VALUE)
		if err != nil {
			t.Fatal(err)
		}

		k.Close()
		t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, userUninstall) })
		t.Log("this user had no uninstall key of their own; made one")
	}

	f := newFires()
	w := Watch(context.Background(), map[Kind][]Target{Win: s.Targets(context.Background(), Win)}, quick(), f.fire)
	defer w.Stop()

	if st := w.State(); len(st) != 1 || st[0].Watched != 3 {
		t.Fatalf("the three uninstall keys are watched: %+v", st)
	}

	name := testKey(t)

	writeEntry(t, name, "1.0")
	f.wait(t, Win, 5*time.Second)

	got, err := s.Scan(context.Background(), Win)
	if err != nil {
		t.Fatal(err)
	}

	p, ok := findIn(got, name)
	if !ok || p.Version != "1.0" || p.Name != "PN Scripts test program "+name || p.Path != `C:\nowhere` {
		t.Fatalf("the new entry: %+v %v", p, ok)
	}

	// An update writes a value: noticed too.
	writeEntry(t, name, "1.1")
	f.wait(t, Win, 5*time.Second)

	if got, _ = s.Scan(context.Background(), Win); func() string { p, _ := findIn(got, name); return p.Version }() != "1.1" {
		t.Fatal("the update is read")
	}

	if err := registry.DeleteKey(registry.CURRENT_USER, userUninstall+`\`+name); err != nil {
		t.Fatal(err)
	}

	f.wait(t, Win, 5*time.Second)

	if got, _ = s.Scan(context.Background(), Win); func() bool { _, ok := findIn(got, name); return ok }() {
		t.Fatal("the removed entry is gone")
	}
}

// shortcut asks Windows itself to make a .lnk.
func shortcut(t *testing.T, path, target string) {
	t.Helper()

	ps := fmt.Sprintf(`$s = (New-Object -ComObject WScript.Shell).CreateShortcut('%s'); $s.TargetPath = '%s'; $s.Arguments = '--from-a-test'; $s.Save()`,
		strings.ReplaceAll(path, "'", "''"), strings.ReplaceAll(target, "'", "''"))

	if out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps).CombinedOutput(); err != nil {
		t.Fatalf("making %s: %v %s", path, err, out)
	}
}

func TestRealShortcutsAreReadAndTheStartMenuIsWatched(t *testing.T) {
	menu := t.TempDir()
	notepad := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")

	os.MkdirAll(filepath.Join(menu, "Accessories"), 0o755)
	shortcut(t, filepath.Join(menu, "Accessories", "Notepad.lnk"), notepad)
	shortcut(t, filepath.Join(menu, "Read me.lnk"), filepath.Join(os.Getenv("SystemRoot"), "win.ini"))

	b, err := os.ReadFile(filepath.Join(menu, "Accessories", "Notepad.lnk"))
	if err != nil {
		t.Fatal(err)
	}

	if target, err := ShortcutTarget(b); err != nil || !strings.EqualFold(expanded(target), notepad) {
		t.Fatalf("Windows' own shortcut: %q %v, want %q", target, err, notepad)
	}

	s := Scanner{Windows: true, StartMenus: []string{menu}}

	got, err := s.Scan(context.Background(), App)
	if err != nil || len(got) != 1 || got[0].ID != "Accessories/Notepad.lnk" || got[0].Name != "Notepad" {
		t.Fatalf("the Start menu: %+v %v", got, err)
	}

	f := newFires()
	w := Watch(context.Background(), map[Kind][]Target{App: s.Targets(context.Background(), App)}, quick(), f.fire)
	defer w.Stop()

	// A program installed into a folder of its own, in a folder of the
	// menu: the whole tree is watched.
	os.MkdirAll(filepath.Join(menu, "New Program"), 0o755)
	f.wait(t, App, 5*time.Second)

	shortcut(t, filepath.Join(menu, "New Program", "Paint.lnk"), filepath.Join(os.Getenv("SystemRoot"), "System32", "mspaint.exe"))
	f.wait(t, App, 5*time.Second)

	if got, _ = s.Scan(context.Background(), App); len(got) != 2 {
		t.Fatalf("after the install: %+v", got)
	}

	os.RemoveAll(filepath.Join(menu, "New Program"))
	f.wait(t, App, 5*time.Second)
}

func TestTheUsersAppPackagesAreRead(t *testing.T) {
	s := FromEnvironment()

	start := time.Now()

	got, err := s.Scan(context.Background(), Msix)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("app packages here: %d, read in %v", len(got), time.Since(start).Round(time.Millisecond))

	for _, p := range got {
		if p.ID == "" || p.Name == "" || p.Kind != Msix {
			t.Errorf("an app package without its names: %+v", p)
		}
	}

	_, there := keyStamp(appxRepository)
	t.Logf("the user's package list key is there: %v; Packages folder: %s", there, filepath.Join(s.LocalAppData, "Packages"))

	f := newFires()
	w := Watch(context.Background(), map[Kind][]Target{Msix: s.Targets(context.Background(), Msix)}, quick(), f.fire)
	defer w.Stop()

	t.Logf("app packages kept current by: %+v", w.State())
}

// expanded is a target with %NAME% replaced by the variable's value.
func expanded(target string) string {
	return regexp.MustCompile(`%([A-Za-z_]+)%`).ReplaceAllStringFunc(target, func(v string) string {
		return os.Getenv(strings.Trim(v, "%"))
	})
}
