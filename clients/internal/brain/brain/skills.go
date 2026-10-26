package brain

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"pn-scripts-assistant/internal/brain/skills"
)

/*
 * What its owner has taught it, kept in step with the folder.
 *
 * The registry is the live list and the folder is the record, and the folder
 * wins: these are hand-editable files by design, so somebody correcting a
 * skill in an editor must not have to restart the program to see it take
 * effect. Reloading makes the registry match what is on disk, which includes
 * removing the ones that are no longer there.
 */
type taught struct {
	mu    sync.Mutex
	names map[string]bool
}

// ReloadSkills makes the live tool list match the skills folder, and says how
// many there are.
func (b *Brain) ReloadSkills() (int, error) {
	// The playbooks it ships with as well as what it was taught (ADR 0007);
	// a folder that cannot be read still leaves the playbooks.
	found, err := skills.All(b.Root)
	if err != nil && len(found) == 0 {
		return 0, err
	}

	b.taught.mu.Lock()
	defer b.taught.mu.Unlock()

	if b.taught.names == nil {
		b.taught.names = map[string]bool{}
	}

	now := map[string]bool{}

	// A skill its owner switched off is not offered at all.
	off := skills.Disabled(b.Root)

	for _, skill := range found {
		if off[skill.Name] {
			continue
		}

		if err := b.Agent.Registry.Register(skills.AsTool(skill)); err != nil {
			/*
			 * Refused rather than allowed to shadow something built in.
			 *
			 * A skill called run_command would not be a skill; it would be a
			 * redefinition of what running a command means, and the approval
			 * gate is written against tool names — so what somebody read in a
			 * summary and what actually happened could differ.
			 */
			b.Log.Warn("a skill could not be added", "skill", skill.Name, "why", err)

			continue
		}

		now[skill.Name] = true
	}

	// Gone from the folder means gone from the list. Deleting a skill and
	// finding the assistant still offering it until the next restart is the
	// kind of thing that teaches somebody the delete button does not work.
	for name := range b.taught.names {
		if !now[name] {
			b.Agent.Registry.Unregister(name)
		}
	}

	b.taught.names = now

	return len(now), nil
}

// Skills is what it has been taught, for the interface.
func (b *Brain) Skills() ([]skills.Skill, error) { return skills.Load(b.Root) }

// SaveSkill writes one and makes it live immediately.
func (b *Brain) SaveSkill(skill skills.Skill) error {
	if _, taken := b.Agent.Registry.Get(skill.Name); taken {
		if existing, err := skills.One(b.Root, skill.Name); err != nil || existing == nil {
			return fmt.Errorf("%q is already the name of something built in", skill.Name)
		}
	}

	if err := skills.Save(b.Root, skill); err != nil {
		return err
	}

	_, err := b.ReloadSkills()

	return err
}

// ForgetSkill removes one and stops offering it at once.
func (b *Brain) ForgetSkill(name string) error {
	if err := skills.Remove(b.Root, name); err != nil {
		return err
	}

	_, err := b.ReloadSkills()

	return err
}

/*
 * teachingOf is the brain, seen by the tools that write skills.
 *
 * An interface rather than the brain itself, because a tool that imported the
 * brain would be a tool the brain could not hold.
 */
type teachingOf struct{ b *Brain }

func (t teachingOf) SaveSkill(name, when, how string) error {
	return t.b.SaveSkill(skills.Skill{Name: name, When: when, How: how, Taught: true})
}

func (t teachingOf) ForgetSkill(name string) error { return t.b.ForgetSkill(name) }

func (t teachingOf) SkillNames() []string {
	found, err := t.b.Skills()
	if err != nil {
		return nil
	}

	out := make([]string, 0, len(found))

	for _, skill := range found {
		out = append(out, skill.Name)
	}

	return out
}

// MySkillsFolder is the folder of agent skills inside the brain, read like
// any other: a skill copied there is learnt on the next reload.
const MySkillsFolder = "my-skills"

// SkillFolders are where skills are read from, in the order read: the
// default agent folders, this brain's own folder of skills, and the ones its
// owner added in the settings.
func (b *Brain) SkillFolders() []string {
	out := append([]string{}, skills.DefaultSkillFolders()...)
	out = append(out, filepath.Join(b.Root, MySkillsFolder))

	for _, f := range filepath.SplitList(b.Cfg.SkillFolders) {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}

	return out
}

/*
 * RefreshSkills reads every skill folder again and makes the live list
 * match: what the Skills page's Reload does, and what start and every
 * learning pass do, so a skill added or changed anywhere is picked up.
 */
func (b *Brain) RefreshSkills() (int, error) {
	b.importSkills()

	return b.ReloadSkills()
}

// SetSkillEnabled switches a skill on or off and makes the list match.
func (b *Brain) SetSkillEnabled(name string, on bool) error {
	if err := skills.SetEnabled(b.Root, name, on); err != nil {
		return err
	}

	_, err := b.ReloadSkills()

	return err
}

// SetSkillFolders keeps the owner's own skill folders and reads them.
func (b *Brain) SetSkillFolders(folders []string) (int, error) {
	var kept []string

	for _, f := range folders {
		if f = strings.TrimSpace(f); f != "" {
			kept = append(kept, f)
		}
	}

	b.Cfg.SkillFolders = strings.Join(kept, string(filepath.ListSeparator))

	if err := b.Cfg.Save(b.Root); err != nil {
		return 0, err
	}

	return b.RefreshSkills()
}

// SkillCatalogue is every skill with its source and whether it is on.
func (b *Brain) SkillCatalogue() ([]skills.Entry, error) { return skills.Catalogue(b.Root) }
