package programs

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

/*
 * A menu entry has no version, so its "always" has nothing to sleep on
 * after an update. Pin gives it something to follow instead: a
 * fingerprint of what the entry starts and of the files that really are.
 *
 * In it: the entry's own Exec line, whole (a line rewritten to start
 * something else is another program); the file its first word runs here,
 * found the way a command is found and with links followed (a same-named
 * file put earlier where programs are looked for is another file); and
 * every file the line names by its whole place (the script an interpreter
 * is given). Of each file: where it really is, its size, when it was last
 * written and when it last changed. The file's contents are not read: a
 * program can be hundreds of megabytes, and this is worked out each time
 * the "always" is about to be acted on.
 *
 * Three choices were weighed (brain profile, 16.11.2026): writing the
 * fingerprint into the inventory as the entry's version (every program
 * would be reported as updated, and the page would show a digest as a
 * version); keeping it in the inventory beside the version (stale between
 * looks: the inventory is told when a menu entry changes, not when the
 * file it starts does); and working it out at the moment of acting, which
 * is this one.
 */

// pinPrefix marks a permit's version as a fingerprint, not a version.
const pinPrefix = "pin:"

// IsPin is whether a permit's version is a menu entry's fingerprint.
func IsPin(version string) bool { return strings.HasPrefix(version, pinPrefix) }

// mostEntry bounds a menu entry read for its fingerprint.
const mostEntry = 1 << 20

/*
 * Pin is the fingerprint of a menu entry as it is now, or empty when it
 * has none: another kind of program, one with a version of its own, or an
 * entry that cannot be read (then nothing can be followed, and no
 * "always" answers for it). look finds a bare name the way a command is
 * found.
 */
func Pin(r Record, look func(names ...string) (string, bool)) string {
	if r.Kind != App || r.Version != "" || r.Path == "" {
		return ""
	}

	starts, line, ok := whatItStarts(r.Path)
	if !ok {
		return ""
	}

	h := sha256.New()

	fmt.Fprintf(h, "starts %d:%s\n", len(starts), starts)

	// The file its first word runs here.
	target := r.Exec

	if target != "" && !namedInFull(target) && look != nil {
		if found, ok := look(target); ok {
			target = found
		}
	}

	fmt.Fprintf(h, "runs %s\n", fileState(target))

	// Every other file the line names by its whole place.
	for _, word := range splitExec(line) {
		if word != r.Exec && namedInFull(word) {
			fmt.Fprintf(h, "names %s\n", fileState(word))
		}
	}

	// What stands behind it that its own file does not show.
	for _, b := range behind(r, line, strings.HasSuffix(starts, "\x00true"), target, look, pinScanner()) {
		fmt.Fprintf(h, "%s\n", b)
	}

	return pinPrefix + hex.EncodeToString(h.Sum(nil)[:12])
}

/*
 * behind is what really runs when a menu entry is started, where its own
 * Exec line and the file its first word names do not show it (found by
 * the read after 1.2.0, ASSISTANT-PROGRAM-USE-NOTES-1121):
 *
 *   - a snap's entry runs /snap/bin/<name>, a link to /usr/bin/snap that
 *     a refresh never changes: the revision behind /snap/<name>/current
 *     is followed;
 *   - a Flatpak's entry runs /usr/bin/flatpak: the deployed commit behind
 *     <installation>/app/<id>/current/active is followed, for the system
 *     and the user's own installation;
 *   - an entry started over D-Bus runs what its service file says: the
 *     first service file of that name (the user's folder first), its
 *     words and the file its Exec line runs;
 *   - a wrapper script runs another program: every file the script names
 *     by its whole place (not its #! line) is followed, up to mostNamed;
 *   - a same-id entry put earlier in the menu folders (the user's own)
 *     starts instead of this one from the moment it is there: that file
 *     makes the fingerprint another one, so the "always" sleeps before
 *     the inventory has looked again.
 *
 * Each comes as a line for the fingerprint; what is not there is said as
 * missing, which is a state like any other.
 */
func behind(r Record, line string, dbus bool, target string, look func(names ...string) (string, bool), s Scanner) []string {
	var out []string

	desktop := strings.HasSuffix(r.ID, ".desktop")

	if desktop {
		if other := maskedBy(r, s); other != "" {
			out = append(out, "masked "+fileState(other))
		}
	}

	if name := snapOfEntry(r.Exec, line); name != "" && desktop {
		out = append(out, "snap "+fileState(s.at(filepath.Join("/snap", name, "current"))))
	}

	if id := flatpakOfEntry(r.Exec, line); id != "" && desktop {
		for _, base := range s.flatpakBases() {
			out = append(out, "flatpak "+fileState(filepath.Join(base, "app", id, "current", "active")))
		}
	}

	if dbus && desktop {
		out = append(out, "dbus "+dbusService(strings.TrimSuffix(r.ID, ".desktop"), look, s))
	}

	for _, named := range namedByScript(target) {
		out = append(out, "script names "+fileState(named))
	}

	return out
}

/*
 * maskedBy is the file that starts for this entry's desktop id when it is
 * not the entry's own: a file of the same id in a menu folder searched
 * before the entry's own folder. Only ids of files directly in a folder
 * are looked for (an id read from a subfolder, kde4-x.desktop for
 * kde4/x.desktop, is looked for as the file kde4-x.desktop).
 */
func maskedBy(r Record, s Scanner) string {
	own := filepath.Clean(r.Path)

	for _, dir := range s.appDirs() {
		if own == dir || strings.HasPrefix(own, dir+string(filepath.Separator)) {
			return ""
		}

		if c := filepath.Join(dir, r.ID); c != own && isFile(c) {
			return c
		}
	}

	return ""
}

// snapOfEntry is the snap a menu entry starts: /snap/bin/<name> (or
// <name>.<app>), or `snap run <name>`.
func snapOfEntry(exec, line string) string {
	if s := snapOfPath(exec); s != "" {
		return s
	}

	if filepath.Base(exec) != "snap" {
		return ""
	}

	name := runWord(line)
	name, _, _ = strings.Cut(name, ".")

	return name
}

// flatpakOfEntry is the application a Flatpak's menu entry runs:
// `flatpak run [options] <id>`.
func flatpakOfEntry(exec, line string) string {
	if filepath.Base(exec) != "flatpak" {
		return ""
	}

	id := runWord(line)
	if id == "" || strings.ContainsAny(id, `/\`) || strings.HasPrefix(id, ".") {
		return ""
	}

	return id
}

/*
 * runWord is the first word after "run" in an Exec line that is neither
 * an option nor a field code: menu entries of snaps and Flatpaks write
 * every option's value after "=", so no option takes the next word.
 */
func runWord(line string) string {
	ran := false

	for _, w := range splitExec(line) {
		switch {
		case !ran && w == "run":
			ran = true
		case !ran, strings.HasPrefix(w, "-"), strings.HasPrefix(w, "%"), strings.HasPrefix(w, "@@"):
		default:
			return w
		}
	}

	return ""
}

/*
 * dbusService is a D-Bus activated entry's service as a fingerprint sees
 * it: the first service file of its name in the D-Bus folders (the
 * user's first), a digest of its words, and the file its Exec line runs.
 */
func dbusService(name string, look func(names ...string) (string, bool), s Scanner) string {
	var bases []string
	if s.DataHome != "" {
		bases = append(bases, s.DataHome)
	}

	bases = append(bases, s.DataDirs...)

	for _, b := range bases {
		if b == "" || !filepath.IsAbs(b) {
			continue
		}

		path := s.at(filepath.Join(filepath.Clean(b), "dbus-1", "services", name+".service"))

		raw, err := readSmall(path)
		if err != nil {
			continue
		}

		sum := sha256.Sum256(raw)

		runs := ""

		for _, l := range strings.Split(string(raw), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "Exec="); ok {
				runs = programOf(unescape(strings.TrimSpace(v)))

				break
			}
		}

		if runs != "" && !namedInFull(runs) && look != nil {
			if found, ok := look(runs); ok {
				runs = found
			}
		}

		return fmt.Sprintf("%d:%s %s runs %s", len(path), path, hex.EncodeToString(sum[:12]), fileState(runs))
	}

	return "none"
}

// mostScript bounds a wrapper script read for the files it names, and
// mostNamed how many of them are followed.
const (
	mostScript = 64 << 10
	mostNamed  = 16
)

/*
 * namedByScript is every program file (regular, executable by someone) a
 * script names by its whole place, in the order they come, each once, at
 * most mostNamed: the program a wrapper script runs is one of them. Not
 * its #! line (the interpreter's updates are not this program's) and not
 * the script itself. A file that is not a script (it does not begin with
 * "#!"), or is larger than mostScript, names nothing. A place the script
 * puts together while it runs ($HERE/bin/x) is not seen.
 */
func namedByScript(path string) []string {
	if path == "" || !namedInFull(path) {
		return nil
	}

	f, err := openToRead(path)
	if err != nil {
		return nil
	}

	defer f.Close()

	st, err := f.Stat()
	if err != nil || !plainFile(st.Mode()) || st.Size() > mostScript {
		return nil
	}

	raw, err := io.ReadAll(io.LimitReader(f, mostScript+1))
	if err != nil || len(raw) > mostScript || !bytes.HasPrefix(raw, []byte("#!")) {
		return nil
	}

	body := raw
	if i := bytes.IndexByte(raw, '\n'); i >= 0 {
		body = raw[i+1:]
	} else {
		body = nil
	}

	self := filepath.Clean(path)
	seen := map[string]bool{}

	var out []string

	for _, m := range placeInScript.FindAll(body, -1) {
		p := filepath.Clean(string(m))

		if seen[p] || p == self {
			continue
		}

		seen[p] = true

		// A program: a regular file someone may run. Not a log, a pid
		// file or a setting the script writes to, which change with
		// every start and would make the "always" sleep each time.
		if st, err := os.Stat(p); err != nil || !plainFile(st.Mode()) || st.Mode().Perm()&0o111 == 0 {
			continue
		}

		out = append(out, p)

		if len(out) == mostNamed {
			break
		}
	}

	return out
}

// placeInScript is a place written from the root in a script.
var placeInScript = regexp.MustCompile(`/[A-Za-z0-9._+@-]+(?:/[A-Za-z0-9._+@-]+)*`)

/*
 * whatItStarts is what a menu entry says it starts: for a .desktop file
 * the Exec line of its [Desktop Entry] group (and whether it is started
 * over D-Bus), not its names or its other actions; for anything else (a
 * Start menu shortcut) the whole file. line is the Exec line by itself,
 * empty for what is not a .desktop file.
 */
func whatItStarts(entry string) (starts, line string, ok bool) {
	f, err := openToRead(entry)
	if err != nil {
		return "", "", false
	}

	defer f.Close()

	if st, err := f.Stat(); err != nil || !plainFile(st.Mode()) {
		return "", "", false
	}

	raw, err := io.ReadAll(io.LimitReader(f, mostEntry+1))
	if err != nil || len(raw) > mostEntry {
		return "", "", false
	}

	if !strings.HasSuffix(strings.ToLower(entry), ".desktop") {
		sum := sha256.Sum256(raw)

		return hex.EncodeToString(sum[:]), "", true
	}

	var (
		exec, dbus string
		inGroup    bool
	)

	in := bufio.NewScanner(bytes.NewReader(raw))
	in.Buffer(make([]byte, 0, 4096), mostEntry)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())

		switch {
		case strings.HasPrefix(line, "["):
			inGroup = line == "[Desktop Entry]"
		case !inGroup:
		default:
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}

			switch strings.TrimSpace(key) {
			case "Exec":
				exec = unescape(strings.TrimSpace(value))
			case "DBusActivatable":
				dbus = strings.TrimSpace(value)
			}
		}
	}

	return exec + "\x00" + dbus, exec, true
}

/*
 * fileState is a file as a fingerprint sees it: where it really is (links
 * followed), its size, when it was written and when it last changed; or
 * that it is not there.
 */
func fileState(path string) string {
	if path == "" {
		return "nothing"
	}

	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "missing " + path
	}

	info, err := os.Stat(real)
	if err != nil {
		return "missing " + path
	}

	return fmt.Sprintf("%d:%s %d %d %d %v", len(real), real, info.Size(), info.ModTime().UnixNano(), changedAt(info), info.IsDir())
}

/*
 * Follows is what a program's "always" is given to and compared with: its
 * version, or for a menu entry, which has none, its fingerprint (Pin, set
 * by whoever read the record; empty when nobody did or it has none).
 */
func (r Record) Follows() string {
	if r.followsItsPin() {
		return r.Pin
	}

	return r.Version
}

func (r Record) followsItsPin() bool { return r.Kind == App && r.Version == "" }

/*
 * AsleepOn is whether an "always" does not answer for the program as it
 * is now: given to another version, or, for a menu entry, to another
 * fingerprint. A menu entry with nothing to follow (never looked at, or
 * unreadable) has every "always" asleep: what cannot be checked is asked.
 */
func (p Permit) AsleepOn(r Record) bool {
	if p.Answer != Always || p.Verb == VerbRead {
		return false
	}

	if r.followsItsPin() {
		return r.Pin == "" || p.Version != r.Pin
	}

	return p.Asleep(r.Version)
}

/*
 * Pinned is records with the fingerprint of every menu entry that has an
 * "always" (the only ones it is compared for), in a copy; the others are
 * left without one, which costs nothing.
 */
func Pinned(records []Record, permits map[int64][]Permit, look func(names ...string) (string, bool)) []Record {
	out := records

	copied := false

	for i, r := range records {
		if !r.followsItsPin() || !r.Present {
			continue
		}

		for _, p := range permits[r.RowID] {
			if p.Answer != Always {
				continue
			}

			if !copied {
				out = append([]Record(nil), records...)
				copied = true
			}

			out[i].Pin = Pin(r, look)

			break
		}
	}

	return out
}

// pinScanner is the machine a fingerprint looks at; tests replace it.
var pinScanner = FromEnvironment

/*
 * plainFile is a file that holds bytes to read: not a folder, a pipe, a
 * device or a socket. Not IsRegular: on Windows a file held as a cloud
 * placeholder (OneDrive) reports itself irregular and is still a file.
 */
func plainFile(m os.FileMode) bool {
	return !m.IsDir() && m&(os.ModeNamedPipe|os.ModeDevice|os.ModeCharDevice|os.ModeSocket) == 0
}
