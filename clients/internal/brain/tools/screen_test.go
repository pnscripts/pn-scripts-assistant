package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

/*
 * The picture has to reach a model that can actually see it.
 *
 * Taking a screenshot is the easy half and worth nothing alone: the model
 * holding the conversation reads text, so handing it an image gives a file path
 * and a shrug. What matters is that the bytes go to a model that takes images,
 * as an image, and that the answer comes back as words.
 */
func TestThePictureIsSentAsAnImage(t *testing.T) {
	var got struct {
		Model  string   `json:"model"`
		Prompt string   `json:"prompt"`
		Images []string `json:"images"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{
					{"name": "qwen2.5-coder:7b"},
					{"name": "gemma3:4b"},
				},
			})
		case "/api/generate":
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"response": "A terminal showing a failed build.",
			})
		}
	}))

	defer server.Close()

	look := LookAtScreen{OllamaURL: server.URL}

	answer, err := look.ask(context.Background(), "gemma3:4b",
		"what does the error say", []byte{0x89, 'P', 'N', 'G'})
	if err != nil {
		t.Fatal(err)
	}

	if answer != "A terminal showing a failed build." {
		t.Errorf("the answer did not come back: %q", answer)
	}

	if len(got.Images) != 1 {
		t.Fatalf("the picture was not sent: %+v", got)
	}

	raw, err := base64.StdEncoding.DecodeString(got.Images[0])
	if err != nil {
		t.Fatalf("the picture was not encoded for the model: %v", err)
	}

	if string(raw) != "\x89PNG" {
		t.Errorf("the bytes changed on the way: %q", raw)
	}

	if got.Prompt != "what does the error say" {
		t.Errorf("the question was lost: %q", got.Prompt)
	}
}

/*
 * A machine with nothing that can see says so.
 *
 * The alternative is asking a text model to look at an image, which answers
 * confidently about a picture it never received — the worst possible outcome,
 * because it is indistinguishable from working.
 */
func TestNoModelThatCanSeeIsSaidPlainly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{{"name": "qwen2.5-coder:7b"}},
		})
	}))

	defer server.Close()

	_, err := (LookAtScreen{OllamaURL: server.URL}).visionModel(context.Background())
	if err == nil {
		t.Fatal("a text-only machine was told it could look at pictures")
	}

	if !strings.Contains(err.Error(), "ollama pull") {
		t.Errorf("the error does not say how to fix it: %v", err)
	}
}

// The best installed model wins, rather than the first one listed.
func TestTheCheaperEyeIsPreferred(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{
				{"name": "moondream:latest"},
				{"name": "gemma3:12b"},
				{"name": "gemma3:4b"},
			},
		})
	}))

	defer server.Close()

	model, err := (LookAtScreen{OllamaURL: server.URL}).visionModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The small one, not the big one: on a processor, a twelve-billion model
	// loaded to read a screenshot evicts the model holding the conversation.
	if model != "gemma3:4b" {
		t.Errorf("picked %q rather than the small model that costs least", model)
	}
}
