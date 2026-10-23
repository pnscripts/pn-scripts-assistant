package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * A brain on a USB stick, plugged into another machine.
 *
 * Imported on one, moved to a different path (another mount point, another
 * drive letter), and opened where the agent's own skill folder does not
 * exist: every playbook still reads, and its pointer leads to a file that is
 * really there — the copy the brain carried with it.
 */
func TestPlaybooksTravelWithTheBrain(t *testing.T) {
	home, away := t.TempDir(), t.TempDir()
	root := filepath.Join(home, "stick", "data")
	agent := filepath.Join(home, "claude", "skills")

	writeAgentSkill(t, agent, "pn-release-gate", "---\ndescription: before publishing\n---\nCheck the gate.\n")

	refs := filepath.Join(agent, "pn-release-gate", "references")
	_ = os.MkdirAll(refs, 0o755)
	_ = os.WriteFile(filepath.Join(refs, "checklist.md"), []byte("- legal\n"), 0o644)
	_ = os.WriteFile(filepath.Join(agent, "pn-release-gate", ".env"), []byte("SECRET=1"), 0o644)

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := ImportFolders(root, []string{agent}); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(Path(root, "pn_release_gate"))
	_, body, _ := strings.Cut(string(raw), "\n---")
	if strings.Contains(body, home) {
		t.Fatalf("an absolute path to this machine was written into the playbook:\n%s", raw)
	}

	// Moved: a different mount point, and the agent's folder left behind.
	moved := filepath.Join(away, "E", "data")
	_ = os.MkdirAll(filepath.Dir(moved), 0o755)

	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(home, "claude")); err != nil {
		t.Fatal(err)
	}

	s, err := One(moved, "pn_release_gate")
	if err != nil || s == nil {
		t.Fatalf("the playbook did not travel: %v", err)
	}

	pointer := filepath.Join(moved, "knowledge", "skills", "pn-release-gate", SkillFile)

	if !strings.Contains(s.How, pointer) {
		t.Fatalf("the playbook does not point into the moved brain: %q", s.How)
	}

	for _, carried := range []string{pointer, filepath.Join(filepath.Dir(pointer), "references", "checklist.md")} {
		if _, err := os.Stat(carried); err != nil {
			t.Errorf("%s did not travel: %v", carried, err)
		}
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(pointer), ".env")); err == nil {
		t.Error("a secret was carried with the brain")
	}

	// And a learning pass on the new machine keeps it working.
	if _, err := ImportFolders(moved, []string{filepath.Join(away, "no-agent-here")}); err != nil {
		t.Fatal(err)
	}

	again, _ := One(moved, "pn_release_gate")
	if again == nil || !strings.Contains(again.How, pointer) || !strings.Contains(again.How, "Check the gate.") {
		t.Fatalf("after a pass away from home: %+v", again)
	}
}
