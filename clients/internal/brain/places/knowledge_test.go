package places

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/learning"
)

func TestTheKnowledgeFolderIsAPlaceDespiteBeingInsideTheBrain(t *testing.T) {
	root := t.TempDir()

	if CanWatch(root, root) == nil {
		t.Fatal("the brain's own folder became learnable")
	}

	if CanWatch(root, filepath.Join(root, "memory")) == nil {
		t.Fatal("another folder inside the brain became learnable")
	}

	if err := CanWatch(root, Knowledge(root)); err != nil {
		t.Fatalf("the knowledge folder was refused: %v", err)
	}

	list, _ := List(root)
	if len(list) != 0 {
		t.Fatalf("a knowledge folder that does not exist was listed: %+v", list)
	}

	if err := os.MkdirAll(Knowledge(root), 0o700); err != nil {
		t.Fatal(err)
	}

	list, _ = Status(root)
	if len(list) != 1 || list[0].Path != Knowledge(root) || !list[0].Reachable || list[0].Kind != Documents {
		t.Fatalf("the knowledge folder is not a place: %+v", list)
	}
}

/*
 * The brain moved to another mount point: the knowledge folder is still a
 * place, still reachable, and what was counted about it came along.
 */
func TestTheKnowledgeFolderTravelsWithTheBrain(t *testing.T) {
	home, away := t.TempDir(), t.TempDir()
	root := filepath.Join(home, "data")

	if err := os.MkdirAll(Knowledge(root), 0o700); err != nil {
		t.Fatal(err)
	}

	list, _ := Status(root)
	done := list[0]
	done.Learned, done.LastLearned = 7, time.Now().UTC()

	if err := Note(root, done); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(filepath.Join(root, ListFile))
	if strings.Contains(string(raw), home) || !strings.Contains(string(raw), `"path": "knowledge"`) {
		t.Fatalf("written with an absolute path:\n%s", raw)
	}

	moved := filepath.Join(away, "data")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}

	list, err := Status(moved)
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v %v", list, err)
	}

	if list[0].Path != Knowledge(moved) || !list[0].Reachable || list[0].Learned != 7 {
		t.Fatalf("after the move: %+v", list[0])
	}
}

// The two packages spell the knowledge folder the same way, or readings from
// it go back to waiting for a person.
func TestTheKnowledgeFolderIsSpelledOnce(t *testing.T) {
	if learning.OwnKnowledge != learning.BrainPrefix+KnowledgeFolder+"/" {
		t.Errorf("learning.OwnKnowledge %q does not match KnowledgeFolder %q",
			learning.OwnKnowledge, KnowledgeFolder)
	}
}

/*
 * The built-in skills are left out of the knowledge folder's reading, and
 * nothing else is.
 *
 * Only in the brain's own knowledge folder: somebody's own drive with a folder
 * that happens to be called pack-skills is read like any other.
 */
func TestTheBuiltInSkillsAreNotReadIntoMemory(t *testing.T) {
	root := t.TempDir()

	learning.SetBrainRoot(root)
	t.Cleanup(func() { learning.SetBrainRoot("") })

	write := func(path string) {
		t.Helper()

		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}

		body := "# Notes\n\nThe greenhouse door sticks in wet weather and needs oiling every spring.\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	knowledge := Knowledge(root)
	write(filepath.Join(knowledge, "guides", "garden.md"))
	write(filepath.Join(knowledge, PackSkillsFolder, "bookkeeping", "SKILL.md"))
	write(filepath.Join(knowledge, "imported", "my-skill", "SKILL.md"))

	elsewhere := t.TempDir()
	write(filepath.Join(elsewhere, PackSkillsFolder, "notes.md"))

	sources := func(p Place) string {
		t.Helper()

		obs, err := scan(p, "Alex")
		if err != nil {
			t.Fatal(err)
		}

		var all []string
		for _, o := range obs {
			all = append(all, o.Source)
		}

		return strings.Join(all, "\n")
	}

	inBrain := sources(Place{Path: knowledge, Kind: Documents})

	if strings.Contains(inBrain, PackSkillsFolder) {
		t.Errorf("the built-in skills are read into memory:\n%s", inBrain)
	}

	for _, want := range []string{"guides/garden.md", "imported/my-skill/SKILL.md"} {
		if !strings.Contains(inBrain, want) {
			t.Errorf("%s was left out of the knowledge folder:\n%s", want, inBrain)
		}
	}

	if outside := sources(Place{Path: elsewhere, Kind: Documents}); !strings.Contains(outside, PackSkillsFolder+"/notes.md") {
		t.Errorf("a folder of somebody's own called %s was skipped:\n%s", PackSkillsFolder, outside)
	}
}

/*
 * The copies of imported agent skills are not read into memory either.
 *
 * They sit in the knowledge folder (skills.KnowledgeSkills) so they travel
 * with the brain, and they are other people's writing: a folder of skills from
 * anywhere on the disk. Read as documents they went into memory unreviewed.
 * They are already the brain's skills, chosen by what a message is about.
 */
func TestImportedSkillsAreNotReadIntoMemory(t *testing.T) {
	root := t.TempDir()

	learning.SetBrainRoot(root)
	t.Cleanup(func() { learning.SetBrainRoot("") })

	knowledge := Knowledge(root)

	for _, path := range []string{
		filepath.Join(knowledge, "guides", "garden.md"),
		filepath.Join(knowledge, ImportedSkillsFolder, "someones-skill", "SKILL.md"),
		filepath.Join(knowledge, ImportedSkillsFolder, "someones-skill", "references", "notes.md"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}

		body := "# Notes\n\nThe greenhouse door sticks in wet weather and needs oiling every spring.\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	obs, err := scan(Place{Path: knowledge, Kind: Documents}, "Alex")
	if err != nil {
		t.Fatal(err)
	}

	var sources []string
	for _, o := range obs {
		sources = append(sources, o.Source)
	}

	all := strings.Join(sources, "\n")

	if strings.Contains(all, "/"+ImportedSkillsFolder+"/") {
		t.Errorf("an imported skill was read into memory:\n%s", all)
	}

	if !strings.Contains(all, "guides/garden.md") {
		t.Errorf("the brain's own guide was left out:\n%s", all)
	}

	// The same folder as the skills package writes the copies to, and the
	// same names the learning package refuses to trust.
	if want := filepath.Join(KnowledgeFolder, ImportedSkillsFolder); want != filepath.Join("knowledge", "skills") {
		t.Errorf("ImportedSkillsFolder is %q", ImportedSkillsFolder)
	}

	for _, folder := range []string{PackSkillsFolder, ImportedSkillsFolder} {
		found := false
		for _, theirs := range learning.NotOwnKnowledge {
			found = found || theirs == folder+"/"
		}

		if !found {
			t.Errorf("learning.NotOwnKnowledge %v does not name %s", learning.NotOwnKnowledge, folder)
		}
	}
}
