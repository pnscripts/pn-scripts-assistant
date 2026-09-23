package preflight

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

/*
 * Ollama answering is Ollama being here.
 *
 * Every check used to look for the `ollama` command, so a machine where it
 * runs as a system service, or lives in a home directory this program cannot
 * see, or answers on another machine through OLLAMA_BASE_URL, was told Ollama
 * was missing — and offered a 1.5 GB download while a perfectly good one
 * answered on the port the assistant talks to. Found by launching the
 * packaged program with a PATH that did not have the command on it.
 */
func TestOllamaCountsWhenItAnswersWithoutItsCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)

			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{
				{"name": "qwen2.5-coder:7b"},
				{"name": "nomic-embed-text:latest"},
			},
		})
	}))
	defer server.Close()

	t.Setenv("OLLAMA_BASE_URL", server.URL)
	t.Setenv("PATH", t.TempDir()) // no ollama command anywhere

	if !ollamaAnswering() {
		t.Fatal("it did not see an Ollama that answered")
	}

	if !ollamaHere() {
		t.Error("Ollama answering is not counted as Ollama being here")
	}

	if got := aChatModelHere(); got != "qwen2.5-coder:7b" {
		t.Errorf("the chat model it found is %q", got)
	}

	if !ollamaHasModel("nomic-embed-text") {
		t.Error("it did not find the embedding model the service listed")
	}

	// And the requirements themselves, which is what setup asks.
	for _, result := range Check() {
		switch result.Requirement.Name {
		case "Ollama", "Chat model", "Embedding model":
			if result.State != OK {
				t.Errorf("%s: %s (%s)", result.Requirement.Name,
					result.State.Label(), result.Detail)
			}
		}
	}
}

// Nothing answering and no command is still missing: the point is not to
// declare everything fine, it is to ask the right question.
func TestNothingAnsweringIsStillMissing(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("PATH", t.TempDir())

	if ollamaHere() {
		t.Fatal("it said Ollama was here with nothing answering and no command")
	}

	for _, result := range Check() {
		switch result.Requirement.Name {
		case "Ollama":
			if result.State != Missing {
				t.Errorf("Ollama is %s, not missing", result.State.Label())
			}
		case "Chat model", "Embedding model":
			if result.State != Unknown {
				t.Errorf("%s is %s; without Ollama it cannot be known",
					result.Requirement.Name, result.State.Label())
			}
		}
	}
}
