// Package models manages the local models the brain runs on.
//
// Two jobs. Making sure the models it needs are actually installed, on the
// first run and every run after — a brain whose embedding model has been
// deleted cannot remember anything, and the honest thing is to fetch it rather
// than to fail quietly. And measuring the ones that are installed, because
// which model to use is a real decision and it should be made from numbers
// taken on this machine rather than from what a model is said to be good at.
//
// The measurement is small on purpose: one question, one tool, and the two
// things that decide whether a model is usable here. How long it takes, since
// on a processor without a graphics card that is the whole experience of using
// the brain. And whether it asks for the tool properly or writes the call out
// as prose — the loop copes with prose, but a model that does it is a model
// that will also, sometimes, write something that cannot be recovered.
package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Model is one model Ollama has.
type Model struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Size  string `json:"size"`
}

// Measurement is what one model did when asked to use a tool.
type Measurement struct {
	Model   string  `json:"model"`
	Seconds float64 `json:"seconds"`
	// "proper" when the model returned a real tool call, "text" when it wrote
	// the call out as prose, "none" when it did not ask for the tool at all.
	ToolCall string `json:"tool_call"`
	Note     string `json:"note"`
}

// Client talks to Ollama.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New builds a client. The timeout is long because these are slow operations on
// a processor: a pull is a download and a measurement runs a model.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}

	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Minute},
	}
}

// List reports the models Ollama has.
func (c *Client) List(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var out struct {
		Models []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"models"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}

	list := make([]Model, 0, len(out.Models))

	for _, m := range out.Models {
		list = append(list, Model{Name: m.Name, Bytes: m.Size, Size: readableSize(m.Size)})
	}

	return list, nil
}

// Has reports whether a model is installed.
//
// Matches on the tag as well as without it, because "llama3.2:3b" and
// "llama3.2:3b" are the same thing while "nomic-embed-text" is stored as
// "nomic-embed-text:latest" — and a check that misses that would pull a model
// that is already there on every single start.
func (c *Client) Has(ctx context.Context, name string) bool {
	list, err := c.List(ctx)
	if err != nil {
		return false
	}

	for _, m := range list {
		if m.Name == name || strings.TrimSuffix(m.Name, ":latest") == strings.TrimSuffix(name, ":latest") {
			return true
		}
	}

	return false
}

// Pull downloads a model, reporting what it is doing as it goes.
//
// Ollama streams progress as a line of JSON per update. The callback is given
// something a person can read rather than the raw status, because "pulling
// 3.2GB, 41%" answers the question somebody staring at it actually has.
func (c *Client) Pull(ctx context.Context, name string, onProgress func(string)) error {
	body, err := json.Marshal(map[string]any{"name": name, "stream": true})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var update struct {
			Status    string `json:"status"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}

		if err := json.Unmarshal(scanner.Bytes(), &update); err != nil {
			continue
		}

		if update.Error != "" {
			return fmt.Errorf("pulling %s: %s", name, update.Error)
		}

		if onProgress == nil {
			continue
		}

		if update.Total > 0 && update.Completed > 0 {
			onProgress(fmt.Sprintf("Downloading %s — %s of %s",
				name, readableSize(update.Completed), readableSize(update.Total)))

			continue
		}

		onProgress(fmt.Sprintf("%s — %s", name, update.Status))
	}

	return scanner.Err()
}

// probe is the question every model is measured with.
//
// One tool and one unambiguous instruction. Anything longer measures how well a
// model follows a long prompt, which is a different question.
const probe = "List the files in /tmp. Use your tools."

// Measure times one model and sees how it asks for a tool.
func (c *Client) Measure(ctx context.Context, name string) (Measurement, error) {
	body, err := json.Marshal(map[string]any{
		"model":    name,
		"stream":   false,
		"messages": []map[string]string{{"role": "user", "content": probe}},
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        "list_directory",
				"description": "List the files in a directory on this machine.",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"path": map[string]any{"type": "string"}},
					"required":   []string{"path"},
				},
			},
		}},
	})
	if err != nil {
		return Measurement{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Measurement{}, err
	}

	req.Header.Set("Content-Type", "application/json")

	started := time.Now()

	res, err := c.HTTP.Do(req)
	if err != nil {
		return Measurement{}, err
	}
	defer res.Body.Close()

	var out struct {
		Message struct {
			Content   string            `json:"content"`
			ToolCalls []json.RawMessage `json:"tool_calls"`
		} `json:"message"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return Measurement{}, err
	}

	if out.Error != "" {
		return Measurement{}, fmt.Errorf("%s: %s", name, out.Error)
	}

	m := Measurement{Model: name, Seconds: time.Since(started).Seconds()}

	switch {
	case len(out.Message.ToolCalls) > 0:
		m.ToolCall = "proper"
		m.Note = "Asks for tools properly."
	case strings.Contains(out.Message.Content, `"name"`):
		m.ToolCall = "text"
		m.Note = "Writes the call out as text. The brain recovers it, but not always."
	default:
		m.ToolCall = "none"
		m.Note = "Did not reach for the tool at all."
	}

	return m, nil
}

func readableSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// Recommended is what to install when there is nothing usable here.
//
// Chosen from measurement on this project's own machine rather than from
// reputation. Asked to use a tool, on four processor cores with no graphics
// acceleration: llama3.2:3b answered in 24 seconds and asked for the tool
// properly; qwen2.5-coder:7b took 41 seconds and wrote the call out as text;
// qwen3 at 8B took 106 seconds. The smallest of the three was both the fastest
// and the only one of the larger pair that used its tools correctly, which is
// the opposite of what picking by parameter count would suggest.
const Recommended = "llama3.2:3b"

// Ensure makes sure the brain has models to run on.
//
// Called at every start, not only the first. A model can be deleted between
// runs, and a brain whose embedding model has gone cannot remember anything —
// it would keep working, answer worse, and never say why.
//
// Returns the model to use for replies, which is the configured one unless
// nothing at all is installed.
func Ensure(ctx context.Context, c *Client, chat, embed string, note func(string)) (string, error) {
	return EnsureAllowed(ctx, c, chat, embed, nil, note)
}

/*
 * EnsureAllowed is Ensure under its owner's rules: allow says whether one
 * model may be fetched without asking — its size, the room left on the disk,
 * whether installing models without asking is allowed at all — and a model it
 * refuses is not fetched, and the reason is said. Nil allows everything, as
 * before there were rules.
 */
func EnsureAllowed(ctx context.Context, c *Client, chat, embed string, allow func(name string) (bool, string),
	note func(string)) (string, error) {
	pull := func(name string) error {
		if allow != nil {
			if ok, why := allow(name); !ok {
				return fmt.Errorf("%s was not fetched: %s", name, why)
			}
		}

		return c.Pull(ctx, name, note)
	}

	installed, err := c.List(ctx)
	if err != nil {
		// Ollama not answering is not something to fix by downloading: it is
		// either not installed or not running, and the interface already says
		// so in a way this cannot improve on.
		return chat, fmt.Errorf("ollama is not answering: %w", err)
	}

	// Recall is embedding. Without this model the brain remembers nothing, so
	// it comes first even though the chat model is the visible one.
	if !has(installed, embed) {
		if note != nil {
			note("Fetching the memory model — the brain cannot recall anything without it")
		}

		if err := pull(embed); err != nil {
			return chat, err
		}
	}

	if has(installed, chat) {
		return chat, nil
	}

	// A configured model that is not here gets fetched: somebody asked for it.
	if chat != "" {
		if note != nil {
			note("Fetching " + chat)
		}

		if err := pull(chat); err == nil {
			return chat, nil
		} else if note != nil {
			note(err.Error())
		}
	}

	// Nothing configured, or the configured one could not be had. If there is
	// anything else installed, use it rather than downloading gigabytes over
	// somebody's connection without being asked.
	for _, m := range installed {
		if !isEmbedding(m.Name) {
			return m.Name, nil
		}
	}

	if note != nil {
		note("No model installed — fetching " + Recommended)
	}

	if err := pull(Recommended); err != nil {
		return chat, err
	}

	return Recommended, nil
}

func has(installed []Model, name string) bool {
	for _, m := range installed {
		if m.Name == name || strings.TrimSuffix(m.Name, ":latest") == strings.TrimSuffix(name, ":latest") {
			return true
		}
	}

	return false
}

// isEmbedding keeps an embedding model from being chosen to hold a
// conversation, which it cannot do.
func isEmbedding(name string) bool {
	lower := strings.ToLower(name)

	return strings.Contains(lower, "embed") || strings.Contains(lower, "bge")
}
