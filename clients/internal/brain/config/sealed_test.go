package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
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

/*
 * A file written by hand, before the first start, reads back in the clear.
 *
 * The first start seals the personal lines of such a file in place and only
 * then loads it. The file has no "opened" marker yet, so loading opens it up
 * and reads it again; that second read kept the sealed text, and the owner
 * was greeted as "pnsealed1:..." until the next start (found 10.11.2026 with
 * BRAIN_OWNER=Sam). Every personal setting the program reads is checked, and
 * no setting of any kind may come back as ciphertext.
 */
func TestAHandWrittenFileSealedOnTheFirstStartReadsBackInTheClear(t *testing.T) {
	plain := map[string]string{
		"BRAIN_OWNER":          "Sam Marker",
		"MAIL_USER":            "sam-user-marker",
		"MAIL_PASSWORD":        "hunter2-marker",
		"MAIL_FROM":            "sam-from-marker",
		"BRAVE_SEARCH_KEY":     "brave-marker",
		"HOME_ASSISTANT_TOKEN": "ha-token-marker",
	}

	for _, svc := range llm.Services() {
		if svc.NeedsKey {
			plain[llm.KeySetting(svc.ID)] = svc.ID + "-key-marker"
		}
	}

	for key := range plain {
		if !Personal(key) {
			t.Fatalf("%s is expected to be personal", key)
		}

		t.Setenv(key, "")
	}

	k, _ := vault.Generate()
	vault.Use(k)
	t.Cleanup(func() { vault.Use(nil) })

	root := t.TempDir()

	var text strings.Builder
	text.WriteString("# written by hand\nBRAIN_NAME=Ariel\n")

	for key, value := range plain {
		text.WriteString(key + "=" + value + "\n")
	}

	if err := os.WriteFile(Path(root), []byte(text.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	// The first start's order: seal the file in place, then load it.
	if err := SealFile(Path(root)); err != nil {
		t.Fatal(err)
	}

	for start := 1; start <= 2; start++ {
		got, err := Load(root)
		if err != nil {
			t.Fatalf("start %d: %v", start, err)
		}

		want := map[string]string{
			"BRAIN_OWNER":          got.Owner,
			"MAIL_USER":            got.MailUser,
			"MAIL_PASSWORD":        got.MailPassword,
			"MAIL_FROM":            got.MailFrom,
			"BRAVE_SEARCH_KEY":     got.BraveKey,
			"HOME_ASSISTANT_TOKEN": got.HomeAssistantToken,
		}

		for _, svc := range llm.Services() {
			if svc.NeedsKey {
				want[llm.KeySetting(svc.ID)] = got.ProviderKeys[svc.ID]
			}
		}

		for key, value := range plain {
			if want[key] != value {
				t.Errorf("start %d: %s reads %q, want %q", start, key, want[key], value)
			}
		}

		if got.Name != "Ariel" {
			t.Errorf("start %d: BRAIN_NAME reads %q", start, got.Name)
		}

		noCiphertextIn(t, fmt.Sprintf("start %d", start), got)
	}

	raw, _ := os.ReadFile(Path(root))

	for key, value := range plain {
		if strings.Contains(string(raw), value) {
			t.Errorf("%s is in the settings file in the clear", key)
		}
	}
}

// A sealed value whose key is not here reads as unset on the first start
// too, never as ciphertext.
func TestASealedFileWithoutItsKeyReadsAsUnsetOnTheFirstStart(t *testing.T) {
	t.Setenv("BRAIN_OWNER", "")
	t.Setenv("MAIL_PASSWORD", "")

	k, _ := vault.Generate()
	vault.Use(k)
	t.Cleanup(func() { vault.Use(nil) })

	root := t.TempDir()
	os.WriteFile(Path(root), []byte("BRAIN_OWNER=Sam Marker\nMAIL_PASSWORD=hunter2-marker\n"), 0o600)

	if err := SealFile(Path(root)); err != nil {
		t.Fatal(err)
	}

	vault.Use(nil)

	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if got.Owner != "" || got.MailPassword != "" {
		t.Errorf("without the key: %q %q", got.Owner, got.MailPassword)
	}

	noCiphertextIn(t, "without the key", got)
}

// noCiphertextIn fails when any text setting, or any provider key, still
// carries the sealed prefix.
func noCiphertextIn(t *testing.T, when string, cfg Config) {
	t.Helper()

	v := reflect.ValueOf(cfg)

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)

		switch f.Kind() {
		case reflect.String:
			if vault.IsSealedString(f.String()) {
				t.Errorf("%s: %s is ciphertext", when, v.Type().Field(i).Name)
			}
		case reflect.Map:
			if f.Type().Elem().Kind() != reflect.String {
				continue
			}

			for _, key := range f.MapKeys() {
				if vault.IsSealedString(f.MapIndex(key).String()) {
					t.Errorf("%s: %s[%v] is ciphertext", when, v.Type().Field(i).Name, key)
				}
			}
		}
	}
}
