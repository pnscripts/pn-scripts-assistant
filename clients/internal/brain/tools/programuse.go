package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/exe"
	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * Tools that use another program say which one, and how (package programs,
 * permits): the loop then weighs what the person decided on that program's
 * card before it asks or acts. A removed program is never used, a "never"
 * is refused, an "always" runs without asking.
 *
 * Saying which program is reading only: a menu entry, a program file on
 * PATH, the process behind a window. Nothing is started to find out.
 */
type ProgramBound interface {
	ProgramUse(ctx context.Context, args json.RawMessage) programs.Use
}

// ProgramUseOf is what a call does with another program, and whether it
// uses one at all.
func ProgramUseOf(ctx context.Context, t Tool, args json.RawMessage) (programs.Use, bool) {
	b, ok := t.(ProgramBound)
	if !ok {
		return programs.Use{}, false
	}

	return b.ProgramUse(ctx, args), true
}

/*
 * ActsInOtherPrograms is a tool that acts in other programs' windows
 * without saying which one (scroll): nothing to weigh against a card, but
 * stopped like the rest by "Stop using other programs".
 */
type ActsInOtherPrograms interface {
	ActsInOtherPrograms()
}

// UsesOtherPrograms is whether a tool opens, types or clicks in, scrolls,
// or runs another program: every one of them is stopped by "Stop using
// other programs". Tools that only read (list_windows, list_programs)
// are not.
func UsesOtherPrograms(t Tool) bool {
	if _, ok := t.(ProgramBound); ok {
		return true
	}

	_, ok := t.(ActsInOtherPrograms)

	return ok
}

// Scrolling moves another program's window.
func (Scroll) ActsInOtherPrograms() {}

// The places a tool finds a program; tests replace them.
var (
	findDesktopEntry = desktopEntryFor
	lookProgram      = exe.Look

	// windowProgram is the program file (and Flatpak id) of the window
	// in front, or of the window at a point when atPoint is true.
	windowProgram = windowProgramOf
)

// Opening an application starts it.
func (OpenApp) ProgramUse(_ context.Context, raw json.RawMessage) programs.Use {
	var a struct {
		Name string `json:"name"`
	}

	_ = json.Unmarshal(raw, &a)

	name := strings.TrimSpace(a.Name)
	u := programs.Use{Verb: programs.VerbStart, Name: name}

	if name == "" {
		return u
	}

	// The same order Execute tries: the menu entry, then a command.
	if entry, ok := findDesktopEntry(name); ok {
		u.Desktop = entry
		return u
	}

	if p, ok := lookProgram(strings.Fields(strings.ToLower(name))[0]); ok {
		u.Path = p
	}

	return u
}

// A command runs its program with arguments.
func (RunCommand) ProgramUse(_ context.Context, raw json.RawMessage) programs.Use {
	var a commandArgs

	_ = json.Unmarshal(raw, &a)

	return commandUse(a.Argv, a.Dir)
}

// As administrator is a command like any other: its program's card and
// "Stop using other programs" decide it before the system is asked.
func (RunAsAdmin) ProgramUse(_ context.Context, raw json.RawMessage) programs.Use {
	var a adminArgs

	_ = json.Unmarshal(raw, &a)

	return commandUse(a.Argv, "")
}

/*
 * commandUse is the program a command line runs, seen through what wraps
 * it (unwrapped): `flatpak run <id>`, `snap run <name>` and `gtk-launch
 * <entry>` name the program they start.
 *
 * A program given by its place rather than its name is said to be
 * Elsewhere unless it is the file that name runs here or lies where only
 * the system writes: a file called gimp in a downloaded folder shares a
 * name with GIMP and nothing else. A place given from the folder the
 * command runs in ("./gimp") is always elsewhere.
 */
func commandUse(argv []string, dir string) programs.Use {
	u := programs.Use{Verb: programs.VerbChange}

	argv = unwrapped(argv)

	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return u
	}

	first := filepath.Base(argv[0])

	switch first {
	case "flatpak":
		u.Flatpak = ranBy(argv, flatpakValued)
	case "snap":
		u.Snap = ranBy(argv, snapValued)
		if s, _, ok := strings.Cut(u.Snap, "."); ok {
			u.Snap = s
		}
	case "gtk-launch":
		if len(argv) > 1 {
			u.Desktop = argv[1]
		}
	}

	switch {
	case filepath.IsAbs(argv[0]):
		u.Path = filepath.Clean(argv[0])
		u.Elsewhere = elsewhere(u.Path)
	case strings.ContainsAny(argv[0], `/\`):
		u.Path = filepath.Clean(argv[0])
		if filepath.IsAbs(dir) {
			u.Path = filepath.Join(dir, argv[0])
		}

		u.Elsewhere = true
	default:
		if p, ok := lookProgram(argv[0]); ok {
			u.Path = p
		}
	}

	u.Name = first

	return u
}

// Options of `flatpak run` and `snap run` (and of flatpak itself) whose
// value is the next word: the program's name is not one of those.
var (
	flatpakValued = map[string]bool{
		"--installation": true, "--arch": true, "--command": true, "--cwd": true, "--branch": true,
		"--runtime": true, "--runtime-version": true, "--env": true, "--env-fd": true, "--filesystem": true,
		"--nofilesystem": true, "--share": true, "--unshare": true, "--socket": true, "--nosocket": true,
		"--device": true, "--nodevice": true, "--allow": true, "--disallow": true, "--persist": true,
		"--own-name": true, "--talk-name": true, "--no-talk-name": true, "--system-own-name": true,
		"--system-talk-name": true, "--system-no-talk-name": true, "--add-policy": true, "--remove-policy": true,
		"--instance-id-fd": true, "--app-path": true, "--usr-path": true, "--parent-pid": true,
		"--a11y-own-name": true, "--unset-env": true,
	}
	snapValued = map[string]bool{"--hook": true, "-r": true, "--command": true, "--experimental-gdbserver": true}
)

// ranBy is the program `<tool> ... run [options] <name>` runs: the first
// word after "run" that is neither an option nor an option's value.
func ranBy(argv []string, valued map[string]bool) string {
	ran := false

	for i := 1; i < len(argv); i++ {
		a := argv[i]

		switch {
		case !ran && a == "run":
			ran = true
		case valued[a]:
			i++
		case strings.HasPrefix(a, "-"):
		case ran:
			return a
		}
	}

	return ""
}

/*
 * unwrapped is a command line without what only wraps its program: env
 * and its settings and options, nohup, setsid, nice, ionice, timeout,
 * stdbuf, time, and a shell given one command to run (`sh -c "..."`: the
 * first command of the line). What a script or a longer shell line starts
 * further on is not followed; the guide's known limits say so.
 */
func unwrapped(argv []string) []string {
	for depth := 0; depth < 12 && len(argv) > 0; depth++ {
		rest, wrapped := insideOf(argv)
		if !wrapped {
			return argv
		}

		argv = rest
	}

	return argv
}

// insideOf is what one wrapper runs, and whether argv starts with one.
func insideOf(argv []string) ([]string, bool) {
	rest := argv[1:]

	// skip drops the options in front: every word starting with "-",
	// and the next word too for the ones in valued.
	skip := func(valued string) []string {
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
			word := rest[0]
			rest = rest[1:]

			if word == "--" {
				break
			}

			if len(word) == 2 && strings.Contains(valued, word[1:]) && len(rest) > 0 {
				rest = rest[1:]
			}
		}

		return rest
	}

	// again is env's own words once more, with a line it was told to
	// split put in front: the line may open with settings too.
	again := func(split, after []string) []string {
		return append(append([]string{"env"}, split...), after...)
	}

	switch filepath.Base(argv[0]) {
	case "env":
		for len(rest) > 0 {
			word := rest[0]

			switch {
			case word == "-S" || word == "--split-string":
				if len(rest) < 2 {
					return nil, true
				}

				return again(strings.Fields(rest[1]), rest[2:]), true
			case strings.HasPrefix(word, "--split-string="):
				return again(strings.Fields(strings.TrimPrefix(word, "--split-string=")), rest[1:]), true
			case strings.HasPrefix(word, "-S") && !strings.HasPrefix(word, "--"):
				return again(strings.Fields(word[2:]), rest[1:]), true
			case word == "--":
				return rest[1:], true
			case word == "-u" || word == "-C" || word == "--unset" || word == "--chdir":
				rest = rest[1:]
				if len(rest) > 0 {
					rest = rest[1:]
				}
			case strings.HasPrefix(word, "-"), strings.Contains(word, "="):
				rest = rest[1:]
			default:
				return rest, true
			}
		}

		return nil, true
	case "nohup", "setsid", "time":
		return skip(""), true
	case "nice":
		return skip("n"), true
	case "ionice":
		return skip("cn"), true
	case "stdbuf":
		return skip("ioe"), true
	case "timeout":
		rest = skip("sk")

		// The time allowed stands before the command.
		if len(rest) > 0 {
			rest = rest[1:]
		}

		return rest, true
	case "sh", "bash", "dash", "zsh", "ksh":
		for i, word := range rest {
			if !strings.HasPrefix(word, "-") || word == "--" {
				break
			}

			// "-c", or letters that end in it ("-lc", "-ec").
			if !strings.HasPrefix(word, "--") && strings.HasSuffix(word, "c") {
				if i+1 >= len(rest) {
					return argv, false
				}

				if inside := firstCommand(rest[i+1]); len(inside) > 0 {
					return inside, true
				}

				return argv, false
			}
		}
	}

	return argv, false
}

// firstCommand is the first command of a shell line as words: up to the
// first sign that ends it, without settings in front, "exec" and quotes
// around a word.
func firstCommand(line string) []string {
	if i := strings.IndexAny(line, ";&|\n()<>`"); i >= 0 {
		line = line[:i]
	}

	words := strings.Fields(line)

	for len(words) > 0 {
		word := strings.Trim(words[0], `'"`)

		if word != "exec" && word != "command" && !(strings.Contains(word, "=") && !strings.ContainsAny(word, `/\`)) {
			break
		}

		words = words[1:]
	}

	for i := range words {
		words[i] = strings.Trim(words[i], `'"`)
	}

	return words
}

// sameFile is whether two paths are one file, links followed. Tests
// replace it.
var sameFile = func(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}

	rb, err := filepath.EvalSymlinks(b)

	return err == nil && ra == rb
}

/*
 * elsewhere is whether a program file, given by its place, only shares
 * its name with the program of that name: it is not the file the name
 * runs here (links followed), and it does not lie where only the system
 * writes. See programs.Use.Elsewhere.
 */
func elsewhere(path string) bool {
	if path == "" || inSystemFolder(path) {
		return false
	}

	if p, ok := lookProgram(filepath.Base(path)); ok && (p == path || sameFile(p, path)) {
		return false
	}

	return true
}

// inSystemFolder is whether a file lies where programs are installed for
// the whole machine, which takes the administrator to write.
func inSystemFolder(path string) bool {
	if runtime.GOOS == "windows" {
		p := strings.ToLower(filepath.Clean(path))

		for _, name := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "SystemRoot"} {
			if dir := strings.ToLower(filepath.Clean(os.Getenv(name))); len(dir) > 3 && strings.HasPrefix(p, dir+`\`) {
				return true
			}
		}

		return false
	}

	p := filepath.ToSlash(filepath.Clean(path))

	for _, dir := range []string{"/usr/", "/bin/", "/sbin/", "/opt/", "/snap/", "/var/lib/flatpak/", "/var/lib/snapd/", "/nix/store/"} {
		if strings.HasPrefix(p, dir) {
			return true
		}
	}

	return false
}

// Typing goes to the window in front.
func (TypeText) ProgramUse(ctx context.Context, _ json.RawMessage) programs.Use {
	path, flatpak := windowProgram(ctx, false, 0, 0)

	return programs.Use{Verb: programs.VerbInput, Path: path, Flatpak: flatpak, Elsewhere: elsewhere(path)}
}

// A click goes to the window under the point.
func (Click) ProgramUse(ctx context.Context, raw json.RawMessage) programs.Use {
	var a clickArgs

	_ = json.Unmarshal(raw, &a)

	path, flatpak := windowProgram(ctx, true, a.X, a.Y)

	return programs.Use{Verb: programs.VerbInput, Path: path, Flatpak: flatpak, Elsewhere: elsewhere(path)}
}

/*
 * windowProgramOf asks X11 (the computer tools' only display today) which
 * process owns the window, then reads that process's program file and,
 * for a Flatpak, its application id. Empty when it cannot tell: the loop
 * then asks, since typing into an unknown program is never allowed by a
 * general setting.
 */
func windowProgramOf(ctx context.Context, atPoint bool, x, y int) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	id := ""

	if atPoint {
		id = windowAt(ctx, x, y)
	} else if out, err := pointer(ctx, "getactivewindow"); err == nil {
		id = strings.TrimSpace(out)
	} else if out, err := pointer(ctx, "getwindowfocus", "-f"); err == nil {
		// A window manager that does not say which window is active still
		// knows which has the keyboard.
		id = strings.TrimSpace(out)
	}

	if id == "" {
		return "", ""
	}

	out, err := pointer(ctx, "getwindowpid", id)
	if err != nil {
		return "", ""
	}

	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || pid <= 0 {
		return "", ""
	}

	return processProgram("/proc", pid)
}

// processProgram is a process's program file and Flatpak id, read from
// proc (a folder in tests).
func processProgram(proc string, pid int) (string, string) {
	dir := filepath.Join(proc, strconv.Itoa(pid))

	path, _ := os.Readlink(filepath.Join(dir, "exe"))
	path = strings.TrimSuffix(path, " (deleted)")

	return path, flatpakInfo(filepath.Join(dir, "root", ".flatpak-info"))
}

// flatpakInfo is the application id in a sandbox's .flatpak-info.
func flatpakInfo(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}

	defer f.Close()

	in := bufio.NewScanner(f)
	group := ""

	for in.Scan() {
		line := strings.TrimSpace(in.Text())

		if strings.HasPrefix(line, "[") {
			group = line
			continue
		}

		if group == "[Application]" {
			if v, ok := strings.CutPrefix(line, "name="); ok {
				return strings.TrimSpace(v)
			}
		}
	}

	return ""
}

/*
 * windowAt is the topmost visible window containing a point: the window
 * manager's stacking list (bottom to top), the visible windows, and each
 * one's place, from the top down.
 */
func windowAt(ctx context.Context, x, y int) string {
	stack, err := xprop(ctx, "-root", "_NET_CLIENT_LIST_STACKING")
	if err != nil {
		return ""
	}

	visible := map[int64]bool{}

	if out, err := pointer(ctx, "search", "--onlyvisible", "--name", "."); err == nil {
		for _, w := range strings.Fields(out) {
			if n, err := strconv.ParseInt(w, 10, 64); err == nil {
				visible[n] = true
			}
		}
	}

	ids := stackingIDs(stack)

	for i := len(ids) - 1; i >= 0; i-- {
		if !visible[ids[i]] {
			continue
		}

		id := strconv.FormatInt(ids[i], 10)

		geo, err := pointer(ctx, "getwindowgeometry", "--shell", id)
		if err != nil {
			continue
		}

		if geometryHolds(geo, x, y) {
			return id
		}
	}

	return ""
}

// stackingIDs reads `xprop -root _NET_CLIENT_LIST_STACKING`:
// "_NET_CLIENT_LIST_STACKING(WINDOW): window id # 0x1e00003, 0x2a00007".
func stackingIDs(out string) []int64 {
	_, list, ok := strings.Cut(out, "#")
	if !ok {
		return nil
	}

	var ids []int64

	for _, w := range strings.Split(list, ",") {
		w = strings.TrimSpace(w)

		if n, err := strconv.ParseInt(strings.TrimPrefix(w, "0x"), 16, 64); err == nil && strings.HasPrefix(w, "0x") {
			ids = append(ids, n)
		}
	}

	return ids
}

// geometryHolds is whether xdotool's --shell geometry holds the point.
func geometryHolds(shell string, x, y int) bool {
	v := map[string]int{}

	for _, line := range strings.Split(shell, "\n") {
		k, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}

		if n, err := strconv.Atoi(val); err == nil {
			v[k] = n
		}
	}

	w, h := v["WIDTH"], v["HEIGHT"]

	return w > 0 && h > 0 && x >= v["X"] && x < v["X"]+w && y >= v["Y"] && y < v["Y"]+h
}

func xprop(ctx context.Context, args ...string) (string, error) {
	tool, found := exe.Look("xprop")
	if !found {
		return "", os.ErrNotExist
	}

	out, err := display(exec.CommandContext(ctx, tool, args...)).Output()

	return string(out), err
}
