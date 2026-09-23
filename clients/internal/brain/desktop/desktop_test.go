package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * The entry has to point at this program and group with its own window.
 *
 * StartupWMClass is the part that is easy to leave out and hard to notice: the
 * program still launches, but the window it opens is not connected to the icon
 * that launched it, so the dock shows the launcher and a second nameless entry
 * beside it, and pressing the launcher again does nothing visible.
 */
func TestTheEntryDescribesThisProgram(t *testing.T) {
	text := entryText("Ariel", "/home/somebody/Apps/PN-Scripts-Assistant.AppImage")

	for _, want := range []string{
		"[Desktop Entry]\n",
		"Type=Application\n",
		"Name=Ariel\n",
		"Exec=/home/somebody/Apps/PN-Scripts-Assistant.AppImage\n",
		"Icon=pn-scripts-assistant\n",
		"Terminal=false\n",
		"StartupWMClass=pn-scripts-assistant\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the entry is missing %q", strings.TrimSuffix(want, "\n"))
		}
	}
}

// A name is typed by a person and goes into a line-based file.
//
// A newline in it would end the Name line and turn whatever came after into
// another key, which is at best a broken entry and at worst one that runs
// something else.
func TestANameCannotBreakOutOfItsLine(t *testing.T) {
	text := entryText("Ariel\nExec=/usr/bin/anything-else", "/opt/brain")

	// By key, not by substring: the injected text ending up inside the Name
	// value is harmless and expected — what must not happen is it becoming a
	// key of its own on a line of its own.
	var execs []string

	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "Exec=") {
			execs = append(execs, line)
		}
	}

	/*
	 * Two Exec lines are correct: the program, and the setup action beside it.
	 * Both must name the binary this was built with and nothing else — an
	 * injected third, or either of these rewritten, is the failure being
	 * guarded against.
	 */
	want := []string{"Exec=/opt/brain", "Exec=/opt/brain setup"}

	if len(execs) != len(want) {
		t.Fatalf("a newline in the name changed what gets run: %q", execs)
	}

	for i, exec := range execs {
		if exec != want[i] {
			t.Errorf("exec %d is %q, want %q", i, exec, want[i])
		}
	}
}

// The setup action has to be declared before it is defined, or the desktop
// ignores it — a section nothing lists is a section nothing reads.
func TestSetupIsOfferedFromTheIcon(t *testing.T) {
	text := entryText("PN Scripts Assistant", "/opt/brain")

	for _, want := range []string{
		"Actions=setup;\n",
		"[Desktop Action setup]\n",
		"Exec=/opt/brain setup\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the entry is missing %q", strings.TrimSuffix(want, "\n"))
		}
	}
}

// Every size the icon theme is told about has to actually be in the binary.
func TestTheIconTravelsInside(t *testing.T) {
	for _, size := range Sizes {
		name := filepath.Join("icons", "pn-scripts-assistant-"+itoa(size)+".png")

		body, err := icons.ReadFile(name)
		if err != nil {
			t.Errorf("%s is not embedded: %v", name, err)

			continue
		}

		if len(body) < 500 {
			t.Errorf("%s is %d bytes, which is not an icon", name, len(body))
		}

		if !strings.HasPrefix(string(body[:8]), "\x89PNG\r\n\x1a\n") {
			t.Errorf("%s is not a PNG", name)
		}
	}
}

// Installing writes only inside the user's own data directory, and removing
// puts it back the way it was.
func TestItWritesOnlyWhereItSaidItWould(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)

	entry, iconRoot := Where()

	if !strings.HasPrefix(entry, home) || !strings.HasPrefix(iconRoot, home) {
		t.Fatalf("it would write outside the data directory: %s and %s", entry, iconRoot)
	}

	if _, err := Install("PN Scripts Assistant"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(entry); err != nil {
		t.Fatalf("no entry was written: %v", err)
	}

	for _, size := range Sizes {
		at := filepath.Join(iconRoot, itoa(size)+"x"+itoa(size), "apps", "pn-scripts-assistant.png")

		if _, err := os.Stat(at); err != nil {
			t.Errorf("no %dpx icon: %v", size, err)
		}
	}

	if err := Remove(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Error("the entry survived being removed")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var out []byte

	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}

	return string(out)
}

/*
 * An AppImage is not where it thinks it is.
 *
 * It mounts itself under /tmp and runs from there, so the path the operating
 * system reports is gone the moment the program exits — and a menu entry
 * pointing at it does nothing whatsoever when pressed, which is a worse outcome
 * than never having added it. The runtime puts the real path of the downloaded
 * file in APPIMAGE.
 */
func TestAnAppImageRecordsTheFileSomebodyDownloaded(t *testing.T) {
	t.Setenv("APPIMAGE", "/home/somebody/Apps/PN-Scripts-Assistant-x86_64.AppImage")

	at, err := self()
	if err != nil {
		t.Fatal(err)
	}

	if at != "/home/somebody/Apps/PN-Scripts-Assistant-x86_64.AppImage" {
		t.Errorf("the entry would run %q, which is a mount point that will be gone", at)
	}

	// And without it, the ordinary answer still applies.
	t.Setenv("APPIMAGE", "")

	if at, err := self(); err != nil || at == "" {
		t.Errorf("a plain binary could not say where it is: %q %v", at, err)
	}
}

/*
 * A menu entry that points at a program which has moved is repaired.
 *
 * The folder holding this program was renamed, and from that moment the icon
 * in the menu named a path that did not exist. Nothing failed and nothing was
 * reported: the entry sat in the menu looking exactly as it should, and
 * pressing it did nothing at all.
 */
func TestAMovedProgramFixesItsOwnMenuEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	t.Setenv("HOME", home)

	entry, _ := Where()

	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}

	// An entry from a build that lived somewhere else.
	stale := entryText("PN Scripts Assistant", "/somewhere/that/is/gone/pn-scripts-assistant")

	if err := os.WriteFile(entry, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	fixed, err := RepairIfStale("PN Scripts Assistant")
	if err != nil {
		t.Fatal(err)
	}

	if !fixed {
		t.Fatal("a stale entry was left pointing at a program that is not there")
	}

	if !Installed() {
		t.Error("the rewritten entry still does not point at this program")
	}

	// Repairing twice is not writing twice.
	if again, _ := RepairIfStale("PN Scripts Assistant"); again {
		t.Error("an entry that was already correct was rewritten anyway")
	}
}

/*
 * And a machine that has never had a menu entry does not get one for starting
 * the program. Putting itself in somebody's menu uninvited is a thing to be
 * asked for, not a side effect of being run.
 */
func TestStartingUpDoesNotInstallAMenuEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	t.Setenv("HOME", home)

	if fixed, err := RepairIfStale("PN Scripts Assistant"); fixed || err != nil {
		t.Errorf("it installed itself into the menu uninvited (%v, %v)", fixed, err)
	}

	entry, _ := Where()

	if _, err := os.Stat(entry); err == nil {
		t.Error("a menu entry appeared without being asked for")
	}
}

/*
 * A menu entry from before the rename is replaced, not left beside the new one.
 *
 * Left, the program is in the menu twice — once under the name it has and once
 * under the name it had, with an icon and a window class that no longer match
 * the window it opens.
 */
func TestAnEntryFromBeforeTheRenameIsReplaced(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)

	old, iconRoot := whereFor(LegacyEntryName)
	oldIcon := filepath.Join(iconRoot, "64x64", "apps", LegacyEntryName+".png")

	for _, f := range []string{old, oldIcon} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(f, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fixed, err := RepairIfStale("Assistant")
	if err != nil || !fixed {
		t.Fatalf("the old entry was not replaced: %v %v", fixed, err)
	}

	entry, _ := Where()

	if _, err := os.Stat(entry); err != nil {
		t.Errorf("no entry under the new name: %v", err)
	}

	for _, f := range []string{old, oldIcon} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s outlived the rename", f)
		}
	}
}

/*
 * Installed with the package, this program writes nothing.
 *
 * The .deb puts an entry in /usr/share/applications and the program in the
 * menu. A second copy in the user's own folder shadows the packaged one, is
 * not updated when the package is, and keeps pointing at a path that an
 * uninstall takes away — a launcher that does nothing when pressed.
 */
func TestAPackagedEntryIsLeftAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)

	me := "/usr/bin/" + EntryName
	t.Setenv("APPIMAGE", me) // stands in for "where this program is"

	system := t.TempDir()
	t.Setenv("XDG_DATA_DIRS", system)

	packaged := filepath.Join(system, "applications", EntryName+".desktop")

	if err := os.MkdirAll(filepath.Dir(packaged), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(packaged, []byte(entryText("PN Scripts Assistant", me)), 0o644); err != nil {
		t.Fatal(err)
	}

	if SystemWide() != packaged {
		t.Fatalf("the packaged entry was not found: %q", SystemWide())
	}

	if !Installed() {
		t.Error("it says the program is not in the menu when the package put it there")
	}

	entry, err := Install("PN Scripts Assistant")
	if err != nil {
		t.Fatal(err)
	}

	if entry != packaged {
		t.Errorf("it pointed at %s instead of the packaged entry", entry)
	}

	mine, _ := Where()

	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Errorf("it wrote a second entry at %s", mine)
	}
}

// A packaged entry for a different copy of the program is not this one.
//
// A .deb in /usr/bin and a downloaded file in ~/Downloads are two programs,
// and the one running is the one that belongs in the menu.
func TestAPackagedEntryForAnotherCopyIsNotMine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	t.Setenv("APPIMAGE", "/home/somebody/Downloads/PN-Scripts-Assistant.AppImage")

	system := t.TempDir()
	t.Setenv("XDG_DATA_DIRS", system)

	packaged := filepath.Join(system, "applications", EntryName+".desktop")

	if err := os.MkdirAll(filepath.Dir(packaged), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(packaged, []byte(entryText("PN Scripts Assistant", "/usr/bin/"+EntryName)), 0o644); err != nil {
		t.Fatal(err)
	}

	if at := SystemWide(); at != "" {
		t.Fatalf("somebody else's entry was taken for mine: %s", at)
	}

	entry, err := Install("PN Scripts Assistant")
	if err != nil {
		t.Fatal(err)
	}

	mine, _ := Where()

	if entry != mine {
		t.Errorf("it wrote to %s rather than this user's own folder", entry)
	}

	if !Installed() {
		t.Error("it wrote the entry and then said it was not installed")
	}
}

/*
 * A packaged entry says "Exec=pn-scripts-assistant", not a path.
 *
 * This is the shape a .deb installs, and the first version of this check did
 * not recognise it: it compared the full path of the running binary against
 * the Exec line, passed its own tests — which wrote entries the way this
 * program writes them — and then wrote a second entry over the packaged one
 * the first time it met a real package. Found by running the real .deb's
 * files, which is why this test uses them rather than a made-up entry.
 */
func TestAPackagedEntryNamesTheProgramWithoutAPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)

	// A program on the PATH, as a package puts it there.
	bin := t.TempDir()
	me := filepath.Join(bin, EntryName)

	if err := os.WriteFile(me, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin)
	t.Setenv("APPIMAGE", me) // stands in for "where this program is"

	system := t.TempDir()
	t.Setenv("XDG_DATA_DIRS", system)

	packaged := filepath.Join(system, "applications", EntryName+".desktop")

	if err := os.MkdirAll(filepath.Dir(packaged), 0o755); err != nil {
		t.Fatal(err)
	}

	// Exactly what scripts/build-packages.sh writes.
	if err := os.WriteFile(packaged, []byte(
		"[Desktop Entry]\nType=Application\nName=PN Scripts Assistant\n"+
			"Exec="+EntryName+"\nIcon="+EntryName+"\nTerminal=false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if at := SystemWide(); at != packaged {
		t.Fatalf("the packaged entry was not recognised: %q", at)
	}

	if _, err := Install("PN Scripts Assistant"); err != nil {
		t.Fatal(err)
	}

	mine, _ := Where()

	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Errorf("it wrote a second entry at %s", mine)
	}

	// A bare name on the PATH that is a different copy of the program is not
	// this one.
	elsewhere := filepath.Join(t.TempDir(), EntryName)

	if err := os.WriteFile(elsewhere, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APPIMAGE", elsewhere)

	if at := SystemWide(); at != "" {
		t.Errorf("another copy's packaged entry was taken for mine: %s", at)
	}
}
