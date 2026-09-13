package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

// Releases compare as numbers: 31.0 is newer than 30.2, and 4.10 than 4.9.
func TestAReleaseIsComparedAsNumbers(t *testing.T) {
	cases := []struct {
		a, b  string
		newer bool
	}{
		{"31_0", "30_2", true},
		{"30_2", "31_0", false},
		{"4_10", "4_9", true},
		{"31_0", "31_0", false},
	}

	for _, c := range cases {
		if got := newerRelease(c.a, c.b); got != c.newer {
			t.Errorf("%s newer than %s: %v, want %v", c.a, c.b, got, c.newer)
		}
	}
}

// Downloaded means a zip is in the folder, and removing takes the folder.
func TestACatalogueIsDownloadedWhenItsZipIsThere(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	check := catalogueCheck("esco")

	if state, _ := check(); state != Missing {
		t.Fatalf("an empty machine has ESCO: %v", state)
	}

	folder := CatalogueFolder("esco")

	os.MkdirAll(folder, 0o755)
	os.WriteFile(filepath.Join(folder, "esco-v1.2.1-en.zip"), []byte("zip"), 0o644)

	if state, detail := check(); state != OK || detail != "esco-v1.2.1-en.zip" {
		t.Errorf("a downloaded zip was not seen: %v %q", state, detail)
	}

	if err := removeCatalogue("esco")(os.Stderr); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Error("removing left the folder behind")
	}
}

// Both classifications' servers may be fetched from, and nothing near them.
func TestTheCatalogueServersAreAllowed(t *testing.T) {
	for _, ok := range []string{escoURL("v1.2.1", "bg"),
		"https://www.onetcenter.org/dl_files/database/db_31_0_text.zip"} {
		if err := allowed(ok); err != nil {
			t.Errorf("%s was refused: %v", ok, err)
		}
	}

	for _, no := range []string{"https://ec.europa.eu.example.com/x.zip", "http://ec.europa.eu/esco/x.zip"} {
		if allowed(no) == nil {
			t.Errorf("%s was allowed", no)
		}
	}
}
