package models

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A model its owner's policy does not allow is not fetched at start, and
// the reason is said; one it allows is.
func TestStartUpFetchesOnlyWhatThePolicyAllows(t *testing.T) {
	var pulled []string

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Write([]byte(`{"models":[]}`))
		case "/api/pull":
			buf := make([]byte, 512)
			n, _ := r.Body.Read(buf)
			pulled = append(pulled, string(buf[:n]))
			w.Write([]byte(`{"status":"success"}` + "\n"))
		}
	}))
	defer ollama.Close()

	allow := func(name string) (bool, string) {
		if strings.Contains(name, "7b") {
			return false, "it is 4.7 GB, more than the 1.2 GB your policy allows without asking"
		}

		return true, ""
	}

	var said []string

	EnsureAllowed(context.Background(), New(ollama.URL), "qwen2.5-coder:7b", "nomic-embed-text", allow,
		func(s string) { said = append(said, s) })

	joined := strings.Join(pulled, "\n")

	if strings.Contains(joined, "qwen2.5-coder:7b") {
		t.Error("a model the policy refused was fetched")
	}

	if !strings.Contains(joined, "nomic-embed-text") {
		t.Error("a model the policy allowed was not fetched")
	}

	if !strings.Contains(strings.Join(said, "\n"), "1.2 GB") {
		t.Errorf("the refusal was not said: %v", said)
	}
}
