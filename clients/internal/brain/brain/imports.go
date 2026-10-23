package brain

import (
	"path/filepath"

	"pn-scripts-assistant/internal/brain/skills"
)

/*
 * importSkills learns the agent skills on this machine as playbooks.
 *
 * On start and once a round of the learning, so a skill written or corrected
 * in Claude Code is known here within minutes without anybody asking. Cheap
 * when nothing changed: one walk of each skill folder and no writes. The live
 * list is reloaded only when something was written.
 */
func (b *Brain) importSkills() bool {
	folders := append(skills.DefaultSkillFolders(), filepath.SplitList(b.Cfg.SkillFolders)...)

	done, err := skills.ImportFolders(b.Root, folders)
	if err != nil {
		b.Log.Warn("an agent skill could not be imported", "error", err)
	}

	if len(done.Kept) > 0 {
		b.Log.Debug("kept its owner's own skills over imported ones", "skills", done.Kept)
	}

	if !done.Changed() {
		return false
	}

	b.Log.Info("imported agent skills as playbooks", "written", done.Written)

	return true
}
