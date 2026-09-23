package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	Role string `json:"role"`

	// Content is a string for an ordinary turn and a list of blocks for one
	// that used a tool. Anthropic accepts both, and the string form is what
	// nearly every message here is.
	Content any `json:"content"`
}

/*
 * anthropicBlock is one piece of a turn.
 *
 * Anthropic does not have a role for tool output. A call is a tool_use block
 * on the assistant's own message and its result is a tool_result block on the
 * next user message — which is why sending tool output as a role of its own
 * did not fail loudly, it simply matched no case and was dropped, and the
 * model was asked the same question again with no sign it had ever looked.
 */
type anthropicBlock struct {
	Type string `json:"type"` // text | tool_use | tool_result

	Text string `json:"text,omitempty"`

	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
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

	body := a.requestBody(req)

	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	started := time.Now()

	resp, err := sendWithOneMoreTry(ctx, a.HTTPClient, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			a.BaseURL+"/v1/messages", bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", a.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")

		return httpReq, nil
	})
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

// requestBody maps one conversation onto the shape Anthropic expects.
func (a *Anthropic) requestBody(req Request) anthropicRequest {
	model := req.Model
	if model == "" {
		model = a.Model
	}

	maxTokens := 4096
	if req.MaxTokens > 0 {
		maxTokens = req.MaxTokens
	}

	body := anthropicRequest{Model: model, MaxTokens: maxTokens}

	messages := req.Messages

	// A turn offered no tools cannot carry a call in its history; see
	// withoutToolCalls.
	if len(req.Tools) == 0 {
		messages = withoutToolCalls(messages)
	}

	messages = Replayable(messages)

	// Anthropic takes the system prompt as its own field rather than as a
	// message, so it is lifted out of the conversation here.
	for i := 0; i < len(messages); i++ {
		m := messages[i]

		switch m.Role {
		case RoleSystem:
			if body.System != "" {
				body.System += "\n\n"
			}

			body.System += m.Content

		case RoleUser:
			body.Messages = append(body.Messages,
				anthropicMessage{Role: RoleUser, Content: m.Content})

		case RoleAssistant:
			if len(m.ToolCalls) == 0 {
				body.Messages = append(body.Messages,
					anthropicMessage{Role: RoleAssistant, Content: m.Content})

				break
			}

			var blocks []anthropicBlock

			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: m.Content})
			}

			for _, c := range m.ToolCalls {
				blocks = append(blocks, anthropicBlock{
					Type:  "tool_use",
					ID:    c.ID,
					Name:  c.Name,
					Input: AsObject(c.Arguments),
				})
			}

			body.Messages = append(body.Messages,
				anthropicMessage{Role: RoleAssistant, Content: blocks})

		case RoleTool:
			/*
			 * Every result for one assistant turn travels in a single message.
			 *
			 * Anthropic wants each tool_use answered in the user turn that
			 * follows it, so two calls answered by two separate user messages
			 * is a refused request — the second one arrives after the turn
			 * that was supposed to contain it had already ended.
			 */
			var blocks []anthropicBlock

			for ; i < len(messages) && messages[i].Role == RoleTool; i++ {
				blocks = append(blocks, anthropicBlock{
					Type:      "tool_result",
					ToolUseID: messages[i].ToolCallID,
					Content:   messages[i].Content,
				})
			}

			i--

			body.Messages = append(body.Messages,
				anthropicMessage{Role: RoleUser, Content: blocks})
		}
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	return body
}
