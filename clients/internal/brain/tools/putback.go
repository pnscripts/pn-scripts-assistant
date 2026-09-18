package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/undo"
)

/*
 * Putting a file back the way it was.
 *
 * The one word in the vocabulary of interrupting that used to mean nothing.
 * The brain could be stopped mid-answer, the last exchange forgotten, a whole
 * conversation deleted — and a file it had written over was simply gone,
 * because nothing had taken a copy first.
 *
 * A copy is taken before every write now, so this has something to work with.
 * It covers the file the brain created as well as the file it overwrote: "put
 * it back" about something that did not exist an hour ago means remove it, and
 * that is the more likely of the two.
 */
type PutBack struct {
	// Root is the brain's own folder, where the previous versions are kept.
	Root string
}

func (PutBack) Name() string { return "put_it_back" }

func (PutBack) Description() string {
	return "Undo a change to a file: restore what was there before the brain wrote or " +
		"edited it, or remove a file the brain created. Use this for \"put it back\", " +
		"\"undo that\", \"revert it\", \"that was wrong, restore the old one\". Without a " +
		"path it undoes the most recent change; list first if you are not sure what that is."
}

func (PutBack) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"path":{"type":"string","description":"The file to put back. Leave it out for the most recent change."},
			"list":{"type":"boolean","description":"Only list what can be put back, changing nothing."}
		},
		"additionalProperties":false
	}`)
}

/*
 * Mutating, because it writes over the file that is there now.
 *
 * Undoing is still changing something, and a misheard "put it back" that
 * restored an hour-old version over an hour of work would be a worse mistake
 * than the one it was undoing. Listing is not — see Summarize, which says
 * which of the two is about to happen.
 */
func (PutBack) Risk() Risk { return Mutating }

func (PutBack) Summarize(raw json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
		List bool   `json:"list"`
	}

	json.Unmarshal(raw, &a)

	if a.List {
		return "List the changes that can be undone"
	}

	if a.Path != "" {
		return "Put " + a.Path + " back the way it was"
	}

	return "Undo the last change to a file"
}

func (t PutBack) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
		List bool   `json:"list"`
	}

	json.Unmarshal(raw, &a)

	root := t.Root
	if root == "" {
		root = Root
	}

	if a.List {
		return t.whatCanBeUndone(root)
	}

	change, found := undo.Latest(root, strings.TrimSpace(a.Path))

	if !found {
		if a.Path != "" {
			return "I have not changed " + a.Path + ", so there is nothing to put back.", nil
		}

		return "I have not changed any file recently, so there is nothing to put back.", nil
	}

	done, err := undo.PutBack(root, change.ID)
	if err != nil {
		return "", err
	}

	if done.New {
		return fmt.Sprintf("Removed %s — I created it %s and it was not there before.",
			done.Path, howLongSince(done.At)), nil
	}

	return fmt.Sprintf("Put %s back the way it was before I %s it %s. "+
		"What I replaced is kept too, so this is undoable in turn.",
		done.Path, done.What, howLongSince(done.At)), nil
}

func (t PutBack) whatCanBeUndone(root string) (string, error) {
	changes := undo.List(root)

	if len(changes) == 0 {
		return "I have not changed any files, so there is nothing to undo.", nil
	}

	var b strings.Builder

	b.WriteString("Changes I could put back, newest first:\n")

	for i, change := range changes {
		if i >= 12 {
			fmt.Fprintf(&b, "…and %d older ones.\n", len(changes)-i)

			break
		}

		what := change.What

		if change.New {
			what = "created (putting it back removes it)"
		}

		fmt.Fprintf(&b, "%s — %s %s\n", change.Path, what, howLongSince(change.At))
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// howLongSince is a gap in the words somebody would use, since "at
// 14:07:32.918" is not how anybody refers to something they did earlier.
func howLongSince(at time.Time) string {
	since := time.Since(at)

	switch {
	case since < 2*time.Minute:
		return "a moment ago"

	case since < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(since.Minutes()))

	case since < 48*time.Hour:
		return fmt.Sprintf("%d hours ago", int(since.Hours()))
	}

	return fmt.Sprintf("%d days ago", int(since.Hours()/24))
}

// Touches is the file it would put back. With none named it is the last
// change anywhere, which is not something work on a project may reach for.
func (PutBack) Touches(raw json.RawMessage) ([]string, []string) {
	var a struct {
		Path string `json:"path"`
	}

	json.Unmarshal(raw, &a)

	if a.Path == "" {
		return []string{"/"}, nil
	}

	return []string{a.Path}, nil
}
