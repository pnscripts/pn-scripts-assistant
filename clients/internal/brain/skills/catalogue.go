package skills

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

/*
 * The catalogue of what the assistant knows how to do, as its Skills page
 * shows it: where each skill came from, whether it is switched on, and its
 * words in Bulgarian when it has them.
 *
 * Four sources, and the first wins where two share a name: what its owner
 * wrote or taught, what was imported from another agent's skill folders,
 * the playbooks built into the program, and the built-in skill pack.
 */

// Sources of a skill.
const (
	SourceTaught   = "taught"   // written or taught by its owner
	SourceImported = "imported" // from a folder of agent skills
	SourcePlaybook = "playbook" // a playbook built into the program
	SourcePack     = "pack"     // the built-in skill pack
)

// SourceOf is where a skill came from.
func SourceOf(s Skill) string {
	switch {
	case s.Pack:
		return SourcePack
	case s.Builtin:
		return SourcePlaybook
	case s.Imported != "":
		return SourceImported
	}

	return SourceTaught
}

//go:embed all:pack
var packFS embed.FS

// packSource is the pack read: the embedded one, replaceable in tests.
var packSource fs.FS = packFS

// PackFolder is where the built-in pack is written in a brain.
var PackFolder = filepath.Join("knowledge", "pack-skills")

// Pack is every skill of the built-in pack, with its Bulgarian words when it
// has them, sorted by name. A folder whose SKILL.md cannot be read as a
// skill is left out, never guessed at.
func Pack() ([]Skill, error) {
	entries, err := fs.ReadDir(packSource, "pack")
	if err != nil {
		return nil, err
	}

	var out []Skill

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		raw, err := fs.ReadFile(packSource, path.Join("pack", e.Name(), SkillFile))
		if err != nil {
			continue
		}

		meta, body := frontMatter(string(raw))

		name := NormaliseName(firstNonEmpty(meta["name"], e.Name()))
		if CheckName(name) != nil || strings.TrimSpace(meta["description"]) == "" || strings.TrimSpace(body) == "" {
			continue
		}

		s := Skill{
			Name: name, When: oneLine(meta["description"]), How: capRunes(strings.TrimSpace(body), MostPlaybookRunes),
			Builtin: true, Pack: true, Folder: e.Name(), Version: 1,
		}

		if bg, err := fs.ReadFile(packSource, path.Join("pack", e.Name(), "SKILL.bg.md")); err == nil {
			bmeta, bbody := frontMatter(string(bg))
			s.WhenBG = oneLine(bmeta["description"])
			s.HowBG = capRunes(strings.TrimSpace(bbody), MostPlaybookRunes)
		}

		if hasExtra(e.Name()) {
			s.How += "\n\nIts references and scripts: read_file " + BrainToken + string(filepath.Separator) +
				filepath.Join(PackFolder, e.Name(), SkillFile) + " (and the files next to it)."
		}

		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

func hasExtra(folder string) bool {
	entries, err := fs.ReadDir(packSource, path.Join("pack", folder))
	if err != nil {
		return false
	}

	for _, e := range entries {
		if e.IsDir() {
			return true
		}
	}

	return false
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) > n {
		return string([]rune(s)[:n])
	}

	return s
}

/*
 * InstallPack writes the built-in pack into the brain's knowledge folder,
 * so a skill's references and scripts can be read like any file. It is the
 * program's own folder: written again whole when the pack changes, and
 * nothing of its owner's is ever in it.
 */
func InstallPack(root string) error {
	if root == "" {
		return nil
	}

	target := filepath.Join(root, PackFolder)

	return fs.WalkDir(packSource, "pack", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel := strings.TrimPrefix(strings.TrimPrefix(p, "pack"), "/")
		dst := filepath.Join(target, filepath.FromSlash(rel))

		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}

		data, err := fs.ReadFile(packSource, p)
		if err != nil {
			return err
		}

		if old, err := os.ReadFile(dst); err == nil && string(old) == string(data) {
			return nil
		}

		mode := os.FileMode(0o644)
		if strings.Contains(p, "/scripts/") {
			mode = 0o755
		}

		return os.WriteFile(dst, data, mode)
	})
}

// disabledFile keeps the names of the skills switched off, in the brain.
const disabledFile = ".disabled.json"

// Disabled is the names of the skills switched off.
func Disabled(root string) map[string]bool {
	out := map[string]bool{}

	data, err := os.ReadFile(filepath.Join(Folder(root), disabledFile))
	if err != nil {
		return out
	}

	var names []string
	if json.Unmarshal(data, &names) == nil {
		for _, n := range names {
			out[n] = true
		}
	}

	return out
}

// SetEnabled switches a skill on or off.
func SetEnabled(root, name string, on bool) error {
	if CheckName(name) != nil {
		return errors.New("that is not the name of a skill")
	}

	off := Disabled(root)

	if on {
		delete(off, name)
	} else {
		off[name] = true
	}

	names := make([]string, 0, len(off))
	for n := range off {
		names = append(names, n)
	}

	sort.Strings(names)

	if err := os.MkdirAll(Folder(root), 0o755); err != nil {
		return err
	}

	data, _ := json.MarshalIndent(names, "", "  ")

	return os.WriteFile(filepath.Join(Folder(root), disabledFile), data, 0o644)
}

// Entry is one skill as the Skills page shows it.
type Entry struct {
	Skill

	Source string `json:"source"`
	On     bool   `json:"on"`
}

// Catalogue is every skill there is, with its source and whether it is on.
func Catalogue(root string) ([]Entry, error) {
	all, err := All(root)

	off := Disabled(root)

	out := make([]Entry, 0, len(all))
	for _, s := range all {
		out = append(out, Entry{Skill: s, Source: SourceOf(s), On: !off[s.Name]})
	}

	return out, err
}
