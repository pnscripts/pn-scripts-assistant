package askclaude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * A folder that is a link is followed before anything is decided.
 *
 * Found by the 1.1.0 security review (F13): the protected check and the
 * link check looked at the path as it was written and at its last part
 * only, so "shared/id_ed25519", with "shared" a link to the keys' folder,
 * was neither protected nor a link, and the key went to Claude.
 */
func TestAFolderThatIsALinkDoesNotHideAProtectedFile(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".ssh", "id_ed25519"), "-----BEGIN OPENSSH PRIVATE KEY-----\n")
	write(t, filepath.Join(home, "elsewhere", "deep", "notes.md"), "fine\n")

	if os.Symlink(filepath.Join(home, ".ssh"), filepath.Join(home, "shared")) != nil ||
		os.Symlink(filepath.Join(home, "elsewhere"), filepath.Join(home, "work")) != nil {
		t.Skip("this system does not let the test make links")
	}

	protected := func(path string) bool { return strings.Contains(path, ".ssh") }

	docs, err := Gather([]string{"shared/id_ed25519"}, home, protected)
	if err == nil || !strings.Contains(err.Error(), "protected") || docs != nil {
		t.Fatalf("a protected file behind a linked folder: %v, %+v", err, docs)
	}

	// The error names where the file really is.
	if !strings.Contains(err.Error(), filepath.Join(home, ".ssh", "id_ed25519")) {
		t.Errorf("the refusal does not say where the file really is: %v", err)
	}

	// A linked folder that is not protected still works (a Documents
	// folder kept on another drive), and the file is shown where it
	// really is, which is what goes.
	docs, err = Gather([]string{"work/deep/notes.md"}, home, protected)
	if err != nil || len(docs) != 1 {
		t.Fatalf("a file in a linked folder: %v, %+v", err, docs)
	}

	real, err := filepath.EvalSymlinks(filepath.Join(home, "elsewhere", "deep", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}

	if docs[0].Path != real || docs[0].Text != "fine\n" {
		t.Errorf("shown as %q, want %q", docs[0].Path, real)
	}

	// The same file named twice, once through the link, is one file.
	if _, err := Gather([]string{"work/deep/notes.md", "elsewhere/deep/notes.md"}, home, protected); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("one file through two names: %v", err)
	}

	// Without links in the way, a path is left exactly as it was named.
	docs, err = Gather([]string{"elsewhere/deep/notes.md"}, home, protected)
	if err != nil || len(docs) != 1 || docs[0].Path != filepath.Join(home, "elsewhere", "deep", "notes.md") {
		t.Errorf("a plain path: %v, %+v", err, docs)
	}
}

func TestWhereAFileReallyIs(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "real", "a.txt"), "a\n")

	if os.Symlink(filepath.Join(home, "real"), filepath.Join(home, "link")) != nil {
		t.Skip("this system does not let the test make links")
	}

	want, _ := filepath.EvalSymlinks(filepath.Join(home, "real"))

	if got, err := Real(filepath.Join(home, "link", "new.txt")); err != nil || got != filepath.Join(want, "new.txt") {
		t.Errorf("a new file in a linked folder: %q, %v", got, err)
	}

	if got, err := Real(filepath.Join(home, "real", "a.txt")); err != nil || got != filepath.Join(home, "real", "a.txt") {
		t.Errorf("a plain path changed: %q, %v", got, err)
	}

	if _, err := Real(filepath.Join(home, "nowhere", "a.txt")); err == nil {
		t.Error("a folder that is not there has a real place")
	}
}
