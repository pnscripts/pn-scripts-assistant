package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResearchReportsAreKeptListedAndRemoved(t *testing.T) {
	db := openDeeds(t)

	if _, err := db.SaveResearch(ResearchReport{Question: " "}); err == nil {
		t.Error("a report with no question was kept")
	}

	first, err := db.SaveResearch(ResearchReport{Question: "VAT in Bulgaria", Lang: "en", Depth: "quick",
		Report: "# VAT", Sources: json.RawMessage(`[{"n":1}]`), NSources: 1, NClaims: 3, NFlagged: 1})
	if err != nil {
		t.Fatal(err)
	}

	second, _ := db.SaveResearch(ResearchReport{Question: "Second", Lang: "bg", Depth: "thorough"})

	list, err := db.ResearchReports()
	if err != nil || len(list) != 2 || list[0].ID != second.ID || list[1].Report != "" || list[1].NFlagged != 1 {
		t.Fatalf("%v %+v", err, list)
	}

	back, err := db.Research(first.ID)
	if err != nil || back.Report != "# VAT" || string(back.Sources) != `[{"n":1}]` || string(back.Claims) != "[]" {
		t.Fatalf("%v %+v", err, back)
	}

	if err := db.RemoveResearch(first.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Research(first.ID); !errors.Is(err, ErrNoResearch) {
		t.Errorf("a removed report is still there: %v", err)
	}

	if err := db.RemoveResearch(first.ID); !errors.Is(err, ErrNoResearch) {
		t.Errorf("removing twice: %v", err)
	}
}

// What somebody looks into is sealed like the rest of their words.
func TestResearchIsSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	r, _ := d.SaveResearch(ResearchReport{Question: "Treatments for a rare illness", Report: "report words",
		Sources: json.RawMessage(`[{"title":"source words"}]`), Claims: json.RawMessage(`[{"text":"claim words"}]`)})

	if back, _ := d.Research(r.ID); back.Report != "report words" {
		t.Fatalf("%+v", back)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	for _, word := range []string{"rare illness", "report words", "source words", "claim words"} {
		if strings.Contains(string(raw)+string(wal), word) {
			t.Errorf("%q is readable on disk", word)
		}
	}
}
