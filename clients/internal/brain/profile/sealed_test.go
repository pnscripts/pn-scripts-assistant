package profile

import (
	"os"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/vault"
)

func TestTheProfileIsSealedOnDisk(t *testing.T) {
	k, _ := vault.Generate()
	vault.Use(k)
	t.Cleanup(func() { vault.Use(nil) })

	root := t.TempDir()

	if err := Write(root, "I run a studio and my bank is the MARKER bank"); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(Path(root))
	if strings.Contains(string(raw), "MARKER") {
		t.Fatal("the profile is on disk in the clear")
	}

	if got, err := Read(root); err != nil || !strings.Contains(got, "MARKER bank") {
		t.Errorf("read back: %q %v", got, err)
	}
}
