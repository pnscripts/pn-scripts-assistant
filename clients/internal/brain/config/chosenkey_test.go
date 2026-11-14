package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * The Anthropic key the assistant may give to "ask Claude" is only one its
 * person typed into the assistant (setup or the Models page). A key that is
 * only in the environment is somebody's shell, a CI runner, another tool's
 * setup: never chosen for this, and never used for it.
 */
func TestAKeyTypedIntoTheAssistantIsTheChosenOne(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	root := t.TempDir()

	cfg := Default()
	cfg.TypeAnthropicKey("sk-ant-typed-1")

	if cfg.ChosenAnthropicKey() != "sk-ant-typed-1" || cfg.AnthropicKey != "sk-ant-typed-1" {
		t.Fatalf("typed, not chosen: %q / %q", cfg.ChosenAnthropicKey(), cfg.AnthropicKey)
	}

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(filepath.Join(root, FileName))
	if !strings.Contains(string(raw), KeyTypedSetting+"=") {
		t.Fatalf("the mark is not written down:\n%s", raw)
	}

	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if again.ChosenAnthropicKey() != "sk-ant-typed-1" {
		t.Errorf("after a restart the typed key is not the chosen one: %q", again.ChosenAnthropicKey())
	}

	// Forgotten: nothing is chosen.
	again.TypeAnthropicKey("")

	if again.ChosenAnthropicKey() != "" || again.AnthropicKey != "" {
		t.Errorf("forgotten, still chosen: %q", again.ChosenAnthropicKey())
	}
}

func TestAKeyOnlyInTheEnvironmentIsNeverChosen(t *testing.T) {
	root := t.TempDir()

	if err := Default().Save(root); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-from-the-shell")

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.AnthropicKey != "sk-ant-from-the-shell" {
		t.Fatalf("the environment no longer reaches the model provider: %q", cfg.AnthropicKey)
	}

	if cfg.ChosenAnthropicKey() != "" {
		t.Errorf("a key only in the environment is chosen: %q", cfg.ChosenAnthropicKey())
	}

	/*
	 * Saving the settings keeps the file's own key, not the environment's:
	 * a one-off override is not written down, and so cannot become chosen
	 * either once the environment is gone.
	 */
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	if raw, _ := os.ReadFile(filepath.Join(root, FileName)); strings.Contains(string(raw), "sk-ant-from-the-shell") {
		t.Fatalf("the environment's key was written into the settings:\n%s", raw)
	}

	t.Setenv("ANTHROPIC_API_KEY", "")

	later, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if later.AnthropicKey != "" || later.ChosenAnthropicKey() != "" {
		t.Errorf("after the environment went: key %q, chosen %q", later.AnthropicKey, later.ChosenAnthropicKey())
	}

	// A key the file holds without the mark (an older settings file, or one
	// written by hand) is the provider's, but not chosen for Claude.
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("ANTHROPIC_API_KEY=sk-ant-by-hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	byHand, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if byHand.AnthropicKey != "sk-ant-by-hand" || byHand.ChosenAnthropicKey() != "" {
		t.Errorf("an unmarked key: provider %q, chosen %q", byHand.AnthropicKey, byHand.ChosenAnthropicKey())
	}
}

// Typed in the assistant, and another key in the environment: the typed one
// is the chosen one, whatever the environment says.
func TestTheEnvironmentDoesNotReplaceTheChosenKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	root := t.TempDir()

	cfg := Default()
	cfg.TypeAnthropicKey("sk-ant-typed-2")

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-other")

	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if again.ChosenAnthropicKey() != "sk-ant-typed-2" || again.AnthropicKey != "sk-ant-other" {
		t.Errorf("chosen %q (want the typed key), provider %q (want the environment's)", again.ChosenAnthropicKey(), again.AnthropicKey)
	}

	// Saved again while the environment overrides it: the typed key stays.
	if err := again.Save(root); err != nil {
		t.Fatal(err)
	}

	// A mark in the file for another key than the file holds: not chosen.
	path := filepath.Join(root, FileName)
	raw, _ := os.ReadFile(path)
	edited := strings.Replace(string(raw), "ANTHROPIC_API_KEY=sk-ant-typed-2", "ANTHROPIC_API_KEY=sk-ant-edited", 1)

	if edited == string(raw) {
		t.Fatalf("the key line is not where expected:\n%s", raw)
	}

	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ANTHROPIC_API_KEY", "")

	changed, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if changed.ChosenAnthropicKey() != "" {
		t.Errorf("a key written into the file by something else is chosen: %q", changed.ChosenAnthropicKey())
	}
}

// The mark tells nothing about the key: a fingerprint, not the key.
func TestTheMarkIsNotTheKey(t *testing.T) {
	mark := KeyPrint("sk-ant-secret-value")

	if mark == "" || strings.Contains(mark, "secret") || len(mark) != 32 || KeyPrint("") != "" {
		t.Errorf("mark %q", mark)
	}

	if KeyPrint("sk-ant-secret-value") != mark || KeyPrint("sk-ant-secret-valuf") == mark {
		t.Error("the mark is not a fingerprint")
	}
}
