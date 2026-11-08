package machinecare

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

/*
 * Leftovers: processes whose job looks over.
 *
 * What gets left behind is always the same few kinds: a headless browser a
 * test started, a virtual display, a "sleep" or "timeout" that was waiting
 * for something that has gone, a "tail -f" on a log nobody reads, a little
 * web server for a page that was looked at once. Such a process is a
 * leftover when it belongs to this user, has no terminal, has been running
 * a while, and either this assistant started it or whatever started it is
 * gone (it now hangs from the init process or the user's service manager).
 *
 * Everything else is somebody's program and is never on the list: a
 * browser with a window, an editor, anything started from a terminal.
 */

// Proc is one process as the leftover check reads it.
type Proc struct {
	PID     int
	PPID    int
	UID     int
	TTY     bool // has a controlling terminal
	Age     time.Duration
	Command string // the full command line
	Parent  string // the parent's program name
}

// Leftover is one process whose job looks over, and why.
type Leftover struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Age     string `json:"age"`
	Kind    string `json:"kind"`
	Why     string `json:"why"`
	Ours    bool   `json:"ours"`
}

// MinAge is how long a process runs before it can be a leftover: a test's
// browser is not left over while the test is still going.
const MinAge = 30 * time.Minute

// The kinds, by what their command lines look like.
var kinds = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"headless browser", regexp.MustCompile(`(?i)(chrome|chromium|msedge|firefox)[^ ]*( .*)? --headless`)},
	{"virtual display", regexp.MustCompile(`^(/\S*/)?(Xvfb|Xvnc|Xephyr)( |$)`)},
	{"waiting command", regexp.MustCompile(`^(/\S*/)?(sleep|timeout) `)},
	{"log follower", regexp.MustCompile(`^(/\S*/)?(tail -[fF]|tail --follow|inotifywait|journalctl -f)`)},
	{"small web server", regexp.MustCompile(`(python3? -m http\.server|php -S |npx (serve|http-server)|vite( |$)|webpack(-dev-server)? serve)`)},
}

// Find is the leftovers among procs, for this user (uid). self is the
// assistant's own process id; its children count as its own.
func Find(procs []Proc, uid, self int) []Leftover {
	byPID := map[int]Proc{}
	for _, p := range procs {
		byPID[p.PID] = p
	}

	var out []Leftover

	for _, p := range procs {
		if p.UID != uid || p.TTY || p.Age < MinAge || p.PID == self {
			continue
		}

		kind := kindOf(p.Command)
		if kind == "" {
			continue
		}

		ours := p.PPID == self || descends(byPID, p.PID, self)
		orphan := p.PPID == 1 || p.Parent == "systemd" || p.Parent == "init"

		if !ours && !orphan {
			continue
		}

		why := "whatever started it has gone"
		if ours {
			why = "this assistant started it and its job is over"
		}

		out = append(out, Leftover{PID: p.PID, Command: shorten(p.Command), Age: roughly(p.Age), Kind: kind, Why: why, Ours: ours})
	}

	return out
}

func kindOf(command string) string {
	command = strings.TrimSpace(command)

	for _, k := range kinds {
		if k.re.MatchString(command) {
			return k.kind
		}
	}

	return ""
}

// descends is whether pid has self somewhere above it.
func descends(byPID map[int]Proc, pid, self int) bool {
	for i := 0; i < 32; i++ {
		p, ok := byPID[pid]
		if !ok || p.PPID <= 1 {
			return false
		}

		if p.PPID == self {
			return true
		}

		pid = p.PPID
	}

	return false
}

func shorten(command string) string {
	fields := strings.Fields(command)
	if len(fields) > 0 {
		fields[0] = filepath.Base(fields[0])
	}

	s := strings.Join(fields, " ")
	if r := []rune(s); len(r) > 160 {
		s = string(r[:159]) + "…"
	}

	return s
}

// roughly is a duration as a person says it: 40 min, 3 h, 2 days.
func roughly(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + " days"
	case d >= 2*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " h"
	default:
		return strconv.Itoa(int(d.Minutes())) + " min"
	}
}

// Kinds is every kind of leftover, for the page's words.
func Kinds() []string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, k.kind)
	}

	return out
}
