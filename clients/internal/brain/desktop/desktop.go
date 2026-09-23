// Package desktop puts the brain in the applications menu.
//
// A single downloaded file that runs when you double-click it is the right
// shape for this program, but it is not how anybody launches anything twice.
// The second time, they press the key with the Ubuntu logo on it and type the
// first few letters of the name — and a program that is not in that list may as
// well not be installed.
//
// Everything here writes to the user's own directories. Nothing needs a
// password, nothing touches anything outside the home directory, and removing
// it is the same two files going away again.
package desktop

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/paths"
	"strings"
)

/*
 * The icon travels inside the binary.
 *
 * The alternative is a file beside it, which works until somebody moves the one
 * file this program is supposed to be — and then the menu entry points at an
 * icon that is not there, which shows as a blank square with no explanation.
 * Drawn by scripts/make-icon.py from the same geometry as the core.
 */
//go:embed icons/*.png
var icons embed.FS

// Sizes the icon is drawn at, largest first.
var Sizes = []int{512, 256, 128, 64, 48}

/*
 * EntryName is the desktop entry's file name, the icon's name in the theme and
 * the window's class; LegacyEntryName is what all three were when the program
 * was PN Brain.
 *
 * The window's class is the program's own file name, so these have to agree
 * with what the binary is called or the dock shows the launcher and the window
 * as two different things.
 */
const (
	EntryName       = paths.Name
	LegacyEntryName = paths.LegacyName
)

// Where returns the paths this would write, without writing them.
func Where() (entry string, iconDir string) {
	return whereFor(EntryName)
}

func whereFor(name string) (entry string, iconDir string) {
	data := dataHome()

	return filepath.Join(data, "applications", name+".desktop"),
		filepath.Join(data, "icons", "hicolor")
}

// Installed reports whether the menu entry is there and points at this program.
//
// "Points at this program" matters: an entry left behind by a copy that has
// since been moved or deleted is worse than no entry, because it is in the menu
// and does nothing when pressed.
func Installed() bool {
	if SystemWide() != "" {
		return true
	}

	entry, _ := Where()

	return pointsAtMe(entry)
}

func pointsAtMe(entry string) bool {
	body, err := os.ReadFile(entry)
	if err != nil {
		return false
	}

	me, err := self()
	if err != nil {
		return false
	}

	return strings.Contains(string(body), "Exec="+me+"\n")
}

/*
 * SystemWide is the menu entry a package installed for this same program, or
 * empty when there is none.
 *
 * Installed from the .deb, the entry is already in /usr/share/applications and
 * the program is already in the menu. Writing a second copy into the user's own
 * folder would not add anything: the user's copy shadows the packaged one, it
 * is not updated when the package is, and it keeps pointing at a path that an
 * uninstall takes away — an entry in the menu that does nothing when pressed.
 *
 * "For this same program" is the whole check. A packaged copy at /usr/bin and a
 * downloaded copy in ~/Downloads are two different programs, and the one that
 * is running should be the one in the menu.
 */
func SystemWide() string {
	me, err := self()
	if err != nil {
		return ""
	}

	for _, dir := range systemDataDirs() {
		entry := filepath.Join(dir, "applications", EntryName+".desktop")

		body, err := os.ReadFile(entry)
		if err != nil {
			continue
		}

		if strings.Contains(string(body), "Exec="+me+"\n") {
			return entry
		}
	}

	return ""
}

// systemDataDirs is XDG_DATA_DIRS, or what the specification says when it is
// not set.
func systemDataDirs() []string {
	dirs := os.Getenv("XDG_DATA_DIRS")
	if strings.TrimSpace(dirs) == "" {
		dirs = "/usr/local/share:/usr/share"
	}

	var out []string

	for _, dir := range strings.Split(dirs, ":") {
		if dir = strings.TrimSpace(dir); dir != "" {
			out = append(out, dir)
		}
	}

	return out
}

/*
 * Install writes the menu entry and the icons, and returns where the entry went.
 *
 * name is what the brain is called, so the menu says what its owner named it
 * rather than what this program is called.
 */
func Install(name string) (string, error) {
	// Already in the menu, put there by the package. Nothing to write, and
	// writing anyway is how the menu ends up with a stale entry in it.
	if entry := SystemWide(); entry != "" {
		return entry, nil
	}

	self, err := self()
	if err != nil {
		return "", err
	}

	entry, iconRoot := Where()

	for _, size := range Sizes {
		body, err := icons.ReadFile(fmt.Sprintf("icons/%s-%d.png", EntryName, size))
		if err != nil {
			return "", fmt.Errorf("reading the %dpx icon: %w", size, err)
		}

		at := filepath.Join(iconRoot, fmt.Sprintf("%dx%d", size, size), "apps",
			EntryName+".png")

		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			return "", err
		}

		if err := os.WriteFile(at, body, 0o644); err != nil {
			return "", fmt.Errorf("writing %s: %w", at, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		return "", err
	}

	if err := os.WriteFile(entry, []byte(entryText(name, self)), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", entry, err)
	}

	// The entry from before the rename would sit beside this one: the same
	// program in the menu twice, one of them under a name it no longer has.
	if err := removeEntry(LegacyEntryName); err != nil {
		return "", err
	}

	refresh(iconRoot, filepath.Dir(entry))

	return entry, nil
}

// Remove takes the entry and the icons out again, under either name.
func Remove() error {
	for _, name := range []string{EntryName, LegacyEntryName} {
		if err := removeEntry(name); err != nil {
			return err
		}
	}

	entry, iconRoot := Where()

	refresh(iconRoot, filepath.Dir(entry))

	return nil
}

func removeEntry(name string) error {
	entry, iconRoot := whereFor(name)

	if err := os.Remove(entry); err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, size := range Sizes {
		at := filepath.Join(iconRoot, fmt.Sprintf("%dx%d", size, size), "apps", name+".png")

		if err := os.Remove(at); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

/*
 * entryText is the menu entry itself.
 *
 * StartupWMClass is not optional here. Without it the window this program opens
 * is not connected to the icon that launched it, so the dock shows two things —
 * the launcher and a second, unnamed entry with a blank icon — and pressing the
 * launcher again opens nothing while the window sits there already.
 */
func entryText(name, exec string) string {
	if strings.TrimSpace(name) == "" {
		name = config.DefaultName
	}

	// Newlines in either would end the line and start something else; a name is
	// typed by a person and goes in a file with a line-based format.
	name = strings.NewReplacer("\n", " ", "\r", " ").Replace(name)
	exec = strings.NewReplacer("\n", "", "\r", "").Replace(exec)

	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Version=1.0\n" +
		"Name=" + name + "\n" +
		"Comment=A private assistant that runs entirely on this machine\n" +
		"Exec=" + exec + "\n" +
		"Icon=" + EntryName + "\n" +
		"Terminal=false\n" +
		// One main category only: two of them puts the program in the menu
		// twice, which desktop-file-validate warns about and a person notices.
		"Categories=Utility;\n" +
		"Keywords=assistant;brain;voice;memory;\n" +
		"StartupNotify=true\n" +
		"StartupWMClass=" + EntryName + "\n" +
		/*
		 * Setup on the right-click menu of the icon.
		 *
		 * Changing the drive or the model meant a terminal, which for this
		 * program is the same as saying it cannot be done — the icon is the
		 * only handle most people ever have on it. A desktop action puts it
		 * where anyone would look for it, one press from the launcher.
		 */
		"Actions=setup;\n" +
		"\n[Desktop Action setup]\n" +
		"Name=Set up again\n" +
		"Exec=" + exec + " setup\n"
}

// refresh tells the desktop to look again.
//
// Both commands are optional: GNOME notices a new file by itself within a few
// seconds, and these only make it immediate. A machine without them is not an
// error, it is a machine where the icon appears a moment later.
func refresh(iconRoot, appDir string) {
	if tool, err := exec.LookPath("gtk-update-icon-cache"); err == nil {
		_ = exec.Command(tool, "-f", "-t", iconRoot).Run()
	}

	if tool, err := exec.LookPath("update-desktop-database"); err == nil {
		_ = exec.Command(tool, appDir).Run()
	}
}

/*
 * self is the path a menu entry should run.
 *
 * Inside an AppImage this is not what the program thinks it is. An AppImage
 * mounts itself under /tmp and runs from there, so asking the operating system
 * where this executable is gives a path like /tmp/.mount_PN-Scrxyz/usr/bin/pn-scripts-assistant
 * — which is gone the moment the program exits, leaving a menu entry that does
 * nothing at all when pressed. The runtime puts the real path of the file
 * somebody downloaded in APPIMAGE, and that is the one to run.
 */
func self() (string, error) {
	if at := os.Getenv("APPIMAGE"); at != "" {
		return at, nil
	}

	at, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("finding this program on disk: %w", err)
	}

	// A symlink into a directory that later gets tidied away leaves an entry
	// that does nothing, so the entry records where the file actually is.
	if resolved, err := filepath.EvalSymlinks(at); err == nil {
		at = resolved
	}

	return at, nil
}

// dataHome is where a user's own applications and icons live.
func dataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".local/share"
	}

	return filepath.Join(home, ".local", "share")
}

/*
 * RepairIfStale fixes a menu entry that points at a program that has moved.
 *
 * Only a repair, never an installation. Somebody who has never asked for a
 * menu entry does not get one because they started the program; but somebody
 * who has one is entitled to it working, and an entry that opens nothing is
 * worse than no entry at all — it is in the menu, it looks right, and pressing
 * it does nothing anybody can see.
 *
 * This is not hypothetical here: the folder holding the program was moved, and
 * from that moment the icon in the menu pointed at a path that did not exist.
 *
 * Best effort. A menu entry is a convenience, and failing to rewrite one must
 * never be a reason the brain does not start.
 */
func RepairIfStale(name string) (bool, error) {
	entry, _ := Where()
	legacy, _ := whereFor(LegacyEntryName)

	if _, err := os.Stat(entry); err != nil {
		// One made before the rename is somebody having asked for one, and it
		// is replaced under the new name.
		if _, err := os.Stat(legacy); err == nil {
			if _, err := Install(name); err != nil {
				return false, err
			}

			return true, nil
		}

		// Nobody asked for one. Not this function's business.
		return false, nil
	}

	if Installed() {
		return false, nil
	}

	if _, err := Install(name); err != nil {
		return false, err
	}

	return true, nil
}
