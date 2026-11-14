package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

	return commandUse(a.Argv)
}

/*
 * commandUse is the program a command line runs: past env and its
 * settings; `flatpak run <id>`, `snap run <name>` and `gtk-launch <entry>`
 * name the program they start.
 */
func commandUse(argv []string) programs.Use {
	u := programs.Use{Verb: programs.VerbChange}

	for len(argv) > 0 && (filepath.Base(argv[0]) == "env") {
		argv = argv[1:]

		for len(argv) > 0 && strings.Contains(argv[0], "=") && !strings.HasPrefix(argv[0], "-") {
			argv = argv[1:]
		}
	}

	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return u
	}

	first := filepath.Base(argv[0])
	after := func(word string) string {
		for i, a := range argv[1:] {
			if a == word {
				for _, b := range argv[i+2:] {
					if !strings.HasPrefix(b, "-") {
						return b
					}
				}
			}
		}

		return ""
	}

	switch first {
	case "flatpak":
		u.Flatpak = after("run")
	case "snap":
		u.Snap = after("run")
		if s, _, ok := strings.Cut(u.Snap, "."); ok {
			u.Snap = s
		}
	case "gtk-launch":
		if len(argv) > 1 {
			u.Desktop = argv[1]
		}
	}

	if filepath.IsAbs(argv[0]) {
		u.Path = argv[0]
	} else if p, ok := lookProgram(argv[0]); ok {
		u.Path = p
	}

	u.Name = first

	return u
}

// Typing goes to the window in front.
func (TypeText) ProgramUse(ctx context.Context, _ json.RawMessage) programs.Use {
	path, flatpak := windowProgram(ctx, false, 0, 0)

	return programs.Use{Verb: programs.VerbInput, Path: path, Flatpak: flatpak}
}

// A click goes to the window under the point.
func (Click) ProgramUse(ctx context.Context, raw json.RawMessage) programs.Use {
	var a clickArgs

	_ = json.Unmarshal(raw, &a)

	path, flatpak := windowProgram(ctx, true, a.X, a.Y)

	return programs.Use{Verb: programs.VerbInput, Path: path, Flatpak: flatpak}
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
