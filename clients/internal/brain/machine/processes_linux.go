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
	mu       sync.Mutex
	seen     map[int]procSample
	watching map[int]bool
	sweeps   int
}

/*
 * FullSweepEvery is how often every process on the machine is read.
 *
 * Between sweeps only the working set is looked at. Five seconds is the lag on
 * noticing that something long-running and previously idle has started
 * working; anything that starts up is a new process and is caught at once.
 */
const FullSweepEvery = 5

func heaviest(memoryTotal uint64) []Process {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	lastProcs.mu.Lock()
	defer lastProcs.mu.Unlock()

	if lastProcs.seen == nil {
		lastProcs.seen = map[int]procSample{}
		lastProcs.watching = map[int]bool{}
	}

	/*
	 * The ones worth reading, rather than all of them.
	 *
	 * Opening three hundred files a second to show fourteen of them is most of
	 * what a reading costs, and almost all of it is spent on processes that
	 * have used no processor time since the machine started and will use none
	 * before it stops.
	 *
	 * So between full sweeps only the working set is read: whatever was on the
	 * list last time, and anything that has appeared since. New processes are
	 * caught immediately, because noticing them is one directory listing
	 * rather than three hundred file opens — and a build or a model starting
	 * up is always a new process.
	 *
	 * What lags is the other case: something long-running and idle that
	 * suddenly gets busy. That is caught by the next full sweep, which is a
	 * few seconds away. Worth saying plainly rather than implying the list is
	 * exact to the second.
	 */
	lastProcs.sweeps++

	full := lastProcs.sweeps%FullSweepEvery == 1 || len(lastProcs.watching) == 0

	ticks := float64(clockTicks())
	now := time.Now()
	seen := make(map[int]procSample, len(lastProcs.seen))

	/*
	 * One small file each, and nothing else yet.
	 *
	 * This used to read three files per process and look up the owner of each,
	 * for every process on the machine, in order to show fourteen of them.
	 * Three hundred processes is nine hundred opens a second to throw away
	 * ninety-five per cent of the answer.
	 *
	 * So the first pass reads only /proc/<pid>/stat, which carries the
	 * processor time and the resident pages — everything the ordering depends
	 * on. What a person reads, the command line and the owner, is looked up
	 * afterwards for the few that survive the sort.
	 */
	type candidate struct {
		Process

		jiffies float64
	}

	var found []candidate

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		// Between sweeps: the ones being followed, and anything new.
		if !full && !lastProcs.watching[pid] {
			if _, known := lastProcs.seen[pid]; known {
				// Seen before, not on the list, and this is not a sweep.
				seen[pid] = lastProcs.seen[pid]

				continue
			}
		}

		p, jiffies, ok := quickLook(pid, memoryTotal, ticks)
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

		found = append(found, candidate{Process: p, jiffies: jiffies})
	}

	lastProcs.seen = seen

	/*
	 * Busiest first, and memory breaks the tie.
	 *
	 * On an idle machine every process is at zero and the order would be
	 * whatever the directory listing happened to be — so the list would
	 * reshuffle itself every second and be unreadable. Memory is stable.
	 */
	sort.Slice(found, func(i, j int) bool {
		if found[i].CPUPercent != found[j].CPUPercent {
			return found[i].CPUPercent > found[j].CPUPercent
		}

		return found[i].RSSBytes > found[j].RSSBytes
	})

	if len(found) > MostProcesses {
		found = found[:MostProcesses]
	}

	/*
	 * And these are the ones to keep following.
	 *
	 * A few more than are shown, so something climbing towards the list is
	 * already being watched by the time it arrives — otherwise a process
	 * rising steadily would appear only at the next full sweep.
	 */
	watching := make(map[int]bool, MostProcesses*2)

	for i, c := range found {
		if i >= MostProcesses*2 {
			break
		}

		watching[c.PID] = true
	}

	lastProcs.watching = watching

	// And now the expensive half, for the fourteen that are actually shown.
	out := make([]Process, 0, len(found))

	for _, c := range found {
		c.User = ownerOf(c.PID)
		c.Command = fullCommand(c.PID, c.Command)
		c.Ours = isOurs(c.Command)

		out = append(out, c.Process)
	}

	return out
}

/*
 * quickLook reads the one file the ordering depends on.
 *
 * /proc/<pid>/stat carries the command name, the processor time and the
 * resident page count — so the list can be built and sorted without opening
 * anything else. The name here is the kernel's, truncated to fifteen
 * characters; the real command line is fetched later for the few that are
 * shown.
 */
func quickLook(pid int, memoryTotal uint64, ticks float64) (Process, float64, bool) {
	line, ok := readStat(pid)
	if !ok {
		return Process{}, 0, false
	}

	/*
	 * The command is in brackets and can contain anything, spaces included, so
	 * the fields after it are found from the last bracket rather than by
	 * splitting the whole line. A process named "(my app) 1 2 3" is a real
	 * thing, and splitting on spaces reads its name as its state.
	 */
	open := indexByte(line, '(')
	closed := lastIndexByte(line, ')')

	if open < 0 || closed < open {
		return Process{}, 0, false
	}

	name := string(line[open+1 : closed])

	/*
	 * Only the three numbers that matter, picked out by counting spaces.
	 *
	 * This used to split the rest of the line into strings and parse them —
	 * about fifty allocations per process, three hundred processes, once a
	 * second. Scanning for the fields wanted is the same work without the
	 * fifteen thousand strings, and this is ninety per cent of what a reading
	 * costs.
	 *
	 * The numbering is from the proc manual. After the closing bracket the
	 * fields are the third onward, so utime (14) is the twelfth here, stime
	 * (15) the thirteenth, and rss in pages (24) the twenty-second.
	 */
	const (
		utimeAt = 11
		stimeAt = 12
		rssAt   = 21
	)

	utime, okUtime := fieldNumber(line[closed+1:], utimeAt)
	stime, okStime := fieldNumber(line[closed+1:], stimeAt)

	if !okUtime || !okStime {
		return Process{}, 0, false
	}

	p := Process{PID: pid, Command: name}
	p.CPUTime = asClock((utime + stime) / ticks)

	if pages, ok := fieldNumber(line[closed+1:], rssAt); ok {
		p.RSSBytes = uint64(pages) * uint64(os.Getpagesize())
	}

	if memoryTotal > 0 {
		p.MemPercent = float64(p.RSSBytes) / float64(memoryTotal) * 100
	}

	return p, utime + stime, true
}

/*
 * readStat reads one process's line into a buffer that gets reused.
 *
 * os.ReadFile allocates a fresh slice each time and asks the file how big it
 * is first, which for a /proc file is a lie it then has to work around. One
 * buffer and one read is the whole of what is needed: these lines are a few
 * hundred bytes and never near the size of this one.
 */
var statBuffer struct {
	mu  sync.Mutex
	buf []byte
	at  []byte
}

func readStat(pid int) ([]byte, bool) {
	statBuffer.mu.Lock()
	defer statBuffer.mu.Unlock()

	if statBuffer.buf == nil {
		statBuffer.buf = make([]byte, 4096)
		statBuffer.at = make([]byte, 0, 32)
	}

	// The path, without fmt.Sprintf allocating one per process.
	path := append(statBuffer.at[:0], "/proc/"...)
	path = strconv.AppendInt(path, int64(pid), 10)
	path = append(path, "/stat"...)

	file, err := os.Open(string(path))
	if err != nil {
		return nil, false
	}

	n, err := file.Read(statBuffer.buf)

	file.Close()

	if err != nil && n == 0 {
		return nil, false
	}

	return statBuffer.buf[:n], true
}

// fieldNumber is the nth space-separated number in a line, counting from zero.
func fieldNumber(line []byte, want int) (float64, bool) {
	field := 0
	i := 0

	for i < len(line) {
		for i < len(line) && line[i] == ' ' {
			i++
		}

		start := i

		for i < len(line) && line[i] != ' ' {
			i++
		}

		if start == i {
			break
		}

		if field == want {
			return parseUint(line[start:i])
		}

		field++
	}

	return 0, false
}

// parseUint reads a positive whole number without allocating a string for it.
func parseUint(raw []byte) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}

	var n float64

	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, false
		}

		n = n*10 + float64(c-'0')
	}

	return n, true
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}

	return -1
}

func lastIndexByte(b []byte, c byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == c {
			return i
		}
	}

	return -1
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
