package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func writeAgentSkill(t *testing.T, folder, dir, text string) string {
	t.Helper()

	path := filepath.Join(folder, dir, SkillFile)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestAnAgentSkillBecomesAPlaybook(t *testing.T) {
	root, agent := t.TempDir(), t.TempDir()

	writeAgentSkill(t, agent, "release-gate", "---\nname: release-gate\ndescription: \"Check everything before it is published.\"\n---\n\n# Release gate\n\n1. Read the rules.\n2. Test it.\n")

	done, err := ImportFolders(root, []string{agent})
	if err != nil {
		t.Fatal(err)
	}

	if len(done.Written) != 1 || done.Written[0] != "release_gate" {
		t.Fatalf("wrote %v", done.Written)
	}

	found, err := Load(root)
	if err != nil || len(found) != 1 {
		t.Fatalf("loaded %v, %v", found, err)
	}

	s := found[0]

	if s.When != "Check everything before it is published." {
		t.Errorf("when is %q", s.When)
	}

	if !strings.Contains(s.How, "2. Test it.") || !strings.Contains(s.How, "read_file ") {
		t.Errorf("how is %q", s.How)
	}

	if s.Imported == "" || s.Taught {
		t.Errorf("not marked as imported: %+v", s)
	}
}

func TestALongSkillIsCutWithAPointer(t *testing.T) {
	root, agent := t.TempDir(), t.TempDir()

	writeAgentSkill(t, agent, "long", "---\ndescription: >\n  A long one,\n  folded.\n---\n"+strings.Repeat("Ступка по ступка.\n", 2000))

	if _, err := ImportFolders(root, []string{agent}); err != nil {
		t.Fatal(err)
	}

	s, err := One(root, "long")
	if err != nil || s == nil {
		t.Fatal(err)
	}

	if n := utf8.RuneCountInString(s.How); n > MostRunes {
		t.Errorf("%d runes, over the cap", n)
	}

	if !strings.Contains(s.How, "shortened") || !strings.Contains(s.How, SkillFile) {
		t.Errorf("no pointer to the full file at the end: %q", s.How[len(s.How)-300:])
	}

	if s.When != "A long one, folded." {
		t.Errorf("folded description read as %q", s.When)
	}
}

func TestOnlyAChangedSkillIsWrittenAgain(t *testing.T) {
	root, agent := t.TempDir(), t.TempDir()

	path := writeAgentSkill(t, agent, "invoicing", "---\ndescription: bills\n---\nOld way.\n")

	if _, err := ImportFolders(root, []string{agent}); err != nil {
		t.Fatal(err)
	}

	again, _ := ImportFolders(root, []string{agent})
	if again.Changed() || again.Unchanged != 1 {
		t.Fatalf("an unchanged skill was written again: %+v", again)
	}

	if err := os.WriteFile(path, []byte("---\ndescription: bills\n---\nNew way.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	later := time.Now().Add(5 * time.Second)
	_ = os.Chtimes(path, later, later)

	refreshed, _ := ImportFolders(root, []string{agent})
	if !refreshed.Changed() {
		t.Fatal("a changed skill was not refreshed")
	}

	s, _ := One(root, "invoicing")
	if !strings.Contains(s.How, "New way.") {
		t.Errorf("still says %q", s.How)
	}
}

func TestWhatTheOwnerWroteWins(t *testing.T) {
	root, agent := t.TempDir(), t.TempDir()

	for _, mine := range []Skill{
		{Name: "invoicing", When: "bills", How: "My way, by hand."},
		{Name: "deploy", When: "shipping", How: "Taught in conversation.", Taught: true},
	} {
		if err := Save(root, mine); err != nil {
			t.Fatal(err)
		}
	}

	writeAgentSkill(t, agent, "invoicing", "---\ndescription: x\n---\nThe agent's way.\n")
	writeAgentSkill(t, agent, "deploy", "---\ndescription: x\n---\nThe agent's way.\n")

	done, err := ImportFolders(root, []string{agent})
	if err != nil {
		t.Fatal(err)
	}

	if done.Changed() || len(done.Kept) != 2 {
		t.Fatalf("overwrote its owner's skills: %+v", done)
	}

	for name, want := range map[string]string{"invoicing": "My way, by hand.", "deploy": "Taught in conversation."} {
		s, _ := One(root, name)
		if s.How != want {
			t.Errorf("%s now says %q", name, s.How)
		}
	}
}

func TestMissingFoldersAreNotAnError(t *testing.T) {
	root := t.TempDir()

	done, err := ImportFolders(root, []string{filepath.Join(root, "nowhere"), ""})
	if err != nil || done.Changed() {
		t.Fatalf("%+v %v", done, err)
	}
}

func TestNamesAreMadeCallable(t *testing.T) {
	for raw, want := range map[string]string{
		"pn-scam-guard":            "pn_scam_guard",
		"Release Gate!!":           "release_gate",
		"3d-models":                "skill_3d_models",
		"a":                        "a_skill",
		"--":                       "skill",
		strings.Repeat("abc-", 20): "abc_abc_abc_abc_abc_abc_abc_abc_abc_abc_a",
	} {
		got := NormaliseName(raw)
		if got != want {
			t.Errorf("%q became %q, want %q", raw, got, want)
		}

		if err := CheckName(got); err != nil {
			t.Errorf("%q is not a valid name: %v", got, err)
		}
	}
}
