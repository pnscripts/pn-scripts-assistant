package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mk(t *testing.T, root, path string) {
	t.Helper()

	full := filepath.Join(root, path)

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(full, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(projects []Project) []string {
	out := make([]string, 0, len(projects))

	for _, p := range projects {
		out = append(out, p.Name)
	}

	return out
}

// A project's own dependencies are not projects. This is the difference
// between knowing about 62 things somebody built and 75 things, thirteen of
// which are somebody else's code.
func TestScanStopsAtTheProjectRoot(t *testing.T) {
	root := t.TempDir()

	mk(t, root, "app/composer.json")
	mk(t, root, "app/vendor/some/library/composer.json")
	mk(t, root, "app/node_modules/thing/package.json")

	found, err := ScanProjects(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 || found[0].Name != "app" {
		t.Fatalf("found %v, want just [app]", names(found))
	}
}

// A WordPress site carries hundreds of third-party plugins, each with its own
// package.json. Scanning them presents somebody else's code as the owner's.
func TestBundledThirdPartyCodeIsNotAProject(t *testing.T) {
	root := t.TempDir()

	mk(t, root, "site/composer.json")
	mk(t, root, "site/wp-content/plugins/woocommerce/package.json")
	mk(t, root, "site/wp-includes/js/package.json")

	found, _ := ScanProjects(root)

	if len(found) != 1 {
		t.Fatalf("found %v, want just the site", names(found))
	}
}

// Dated snapshots are not a distinct current project.
func TestBackupDirectoriesAreSkipped(t *testing.T) {
	root := t.TempDir()

	mk(t, root, "live/composer.json")
	mk(t, root, "backup_2024-01/composer.json")
	mk(t, root, "backup-old/composer.json")
	mk(t, root, ".backup/composer.json")

	found, _ := ScanProjects(root)

	if len(found) != 1 || found[0].Name != "live" {
		t.Fatalf("found %v, want just [live]", names(found))
	}
}

func TestStackDetection(t *testing.T) {
	cases := map[string]string{
		"go.mod":           "Go",
		"package.json":     "Node.js",
		"composer.json":    "PHP/Composer",
		"pyproject.toml":   "Python",
		"requirements.txt": "Python",
		"project.godot":    "Godot",
		"app.csproj":       "C#/.NET",
	}

	for marker, want := range cases {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			mk(t, root, "proj/"+marker)

			found, _ := ScanProjects(root)

			if len(found) != 1 {
				t.Fatalf("marker %s found %d projects", marker, len(found))
			}

			if found[0].Stack != want {
				t.Errorf("%s detected as %q, want %q", marker, found[0].Stack, want)
			}
		})
	}
}

// A symlink can point back up the tree, and following one is how a scan
// becomes infinite.
func TestSymlinksAreNotFollowed(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "real/composer.json")

	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	found, _ := ScanProjects(root)

	if len(found) != 1 {
		t.Fatalf("found %v, want just the real project", names(found))
	}
}

func TestReadmeIsExcerptedNotSwallowed(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "proj/go.mod")

	os.WriteFile(filepath.Join(root, "proj", "README.md"),
		[]byte(strings.Repeat("x", ReadmeExcerpt*3)), 0o644)

	found, _ := ScanProjects(root)

	if len(found) != 1 {
		t.Fatal("project not found")
	}

	// Enough to say what it is, little enough that a hundred of them do not
	// overwhelm the store.
	if len([]rune(found[0].Readme)) > ReadmeExcerpt+1 {
		t.Errorf("readme excerpt is %d runes, want at most %d", len([]rune(found[0].Readme)), ReadmeExcerpt+1)
	}
}

// README-AI.md is a file a project can add to tell the brain what it is, so it
// must win over an ordinary README.
func TestReadmeAITakesPrecedence(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "proj/go.mod")

	os.WriteFile(filepath.Join(root, "proj", "README.md"), []byte("ordinary"), 0o644)
	os.WriteFile(filepath.Join(root, "proj", "README-AI.md"), []byte("written for the brain"), 0o644)

	found, _ := ScanProjects(root)

	if !strings.Contains(found[0].Readme, "written for the brain") {
		t.Errorf("README-AI.md was not preferred: %q", found[0].Readme)
	}
}

// A scanned claim names a path, which is what lets the Validator recheck it and
// promote it without a person.
func TestObservationsCarryACheckableSource(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "proj/go.mod")

	found, _ := ScanProjects(root)
	obs := FromProjects(found, "Petar")

	if len(obs) != 1 {
		t.Fatalf("got %d observations", len(obs))
	}

	if !strings.HasPrefix(obs[0].Source, "project:") {
		t.Errorf("source is %q", obs[0].Source)
	}

	var v Validator
	if got := v.StatusFor(obs[0].Source); got != StatusValidated {
		t.Errorf("a real project validated as %q", got)
	}

	if !strings.Contains(obs[0].Content, "Petar") {
		t.Errorf("the fact does not name the owner: %q", obs[0].Content)
	}
}

func TestDocumentScanFindsKnownKindsOnly(t *testing.T) {
	root := t.TempDir()

	for _, f := range []string{"a.pdf", "b.docx", "c.md", "d.xlsx", "e.exe", "f.bin", "g.odt"} {
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}

	found, err := ScanDocuments(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 5 {
		t.Fatalf("found %d documents, want 5: %+v", len(found), found)
	}

	for _, d := range found {
		if d.Kind == "" {
			t.Errorf("%s has no kind", d.Name)
		}
	}
}

func TestDocumentScanSkipsHiddenAndVendorDirectories(t *testing.T) {
	root := t.TempDir()

	os.MkdirAll(filepath.Join(root, "node_modules"), 0o755)
	os.MkdirAll(filepath.Join(root, ".cache"), 0o755)
	os.WriteFile(filepath.Join(root, "node_modules", "readme.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, ".cache", "notes.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "real.md"), []byte("x"), 0o644)

	found, _ := ScanDocuments(root)

	if len(found) != 1 || found[0].Name != "real.md" {
		t.Fatalf("found %+v, want just real.md", found)
	}
}
