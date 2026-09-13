package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/tools"
)

func TestASkillComesBackAsItWasWritten(t *testing.T) {
	root := t.TempDir()

	written := Skill{
		Name:   "invoicing",
		When:   "I ask about invoices, billing or what a client owes",
		How:    "1. Open the ledger.\n2. Bill in advance.\n3. Never chase late payments myself.",
		Taught: true,
	}

	if err := Save(root, written); err != nil {
		t.Fatal(err)
	}

	found, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 {
		t.Fatalf("loaded %d skills", len(found))
	}

	if found[0] != written {
		t.Errorf("came back as %+v", found[0])
	}

	// And it is a file somebody can open and correct, which is most of the
	// point: the ones worth keeping are the ones that get edited.
	raw, err := os.ReadFile(filepath.Join(Folder(root), "invoicing.md"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(raw), "Never chase late payments") {
		t.Errorf("the file does not contain the instructions: %q", raw)
	}
}

/*
 * A skill returns instructions and changes nothing.
 *
 * The property the whole design rests on. Following a skill means calling the
 * ordinary tools, each stopping for approval exactly as it would have anyway —
 * so the worst a bad skill can do is waste a turn. A skill that could act
 * would be a way of writing new capability into the machine without ever
 * passing the gate.
 */
func TestASkillCannotDoAnything(t *testing.T) {
	skill := AsTool(Skill{Name: "invoicing", When: "billing", How: "Bill in advance."})

	if skill.Risk() != tools.Safe {
		t.Error("a skill declares itself able to change something")
	}

	out, err := skill.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Bill in advance.") {
		t.Errorf("it did not return the instructions: %q", out)
	}

	// And what comes back says plainly that the gate still applies.
	if !strings.Contains(out, "still needs approval") {
		t.Errorf("it does not say the ordinary rules still hold: %q", out)
	}
}

/*
 * A skill is offered only when its own words appear.
 *
 * How well a model chooses among tools falls off sharply with its size — the
 * small model here already picks wrongly among thirty-one — so a dozen skills
 * offered on every turn would make it worse at everything rather than better
 * at one thing.
 */
func TestASkillNamesTheWordsThatSummonIt(t *testing.T) {
	skill := AsTool(Skill{
		Name: "invoicing",
		When: "I ask about invoices, billing or what a client owes",
	})

	cued, ok := skill.(tools.Cued)
	if !ok {
		t.Fatal("a skill does not name its cues, so it would be offered on every turn")
	}

	cues := map[string]bool{}
	for _, c := range cued.Cues() {
		cues[c] = true
	}

	for _, want := range []string{"invoicing", "invoices", "billing", "client", "owes"} {
		if !cues[want] {
			t.Errorf("%q is not a cue: %v", want, cued.Cues())
		}
	}

	// Words that appear in any sentence are not cues. A skill that matches
	// everything is a skill with no cues at all.
	for _, notACue := range []string{"about", "what", "asks"} {
		if cues[notACue] {
			t.Errorf("%q was kept as a cue, which would match almost anything", notACue)
		}
	}
}

// Names are narrow because a skill's name is what the model calls, in the same
// namespace as read_file.
func TestNamesAreNarrow(t *testing.T) {
	for _, bad := range []string{"", "a", "Invoicing", "my invoicing", "invoicing!", "1st"} {
		if CheckName(bad) == nil {
			t.Errorf("%q was accepted as a skill name", bad)
		}
	}

	for _, good := range []string{"invoicing", "new_client", "godot_export_2"} {
		if err := CheckName(good); err != nil {
			t.Errorf("%q was refused: %v", good, err)
		}
	}
}

/*
 * A file half written is skipped, not fatal.
 *
 * These are hand-editable by design, so one of them being mid-edit is an
 * ordinary Tuesday and is not a reason for the other nine to stop working.
 */
func TestOneBadFileDoesNotStopTheRest(t *testing.T) {
	root := t.TempDir()

	if err := Save(root, Skill{Name: "good_one", When: "x", How: "Do the thing."}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(Folder(root), "Broken One.md"),
		[]byte("name: Broken One\n---\nsomething"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(Folder(root), "empty.md"),
		[]byte("name: empty\nwhen: never\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := Load(root)
	if err != nil {
		t.Fatalf("one bad file stopped the lot: %v", err)
	}

	if len(found) != 1 || found[0].Name != "good_one" {
		t.Errorf("loaded %+v", found)
	}
}

// A plain text file with no header is still a skill. Somebody who drops a file
// in this folder has said what they meant, and refusing it over a missing
// colon would be the program being pedantic about its own format.
func TestAPlainFileIsStillASkill(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(Folder(root), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(Folder(root), "handover.md"),
		[]byte("Always write the handover note before closing a project."), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 || found[0].Name != "handover" {
		t.Fatalf("loaded %+v", found)
	}

	if !strings.Contains(found[0].How, "handover note") {
		t.Errorf("the instructions were lost: %q", found[0].How)
	}
}

func TestForgettingOneTwiceIsNotAnError(t *testing.T) {
	root := t.TempDir()

	if err := Save(root, Skill{Name: "invoicing", How: "x"}); err != nil {
		t.Fatal(err)
	}

	if err := Remove(root, "invoicing"); err != nil {
		t.Fatal(err)
	}

	if err := Remove(root, "invoicing"); err != nil {
		t.Errorf("deleting it a second time failed: %v", err)
	}
}
