package brain

import (
	"os"
	"path/filepath"

	"pn-scripts-assistant/internal/brain/places"
	"pn-scripts-assistant/internal/brain/seed"

	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/scam"

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
	if b.Root != "" {
		_ = os.MkdirAll(filepath.Join(b.Root, MySkillsFolder), 0o755)
	}

	folders := b.SkillFolders()

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

// scamDNS is shared, so a domain is asked about once per run.
var scamDNS = &scam.DNS{}

/*
 * scamLookup is the scam check's one question to the network — does this
 * domain exist — asked only when the privacy setting lets anything reach the
 * web. Under private, the check still reads the words and the addresses.
 */
func (b *Brain) scamLookup() scam.Lookup {
	if !llm.ParseMode(b.Cfg.Privacy).AllowsWeb() {
		return nil
	}

	return scamDNS
}

// installSeed writes the knowledge the program carries into this brain's
// knowledge folder: once on a new brain, and again after an upgrade for the
// guides its owner has not changed. A brain that cannot be written to still
// starts; it simply has no guides.
func (b *Brain) installSeed() {
	if b.Root == "" {
		return
	}

	// Paths inside the brain are remembered relative to it, so a brain that
	// moves does not learn its own knowledge folder again (see learning.Portable).
	learning.SetBrainRoot(b.Root)

	if n, err := b.DB.PortableSources(b.Root); err != nil {
		b.Log.Warn("could not make the brain's own paths relative", "error", err)
	} else if n > 0 {
		b.Log.Info("made the brain's own paths relative", "rows", n)
	}

	// The built-in skill pack, with its references and scripts, where a
	// skill's own instructions say they are.
	if err := skills.InstallPack(b.Root); err != nil {
		b.Log.Warn("the built-in skills could not be written", "error", err)
	}

	done, err := seed.Install(places.Knowledge(b.Root))
	if err != nil {
		b.Log.Warn("the starting knowledge could not be written", "error", err)

		return
	}

	if done.Changed() {
		b.Log.Info("wrote the starting knowledge", "new", done.Written, "updated", done.Updated, "kept", done.Kept)
	}
}
