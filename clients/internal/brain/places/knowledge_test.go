package places

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
