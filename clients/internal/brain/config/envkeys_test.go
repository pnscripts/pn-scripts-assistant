package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * Every key, token and password the settings file can hold follows the rule
 * the Anthropic key got in 1.1.0 step 8: a value that is only in the
 * environment is used for this run and never written down, never replaces
 * what the file holds, and only a value its person typed into the assistant
 * is saved.
 */

// secretSettings is every secret the settings file can hold, by its name.
func secretSettings() []string {
	out := []string{"MAIL_PASSWORD", "CALDAV_PASSWORD", "BRAVE_SEARCH_KEY", "HOME_ASSISTANT_TOKEN"}

	for _, svc := range llm.Services() {
		out = append(out, llm.KeySetting(svc.ID))
	}

	return out
}

// valueOf is what the loaded settings hold for one secret.
func valueOf(c Config, setting string) string {
	switch setting {
	case "MAIL_PASSWORD":
		return c.MailPassword
	case "CALDAV_PASSWORD":
		return c.CalendarPassword
	case "BRAVE_SEARCH_KEY":
		return c.BraveKey
	case "HOME_ASSISTANT_TOKEN":
		return c.HomeAssistantToken
	}

	for _, svc := range llm.Services() {
		if setting == llm.KeySetting(svc.ID) {
			return c.ProviderKeys[svc.ID]
		}
	}

	return "?"
}

func clearSecretsFromTheEnvironment(t *testing.T) {
	t.Helper()

	for _, s := range secretSettings() {
		t.Setenv(s, "")
	}
}

func TestASecretOnlyInTheEnvironmentIsNeverWrittenDown(t *testing.T) {
	for _, setting := range secretSettings() {
		t.Run(setting, func(t *testing.T) {
			clearSecretsFromTheEnvironment(t)

			root := t.TempDir()

			if err := Default().Save(root); err != nil {
				t.Fatal(err)
			}

			const fromTheShell = "zz-env-only-value-7731"

			t.Setenv(setting, fromTheShell)

			cfg, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			// Still used for this run: the environment wins over the file.
			if valueOf(cfg, setting) != fromTheShell {
				t.Fatalf("the environment no longer reaches the program: %q", valueOf(cfg, setting))
			}

			if err := cfg.Save(root); err != nil {
				t.Fatal(err)
			}

			if raw, _ := os.ReadFile(filepath.Join(root, FileName)); strings.Contains(string(raw), fromTheShell) {
				t.Fatalf("a value only in the environment was written into the settings:\n%s", raw)
			}

			t.Setenv(setting, "")

			later, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			if v := valueOf(later, setting); v != "" {
				t.Errorf("after the environment went, the settings still hold %q", v)
			}
		})
	}
}

func TestTheEnvironmentNeverReplacesWhatTheFileHolds(t *testing.T) {
	for _, setting := range secretSettings() {
		t.Run(setting, func(t *testing.T) {
			clearSecretsFromTheEnvironment(t)

			root := t.TempDir()
			path := filepath.Join(root, FileName)

			const typed = "zz-typed-into-setup-4410"

			if err := os.WriteFile(path, []byte(setting+"="+typed+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			t.Setenv(setting, "zz-other-in-the-shell-9902")

			cfg, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			if valueOf(cfg, setting) != "zz-other-in-the-shell-9902" {
				t.Fatalf("the environment no longer wins for this run: %q", valueOf(cfg, setting))
			}

			if err := cfg.Save(root); err != nil {
				t.Fatal(err)
			}

			raw, _ := os.ReadFile(path)
			if strings.Contains(string(raw), "zz-other-in-the-shell-9902") || !strings.Contains(string(raw), setting+"="+typed+"\n") {
				t.Fatalf("saving replaced the file's own value:\n%s", raw)
			}

			t.Setenv(setting, "")

			later, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			if valueOf(later, setting) != typed {
				t.Errorf("after the environment went: %q, want the typed value", valueOf(later, setting))
			}
		})
	}
}

/*
 * Typed into the assistant while the environment holds a value: what was
 * typed is saved, even when it is the same as the environment's (somebody
 * who pastes the key from their shell into the Models page wants it kept
 * for when the program starts from the menu).
 */
func TestASecretTypedIntoTheAssistantIsSaved(t *testing.T) {
	for _, setting := range secretSettings() {
		t.Run(setting, func(t *testing.T) {
			clearSecretsFromTheEnvironment(t)

			root := t.TempDir()

			if err := Default().Save(root); err != nil {
				t.Fatal(err)
			}

			const same = "zz-same-as-the-shell-1188"

			t.Setenv(setting, same)

			cfg, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			cfg.TypeSecret(setting, same)

			if err := cfg.Save(root); err != nil {
				t.Fatal(err)
			}

			t.Setenv(setting, "")

			later, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			if valueOf(later, setting) != same {
				t.Fatalf("typed, not saved: %q", valueOf(later, setting))
			}

			// Typed again, something else, with the environment back.
			t.Setenv(setting, same)

			later.TypeSecret(setting, "zz-typed-later-5521")

			if valueOf(later, setting) != "zz-typed-later-5521" {
				t.Errorf("typing did not change what this run uses: %q", valueOf(later, setting))
			}

			if err := later.Save(root); err != nil {
				t.Fatal(err)
			}

			t.Setenv(setting, "")

			again, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			if valueOf(again, setting) != "zz-typed-later-5521" {
				t.Errorf("the second typed value was not saved: %q", valueOf(again, setting))
			}

			// Forgotten while the environment holds one: the file is emptied.
			t.Setenv(setting, same)

			again.TypeSecret(setting, "")

			if err := again.Save(root); err != nil {
				t.Fatal(err)
			}

			t.Setenv(setting, "")

			gone, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			if v := valueOf(gone, setting); v != "" {
				t.Errorf("forgotten, still saved: %q", v)
			}
		})
	}
}

/*
 * Every company's key, model and address survives a save. The settings
 * file was rewritten with only three companies' lines, so a Gemini or Groq
 * key typed in setup, or the address of one's own server, was gone after
 * the first change of any setting.
 */
func TestEveryProvidersSettingsSurviveASave(t *testing.T) {
	clearSecretsFromTheEnvironment(t)

	root := t.TempDir()
	path := filepath.Join(root, FileName)

	var text strings.Builder

	want := map[string][3]string{}

	for _, svc := range llm.Services() {
		key, model, url := "zz-key-"+svc.ID, "zz-model-"+svc.ID, ""
		if svc.ID == "custom" {
			url = "http://127.0.0.1:8000/v1"
		}

		t.Setenv(llm.ModelSetting(svc.ID), "")
		t.Setenv(llm.BaseURLSetting(svc.ID), "")

		text.WriteString(llm.KeySetting(svc.ID) + "=" + key + "\n")
		text.WriteString(llm.ModelSetting(svc.ID) + "=" + model + "\n")

		if url != "" {
			text.WriteString(llm.BaseURLSetting(svc.ID) + "=" + url + "\n")
		}

		want[svc.ID] = [3]string{key, model, url}
	}

	if err := os.WriteFile(path, []byte(text.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	for id, w := range want {
		got := [3]string{again.ProviderKeys[id], again.ProviderModels[id], again.ProviderURLs[id]}
		if got != w {
			t.Errorf("%s after a save: %q, want %q", id, got, w)
		}
	}
}

/*
 * A key typed into a running program is the one its providers are built
 * from: the named fields and the list by company are the same key, and a
 * copy of the settings that types a key does not change the settings it was
 * copied from (the server types into a copy and keeps it only once saved).
 */
func TestATypedKeyIsTheProvidersKeyAndOnlyInTheCopyThatTypedIt(t *testing.T) {
	clearSecretsFromTheEnvironment(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("OPENAI_API_KEY=zz-before\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	original, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	cfg := original

	cfg.TypeSecret("OPENAI_API_KEY", "zz-openai-typed")
	cfg.TypeSecret("OPENROUTER_API_KEY", "zz-openrouter-typed")
	cfg.TypeSecret("ANTHROPIC_API_KEY", "zz-anthropic-typed")
	cfg.TypeSecret("GROQ_API_KEY", "zz-groq-typed")

	for id, want := range map[string]string{"openai": "zz-openai-typed", "openrouter": "zz-openrouter-typed",
		"anthropic": "zz-anthropic-typed", "groq": "zz-groq-typed"} {
		if cfg.ProviderKeys[id] != want {
			t.Errorf("%s: providers are built from %q, want %q", id, cfg.ProviderKeys[id], want)
		}
	}

	if cfg.OpenAIKey != "zz-openai-typed" || cfg.OpenRouterKey != "zz-openrouter-typed" || cfg.AnthropicKey != "zz-anthropic-typed" {
		t.Errorf("named keys: %q %q %q", cfg.OpenAIKey, cfg.OpenRouterKey, cfg.AnthropicKey)
	}

	if cfg.ChosenAnthropicKey() != "zz-anthropic-typed" {
		t.Errorf("an Anthropic key typed this way is not the chosen one: %q", cfg.ChosenAnthropicKey())
	}

	if original.ProviderKeys["openai"] != "zz-before" || original.ProviderKeys["groq"] != "" || original.OpenAIKey != "zz-before" {
		t.Errorf("typing into a copy changed the original: %q %q", original.ProviderKeys["openai"], original.ProviderKeys["groq"])
	}

	cfg.ChooseModel("openai", "zz-model")
	cfg.ChooseModel("anthropic", "zz-claude")

	if cfg.OpenAIModel != "zz-model" || cfg.ProviderModels["openai"] != "zz-model" ||
		cfg.AnthropicModel != "zz-claude" || cfg.ProviderModels["anthropic"] != "zz-claude" {
		t.Errorf("models: %q %q %q %q", cfg.OpenAIModel, cfg.ProviderModels["openai"], cfg.AnthropicModel, cfg.ProviderModels["anthropic"])
	}

	if original.ProviderModels["openai"] != "" {
		t.Errorf("choosing a model in a copy changed the original: %q", original.ProviderModels["openai"])
	}
}
