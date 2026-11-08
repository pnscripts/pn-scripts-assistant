//go:build linux

package machinecare

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// clockTicks is the kernel's USER_HZ, which is 100 on every Linux this
// program builds for.
const clockTicks = 100

// Processes is every process this user can see, as the leftover check reads
// them.
func Processes() ([]Proc, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	uptime := readUptime()

	var out []Proc

	names := map[int]string{}

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		p, comm, ok := readProc(pid, uptime)
		if !ok {
			continue
		}

		names[pid] = comm
		out = append(out, p)
	}

	for i := range out {
		out[i].Parent = names[out[i].PPID]
	}

	return out, nil
}

func readUptime() float64 {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}

	f, _ := strconv.ParseFloat(strings.Fields(string(raw))[0], 64)

	return f
}

// readProc reads one process: its parent, terminal and start from stat, its
// owner from the file's own owner, its command line.
func readProc(pid int, uptime float64) (Proc, string, bool) {
	dir := "/proc/" + strconv.Itoa(pid)

	stat, err := os.ReadFile(dir + "/stat")
	if err != nil {
		return Proc{}, "", false
	}

	// "pid (comm) state ppid pgrp session tty_nr …": comm may hold spaces
	// and brackets, so the fields are read after its last ')'.
	open, closing := bytes.IndexByte(stat, '('), bytes.LastIndexByte(stat, ')')
	if open < 0 || closing < open {
		return Proc{}, "", false
	}

	comm := string(stat[open+1 : closing])
	fields := strings.Fields(string(stat[closing+1:]))

	if len(fields) < 20 {
		return Proc{}, "", false
	}

	ppid, _ := strconv.Atoi(fields[1])
	tty, _ := strconv.Atoi(fields[4])
	start, _ := strconv.ParseFloat(fields[19], 64)

	info, err := os.Stat(dir)
	if err != nil {
		return Proc{}, "", false
	}

	uid := -1
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		uid = int(st.Uid)
	}

	cmdline, _ := os.ReadFile(dir + "/cmdline")
	command := strings.TrimSpace(string(bytes.ReplaceAll(cmdline, []byte{0}, []byte{' '})))

	if command == "" {
		command = comm
	}

	age := time.Duration((uptime - start/clockTicks) * float64(time.Second))

	return Proc{PID: pid, PPID: ppid, UID: uid, TTY: tty != 0, Age: age, Command: command}, comm, true
}

// ErrNotALeftover is a process that is not, or no longer, on the list.
var ErrNotALeftover = errors.New("that process is not one of the leftovers; nothing else is stopped from here")

/*
 * Stop ends one leftover: asked to stop (SIGTERM), and only if it has not
 * gone within five seconds, made to (SIGKILL). Checked against a fresh list
 * first, so a process id that has since been reused by something else is
 * never touched.
 */
func Stop(pid int, command string) error {
	procs, err := Processes()
	if err != nil {
		return err
	}

	found := false

	for _, l := range Find(procs, os.Getuid(), os.Getpid()) {
		if l.PID == pid && l.Command == command {
			found = true
		}
	}

	if !found {
		return ErrNotALeftover
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}

	for i := 0; i < 50; i++ {
		if syscall.Kill(pid, 0) != nil {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return syscall.Kill(pid, syscall.SIGKILL)
}
