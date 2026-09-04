//go:build linux

package machine

import (
	"fmt"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// MostProcesses is how many rows the list holds.
//
// A wall display is read at a glance from across a room. Two hundred rows is a
// log; the dozen doing the work is a picture.
const MostProcesses = 14

/*
 * heaviest lists the processes actually using the machine.
 *
 * By processor time between two readings, the way top does it, rather than by
 * the lifetime average /proc reports — which on a machine that has been up for
 * days shows whatever was busy on Tuesday and never moves.
 */
type procSample struct {
	jiffies float64
	at      time.Time
}

var lastProcs struct {
	mu   sync.Mutex
	seen map[int]procSample
}

func heaviest(memoryTotal uint64) []Process {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	lastProcs.mu.Lock()
	defer lastProcs.mu.Unlock()

	if lastProcs.seen == nil {
		lastProcs.seen = map[int]procSample{}
	}

	ticks := float64(clockTicks())
	now := time.Now()
	seen := make(map[int]procSample, len(lastProcs.seen))

	var out []Process

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		p, jiffies, ok := readProcess(pid, memoryTotal, ticks)
		if !ok {
			continue
		}

		seen[pid] = procSample{jiffies: jiffies, at: now}

		if was, had := lastProcs.seen[pid]; had {
			if gap := now.Sub(was.at).Seconds(); gap > 0 {
				p.CPUPercent = (jiffies - was.jiffies) / ticks / gap * 100
			}
		} else {
			// Nothing to compare against yet. Said as unknown rather than as
			// zero, which would sort the busiest new process to the bottom.
			p.CPUPercent = -1
		}

		out = append(out, p)
	}

	lastProcs.seen = seen

	/*
	 * Busiest first, and memory breaks the tie.
	 *
	 * On an idle machine every process is at zero and the order would be
	 * whatever the directory listing happened to be — so the list would
	 * reshuffle itself every second and be unreadable. Memory is stable.
	 */
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPUPercent != out[j].CPUPercent {
			return out[i].CPUPercent > out[j].CPUPercent
		}

		return out[i].RSSBytes > out[j].RSSBytes
	})

	if len(out) > MostProcesses {
		out = out[:MostProcesses]
	}

	return out
}

func readProcess(pid int, memoryTotal uint64, ticks float64) (Process, float64, bool) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Process{}, 0, false
	}

	/*
	 * The command is in brackets and can contain anything, spaces included, so
	 * the fields after it are found from the last bracket rather than by
	 * splitting the line. A process called "(my app) 1 2 3" is a real thing and
	 * splitting on spaces reads its name as its state.
	 */
	line := string(raw)
	open := strings.Index(line, "(")
	close := strings.LastIndex(line, ")")

	if open < 0 || close < open {
		return Process{}, 0, false
	}

	name := line[open+1 : close]
	fields := strings.Fields(line[close+1:])

	// utime and stime are the fourteenth and fifteenth fields overall, which
	// after the command are the eleventh and twelfth of what is left.
	if len(fields) < 12 {
		return Process{}, 0, false
	}

	utime, _ := strconv.ParseFloat(fields[11], 64)
	stime, _ := strconv.ParseFloat(fields[12-1], 64)

	p := Process{PID: pid, Command: name}
	p.CPUTime = asClock((utime + stime) / ticks)
	p.RSSBytes = residentBytes(pid)

	if memoryTotal > 0 {
		p.MemPercent = float64(p.RSSBytes) / float64(memoryTotal) * 100
	}

	p.User = ownerOf(pid)
	p.Command = fullCommand(pid, name)
	p.Ours = isOurs(p.Command)

	return p, utime + stime, true
}

func residentBytes(pid int) uint64 {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(raw))

	if len(fields) < 2 {
		return 0
	}

	pages, _ := strconv.ParseUint(fields[1], 10, 64)

	return pages * uint64(os.Getpagesize())
}

/*
 * fullCommand is the command line, falling back to the name from /proc/pid/stat.
 *
 * The name there is truncated to fifteen characters and stripped of its path,
 * so a list of them is full of "python3" and "node" with no way to tell which
 * is which. The command line says which.
 */
func fullCommand(pid int, fallback string) string {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(raw) == 0 {
		// A kernel thread has no command line. Shown in brackets, the way
		// every other tool shows them.
		return "[" + fallback + "]"
	}

	parts := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")

	return strings.TrimSpace(strings.Join(parts, " "))
}

var users struct {
	mu    sync.Mutex
	known map[string]string
}

// ownerOf names the user a process belongs to, remembering the answers: this
// runs for every process on the machine, once a second.
func ownerOf(pid int) string {
	info, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	if err != nil {
		return ""
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}

	uid := strconv.FormatUint(uint64(stat.Uid), 10)

	users.mu.Lock()
	defer users.mu.Unlock()

	if users.known == nil {
		users.known = map[string]string{}
	}

	if name, had := users.known[uid]; had {
		return name
	}

	name := uid

	if u, err := user.LookupId(uid); err == nil {
		name = u.Username
	}

	users.known[uid] = name

	return name
}

/*
 * isOurs marks the brain's own processes and the model server it talks to.
 *
 * In a list of everything running, the rows somebody is looking for are these
 * — and finding them by reading two hundred command lines is exactly the work
 * the list is supposed to save.
 */
func isOurs(command string) bool {
	for _, ours := range []string{"pn-brain", "ollama", "whisper", "piper", "llama-server"} {
		if strings.Contains(command, ours) {
			return true
		}
	}

	return false
}

// asClock writes processor time the way top does: hours, minutes and seconds
// rather than a count of seconds nobody converts in their head.
func asClock(seconds float64) string {
	total := int(seconds)
	h, m, s := total/3600, (total%3600)/60, total%60

	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}

	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}

	return fmt.Sprintf("%ds", s)
}
