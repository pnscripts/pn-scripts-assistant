// Package llm talks to language models.
//
// One contract, two implementations: a local Ollama and Anthropic's API. The
// router decides which to use and refuses the ones privacy forbids, so callers
// never hold a provider they were not allowed to have.
package llm

import (
	"context"
	"encoding/json"
	"time"
)

// Role values used in a conversation.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is one turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	// ToolCallID links a tool result back to the call that produced it. Empty
	// for ordinary messages.
	ToolCallID string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

// ToolSpec describes a tool to the model. Parameters is a JSON Schema object.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is the model asking for a tool to be run.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Request is one exchange with a model.
type Request struct {
	Messages []Message
	Tools    []ToolSpec

	// Model overrides the provider's default when set.
	Model string

	// MaxTokens caps the reply. Zero means the provider's default.
	//
	// It exists for spoken replies. Generation on a CPU is the slowest part of
	// an exchange and scales with how much is produced, so a reply meant to be
	// heard rather than read should be short — which makes it both faster and
	// better suited to being spoken.
	MaxTokens int
}

// Response is what came back.
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Model     string
	Provider  string

	// Elapsed is how long the call took, which matters on CPU inference where
	// a cold model load is a minute and a warm reply is seconds.
	Elapsed time.Duration
}

// Provider is a language model that can be asked for a reply.
type Provider interface {
	// Name is the stable identifier privacy rules are written against.
	Name() string

	// Chat sends a conversation and returns the reply.
	Chat(ctx context.Context, req Request) (Response, error)

	// Available reports whether the provider can currently be reached. A
	// provider that is configured but not running should not be offered.
	Available(ctx context.Context) bool
}

// Embedder turns text into a vector. Kept separate from Provider because the
// model that embeds is not usually the model that chats, and because embedding
// must stay local in every privacy mode.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedModel() string
}
