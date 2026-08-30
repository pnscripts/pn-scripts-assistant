package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Ollama is a model running on this machine.
//
// Timeouts here are generous on purpose. On a CPU-only machine a 7B model takes
// about a minute to load cold and then answers in seconds. A timeout tuned for
// a hosted API turns that first load into a failure, and the failure looks like
// a broken installation rather than a slow one — which is exactly the mistake
// that once got an entire extension blamed for a bug that was a 15-second
// deadline.
type Ollama struct {
	BaseURL   string
	ChatModel string
	EmbedName string

	// KeepAlive is passed to Ollama with each request; see ollamaChatRequest.
	// Empty means Ollama's own default.
	KeepAlive string

	HTTPClient *http.Client
}

// NewOllama builds a client with defaults suited to local inference.
func NewOllama(baseURL, chatModel, embedModel string) *Ollama {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}

	return &Ollama{
		BaseURL:   baseURL,
		ChatModel: chatModel,
		EmbedName: embedModel,
		// Long enough to survive reading an answer and typing the next
		// question, which is the gap that actually hurts.
		KeepAlive: "30m",
		// No Timeout on the client itself: the deadline belongs to the context,
		// so a caller can allow a long first load and a short health check with
		// the same client.
		HTTPClient: &http.Client{},
	}
}

func (o *Ollama) Name() string { return Local }

func (o *Ollama) EmbedModel() string { return o.EmbedName }

// Available reports whether the daemon is reachable, with a short deadline of
// its own so a health check never inherits a caller's long one.
func (o *Ollama) Available(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []ollamaTool    `json:"tools,omitempty"`

	// Options carries generation settings, chiefly num_predict.
	Options map[string]any `json:"options,omitempty"`

	// KeepAlive is how long Ollama holds the model in memory after answering.
	//
	// The default is five minutes, which is shorter than the gaps between
	// messages in a real conversation. On a machine without a GPU, reloading a
	// 7B model costs about a minute, so a user who pauses to read an answer
	// pays that minute again on their next question — and it presents as the
	// assistant being slow rather than as a cache that expired.
	KeepAlive string `json:"keep_alive,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaToolCall struct {
	Function struct {
		Name string `json:"name"`
		// Ollama returns arguments as an object, not a JSON string.
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type ollamaChatResponse struct {
	Model   string        `json:"model"`
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error"`
}

/*
 * ChatStream answers a turn a piece at a time.
 *
 * The whole point is what can be started before the answer is finished. This
 * machine produces about ten tokens a second, so a three-sentence reply is most
 * of a minute of silence followed by all of it at once — which is not a
 * conversation, it is a form submission. Handed the pieces as they arrive, the
 * voice can begin on the first sentence while the rest is still being written,
 * and the wait before somebody hears anything falls from most of a minute to a
 * few seconds.
 *
 * Streaming is only used where there is nothing to decide: a turn that might
 * call a tool has to be read whole before anything can happen, because a tool
 * call is not speakable and half of one is not anything.
 */
func (o *Ollama) ChatStream(ctx context.Context, req Request, onText func(string)) (Response, error) {
	model := req.Model
	if model == "" {
		model = o.ChatModel
	}

	body := ollamaChatRequest{Model: model, Stream: true, KeepAlive: o.KeepAlive}

	if req.MaxTokens > 0 {
		body.Options = map[string]any{"num_predict": req.MaxTokens}
	}

	for _, m := range req.Messages {
		body.Messages = append(body.Messages, ollamaMessage{Role: m.Role, Content: m.Content})
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.BaseURL+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}

	request.Header.Set("Content-Type", "application/json")

	res, err := o.HTTPClient.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("reaching ollama at %s: %w", o.BaseURL, err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("ollama answered %s", res.Status)
	}

	var whole strings.Builder

	// One JSON object per line, each carrying the next few characters.
	decoder := json.NewDecoder(res.Body)

	for {
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done  bool   `json:"done"`
			Error string `json:"error"`
		}

		if err := decoder.Decode(&chunk); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return Response{}, fmt.Errorf("reading the answer: %w", err)
		}

		if chunk.Error != "" {
			return Response{}, fmt.Errorf("%s", chunk.Error)
		}

		if chunk.Message.Content != "" {
			whole.WriteString(chunk.Message.Content)

			if onText != nil {
				onText(chunk.Message.Content)
			}
		}

		if chunk.Done {
			break
		}
	}

	return Response{
		Content:  whole.String(),
		Provider: "ollama",
		Model:    model,
	}, nil
}

func (o *Ollama) Chat(ctx context.Context, req Request) (Response, error) {
	model := req.Model
	if model == "" {
		model = o.ChatModel
	}

	body := ollamaChatRequest{Model: model, Stream: false, KeepAlive: o.KeepAlive}

	if req.MaxTokens > 0 {
		body.Options = map[string]any{"num_predict": req.MaxTokens}
	}

	for _, m := range req.Messages {
		body.Messages = append(body.Messages, ollamaMessage{Role: m.Role, Content: m.Content})
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	started := time.Now()

	var out ollamaChatResponse

	if err := o.post(ctx, "/api/chat", body, &out); err != nil {
		return Response{}, err
	}

	if out.Error != "" {
		return Response{}, fmt.Errorf("ollama: %s", out.Error)
	}

	resp := Response{
		Content:  out.Message.Content,
		Model:    out.Model,
		Provider: Local,
		Elapsed:  time.Since(started),
	}

	for i, c := range out.Message.ToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      c.Function.Name,
			Arguments: c.Function.Arguments,
		})
	}

	return resp, nil
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`

	// KeepAlive matters more here than it looks.
	//
	// Recall embeds the question before every single reply, and without this
	// the embedding model is loaded with Ollama's default lifetime while the
	// chat model is evicted to make room. The next step then reloads 4.7GB
	// from disk to answer. Measured on this machine: the same question takes
	// 14 seconds when the chat model is already resident and 49 through the
	// brain, and the difference is that reload — paid on every turn.
	KeepAlive string `json:"keep_alive,omitempty"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error"`
}

// Embed turns text into a vector using the local embedding model.
//
// Embedding stays local in every privacy mode. It is applied to the contents of
// a person's disk, so sending it out would leak precisely the material the
// privacy rules exist to keep here — and unlike a chat message, nobody typed it
// with the intention of sending it anywhere.
func (o *Ollama) Embed(ctx context.Context, text string) ([]float32, error) {
	var out ollamaEmbedResponse

	err := o.post(ctx, "/api/embeddings", ollamaEmbedRequest{
		Model:     o.EmbedName,
		Prompt:    text,
		KeepAlive: o.KeepAlive,
	}, &out)
	if err != nil {
		return nil, err
	}

	if out.Error != "" {
		return nil, fmt.Errorf("ollama embeddings: %s", out.Error)
	}

	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned an empty embedding for model %q", o.EmbedName)
	}

	return out.Embedding, nil
}

func (o *Ollama) post(ctx context.Context, path string, body, into any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("reaching ollama at %s: %w", o.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)

		return fmt.Errorf("ollama returned %d: %s", resp.StatusCode, truncate(buf.String(), 300))
	}

	return json.NewDecoder(resp.Body).Decode(into)
}

func truncate(s string, n int) string {
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "…"
}

// Warm asks Ollama to load the chat model without generating anything.
//
// Worth doing because the two model loads in a turn were happening one after
// the other. Recall embeds the question first, which on a cold start means
// loading the embedding model and running it — measured at about twenty
// seconds — and only then does the chat model begin loading, for another
// thirty. Nothing about that is parallel, and both are waiting on disk rather
// than on each other.
//
// Starting this the moment a message arrives overlaps the two, so the chat
// model is ready at roughly the moment the context is. It is a request with no
// messages, which Ollama treats as "load and hold" rather than as a question.
func (o *Ollama) Warm(ctx context.Context) error {
	body := ollamaChatRequest{
		Model:     o.ChatModel,
		Stream:    false,
		KeepAlive: o.KeepAlive,
	}

	var out ollamaChatResponse

	return o.post(ctx, "/api/chat", body, &out)
}
