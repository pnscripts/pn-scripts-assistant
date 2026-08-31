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
	text := entryText("Ariel", "/home/somebody/Apps/PN-Brain.AppImage")

	for _, want := range []string{
		"[Desktop Entry]\n",
		"Type=Application\n",
		"Name=Ariel\n",
		"Exec=/home/somebody/Apps/PN-Brain.AppImage\n",
		"Icon=pn-brain\n",
		"Terminal=false\n",
		"StartupWMClass=pn-brain\n",
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
	text := entryText("PN Brain", "/opt/brain")

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
		name := filepath.Join("icons", "pn-brain-"+itoa(size)+".png")

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

	if _, err := Install("PN Brain"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(entry); err != nil {
		t.Fatalf("no entry was written: %v", err)
	}

	for _, size := range Sizes {
		at := filepath.Join(iconRoot, itoa(size)+"x"+itoa(size), "apps", "pn-brain.png")

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
	t.Setenv("APPIMAGE", "/home/somebody/Apps/PN-Brain-x86_64.AppImage")

	at, err := self()
	if err != nil {
		t.Fatal(err)
	}

	if at != "/home/somebody/Apps/PN-Brain-x86_64.AppImage" {
		t.Errorf("the entry would run %q, which is a mount point that will be gone", at)
	}

	// And without it, the ordinary answer still applies.
	t.Setenv("APPIMAGE", "")

	if at, err := self(); err != nil || at == "" {
		t.Errorf("a plain binary could not say where it is: %q %v", at, err)
	}
}
