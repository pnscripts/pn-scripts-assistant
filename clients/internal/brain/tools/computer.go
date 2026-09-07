package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/exe"
)

/*
 * Using the computer.
 *
 * Reading the screen was already possible and acting on it was not, which is a
 * strange place to stop: the brain could describe the button and not press it.
 *
 * Everything that changes what is on screen waits for its owner — a click can
 * send a message, empty a folder or confirm a purchase, and none of that is
 * distinguishable from any other click by the thing making it. What each of
 * these shows before it runs is therefore the whole of what it will do, in the
 * terms a person can check: which window, which point, which words.
 */

// display makes an X11 tool work when the brain was started without one, which
// is what happens when it is launched from the applications menu.
func display(cmd *exec.Cmd) *exec.Cmd {
	if os.Getenv("DISPLAY") == "" {
		cmd.Env = append(os.Environ(), "DISPLAY=:0")
	}

	return cmd
}

// pointer runs one xdotool command.
func pointer(ctx context.Context, args ...string) (string, error) {
	tool, found := exe.Look("xdotool")
	if !found {
		return "", fmt.Errorf(
			"this needs xdotool, which is not installed. It is one button in " +
				"System — 'Using the computer' — or: sudo apt install xdotool")
	}

	out, err := display(exec.CommandContext(ctx, tool, args...)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}

	return strings.TrimSpace(string(out)), nil
}

/* ---------- looking ---------- */

// ListWindows reports what is open.
type ListWindows struct{}

func (ListWindows) Name() string { return "list_windows" }

func (ListWindows) Description() string {
	return "List the windows that are open, with their titles and which is in " +
		"front. Use before clicking or typing, to know what is actually on " +
		"screen rather than guessing."
}

func (ListWindows) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (ListWindows) Risk() Risk { return Safe }

func (ListWindows) Summarize(json.RawMessage) string { return "List the open windows" }

func (ListWindows) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	ids, err := pointer(ctx, "search", "--onlyvisible", "--name", ".")
	if err != nil {
		return "", err
	}

	active, _ := pointer(ctx, "getactivewindow")

	var b strings.Builder

	seen := 0

	for _, id := range strings.Fields(ids) {
		name, err := pointer(ctx, "getwindowname", id)
		if err != nil || strings.TrimSpace(name) == "" {
			continue
		}

		mark := " "
		if id == active {
			mark = "*"
		}

		geometry, _ := pointer(ctx, "getwindowgeometry", "--shell", id)

		fmt.Fprintf(&b, "%s %s — %s\n", mark, name, oneLine(geometry))

		seen++

		if seen >= 25 {
			break
		}
	}

	if seen == 0 {
		return "No windows are open.", nil
	}

	return "* is the window in front.\n" + strings.TrimRight(b.String(), "\n"), nil
}

// oneLine turns xdotool's shell output into something readable.
func oneLine(shell string) string {
	var parts []string

	for _, line := range strings.Split(shell, "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}

		switch name {
		case "WIDTH", "HEIGHT", "X", "Y":
			parts = append(parts, strings.ToLower(name)+" "+value)
		}
	}

	return strings.Join(parts, ", ")
}

/* ---------- opening ---------- */

// OpenApp starts an application.
type OpenApp struct{}

func (OpenApp) Name() string { return "open_app" }

func (OpenApp) Description() string {
	return "Open an application by name, as if it had been chosen from the " +
		"applications menu. Use the name a person would use: firefox, files, " +
		"terminal, calculator."
}

func (OpenApp) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "The application to open"}
		},
		"required": ["name"]
	}`)
}

// Mutating: starting a program is a change to what the machine is doing, and
// which program it turns out to be is not always what was meant.
func (OpenApp) Risk() Risk { return Mutating }

func (OpenApp) Summarize(raw json.RawMessage) string {
	var a struct {
		Name string `json:"name"`
	}

	_ = json.Unmarshal(raw, &a)

	return "Open " + a.Name
}

func (OpenApp) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name string `json:"name"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	name := strings.TrimSpace(a.Name)
	if name == "" {
		return "", fmt.Errorf("say which application")
	}

	/*
	 * By desktop entry first, then as a command.
	 *
	 * gtk-launch starts it the way the menu does, which gets the right
	 * environment and the right icon. A bare command works for the things that
	 * have no entry, and is tried second because "files" is a menu entry and
	 * not a program.
	 */
	if entry, ok := desktopEntryFor(name); ok {
		if tool, found := exe.Look("gtk-launch"); found {
			cmd := display(exec.Command(tool, entry))

			if err := cmd.Start(); err == nil {
				go cmd.Wait()

				return "Opened " + name, nil
			}
		}
	}

	program, found := exe.Look(strings.Fields(strings.ToLower(name))[0])
	if !found {
		return "", fmt.Errorf("there is no application called %q on this machine", name)
	}

	cmd := display(exec.Command(program))

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("could not open %s: %w", name, err)
	}

	go cmd.Wait()

	return "Opened " + name, nil
}

// desktopEntryFor finds an application's menu entry by rough name.
func desktopEntryFor(name string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}

	wanted := strings.ToLower(strings.TrimSpace(name))

	for _, dir := range []string{
		home + "/.local/share/applications",
		"/usr/share/applications",
		"/var/lib/snapd/desktop/applications",
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			file := e.Name()

			if !strings.HasSuffix(file, ".desktop") {
				continue
			}

			stem := strings.ToLower(strings.TrimSuffix(file, ".desktop"))

			if stem == wanted || strings.Contains(stem, wanted) {
				return file, true
			}
		}
	}

	return "", false
}

/* ---------- acting ---------- */

// Click presses the mouse somewhere.
type Click struct{}

func (Click) Name() string { return "click" }

func (Click) Description() string {
	return "Move the mouse to a point on screen and click. Look at the screen " +
		"first to find where the thing is."
}

func (Click) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"x": {"type": "integer", "description": "Across, in pixels from the left"},
			"y": {"type": "integer", "description": "Down, in pixels from the top"},
			"button": {"type": "string", "enum": ["left", "right", "middle"], "description": "Which button, default left"},
			"double": {"type": "boolean", "description": "Click twice"},
			"what": {"type": "string", "description": "What is being clicked, in words, so the owner can check"}
		},
		"required": ["x", "y", "what"]
	}`)
}

// Mutating, and the clearest case for it: a click can send a message, empty a
// folder or confirm a purchase, and nothing about the click itself says which.
func (Click) Risk() Risk { return Mutating }

func (Click) Summarize(raw json.RawMessage) string {
	var a clickArgs

	_ = json.Unmarshal(raw, &a)

	kind := "Click"
	if a.Double {
		kind = "Double-click"
	}

	if a.Button != "" && a.Button != "left" {
		kind = strings.Title(a.Button) + "-click" //nolint:staticcheck
	}

	return fmt.Sprintf("%s %s, at %d across and %d down", kind, a.What, a.X, a.Y)
}

type clickArgs struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Button string `json:"button"`
	Double bool   `json:"double"`
	What   string `json:"what"`
}

func (Click) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a clickArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	button := "1"

	switch a.Button {
	case "right":
		button = "3"
	case "middle":
		button = "2"
	}

	if _, err := pointer(ctx, "mousemove", strconv.Itoa(a.X), strconv.Itoa(a.Y)); err != nil {
		return "", err
	}

	args := []string{"click", button}
	if a.Double {
		args = []string{"click", "--repeat", "2", "--delay", "80", button}
	}

	if _, err := pointer(ctx, args...); err != nil {
		return "", err
	}

	return fmt.Sprintf("Clicked %s at %d, %d", a.What, a.X, a.Y), nil
}

// TypeText types into whatever has the keyboard.
type TypeText struct{}

func (TypeText) Name() string { return "type_text" }

func (TypeText) Description() string {
	return "Type text into whatever window is in front. Click where it should " +
		"go first. Use the key field for Return, Tab, Escape and the like."
}

func (TypeText) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "The words to type"},
			"key": {"type": "string", "description": "A single key instead: Return, Tab, Escape, ctrl+s"}
		}
	}`)
}

// Mutating: typing into an unknown window is how a sentence becomes a sent
// message, and Return is the difference between a draft and a decision.
func (TypeText) Risk() Risk { return Mutating }

func (TypeText) Summarize(raw json.RawMessage) string {
	var a typeArgs

	_ = json.Unmarshal(raw, &a)

	if a.Key != "" {
		return "Press " + a.Key
	}

	// The whole text, because that is what is being agreed to.
	return "Type into the window in front:\n\n" + a.Text
}

type typeArgs struct {
	Text string `json:"text"`
	Key  string `json:"key"`
}

func (TypeText) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a typeArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if a.Key != "" {
		if _, err := pointer(ctx, "key", "--clearmodifiers", a.Key); err != nil {
			return "", err
		}

		return "Pressed " + a.Key, nil
	}

	if a.Text == "" {
		return "", fmt.Errorf("give something to type, or a key to press")
	}

	// A small delay per character, because applications drop keystrokes sent
	// faster than a person could produce them.
	if _, err := pointer(ctx, "type", "--clearmodifiers", "--delay", "12", a.Text); err != nil {
		return "", err
	}

	return fmt.Sprintf("Typed %d characters", len(a.Text)), nil
}

// Scroll moves a view without changing anything in it.
type Scroll struct{}

func (Scroll) Name() string { return "scroll" }

func (Scroll) Description() string {
	return "Scroll the window under the mouse up or down, to see more of it."
}

func (Scroll) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"direction": {"type": "string", "enum": ["up", "down"]},
			"amount": {"type": "integer", "description": "How many notches, default 3"}
		},
		"required": ["direction"]
	}`)
}

/*
 * Safe, unlike clicking.
 *
 * Scrolling shows more of something and changes nothing in it. Requiring
 * approval for it would mean asking permission to look further down a page,
 * which teaches its owner to approve without reading — and that habit is what
 * makes the approvals that matter worthless.
 */
func (Scroll) Risk() Risk { return Safe }

func (Scroll) Summarize(raw json.RawMessage) string {
	var a struct {
		Direction string `json:"direction"`
	}

	_ = json.Unmarshal(raw, &a)

	return "Scroll " + a.Direction
}

func (Scroll) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Direction string `json:"direction"`
		Amount    int    `json:"amount"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	button := "5"
	if strings.EqualFold(a.Direction, "up") {
		button = "4"
	}

	amount := a.Amount
	if amount <= 0 || amount > 30 {
		amount = 3
	}

	for i := 0; i < amount; i++ {
		if _, err := pointer(ctx, "click", button); err != nil {
			return "", err
		}

		time.Sleep(40 * time.Millisecond)
	}

	return fmt.Sprintf("Scrolled %s", a.Direction), nil
}
