package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MaxReadBytes caps what one read returns.
//
// A model's context is finite and a large file would fill it, pushing out the
// conversation that gives the file meaning. Truncation is reported rather than
// silent, so the model knows it saw part of something.
const MaxReadBytes = 200 << 10 // 200KB

// ReadFile reads a file from disk.
//
// Safe: reading changes nothing. The exfiltration risk that pairs reading with
// fetching is handled by GuardSensitive, not by making this need approval —
// asking permission for every read would train the user to click yes.
type ReadFile struct{}

func (ReadFile) Name() string { return "read_file" }

func (ReadFile) Description() string {
	return "Read a text file from the computer. Give an absolute path."
}

func (ReadFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the file"}
		},
		"required": ["path"]
	}`)
}

func (ReadFile) Risk() Risk { return Safe }

func (ReadFile) Summarize(raw json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
	}
	argsOf(raw, &a)

	return "Read " + a.Path
}

func (ReadFile) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := GuardSensitive(a.Path); err != nil {
		return "", err
	}

	info, err := os.Stat(a.Path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", a.Path, err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory; use list_directory", a.Path)
	}

	f, err := os.Open(a.Path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", a.Path, err)
	}
	defer f.Close()

	buf := make([]byte, MaxReadBytes)
	n, _ := f.Read(buf)
	text := string(buf[:n])

	if info.Size() > int64(n) {
		text += fmt.Sprintf("\n\n[truncated: showing %d of %d bytes]", n, info.Size())
	}

	return text, nil
}

// ListDirectory lists what is in a directory.
type ListDirectory struct{}

func (ListDirectory) Name() string { return "list_directory" }

func (ListDirectory) Description() string {
	return "List the files and folders in a directory. Give an absolute path."
}

func (ListDirectory) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the directory"}
		},
		"required": ["path"]
	}`)
}

func (ListDirectory) Risk() Risk { return Safe }

func (ListDirectory) Summarize(raw json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
	}
	argsOf(raw, &a)

	return "List " + a.Path
}

// MaxEntries caps a listing. A home directory or node_modules can hold tens of
// thousands of entries, which is context exhausted for no benefit.
const MaxEntries = 300

func (ListDirectory) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(a.Path)
	if err != nil {
		return "", fmt.Errorf("cannot list %s: %w", a.Path, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}

		return entries[i].Name() < entries[j].Name()
	})

	total := len(entries)

	if len(entries) > MaxEntries {
		entries = entries[:MaxEntries]
	}

	var b strings.Builder

	for _, e := range entries {
		if e.IsDir() {
			fmt.Fprintf(&b, "%s/\n", e.Name())

			continue
		}

		// Names are listed even when sensitive; knowing a .env exists is not
		// the same as reading it, and hiding it would make the brain describe
		// a directory that does not match what the user sees.
		size := int64(0)
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}

		fmt.Fprintf(&b, "%s  (%d bytes)\n", e.Name(), size)
	}

	if total > MaxEntries {
		fmt.Fprintf(&b, "\n[%d of %d entries shown]", MaxEntries, total)
	}

	if total == 0 {
		return "(empty directory)", nil
	}

	return b.String(), nil
}

// WriteFile writes a file.
//
// Mutating: this changes the machine, so it always stops for approval, and the
// summary shows the path and size so the person approving knows the scale of
// what they are allowing.
type WriteFile struct{}

func (WriteFile) Name() string { return "write_file" }

func (WriteFile) Description() string {
	return "Write text to a file, creating or replacing it. Requires the owner's approval."
}

func (WriteFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to write"},
			"content": {"type": "string", "description": "The full contents of the file"}
		},
		"required": ["path", "content"]
	}`)
}

func (WriteFile) Risk() Risk { return Mutating }

func (WriteFile) Summarize(raw json.RawMessage) string {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	argsOf(raw, &a)

	action := "Create"
	if _, err := os.Stat(a.Path); err == nil {
		// Overwriting is a different decision from creating, and the person
		// approving needs to know which one they are agreeing to.
		action = "OVERWRITE"
	}

	return fmt.Sprintf("%s %s (%d bytes)", action, a.Path, len(a.Content))
}

func (WriteFile) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	// Writing to a credentials path is as bad as reading one: it could replace
	// an ssh key or a .env with content the model chose.
	if err := GuardSensitive(a.Path); err != nil {
		return "", err
	}

	if !filepath.IsAbs(a.Path) {
		return "", fmt.Errorf("give an absolute path, not %q", a.Path)
	}

	if err := os.MkdirAll(filepath.Dir(a.Path), 0o755); err != nil {
		return "", fmt.Errorf("cannot create the folder for %s: %w", a.Path, err)
	}

	if err := os.WriteFile(a.Path, []byte(a.Content), 0o644); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", a.Path, err)
	}

	return fmt.Sprintf("Wrote %d bytes to %s", len(a.Content), a.Path), nil
}
