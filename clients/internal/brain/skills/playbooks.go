package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"
)

/*
 * Playbooks are skills that ship inside the program.
 *
 * Everything its owner has had done for him by hand — a batch of games built
 * and published on itch.io and Game Jolt, a website shipped, a plugin taken
 * through review, social profiles brought into line — written down once as
 * the procedure that worked, versioned, and carried in the binary so that a
 * new install knows it on the first day (ADR 0007).
 *
 * They are the same thing as a skill its owner writes: instructions, not
 * capability. Each is offered only when its own words appear, and reading one
 * changes nothing. A skill of the same name in the brain's own folder wins, so
 * a playbook can be corrected without waiting for a release.
 */

//go:embed playbooks/*.md
var shipped embed.FS

// MostPlaybookRunes caps one playbook. Longer than a taught skill, because a
// publishing procedure with its checks does not fit in six thousand.
const MostPlaybookRunes = 12000

// Builtin is every playbook shipped in this program, sorted by name.
func Builtin() ([]Skill, error) {
	entries, err := fs.ReadDir(shipped, "playbooks")
	if err != nil {
		return nil, fmt.Errorf("reading the playbooks: %w", err)
	}

	var out []Skill

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		raw, err := shipped.ReadFile("playbooks/" + entry.Name())
		if err != nil {
			return nil, err
		}

		skill := parse(string(raw), strings.TrimSuffix(entry.Name(), ".md"))
		skill.Builtin = true
		skill.Taught = false

		if err := CheckName(skill.Name); err != nil {
			return nil, fmt.Errorf("playbook %s: %w", entry.Name(), err)
		}

		if skill.Version == 0 {
			return nil, fmt.Errorf("playbook %s has no version", entry.Name())
		}

		if utf8.RuneCountInString(skill.How) > MostPlaybookRunes {
			skill.How = string([]rune(skill.How)[:MostPlaybookRunes])
		}

		out = append(out, skill)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

/*
 * All is what the assistant knows how to do: the playbooks it ships with and
 * what its owner taught it, his winning where both have the same name.
 */
func All(root string) ([]Skill, error) {
	own, err := Load(root)

	builtin, berr := Builtin()
	if berr != nil && err == nil {
		err = berr
	}

	mine := map[string]bool{}

	for _, s := range own {
		mine[s.Name] = true
	}

	out := append([]Skill{}, own...)

	pack, perr := Pack()
	if perr != nil && err == nil {
		err = perr
	}

	for _, s := range append(builtin, pack...) {
		if !mine[s.Name] {
			mine[s.Name] = true
			out = append(out, s)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, err
}
