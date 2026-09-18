package preflight

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Every pinned file comes from a host this program already fetches from, and
// carries a whole digest of the right length for its algorithm.
func TestEveryPinnedFileIsFetchableAndFullyChecked(t *testing.T) {
	var all []Pinned

	if runtime.GOOS == "linux" {
		if p, err := GodotEngine(); err == nil {
			all = append(all, p)
		}

		if p, err := Node(); err == nil {
			all = append(all, p)
		}
	}

	all = append(all, GodotTemplates())

	for _, p := range all {
		if err := allowed(p.URL); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}

		want := map[string]int{"sha256": 64, "sha512": 128}[p.Algorithm]

		if want == 0 || len(p.Sum) != want {
			t.Errorf("%s: a %s digest of %d characters", p.Name, p.Algorithm, len(p.Sum))
		}

		if p.Licence == "" {
			t.Errorf("%s says nothing about its licence", p.Name)
		}
	}
}

func TestADigestThatDiffersIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")

	if err := os.WriteFile(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// sha256 of "hello\n".
	right := "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"

	if err := Matches(path, "sha256", right); err != nil {
		t.Fatalf("the right digest was refused: %v", err)
	}

	if err := Matches(path, "sha256", "00"+right[2:]); err == nil {
		t.Fatal("a different digest was accepted")
	}

	if err := Matches(path, "md5", right); err == nil {
		t.Fatal("an algorithm nobody should trust was accepted")
	}
}
