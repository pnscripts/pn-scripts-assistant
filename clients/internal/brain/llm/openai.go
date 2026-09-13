package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

/*
 * Everything that speaks the OpenAI chat API, which is most of them.
 *
 * One implementation rather than one per company. OpenAI defined the shape,
 * and OpenRouter, Groq, Together, DeepSeek, Mistral and a dozen others
 * deliberately answer the same requests at a different address — so the only
 * thing that distinguishes them here is a URL, a key and a name. Writing them
 * separately would be the same file copied out with the host changed, and the
 * fourth copy is where the bugs start living in only three of them.
 *
 * OpenRouter is the one worth naming: a single key there reaches models from
 * every major provider, which is a better answer to "why only one API" than
 * adding two more of them would be.
 *
 * Anthropic keeps its own file. Its API is genuinely a different shape — the
 * system prompt is a field rather than a message, and tool results come back
 * in content blocks — and bending it into this one would cost more clarity
 * than the duplication saves.
 */

// OpenAICompatible talks to any service speaking the OpenAI chat API.
type OpenAICompatible struct {
	// ProviderName is what privacy rules are written against, so it has to be
	// stable and distinct: "openai" and "openrouter" are different trust
	// decisions even though they share this code.
	ProviderName string

	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client

	// Referer and Title are sent only by OpenRouter's convention, which uses
	// them to attribute traffic. Empty for everyone else, and harmless.
	Referer string
	Title   string
}

// The defaults each service is usually reached at.
const (
	DefaultOpenAIModel     = "gpt-4o-mini"
	DefaultOpenRouterModel = "anthropic/claude-3.5-sonnet"
)

// NewOpenAI talks to OpenAI itself.
func NewOpenAI(apiKey, model string) *OpenAICompatible {
	if model == "" {
		model = DefaultOpenAIModel
	}

	return &OpenAICompatible{
		ProviderName: "openai",
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      "https://api.openai.com/v1",
		HTTPClient:   &http.Client{Timeout: 120 * time.Second},
	}
}

/*
 * NewOpenRouter reaches many models through one key.
 *
 * Named separately from OpenAI because privacy is decided by provider name and
 * these are not the same promise: a request to OpenRouter may be served by any
 * of the companies behind it, and somebody who agreed to one has not agreed to
 * the rest.
 */
func NewOpenRouter(apiKey, model string) *OpenAICompatible {
	if model == "" {
		model = DefaultOpenRouterModel
	}

	return &OpenAICompatible{
		ProviderName: "openrouter",
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      "https://openrouter.ai/api/v1",
		HTTPClient:   &http.Client{Timeout: 120 * time.Second},
		Referer:      "https://github.com/pnscripts/pn-brain",
		Title:        "PN Scripts Assistant",
	}
}

func (o *OpenAICompatible) Name() string {
	if o.ProviderName == "" {
		return "openai"
	}

	return o.ProviderName
}

// Available is true when a key exists. No network call: this is asked whenever
// the interface is drawn, and probing a paid API to render a status dot would
// be both slow and rude.
func (o *OpenAICompatible) Available(context.Context) bool { return o.APIKey != "" }

type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	Tools     []openAITool    `json:"tools,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
}

type openAIMessage struct {
	Role string `json:"role"`

	// Content is a string, or nil on an assistant message that only asked for
	// tools. Several of these services refuse an empty string alongside
	// tool_calls, and all of them accept a null.
	Content any `json:"content"`

	// ToolCallID ties a result back to the call that asked for it. Only set on
	// messages carrying tool output.
	ToolCallID string `json:"tool_call_id,omitempty"`

	// ToolCalls is what the assistant asked for on its own turn. Without it a
	// message with role "tool" answers nothing, and these services refuse the
	// whole request rather than ignoring the one message.
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"` // always "function"
	Function openAICallFunction `json:"function"`
}

type openAICallFunction struct {
	Name string `json:"name"`

	// A string containing JSON, which is both what these services send and
	// what they expect back. See ToolCall for why that is worth naming.
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`

	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (o *OpenAICompatible) Chat(ctx context.Context, req Request) (Response, error) {
	if o.APIKey == "" {
		return Response{}, fmt.Errorf("no %s API key is configured", o.Name())
	}

	started := time.Now()

	body := o.requestBody(req)

	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(o.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+o.APIKey)

	// OpenRouter's attribution headers. Absent for everyone else.
	if o.Referer != "" {
		request.Header.Set("HTTP-Referer", o.Referer)
		request.Header.Set("X-Title", o.Title)
	}

	client := o.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	resp, err := client.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("could not reach %s: %w", o.Name(), err)
	}
	defer resp.Body.Close()

	answer, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Response{}, err
	}

	var parsed openAIResponse

	if err := json.Unmarshal(answer, &parsed); err != nil {
		return Response{}, fmt.Errorf("%s replied with something unreadable: %w",
			o.Name(), err)
	}

	/*
	 * The error field is checked before the status code.
	 *
	 * These services answer a rejected request with a 200 and an error object
	 * about as often as with a 4xx, and the message inside it is the one worth
	 * showing — "insufficient_quota" says what to do, where "status 429" does
	 * not.
	 */
	if parsed.Error != nil {
		return Response{}, fmt.Errorf("%s: %s", o.Name(), parsed.Error.Message)
	}

	if resp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("%s returned %d: %s",
			o.Name(), resp.StatusCode, truncateForError(string(answer)))
	}

	if len(parsed.Choices) == 0 {
		return Response{}, fmt.Errorf("%s returned no reply", o.Name())
	}

	out := Response{
		Content:  parsed.Choices[0].Message.Content,
		Model:    body.Model,
		Provider: o.Name(),
		Elapsed:  time.Since(started),
	}

	for _, call := range parsed.Choices[0].Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:   call.ID,
			Name: call.Function.Name,

			// Unquoted here rather than at the tool, which is where it used to
			// arrive still wrapped in its own quotes and fail as though the
			// tool were at fault.
			Arguments: AsObject(call.Function.Arguments),
		})
	}

	return out, nil
}

// requestBody maps one conversation onto the shape these services expect.
func (o *OpenAICompatible) requestBody(req Request) openAIRequest {
	model := req.Model
	if model == "" {
		model = o.Model
	}

	body := openAIRequest{Model: model, MaxTokens: req.MaxTokens}

	/*
	 * The system prompt stays a message here.
	 *
	 * Unlike Anthropic, which takes it as its own field. Several messages with
	 * that role are allowed and are simply read in order, so nothing has to be
	 * joined together first.
	 */
	messages := req.Messages

	// A turn offered no tools cannot carry a call in its history; see
	// withoutToolCalls.
	if len(req.Tools) == 0 {
		messages = withoutToolCalls(messages)
	}

	messages = Replayable(messages)

	for _, m := range messages {
		switch m.Role {
		case RoleSystem, RoleUser:
			body.Messages = append(body.Messages,
				openAIMessage{Role: m.Role, Content: m.Content})

		case RoleAssistant:
			out := openAIMessage{Role: m.Role, Content: m.Content}

			for _, c := range m.ToolCalls {
				call := openAIToolCall{ID: c.ID, Type: "function"}
				call.Function.Name = c.Name
				call.Function.Arguments = string(AsObject(c.Arguments))

				out.ToolCalls = append(out.ToolCalls, call)
			}

			// Null rather than an empty string when the turn was only a
			// request to run something.
			if len(out.ToolCalls) > 0 && m.Content == "" {
				out.Content = nil
			}

			body.Messages = append(body.Messages, out)

		case RoleTool:
			// Tool output travels as its own role, and without the id it is
			// silently dropped by some of these services rather than refused.
			body.Messages = append(body.Messages, openAIMessage{
				Role:       "tool",
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
			})
		}
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	return body
}

// truncateForError keeps an unexpected reply short enough to read.
func truncateForError(text string) string {
	const most = 300

	text = strings.TrimSpace(text)
	if len(text) <= most {
		return text
	}

	return text[:most] + "…"
}
