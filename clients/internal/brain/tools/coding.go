package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/*
 * What is needed to actually work on code, rather than talk about it.
 *
 * Writing a whole file is fine for a new one and wrong for an existing one: a
 * model asked to change a line has to reproduce every other line perfectly to
 * do it, and a 7B model will not. It drops a function, or reindents the file,
 * and the change it announced is not the change it made. Replacing one exact
 * piece of text cannot fail that way — either the text is there and it is
 * swapped, or it is not there and nothing happens.
 *
 * Finding things is the other half. Without a search, the only way to locate
 * anything is to read whole files into a context that cannot hold them.
 */

// MaxMatches caps what one search returns, for the same reason reads are
// capped: a search that fills the context with matches destroys the
// conversation that gave it meaning.
const MaxMatches = 60

/* ---------- editing ---------- */

// EditFile replaces an exact piece of text in a file.
type EditFile struct{}

func (EditFile) Name() string { return "edit_file" }

func (EditFile) Description() string {
	return "Change part of an existing file by replacing an exact piece of its " +
		"text. Prefer this over write_file for any file that already exists. " +
		"The old text must appear exactly once; include enough surrounding " +
		"lines to make it unique."
}

func (EditFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the file"},
			"old": {"type": "string", "description": "The exact text to replace, including indentation"},
			"new": {"type": "string", "description": "What to put in its place"}
		},
		"required": ["path", "old", "new"]
	}`)
}

func (EditFile) Risk() Risk { return Mutating }

func (EditFile) Summarize(raw json.RawMessage) string {
	var a editArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Replace %d characters with %d in %s",
		len(a.Old), len(a.New), a.Path)
}

type editArgs struct {
	Path string `json:"path"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

func (EditFile) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a editArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := GuardSensitive(a.Path); err != nil {
		return "", err
	}

	if !filepath.IsAbs(a.Path) {
		return "", fmt.Errorf("give an absolute path, not %q", a.Path)
	}

	if a.Old == "" {
		return "", fmt.Errorf("give the text to replace; to create a file use write_file")
	}

	body, err := os.ReadFile(a.Path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", a.Path, err)
	}

	/*
	 * Exactly once, or not at all.
	 *
	 * Replacing the first of several matches is how an edit silently changes
	 * the wrong line — the tool reports success, the file is wrong, and nobody
	 * finds out until much later. Saying how many were found gives the model
	 * something it can act on: include more surrounding text.
	 */
	switch n := strings.Count(string(body), a.Old); n {
	case 1:
	case 0:
		return "", fmt.Errorf("that text is not in %s; read the file and copy the "+
			"exact text, including indentation", a.Path)
	default:
		return "", fmt.Errorf("that text appears %d times in %s; include more of the "+
			"surrounding lines so it matches only the one you mean", n, a.Path)
	}

	updated := strings.Replace(string(body), a.Old, a.New, 1)

	info, err := os.Stat(a.Path)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(a.Path, []byte(updated), info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", a.Path, err)
	}

	return fmt.Sprintf("Changed %s: %d bytes became %d", a.Path, len(body), len(updated)), nil
}

/* ---------- searching ---------- */

// SearchFiles finds text across a folder.
type SearchFiles struct{}

func (SearchFiles) Name() string { return "search_files" }

func (SearchFiles) Description() string {
	return "Search for a piece of text in the files under a folder, and report " +
		"the file and line of every match. Use this to find where something is " +
		"before reading or changing it."
}

func (SearchFiles) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "The text to look for"},
			"dir": {"type": "string", "description": "Absolute path to the folder to search"},
			"extension": {"type": "string", "description": "Only files ending in this, such as .go"}
		},
		"required": ["text", "dir"]
	}`)
}

func (SearchFiles) Risk() Risk { return Safe }

func (SearchFiles) Summarize(raw json.RawMessage) string {
	var a searchArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Search for %q under %s", a.Text, a.Dir)
}

type searchArgs struct {
	Text      string `json:"text"`
	Dir       string `json:"dir"`
	Extension string `json:"extension"`
}

// skipped are folders that hold no answers and enormous numbers of files.
var skipped = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".cache": true,
	"target": true, "dist": true, "build": true, "__pycache__": true,
	".venv": true, "venv": true, ".next": true, ".idea": true,
}

func (SearchFiles) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a searchArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if a.Text == "" {
		return "", fmt.Errorf("give some text to search for")
	}

	if !filepath.IsAbs(a.Dir) {
		return "", fmt.Errorf("give an absolute path, not %q", a.Dir)
	}

	var (
		found   []string
		scanned int
		capped  bool
	)

	err := filepath.WalkDir(a.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable corner of the tree is not a reason to abandon the
			// search; it is a reason to carry on without it.
			return nil //nolint:nilerr
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			if skipped[d.Name()] || (strings.HasPrefix(d.Name(), ".") && path != a.Dir) {
				return fs.SkipDir
			}

			return nil
		}

		if len(found) >= MaxMatches {
			capped = true

			return fs.SkipAll
		}

		if a.Extension != "" && !strings.HasSuffix(d.Name(), a.Extension) {
			return nil
		}

		// Credentials are off limits to the assistant even when it is only
		// looking, and a search result quotes the line it found.
		if IsSensitive(path) {
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() > MaxReadBytes {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr
		}

		scanned++

		for i, line := range strings.Split(string(body), "\n") {
			if !strings.Contains(line, a.Text) {
				continue
			}

			found = append(found, fmt.Sprintf("%s:%d: %s", path, i+1,
				strings.TrimSpace(truncateLine(line))))

			if len(found) >= MaxMatches {
				capped = true

				break
			}
		}

		return nil
	})
	if err != nil && ctx.Err() != nil {
		return "", err
	}

	if len(found) == 0 {
		return fmt.Sprintf("No match for %q in %d files under %s", a.Text, scanned, a.Dir), nil
	}

	sort.Strings(found)

	out := strings.Join(found, "\n")

	if capped {
		out += fmt.Sprintf("\n\n[stopped at %d matches; narrow the search]", MaxMatches)
	}

	return out, nil
}

// truncateLine keeps one very long line from filling the answer by itself.
func truncateLine(line string) string {
	const most = 200

	r := []rune(line)

	if len(r) <= most {
		return line
	}

	return string(r[:most]) + "…"
}
