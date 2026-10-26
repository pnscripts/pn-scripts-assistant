package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func fakePack(t *testing.T) {
	t.Helper()

	old := packSource
	t.Cleanup(func() { packSource = old })

	packSource = fstest.MapFS{
		"pack/README.md":                       {Data: []byte("# pack")},
		"pack/travel-planning/SKILL.md":        {Data: []byte("---\nname: travel-planning\ndescription: Plan a trip — dates, budget, bookings and a packing list.\n---\n\n# Travel\n\n1. Ask the dates.\n")},
		"pack/travel-planning/SKILL.bg.md":     {Data: []byte("---\nname: travel-planning\ndescription: Планиране на пътуване — дати, бюджет, резервации и списък за багажа.\n---\n\n# Пътуване\n\n1. Попитай за датите.\n")},
		"pack/travel-planning/scripts/days.py": {Data: []byte("print(1)\n")},
		"pack/broken/SKILL.md":                 {Data: []byte("no front matter, no description")},
		"pack/scam_guard/SKILL.md":             {Data: []byte("---\nname: scam_guard\ndescription: shadowing a playbook\n---\nbody")},
	}
}

func TestThePackIsReadWithItsBulgarianWords(t *testing.T) {
	fakePack(t)

	got, err := Pack()
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || got[1].Name != "travel_planning" {
		t.Fatalf("%+v", got)
	}

	s := got[1]
	if !s.Pack || !s.Builtin || !strings.HasPrefix(s.WhenBG, "Планиране") || !strings.Contains(s.HowBG, "Попитай") ||
		!strings.Contains(s.How, "Ask the dates") || !strings.Contains(s.How, filepath.Join(PackFolder, "travel-planning", SkillFile)) {
		t.Errorf("%+v", s)
	}

	root := t.TempDir()
	if err := InstallPack(root); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(root, PackFolder, "travel-planning", "scripts", "days.py"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the script was not installed runnable: %v", err)
	}

	if err := InstallPack(root); err != nil {
		t.Errorf("installing twice: %v", err)
	}
}

func TestTheCatalogueSaysWhereEachSkillCameFromAndWhetherItIsOn(t *testing.T) {
	fakePack(t)

	root := t.TempDir()
	agent := t.TempDir()

	_ = os.MkdirAll(filepath.Join(agent, "code-review"), 0o755)
	_ = os.WriteFile(filepath.Join(agent, "code-review", SkillFile), []byte("---\nname: code-review\ndescription: Review a change for bugs.\n---\nRead the diff."), 0o644)
	_ = os.WriteFile(filepath.Join(agent, "ship_it.md"), []byte("name: ship_it\nversion: 1\nwhen: shipping the site\n---\nRun the train."), 0o644)
	_ = os.WriteFile(filepath.Join(agent, "README.md"), []byte("# not a playbook"), 0o644)
	_ = os.WriteFile(filepath.Join(agent, "notes.md"), []byte("just notes, no header"), 0o644)

	done, err := ImportFolders(root, []string{agent})
	if err != nil || len(done.Written) != 2 {
		t.Fatalf("%v %+v", err, done)
	}

	if err := Save(root, Skill{Name: "travel_planning", When: "my own way of planning trips", How: "mine"}); err != nil {
		t.Fatal(err)
	}

	if err := SetEnabled(root, "code_review", false); err != nil {
		t.Fatal(err)
	}

	if SetEnabled(root, "Not A Name", false) == nil {
		t.Error("a bad name was switched")
	}

	list, err := Catalogue(root)
	if err != nil {
		t.Fatal(err)
	}

	by := map[string]Entry{}
	for _, e := range list {
		by[e.Name] = e
	}

	if by["code_review"].Source != SourceImported || by["code_review"].On {
		t.Errorf("code_review: %+v", by["code_review"])
	}

	if by["ship_it"].Source != SourceImported || by["ship_it"].How != "Run the train." {
		t.Errorf("ship_it: %+v", by["ship_it"])
	}

	if by["travel_planning"].Source != SourceTaught || by["travel_planning"].How != "mine" {
		t.Errorf("the owner's own skill did not win over the pack: %+v", by["travel_planning"])
	}

	if by["scam_guard"].Source != SourcePlaybook {
		t.Errorf("the pack shadowed a built-in playbook: %+v", by["scam_guard"])
	}

	if _, ok := by["notes"]; ok {
		t.Error("a Markdown file without a header became a skill")
	}

	_ = SetEnabled(root, "code_review", true)
	if Disabled(root)["code_review"] {
		t.Error("switching back on did not stick")
	}

	// A changed playbook file is imported again; an unchanged one is not.
	again, _ := ImportFolders(root, []string{agent})
	if len(again.Written) != 0 {
		t.Errorf("unchanged files were written again: %v", again.Written)
	}
}

// The pack ships to every user: nothing of one person or company in it.
func TestTheShippedPackCarriesNothingPrivate(t *testing.T) {
	bad := regexp.MustCompile(`(?i)pnscripts|pn scripts|ai-brain|/home/[a-z]+/|/media/[a-z]+/|@gmail|itch\.io/profile|office@`)
	word := regexp.MustCompile(`\p{L}+`)
	private := map[string]bool{
		"9d6245d7fb961b620b28c91d22e1a585413743388dc1833e678129f1751d94ae": true,
		"b3f73b9f2252e3d0762e50d43e80fe9f31f6a49777c8fe9f88ff06b5c188377c": true,
		"4754ddf3e436fa4c57da68e83e40c55245c3fc6e7cb1ed1d965c11f88e55a52b": true,
	}

	_ = fs.WalkDir(packFS, "pack", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, "README.md") {
			return err
		}

		data, _ := packFS.ReadFile(p)

		if m := bad.FindString(string(data)); m != "" {
			t.Errorf("%s contains %q", p, m)
		}

		for _, w := range word.FindAllString(string(data), -1) {
			sum := sha256.Sum256([]byte(strings.ToLower(w)))
			if private[hex.EncodeToString(sum[:])] {
				t.Errorf("%s contains a private name", p)
			}
		}

		return nil
	})
}
