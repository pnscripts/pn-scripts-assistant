package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Anthropic is a hosted model.
//
// Everything sent here leaves the machine, which is why the router will not
// hand this provider out unless privacy is set to open, and why AllowsMemoryFor
// refuses it even then.
type Anthropic struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

// DefaultAnthropicModel is used when none is configured.
const DefaultAnthropicModel = "claude-sonnet-4-5"

func NewAnthropic(apiKey, model string) *Anthropic {
	if model == "" {
		model = DefaultAnthropicModel
	}

	return &Anthropic{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    "https://api.anthropic.com",
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *Anthropic) Name() string { return "anthropic" }

// Available is true when a key exists. No network call is made: this is asked
// whenever the interface is drawn, and probing a paid API to render a status
// dot would be both slow and rude.
func (a *Anthropic) Available(context.Context) bool { return a.APIKey != "" }

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	Model   string `json:"model"`
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (a *Anthropic) Chat(ctx context.Context, req Request) (Response, error) {
	if a.APIKey == "" {
		return Response{}, fmt.Errorf("no Anthropic API key is configured")
	}

	model := req.Model
	if model == "" {
		model = a.Model
	}

	maxTokens := 4096
	if req.MaxTokens > 0 {
		maxTokens = req.MaxTokens
	}

	body := anthropicRequest{Model: model, MaxTokens: maxTokens}

	// Anthropic takes the system prompt as its own field rather than as a
	// message, so it is lifted out of the conversation here.
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			if body.System != "" {
				body.System += "\n\n"
			}

			body.System += m.Content
		case RoleUser, RoleAssistant:
			body.Messages = append(body.Messages, anthropicMessage{Role: m.Role, Content: m.Content})
		}
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	started := time.Now()

	resp, err := a.HTTPClient.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("reaching Anthropic: %w", err)
	}
	defer resp.Body.Close()

	var out anthropicResponse

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Response{}, fmt.Errorf("reading Anthropic's reply: %w", err)
	}

	if out.Error != nil {
		return Response{}, fmt.Errorf("anthropic: %s", out.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("anthropic returned %d", resp.StatusCode)
	}

	result := Response{Model: out.Model, Provider: a.Name(), Elapsed: time.Since(started)}

	for _, c := range out.Content {
		switch c.Type {
		case "text":
			result.Content += c.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        c.ID,
				Name:      c.Name,
				Arguments: c.Input,
			})
		}
	}

	return result, nil
}
