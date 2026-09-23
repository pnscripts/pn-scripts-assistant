package llm

import (
	"context"
	"strings"
	"testing"
)

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

/*
 * The one dependency this program cannot work without says what to do when it
 * is not there.
 *
 * "connection refused" is accurate and useless. The address is in the message
 * because it is configurable, and half of these are a brain pointed at the
 * wrong one.
 */
func TestItSaysWhatToDoWhenOllamaIsNotThere(t *testing.T) {
	o := NewOllama("http://127.0.0.1:1", "qwen", "nomic")

	_, err := o.Chat(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}})
	if err == nil {
		t.Fatal("something answered on a port nothing listens on")
	}

	for _, want := range []string{"Ollama is not answering", "http://127.0.0.1:1", "ollama serve", "OLLAMA_BASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message does not say %q:\n%s", want, err)
		}
	}
}
