package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

/*
 * Skills it learns from the agents on the same machine.
 *
 * Somebody who works with Claude Code, Codex or any other agent that keeps
 * its procedures as folders of SKILL.md has already written down how their
 * work is done — often at length, and corrected over months. Asking them to
 * teach the assistant the same thing again, in conversation, is asking them
 * to do their work twice. So the folders are read, on start and on every
 * learning pass, and each skill becomes a playbook in the brain's own folder.
 *
 * Three rules keep that from going wrong:
 *
 *   - What its owner wrote or taught wins. A playbook file without the
 *     "imported:" header is his, and is never overwritten by an import of
 *     the same name.
 *   - Only what changed is written again. A skill is refreshed when its
 *     folder's newest file is newer than the one the playbook was made from;
 *     otherwise the pass costs one walk of the folder and nothing else.
 *   - Nothing is ever deleted here. A source folder that is missing is the
 *     ordinary state of a brain carried to a machine without the agent on it,
 *     and that is exactly when the playbooks are most needed.
 */

// BrainToken stands for the brain's own folder inside a playbook's text, so
// a pointer written on one machine still leads somewhere on the next. It is
// replaced with the real folder whenever a skill is read.
const BrainToken = "{brain}"

// SkillFile is the file that makes a folder an agent skill.
const SkillFile = "SKILL.md"

// DefaultSkillFolders are where agent skills are looked for when nothing else
// is said: Claude Code's own folder in the home directory.
func DefaultSkillFolders() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}

	return []string{filepath.Join(home, ".claude", "skills")}
}

// Imported is what one import pass did.
type Imported struct {
	// Written is the playbooks made or refreshed.
	Written []string

	// Kept is the names that were left alone because its owner's own skill
	// of that name is already there.
	Kept []string

	// Unchanged counts the skills whose source had not moved on.
	Unchanged int
}

// Changed reports whether anything the live list depends on was written.
func (i Imported) Changed() bool { return len(i.Written) > 0 }

/*
 * ImportFolders turns every agent skill under the given folders into a
 * playbook in this brain.
 *
 * A folder that is not there is skipped without complaint; see the package
 * comment above for why. A relative folder is taken from the brain's root,
 * so a list written in the settings travels with the brain.
 */
func ImportFolders(root string, folders []string) (Imported, error) {
	var done Imported

	seen := map[string]bool{}

	var firstErr error

	for _, folder := range folders {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			continue
		}

		if !filepath.IsAbs(folder) {
			folder = filepath.Join(root, folder)
		}

		entries, err := os.ReadDir(folder)
		if err != nil {
			continue
		}

		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

		for _, entry := range entries {
			dir := filepath.Join(folder, entry.Name())

			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				continue
			}

			if _, err := os.Stat(filepath.Join(dir, SkillFile)); err != nil {
				continue
			}

			name, err := importOne(root, dir, seen, &done)
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("importing %s: %w", name, err)
			}
		}
	}

	return done, firstErr
}

func importOne(root, dir string, seen map[string]bool, done *Imported) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, SkillFile))
	if err != nil {
		return filepath.Base(dir), err
	}

	meta, body := frontMatter(string(raw))

	name := NormaliseName(firstNonEmpty(meta["name"], filepath.Base(dir)))

	// Two folders offering the same name: the first one listed wins, the
	// same rule the settings line is read in.
	if seen[name] {
		return name, nil
	}

	seen[name] = true

	stamp := newestIn(dir)

	if existing, err := One(root, name); err == nil && existing != nil {
		if existing.Imported == "" {
			done.Kept = append(done.Kept, name)

			return name, nil
		}

		if existing.SourceTime == stamp && existing.Imported == sourceFor(root, dir) {
			done.Unchanged++

			return name, nil
		}
	}

	pointer, err := carry(root, dir)
	if err != nil {
		return name, err
	}

	when := firstNonEmpty(meta["description"], "the "+filepath.Base(dir)+" procedure")
	if utf8.RuneCountInString(when) > 600 {
		when = string([]rune(when)[:600])
	}

	note := "\n\nThe full procedure, with its scripts and references: read_file " + pointer +
		" (and the files next to it)."

	how := strings.TrimSpace(body)
	room := MostRunes - utf8.RuneCountInString(note) - 40

	if utf8.RuneCountInString(how) > room {
		how = string([]rune(how)[:room])
		if cut := strings.LastIndex(how, "\n"); cut > room/2 {
			how = how[:cut]
		}

		how += "\n\n[…shortened; read the full file]"
	}

	skill := Skill{
		Name:       name,
		When:       strings.Join(strings.Fields(when), " "),
		How:        how + note,
		Imported:   sourceFor(root, dir),
		SourceTime: stamp,
	}

	if err := Save(root, skill); err != nil {
		return name, err
	}

	done.Written = append(done.Written, name)

	return name, nil
}

/*
 * carry is where the full skill can be read from afterwards.
 *
 * The source file itself, written with the brain's token when it lies inside
 * the brain's own folder, so the pointer survives the brain being moved.
 */
func carry(root, dir string) (string, error) {
	return sourceFor(root, dir) + string(filepath.Separator) + SkillFile, nil
}

// sourceFor is a skill folder as it is written down: relative to the brain's
// token when it is inside the brain, as it is otherwise.
func sourceFor(root, dir string) string {
	if rel, err := filepath.Rel(root, dir); err == nil && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return BrainToken + string(filepath.Separator) + rel
	}

	return dir
}

// newestIn is the modification time, in seconds, of the newest file in a
// skill's folder — so changing a reference next to SKILL.md refreshes it too.
func newestIn(dir string) int64 {
	var newest time.Time

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() && path != dir && skipped(d.Name()) {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}

		return nil
	})

	return newest.Unix()
}

func skipped(name string) bool {
	return name == "__pycache__" || name == ".git" || name == "node_modules"
}

/*
 * NormaliseName makes any folder or skill name into one the model can call.
 *
 * pn-scam-guard becomes pn_scam_guard; anything outside the narrow alphabet
 * becomes an underscore, runs of them collapse, and a name that would not
 * start with a letter is given one.
 */
func NormaliseName(raw string) string {
	var b strings.Builder

	last := '_'

	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !ok {
			r = '_'
		}

		if r == '_' && last == '_' {
			continue
		}

		b.WriteRune(r)
		last = r
	}

	name := strings.Trim(b.String(), "_")

	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = strings.Trim("skill_"+name, "_")
	}

	if len(name) > 41 {
		name = strings.TrimRight(name[:41], "_")
	}

	for len(name) < 3 {
		name += "_skill"
	}

	return name
}

/*
 * frontMatter reads the YAML header agent skills begin with.
 *
 * Only what is needed: single-line values, quoted or not, and the folded or
 * literal blocks a long description is often written as. Anything it does
 * not understand is left out rather than guessed at.
 */
func frontMatter(raw string) (map[string]string, string) {
	meta := map[string]string{}

	text := strings.ReplaceAll(raw, "\r\n", "\n")

	if !strings.HasPrefix(text, "---\n") {
		return meta, text
	}

	header, body, found := strings.Cut(text[4:], "\n---")
	if !found {
		return meta, text
	}

	body = strings.TrimLeft(strings.TrimPrefix(body, "-"), "-")
	if i := strings.Index(body, "\n"); i >= 0 {
		body = body[i+1:]
	} else {
		body = ""
	}

	lines := strings.Split(header, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		if value == "|" || value == ">" || value == "|-" || value == ">-" || value == "" {
			var parts []string

			for i+1 < len(lines) && (strings.HasPrefix(lines[i+1], " ") || strings.HasPrefix(lines[i+1], "\t")) {
				i++
				parts = append(parts, strings.TrimSpace(lines[i]))
			}

			value = strings.Join(parts, " ")
		}

		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			if unq, err := strconv.Unquote(value); err == nil && value[0] == '"' {
				value = unq
			} else {
				value = value[1 : len(value)-1]
			}
		}

		meta[key] = value
	}

	return meta, body
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}

	return ""
}
