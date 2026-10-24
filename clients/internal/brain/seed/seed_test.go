package seed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func read(t *testing.T, p string) string {
	t.Helper()

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func TestTheSeedIsWrittenOnceIntoAFreshBrain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")

	first, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, _ := Files()
	if len(first.Written) != len(files) || len(files) < 5 {
		t.Fatalf("wrote %v of %v", first.Written, files)
	}

	again, err := Install(dir)
	if err != nil || again.Changed() {
		t.Fatalf("the second start wrote %+v (%v)", again, err)
	}
}

func TestAnEditedOrDeletedGuideIsNeverOverwritten(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}

	edited := filepath.Join(dir, "guides", "scam-guard.md")
	os.WriteFile(edited, []byte("My own rules."), 0o644)

	deleted := filepath.Join(dir, "guides", "trusted-sources.md")
	os.Remove(deleted)

	mine := filepath.Join(dir, "notes.md")
	os.WriteFile(mine, []byte("Mine."), 0o644)

	res, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}

	if read(t, edited) != "My own rules." {
		t.Fatal("an edited guide was overwritten")
	}

	if _, err := os.Stat(deleted); err == nil {
		t.Fatal("a guide its owner deleted came back")
	}

	if read(t, mine) != "Mine." {
		t.Fatal("the owner's own file was touched")
	}

	if res.Changed() {
		t.Fatalf("something was written: %+v", res)
	}
}

// An upgrade replaces a guide still holding the old shipped text, adds new
// ones, and keeps the ones its owner changed.
func TestAnUpgradeUpdatesUntouchedGuidesAndAddsNewOnes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}

	// Pretend the brain was seeded by an older pack: one guide with older
	// text, one guide that did not exist yet, one the owner edited.
	old := manifest{Version: Version - 1, Files: map[string]string{}}
	raw, _ := os.ReadFile(filepath.Join(dir, ManifestName))
	json.Unmarshal(raw, &old)
	old.Version = Version - 1

	stale := filepath.Join(dir, "guides", "getting-started.md")
	os.WriteFile(stale, []byte("old shipped text"), 0o644)
	old.Files["guides/getting-started.md"] = sum([]byte("old shipped text"))

	added := filepath.Join(dir, "guides", "experts-and-models.md")
	os.Remove(added)
	delete(old.Files, "guides/experts-and-models.md")

	edited := filepath.Join(dir, "guides", "portable-and-usb.md")
	os.WriteFile(edited, []byte("edited by the owner"), 0o644)

	raw, _ = json.Marshal(old)
	os.WriteFile(filepath.Join(dir, ManifestName), raw, 0o644)

	res, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(read(t, stale), "Getting started") || len(res.Updated) != 1 {
		t.Fatalf("the untouched guide was not updated: %+v", res)
	}

	if _, err := os.Stat(added); err != nil || len(res.Written) != 1 {
		t.Fatalf("the new guide was not added: %+v", res)
	}

	if read(t, edited) != "edited by the owner" {
		t.Fatal("the owner's edit was lost in the upgrade")
	}
}

// Nothing about one person or company ships in the pack.
func TestThePackCarriesNothingPrivate(t *testing.T) {
	files, _ := Files()

	bad := regexp.MustCompile(`(?i)petar|петър|nikolov|pnscripts\.com|/home/[a-z]+/|/media/[a-z]+/|ai-brain|@gmail`)

	for _, f := range files {
		body, _ := pack.ReadFile("pack/" + f)
		if m := bad.FindString(string(body)); m != "" {
			t.Errorf("%s contains %q", f, m)
		}
	}
}

// The originality rule ships to every user, in English and Bulgarian.
func TestThePackCarriesTheOriginalityGuideInBothLanguages(t *testing.T) {
	for file, want := range map[string]string{
		"guides/originality.md":    "not a licence loophole",
		"guides/originality.bg.md": "вратичка в лиценза",
		"guides/how-i-work.md":     "Only what is needed",
		"guides/how-i-work.bg.md":  "Само каквото е нужно",
	} {
		body, err := pack.ReadFile("pack/" + file)
		if err != nil || !strings.Contains(string(body), want) {
			t.Errorf("%s missing or without %q (%v)", file, want, err)
		}
	}
}
