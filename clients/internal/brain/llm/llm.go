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

	/*
	 * ToolCalls is what an assistant asked for, carried on the assistant's own
	 * message.
	 *
	 * Its absence is why using a tool twice in one turn only ever worked
	 * locally. The loop appended the tool's result and never the message that
	 * asked for it, so the conversation read as a set of answers to questions
	 * nobody had put: Anthropic dropped them on the floor, because its mapping
	 * had no case for a tool at all, and every OpenAI-compatible service
	 * refused the request outright. Ollama is lenient and rendered the orphan
	 * as text, which is the whole of why the local path appeared correct.
	 */
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolSpec describes a tool to the model. Parameters is a JSON Schema object.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

/*
 * ToolCall is the model asking for a tool to be run.
 *
 * Arguments is always a JSON *object*, never a JSON string. That is not
 * pedantry: OpenAI and everything that copied its shape send this field as a
 * string containing JSON, Ollama and Anthropic send an object, and the loop
 * hands it straight to a tool that unmarshals it into a struct. Unquoting
 * where it is parsed rather than where it is used means one rule instead of
 * one per caller — and the caller that got it wrong failed with "cannot
 * unmarshal string into Go value of type struct", which reads like a broken
 * tool rather than a provider that writes its arguments differently.
 */
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
// Streamer is a provider that can answer a piece at a time.
//
// Separate from Provider because not every provider can, and a turn that needs
// tools must not stream anyway — so the loop asks for this only when it can
// actually use it.
type Streamer interface {
	ChatStream(ctx context.Context, req Request, onText func(string)) (Response, error)
}

type Response struct {
	// Spoken is true when the answer was already said aloud as it was written,
	// so that whoever asked does not say the whole thing a second time.
	Spoken    bool
	Content   string
	ToolCalls []ToolCall
	Model     string
	Provider  string

	// Elapsed is how long the call took, which matters on CPU inference where
	// a cold model load is a minute and a warm reply is seconds.
	Elapsed time.Duration

	/*
	 * CutOff marks an answer its owner interrupted part-way through.
	 *
	 * The content is then what was actually delivered rather than what the
	 * model went on to write, because those are different things and the
	 * conversation has to hold the one that happened. Recording the whole
	 * reply meant the next turn was answered as though a paragraph nobody
	 * heard had been heard — they were replying to the first sentence and it
	 * was continuing from the fifth.
	 */
	CutOff bool
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
