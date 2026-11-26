package programs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

/*
 * Notes of the read after 1.2.0 (ASSISTANT-PROGRAM-USE-NOTES-1121).
 *
 * 3. The fingerprint of a snap's or a Flatpak's menu entry followed
 *    /snap/bin/<name> (a link to /usr/bin/snap) or /usr/bin/flatpak,
 *    which a refresh does not change: the entry's "always" stayed awake
 *    after the program behind it was updated. The same for an entry
 *    started over D-Bus (its service file says what runs) and for a
 *    wrapper script (the program it runs was not followed).
 * 6. A same-id entry put into the user's own menu folder starts instead
 *    of the one that was allowed, under the old fingerprint until the
 *    inventory looked again (about two seconds); and desktop ids, which
 *    are told apart by case on Linux, were compared ignoring it.
 */

// machine is a scratch machine for a fingerprint: a root folder that
// stands for "/", with the user's data under home.
type machine struct {
	root string
	s    Scanner
}

func scratchMachine(t *testing.T) machine {
	t.Helper()

	root := t.TempDir()
	s := Scanner{Root: root, DataHome: "/home/you/.local/share", DataDirs: []string{"/usr/local/share", "/usr/share"}}

	keep := pinScanner
	t.Cleanup(func() { pinScanner = keep })

	pinScanner = func() Scanner { return s }

	return machine{root: root, s: s}
}

// at is a place on the scratch machine.
func (m machine) at(path string) string { return filepath.Join(m.root, filepath.FromSlash(path)) }

func (m machine) write(t *testing.T, path, text string, mode os.FileMode) string {
	t.Helper()

	full := m.at(path)

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(full, []byte(text), mode); err != nil {
		t.Fatal(err)
	}

	return full
}

func (m machine) link(t *testing.T, target, path string) {
	t.Helper()

	full := m.at(path)

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}

	os.Remove(full)

	if err := os.Symlink(target, full); err != nil {
		t.Skip("no links here:", err)
	}
}

// entry writes a menu entry into the system's menu folder.
func (m machine) entry(t *testing.T, id, body string) Record {
	t.Helper()

	path := m.write(t, "/usr/share/applications/"+id, "[Desktop Entry]\nType=Application\nName=Demo\n"+body, 0o644)

	e := ParseDesktop(id, mustOpen(t, path))
	e.Program.Path = path

	return Record{Program: e.Program, RowID: 1, Present: true}
}

func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { f.Close() })

	return f
}

// noLook finds nothing by name: every file here is named in full.
func noLook(...string) (string, bool) { return "", false }

func TestASnapsMenuEntryFollowsTheRevisionBehindIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("snaps are a Linux thing")
	}

	m := scratchMachine(t)

	m.write(t, "/snap/demo/101/bin/demo", "one", 0o755)
	m.write(t, "/snap/demo/102/bin/demo", "two", 0o755)
	m.link(t, "101", "/snap/demo/current")

	r := m.entry(t, "demo_demo.desktop", "Exec=env BAMF_DESKTOP_FILE_HINT=/var/lib/snapd/desktop/applications/demo_demo.desktop /snap/bin/demo %U\n")

	before := Pin(r, noLook)

	// A refresh: the same /snap/bin/demo, another revision behind it.
	m.link(t, "102", "/snap/demo/current")

	if after := Pin(r, noLook); !IsPin(before) || before == after {
		t.Errorf("the snap was refreshed and the fingerprint did not change: %q, %q", before, after)
	}

	// An app of a snap ("<snap>.<app>") follows its snap.
	r = m.entry(t, "demo_tool.desktop", "Exec=/snap/bin/demo.tool\n")
	before = Pin(r, noLook)

	m.link(t, "101", "/snap/demo/current")

	if after := Pin(r, noLook); before == after {
		t.Error("an app of a snap did not follow the snap's revision")
	}
}

func TestAFlatpaksMenuEntryFollowsTheCommitBehindIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Flatpak is a Linux thing")
	}

	m := scratchMachine(t)

	app := "/var/lib/flatpak/app/org.example.Demo"
	m.write(t, app+"/x86_64/stable/aaaa/files/bin/demo", "one", 0o755)
	m.write(t, app+"/x86_64/stable/bbbb/files/bin/demo", "two", 0o755)
	m.link(t, "x86_64/stable", app+"/current")
	m.link(t, "aaaa", app+"/x86_64/stable/active")

	r := m.entry(t, "org.example.Demo.desktop",
		"Exec=/usr/bin/flatpak run --branch=stable --arch=x86_64 --command=demo --file-forwarding org.example.Demo @@u %U @@\n")

	before := Pin(r, noLook)

	m.link(t, "bbbb", app+"/x86_64/stable/active")

	if after := Pin(r, noLook); !IsPin(before) || before == after {
		t.Errorf("the Flatpak was updated and the fingerprint did not change: %q, %q", before, after)
	}

	// Installed for the user only: followed there too.
	user := "/home/you/.local/share/flatpak/app/org.example.Mine"
	m.write(t, user+"/x86_64/stable/cccc/files/bin/mine", "one", 0o755)
	m.write(t, user+"/x86_64/stable/dddd/files/bin/mine", "two", 0o755)
	m.link(t, "x86_64/stable", user+"/current")
	m.link(t, "cccc", user+"/x86_64/stable/active")

	r = m.entry(t, "org.example.Mine.desktop", "Exec=flatpak run org.example.Mine\n")
	before = Pin(r, noLook)

	m.link(t, "dddd", user+"/x86_64/stable/active")

	if after := Pin(r, noLook); before == after {
		t.Error("a Flatpak of the user's was updated and the fingerprint did not change")
	}
}

func TestAnEntryStartedOverDBusFollowsItsServiceFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("D-Bus activation is a Linux thing")
	}

	m := scratchMachine(t)

	m.write(t, "/usr/bin/demo", "one", 0o755)
	m.write(t, "/usr/libexec/demo-service", "one", 0o755)
	service := "/usr/share/dbus-1/services/org.example.Demo.service"
	m.write(t, service, "[D-BUS Service]\nName=org.example.Demo\nExec="+m.at("/usr/libexec/demo-service")+" --gapplication-service\n", 0o644)

	r := m.entry(t, "org.example.Demo.desktop", "DBusActivatable=true\nExec="+m.at("/usr/bin/demo")+"\n")

	before := Pin(r, noLook)

	// The service starts something else now.
	m.write(t, service, "[D-BUS Service]\nName=org.example.Demo\nExec=/tmp/other\n", 0o644)

	if after := Pin(r, noLook); !IsPin(before) || before == after {
		t.Error("the D-Bus service file changed and the fingerprint did not")
	}

	// The program the service starts is updated.
	m.write(t, service, "[D-BUS Service]\nName=org.example.Demo\nExec="+m.at("/usr/libexec/demo-service")+" --gapplication-service\n", 0o644)
	before = Pin(r, noLook)

	m.write(t, "/usr/libexec/demo-service", "two, longer", 0o755)

	if after := Pin(r, noLook); before == after {
		t.Error("the program the D-Bus service starts changed and the fingerprint did not")
	}

	// A service file of the user's comes first.
	before = Pin(r, noLook)

	m.write(t, "/home/you/.local/share/dbus-1/services/org.example.Demo.service", "[D-BUS Service]\nName=org.example.Demo\nExec=/tmp/mine\n", 0o644)

	if after := Pin(r, noLook); before == after {
		t.Error("a service file of the user's comes first and the fingerprint did not change")
	}
}

func TestAWrapperScriptsProgramIsFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("scripts with #! are not started this way on Windows")
	}

	m := scratchMachine(t)

	real := m.write(t, "/opt/demo/demo-bin", "one", 0o755)
	wrapper := m.write(t, "/usr/bin/demo", "#!/bin/sh\nexport DEMO_HOME=/opt/demo\nexec "+real+" \"$@\"\n", 0o755)

	r := m.entry(t, "demo.desktop", "Exec="+wrapper+" %F\n")

	before := Pin(r, noLook)

	// Updated: the script is the same, the program it runs is not.
	m.write(t, "/opt/demo/demo-bin", "two, longer", 0o755)

	if after := Pin(r, noLook); !IsPin(before) || before == after {
		t.Error("the program a wrapper script runs changed and the fingerprint did not")
	}
}

func TestASameIDEntryOfTheUsersMakesTheFingerprintAnother(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("menu entries by desktop id are a Linux thing")
	}

	m := scratchMachine(t)

	m.write(t, "/usr/bin/demo", "one", 0o755)
	r := m.entry(t, "demo.desktop", "Exec="+m.at("/usr/bin/demo")+"\n")

	before := Pin(r, noLook)

	// The same id in the user's own folder starts instead, from now on,
	// before the inventory has looked again.
	m.write(t, "/home/you/.local/share/applications/demo.desktop", "[Desktop Entry]\nType=Application\nName=Demo\nExec=/tmp/other\n", 0o644)

	if after := Pin(r, noLook); before == after {
		t.Error("a same-id entry of the user's starts instead and the fingerprint did not change")
	}
}

func TestDesktopIDsAreToldApartByCaseOnLinux(t *testing.T) {
	gimp := Record{Program: Program{Kind: App, ID: "gimp.desktop", Name: "GIMP", Exec: "gimp"}, RowID: 1, Present: true}
	flat := Record{Program: Program{Kind: App, ID: "org.gimp.GIMP.desktop", Name: "GIMP", Exec: "flatpak"}, RowID: 2, Present: true}

	if runtime.GOOS == "windows" {
		t.Skip("Windows tells names apart without case")
	}

	if got := Match([]Record{gimp}, Use{Verb: VerbStart, Desktop: "GIMP.desktop"}); len(got) != 0 {
		t.Errorf("GIMP.desktop is another entry than gimp.desktop: %+v", got)
	}

	if got := Match([]Record{flat}, Use{Verb: VerbChange, Flatpak: "org.gimp.gimp"}); len(got) != 0 {
		t.Errorf("org.gimp.gimp is another Flatpak than org.gimp.GIMP: %+v", got)
	}

	// The same id, as before.
	if got := Match([]Record{gimp}, Use{Verb: VerbStart, Desktop: "gimp.desktop"}); len(got) != 1 {
		t.Errorf("gimp.desktop: %+v", got)
	}

	if got := Match([]Record{flat}, Use{Verb: VerbChange, Flatpak: "org.gimp.GIMP"}); len(got) != 1 {
		t.Errorf("org.gimp.GIMP: %+v", got)
	}
}

/*
 * Found by the review of the fix: a wrapper script that writes a log was
 * followed into the log, so its "always" slept after every start; and a
 * pipe put where a script or a service file was expected held whoever
 * worked out the fingerprint until something wrote to it.
 */
func TestAScriptsLogIsNotFollowedAndAPipeDoesNotHoldTheLook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("scripts with #! and pipes in folders are Linux things")
	}

	m := scratchMachine(t)

	real := m.write(t, "/opt/demo/demo-bin", "one", 0o755)
	log := m.write(t, "/var/log/demo.log", "", 0o644)
	wrapper := m.write(t, "/usr/bin/demo", "#!/bin/sh\nexec "+real+" \"$@\" >> "+log+" 2>&1\n", 0o755)

	r := m.entry(t, "demo.desktop", "Exec="+wrapper+" %F\n")
	before := Pin(r, noLook)

	// Started once: the log grew.
	m.write(t, "/var/log/demo.log", "started\n", 0o644)

	if after := Pin(r, noLook); before != after {
		t.Error("the log a wrapper script writes changed the fingerprint")
	}

	// Pipes where the script and the service file were.
	os.Remove(wrapper)

	if err := mkfifo(wrapper); err != nil {
		t.Skip("no pipes here:", err)
	}

	service := m.at("/home/you/.local/share/dbus-1/services/org.example.Pipe.service")
	os.MkdirAll(filepath.Dir(service), 0o755)

	if err := mkfifo(service); err != nil {
		t.Fatal(err)
	}

	dbus := m.entry(t, "org.example.Pipe.desktop", "DBusActivatable=true\nExec="+wrapper+"\n")

	done := make(chan struct{})

	go func() {
		Pin(r, noLook)
		Pin(dbus, noLook)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a pipe held the fingerprint")
	}

	// A menu entry that is a pipe has nothing to follow, and holds nothing.
	pipeEntry := m.at("/usr/share/applications/pipe.desktop")
	if err := mkfifo(pipeEntry); err != nil {
		t.Fatal(err)
	}

	done = make(chan struct{})

	go func() {
		if got := Pin(Record{Program: Program{Kind: App, ID: "pipe.desktop", Path: pipeEntry}}, noLook); got != "" {
			t.Errorf("a pipe has a fingerprint: %q", got)
		}

		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a menu entry that is a pipe held the fingerprint")
	}
}
