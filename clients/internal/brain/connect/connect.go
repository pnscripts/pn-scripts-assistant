/*
 * Package connect writes the assistant into another program's list of MCP
 * servers, so that program can ask it and hand it things: Claude Code and
 * Claude Desktop today.
 *
 * Those programs keep the list in a settings file of their own (package
 * mcpimport reads the same files the other way). Writing into another
 * program's settings is the kind of thing a person must see before it
 * happens, so this package does it in two steps and the first changes
 * nothing: Plan says which file, what exactly would stand in it and what
 * stands there now; Write does it, only for the very plan its person was
 * shown (a file that changed since is not written), after copying the file
 * as it was beside it. Everything else in the file is kept as it is, in
 * its order.
 */
package connect

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pn-scripts-assistant/internal/brain/mcpimport"
)

// Name is the name the assistant has in another program's list.
const Name = "pn-scripts-assistant"

// Clients are the programs this can write into, in the page's order.
var Clients = []mcpimport.Client{mcpimport.ClaudeCode, mcpimport.ClaudeDesktop}

// Known reports whether c is one of them.
func Known(c mcpimport.Client) bool {
	for _, each := range Clients {
		if c == each {
			return true
		}
	}

	return false
}

// MostBytes is the largest settings file this changes: Claude Code's grows
// with every project it has seen, but never near this.
const MostBytes = mcpimport.MostBytes

// How things stand in the file now.
const (
	StateNoFile    = "nofile"    // the program has no settings file yet: it would be made
	StateNotThere  = "absent"    // the file has no entry for the assistant
	StateSame      = "same"      // the entry is there, as it would be written
	StateDifferent = "different" // an entry of that name is there and says something else
	StateUnread    = "unread"    // the file is there and is not something this can change
)

// Plan is what connecting one program would do, for its person to read
// before it is done.
type Plan struct {
	Client mcpimport.Client `json:"client"`
	Title  string           `json:"title"`

	// File is the settings file, and Entry what would stand in it under
	// "mcpServers", written out as it would be.
	File  string `json:"file"`
	Entry string `json:"entry"`

	State string `json:"state"`

	// Now is the entry of that name that stands there now, when it says
	// something else.
	Now string `json:"now,omitempty"`

	// Seen names this very plan: the file as it was read and the entry as
	// it would be written. Write takes it back and does nothing when the
	// file or the entry is no longer that.
	Seen string `json:"seen"`
}

// entry is the assistant as an MCP server in another program's list.
type entry struct {
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// entryFor is the entry for one program: Claude Code names the transport,
// Claude Desktop knows no other.
func entryFor(c mcpimport.Client, program string) entry {
	e := entry{Command: program, Args: []string{"mcp"}}

	if c == mcpimport.ClaudeCode {
		e.Type = "stdio"
	}

	return e
}

// written is an entry as it is written into the file and shown before.
func written(e entry) string {
	out, _ := json.MarshalIndent(map[string]entry{Name: e}, "", "  ")

	return string(out)
}

// fileOf is the settings file of one program on this kind of computer.
func fileOf(c mcpimport.Client, env mcpimport.Env) (string, error) {
	for _, p := range mcpimport.Places(env) {
		if p.Client == c {
			return p.File, nil
		}
	}

	return "", fmt.Errorf("where %s keeps its settings on this computer is not known", c)
}

func seen(raw []byte, exists bool, entry string) string {
	h := sha256.New()

	if exists {
		h.Write([]byte{1})
		h.Write(raw)
	} else {
		h.Write([]byte{0})
	}

	h.Write([]byte{0})
	h.Write([]byte(entry))

	return hex.EncodeToString(h.Sum(nil))
}

// member is one name of a JSON object with what stands under it, as it was
// written.
type member struct {
	key   string
	value json.RawMessage
}

// object reads one JSON object, keeping its names in their order.
func object(raw []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}

	var out []member

	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}

		key, isText := t.(string)
		if !isText {
			return nil, errors.New("not a JSON object")
		}

		var value json.RawMessage

		if err := dec.Decode(&value); err != nil {
			return nil, err
		}

		out = append(out, member{key, value})
	}

	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return nil, errors.New("not a JSON object")
	}

	// Nothing after it.
	if _, err := dec.Token(); err == nil {
		return nil, errors.New("more than one JSON value")
	}

	// A name written twice is read by one program as the first and by
	// another as the last: such a file is not one to change.
	seen := map[string]bool{}

	for _, m := range out {
		if seen[m.key] {
			return nil, fmt.Errorf("the name %q is written twice", m.key)
		}

		seen[m.key] = true
	}

	return out, nil
}

// find is the place of a name in an object, or -1.
func find(o []member, key string) int {
	for i, m := range o {
		if m.key == key {
			return i
		}
	}

	return -1
}

// put writes an object out, its names in their order, two spaces a level.
func put(o []member) ([]byte, error) {
	var b bytes.Buffer

	b.WriteString("{")

	for i, m := range o {
		if i > 0 {
			b.WriteString(",")
		}

		key, err := json.Marshal(m.key)
		if err != nil {
			return nil, err
		}

		b.WriteString("\n  ")
		b.Write(key)
		b.WriteString(": ")

		var value bytes.Buffer

		if err := json.Indent(&value, m.value, "  ", "  "); err != nil {
			return nil, err
		}

		b.Write(value.Bytes())
	}

	if len(o) > 0 {
		b.WriteString("\n")
	}

	b.WriteString("}\n")

	return b.Bytes(), nil
}

// read is the settings file as it stands: its bytes and whether it is there.
func read(file string) (raw []byte, exists bool, err error) {
	info, err := os.Lstat(file)

	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	case !info.Mode().IsRegular():
		// A link or a folder in its place is somebody's arrangement, and
		// not this program's to write through.
		return nil, true, errors.New("not an ordinary file")
	case info.Size() > MostBytes:
		return nil, true, errors.New("too large")
	}

	raw, err = os.ReadFile(file)

	return raw, true, err
}

/*
 * PlanFor is what connecting c would do on this computer, where program is
 * the full path of the assistant's own program. It reads and changes
 * nothing else.
 */
func PlanFor(c mcpimport.Client, env mcpimport.Env, program string) (Plan, error) {
	if !Known(c) {
		return Plan{}, fmt.Errorf("%q is not a program this connects to", c)
	}

	if !filepath.IsAbs(program) {
		return Plan{}, errors.New("the assistant's own program must be named by its full path")
	}

	file, err := fileOf(c, env)
	if err != nil {
		return Plan{}, err
	}

	e := entryFor(c, program)
	plan := Plan{Client: c, Title: c.Title(), File: file, Entry: written(e)}

	raw, exists, err := read(file)
	plan.Seen = seen(raw, exists, plan.Entry)

	switch {
	case err != nil:
		plan.State = StateUnread

		return plan, nil
	case !exists:
		plan.State = StateNoFile

		return plan, nil
	}

	// An empty file is a file with nothing in it yet.
	top := []member{}

	if len(bytes.TrimSpace(raw)) > 0 {
		if top, err = object(raw); err != nil {
			plan.State = StateUnread

			return plan, nil
		}
	}

	plan.State = StateNotThere

	at := find(top, "mcpServers")
	if at < 0 {
		return plan, nil
	}

	servers, err := object(top[at].value)
	if err != nil {
		plan.State = StateUnread

		return plan, nil
	}

	if i := find(servers, Name); i >= 0 {
		wanted, _ := json.Marshal(e)

		if sameEntry(servers[i].value, wanted) {
			plan.State = StateSame
		} else {
			plan.State = StateDifferent

			var shown bytes.Buffer

			if json.Indent(&shown, servers[i].value, "", "  ") == nil {
				plan.Now = cut(shown.String(), 2000)
			}
		}
	}

	return plan, nil
}

// sameEntry reports whether two entries say the same, whatever the order
// of their names and the spaces between them.
func sameEntry(a, b []byte) bool {
	var x, y any

	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}

	p, _ := json.Marshal(x)
	q, _ := json.Marshal(y)

	return bytes.Equal(p, q)
}

func cut(s string, most int) string {
	if r := []rune(s); len(r) > most {
		return string(r[:most]) + "…"
	}

	return s
}

var (
	// ErrChanged is a file, or an entry, that is no longer what its
	// person was shown: nothing is written, and they are shown again.
	ErrChanged = errors.New("the settings changed since they were shown")

	// ErrUnread is a settings file this cannot change: not JSON, not an
	// ordinary file, or too large.
	ErrUnread = errors.New("the settings file is not something this can change")

	// ErrSame is an entry that is there already, as it would be written.
	ErrSame = errors.New("it is connected already")
)

// Done is what Write did.
type Done struct {
	File string `json:"file"`

	// Backup is the copy of the file as it was, beside it; empty when
	// there was no file before.
	Backup string `json:"backup,omitempty"`
}

/*
 * Write connects c: it puts the assistant's entry into that program's
 * settings file, for the plan its person was shown (seenBefore, Plan.Seen)
 * and no other. The file as it was is copied beside it first, readable by
 * its owner alone, and the new one takes its place in one step, so the
 * other program never reads half of it.
 */
func Write(c mcpimport.Client, env mcpimport.Env, program, seenBefore string, now time.Time) (Done, error) {
	plan, err := PlanFor(c, env, program)
	if err != nil {
		return Done{}, err
	}

	if plan.Seen != seenBefore {
		return Done{}, ErrChanged
	}

	switch plan.State {
	case StateUnread:
		return Done{}, ErrUnread
	case StateSame:
		return Done{}, ErrSame
	}

	raw, exists, err := read(plan.File)
	if err != nil {
		return Done{}, ErrUnread
	}

	// Read twice (the plan, and now): what is changed is what was shown.
	if seen(raw, exists, plan.Entry) != seenBefore {
		return Done{}, ErrChanged
	}

	top := []member{}

	if exists && len(bytes.TrimSpace(raw)) > 0 {
		if top, err = object(raw); err != nil {
			return Done{}, ErrUnread
		}
	}

	servers := []member{}

	at := find(top, "mcpServers")
	if at >= 0 {
		if servers, err = object(top[at].value); err != nil {
			return Done{}, ErrUnread
		}
	}

	value, err := json.Marshal(entryFor(c, program))
	if err != nil {
		return Done{}, err
	}

	if i := find(servers, Name); i >= 0 {
		servers[i].value = value
	} else {
		servers = append(servers, member{Name, value})
	}

	inner, err := put(servers)
	if err != nil {
		return Done{}, err
	}

	if at >= 0 {
		top[at].value = bytes.TrimSpace(inner)
	} else {
		top = append(top, member{"mcpServers", bytes.TrimSpace(inner)})
	}

	out, err := put(top)
	if err != nil {
		return Done{}, err
	}

	// What is about to be written must read back as what was meant.
	if again, err := object(out); err != nil || find(again, "mcpServers") < 0 {
		return Done{}, errors.New("the new settings did not read back")
	}

	dir := filepath.Dir(plan.File)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Done{}, err
	}

	done := Done{File: plan.File}
	mode := os.FileMode(0o600)

	if exists {
		if info, err := os.Stat(plan.File); err == nil {
			mode = info.Mode().Perm()
		}

		// The file as it was, beside it. A name of its own for each time,
		// never over an older copy.
		done.Backup = plan.File + ".before-pn-scripts-assistant-" + now.UTC().Format("20060102-150405")

		backup, err := os.OpenFile(done.Backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return Done{}, fmt.Errorf("the file could not be copied first, so it was not changed: %w", err)
		}

		_, werr := backup.Write(raw)
		cerr := backup.Close()

		if werr != nil || cerr != nil {
			os.Remove(done.Backup)

			return Done{}, errors.New("the file could not be copied first, so it was not changed")
		}
	}

	// From here on, nothing written means no copy left beside the file
	// either: a copy is the way back from a change, not a trace of a try.
	nothing := func(err error) (Done, error) {
		if done.Backup != "" {
			os.Remove(done.Backup)
		}

		return Done{}, err
	}

	tmp, err := os.CreateTemp(dir, ".pn-scripts-assistant-*.json")
	if err != nil {
		return nothing(err)
	}

	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()

		return nothing(err)
	}

	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()

		return nothing(err)
	}

	if err := tmp.Close(); err != nil {
		return nothing(err)
	}

	// Once more, at the last moment: the other program may be running and
	// writing its own settings.
	if last, still, err := read(plan.File); err != nil || still != exists || !bytes.Equal(last, raw) {
		return nothing(ErrChanged)
	}

	if err := os.Rename(tmp.Name(), plan.File); err != nil {
		return nothing(err)
	}

	return done, nil
}
