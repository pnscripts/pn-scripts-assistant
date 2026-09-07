package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * The brain never offers to learn from its own storage.
 *
 * PN-BRAIN-DATA was excluded by name and a backup of it beside it was not, so
 * the brain offered a folder holding a JSON dump of everything it had ever
 * known. That is a loop: it would read its own memories back in as documents,
 * attributed to a file, and propose remembering them again.
 *
 * By what is in the folder rather than what it is called, because a backup is
 * called whatever the person called it — and this program makes them on
 * purpose, under the name they choose.
 */
func TestItsOwnStorageIsNeverOffered(t *testing.T) {
	root := t.TempDir()

	for _, mark := range []string{"brain.sqlite", "brain.conf", ".brain-root.json"} {
		dir := filepath.Join(root, "copy-"+strings.TrimPrefix(mark, "."))
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, mark), []byte("x"), 0o644)

		if !itsOwnData(dir) {
			t.Errorf("a folder holding %s was not recognised as the brain's own", mark)
		}
	}

	// And an ordinary folder is still offered.
	ordinary := filepath.Join(root, "Documents")
	os.MkdirAll(ordinary, 0o755)
	os.WriteFile(filepath.Join(ordinary, "notes.md"), []byte("x"), 0o644)

	if itsOwnData(ordinary) {
		t.Error("an ordinary folder was mistaken for the brain's own storage")
	}
}
