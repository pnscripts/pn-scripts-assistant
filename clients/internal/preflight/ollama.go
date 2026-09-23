package preflight

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

/*
 * Asking Ollama whether it is there, rather than looking for its command.
 *
 * Every one of these checks used to shell out to `ollama`, and a machine
 * without that command on its PATH was told Ollama was missing and offered a
 * 1.5 GB download — while a perfectly good Ollama answered on the port the
 * program talks to. Three ordinary ways to end up in that state: it is
 * installed as a system service and the CLI is not on this user's PATH, it
 * lives in a home directory the program cannot see, or OLLAMA_BASE_URL points
 * at another machine entirely.
 *
 * The program only ever talks to Ollama over HTTP. So that is the question:
 * is something answering, and does it have the models. The command still
 * matters for installing one — `ollama pull` — and that is the only thing it
 * is asked for now.
 */

// HowLongToAsk bounds the question. Local and already running, or not.
const HowLongToAsk = 2 * time.Second

// OllamaURL is where Ollama should be answering.
func OllamaURL() string {
	if set := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL")); set != "" {
		return strings.TrimSuffix(set, "/")
	}

	return "http://127.0.0.1:11434"
}

// ollamaAnswering reports whether something is there to talk to.
func ollamaAnswering() bool {
	_, ok := ollamaModels()

	return ok
}

// ollamaModels is what it has, and whether it answered at all. The two are
// different: no models is a machine that needs one, and no answer is a
// machine that needs Ollama.
func ollamaModels() ([]string, bool) {
	client := &http.Client{Timeout: HowLongToAsk}

	resp, err := client.Get(OllamaURL() + "/api/tags")
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false
	}

	names := make([]string, 0, len(body.Models))

	for _, model := range body.Models {
		if model.Name != "" {
			names = append(names, model.Name)
		}
	}

	return names, true
}

// ollamaHere reports whether Ollama can be used at all: the command, or
// something answering on the address.
func ollamaHere() bool { return commandExists("ollama") || ollamaAnswering() }
