package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * Secrets and who somebody is are sealed in the settings file once the brain
 * has a key; everything else stays readable, and the program reads it all
 * back. Without the key they read as unset, never as ciphertext.
 */
func TestPersonalSettingsAreSealedInTheFile(t *testing.T) {
	t.Setenv("MAIL_PASSWORD", "")
	t.Setenv("BRAIN_OWNER", "")

	k, _ := vault.Generate()
	vault.Use(k)
	t.Cleanup(func() { vault.Use(nil) })

	root := t.TempDir()

	cfg := Default()
	cfg.Owner, cfg.MailPassword, cfg.Name = "Alex Marker", "hunter2-marker", "Ariel"

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(filepath.Join(root, FileName))

	for _, plain := range []string{"Alex Marker", "hunter2-marker"} {
		if strings.Contains(string(raw), plain) {
			t.Errorf("%q is in the settings file in the clear", plain)
		}
	}

	if !strings.Contains(string(raw), "BRAIN_NAME=Ariel") {
		t.Error("an ordinary setting was sealed too")
	}

	got, err := Load(root)
	if err != nil || got.Owner != "Alex Marker" || got.MailPassword != "hunter2-marker" {
		t.Fatalf("read back: %q %q %v", got.Owner, got.MailPassword, err)
	}

	vault.Use(nil)

	if without, _ := Load(root); without.MailPassword != "" || without.Owner != "" {
		t.Errorf("without the key: %q %q", without.Owner, without.MailPassword)
	}
}

func TestAnOlderFileIsSealedInPlace(t *testing.T) {
	k, _ := vault.Generate()
	vault.Use(k)
	t.Cleanup(func() { vault.Use(nil) })

	path := filepath.Join(t.TempDir(), FileName)
	os.WriteFile(path, []byte("# mine\nBRAIN_NAME=Ariel\nOPENAI_API_KEY=sk-marker\n"), 0o600)

	if err := SealFile(path); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "sk-marker") || !strings.Contains(string(raw), "# mine") {
		t.Errorf("after sealing:\n%s", raw)
	}
}
