package llm

import "testing"

/*
 * Only the models that have a thinking mode are told to skip it.
 *
 * Sending "think" to one without it is rejected outright, so a blanket default
 * would break every other model in order to help this one.
 */
func TestOnlyThinkingModelsAreAskedToBeQuiet(t *testing.T) {
	for _, model := range []string{"qwen3:8b", "qwen3", "deepseek-r1:8b", "qwq:32b"} {
		got := quietThinking(model)

		if got == nil {
			t.Errorf("%s thinks out loud and was not asked not to", model)

			continue
		}

		if *got {
			t.Errorf("%s was asked to think, which is the opposite", model)
		}
	}

	for _, model := range []string{"qwen2.5-coder:7b", "llama3.2:3b", "gemma3:4b"} {
		if quietThinking(model) != nil {
			t.Errorf("%s has no thinking mode and would reject the field", model)
		}
	}
}
