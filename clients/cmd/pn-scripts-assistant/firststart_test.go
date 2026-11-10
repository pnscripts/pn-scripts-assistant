package main

import (
	"os"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * The first start, in the order serve takes it: the key is made, whatever is
 * in the clear is sealed in place, and only then are the settings loaded.
 *
 * A settings file written by hand before that start (BRAIN_OWNER=Sam) came
 * back as "pnsealed1:..." on the first start and as "Sam" after a restart
 * (10.11.2026); the status, the CLI greeting and the page all showed the
 * ciphertext.
 */
func TestTheFirstStartReadsAHandWrittenOwnerInTheClear(t *testing.T) {
	t.Setenv("BRAIN_OWNER", "")
	t.Setenv("OPENAI_API_KEY", "")

	k, err := vault.Generate()
	if err != nil {
		t.Fatal(err)
	}

	vault.Use(k)
	t.Cleanup(func() { vault.Use(nil) })

	root := t.TempDir()

	if err := os.WriteFile(config.Path(root), []byte("BRAIN_OWNER=Sam\nOPENAI_API_KEY=sk-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := sealTheRest(root, k); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(config.Path(root))
	if strings.Contains(string(raw), "=Sam") || strings.Contains(string(raw), "sk-marker") {
		t.Fatalf("not sealed on the first start:\n%s", raw)
	}

	for _, start := range []string{"first start", "restart"} {
		cfg, err := config.Load(root)
		if err != nil {
			t.Fatalf("%s: %v", start, err)
		}

		if cfg.Owner != "Sam" || cfg.OpenAIKey != "sk-marker" {
			t.Errorf("%s: owner %q, key %q", start, cfg.Owner, cfg.OpenAIKey)
		}
	}
}
