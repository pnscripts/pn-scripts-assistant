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

	return pinPrefix + hex.EncodeToString(h.Sum(nil)[:12])
}

/*
 * whatItStarts is what a menu entry says it starts: for a .desktop file
 * the Exec line of its [Desktop Entry] group (and whether it is started
 * over D-Bus), not its names or its other actions; for anything else (a
 * Start menu shortcut) the whole file. line is the Exec line by itself,
 * empty for what is not a .desktop file.
 */
func whatItStarts(entry string) (starts, line string, ok bool) {
	f, err := os.Open(entry)
	if err != nil {
		return "", "", false
	}

	defer f.Close()

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
