// Package brain joins the pieces: memory, models and privacy.
//
// It is the layer the HTTP handlers call, and it holds the rules about what
// goes into a prompt. Keeping that here rather than in the handlers means the
// desktop window, the command line and anything added later all get the same
// behaviour without repeating it.
package brain

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"pn-brain/internal/brain/agent"
	"pn-brain/internal/brain/config"
	"pn-brain/internal/brain/learning"
	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/storage"
	"pn-brain/internal/brain/store"
	"pn-brain/internal/brain/tools"
)

// Brain is a running assistant.
type Brain struct {
	DB     *store.DB
	Router *llm.Router
	Cfg    config.Config
	Mode   llm.Mode
	Log    *slog.Logger

	// Root is the data root, and DBPath the file inside it. Held so the
	// interface can report free space on the disk that actually matters —
	// the one the brain grows on, not the one the binary happens to sit on.
	Root   string
	DBPath string

	// Learner runs the Extractor/Validator/Curator pipeline in the background.
	// Nil when no local model is available, since extraction must stay local.
	Learner *learning.Worker

	// Agent runs tools. It stops rather than acting when a tool would change
	// something, so approval stays a real gate rather than a notification.
	Agent *agent.Loop
}

// New assembles a brain from settings.
func New(db *store.DB, cfg config.Config, root, dbPath string, logger *slog.Logger) *Brain {
	mode := llm.ParseMode(cfg.Privacy)

	ollama := llm.NewOllama(cfg.OllamaURL, cfg.OllamaModel, cfg.EmbedModel)

	providers := []llm.Provider{ollama}

	// Anthropic is registered only when a key exists. Registering it without
	// one would show a provider in the interface that fails the moment it is
	// chosen.
	if cfg.AnthropicKey != "" {
		providers = append(providers, llm.NewAnthropic(cfg.AnthropicKey, cfg.AnthropicModel))
	}

	b := &Brain{
		DB:     db,
		Router: llm.NewRouter(mode, cfg.DefaultProvider, providers...),
		Cfg:    cfg,
		Mode:   mode,
		Log:    logger,
		Root:   root,
		DBPath: dbPath,
	}

	b.Agent = &agent.Loop{
		DB:  db,
		Log: logger,
		Registry: tools.NewRegistry(
			tools.ReadFile{},
			tools.ListDirectory{},
			tools.WriteFile{},
			tools.RunCommand{},
		),
	}

	// Learning reads the conversation and must therefore stay on this machine
	// in every privacy mode; it is wired to the local provider directly rather
	// than through the router, so no configuration can point it elsewhere.
	b.Learner = &learning.Worker{
		DB:  db,
		Log: logger,
		Extractor: learning.Extractor{
			Provider: ollama,
			Owner:    cfg.Owner,
			Name:     cfg.Name,
		},
		Curator: learning.Curator{DB: db, Embedder: ollama},
	}

	return b
}

// Start begins background work. Stop must be called to let it finish cleanly.
func (b *Brain) Start(ctx context.Context) {
	if b.Learner != nil {
		b.Learner.Start(ctx)
	}
}

// Stop waits for in-flight learning to finish.
func (b *Brain) Stop() {
	if b.Learner != nil {
		b.Learner.Stop()
	}
}

// SystemPrompt is who this assistant is.
//
// One definition, used by every client, so the personality does not drift
// between the window, the command line and anything added later.
func (b *Brain) SystemPrompt() string {
	name := b.Cfg.Name
	if name == "" {
		name = "PN Brain"
	}

	owner := b.Cfg.Owner
	if owner == "" {
		owner = "your owner"
	}

	return fmt.Sprintf(`You are %s, %s's personal AI assistant — a private, self-learning
brain, not a generic chatbot. Speak with quiet confidence: sharp, warm,
economical with words, a little dry wit when it fits. Address %s
directly. Say when you're unsure rather than guessing. You remember things
about %s across conversations and are meant to keep getting more
useful over time, so when something durable and worth remembering comes up,
you don't need to ask permission to notice it — that happens automatically.

You have tools and can act, not just talk. Use them when a question is
better answered by looking than by guessing: read the file, list the
directory, check. Prefer one purposeful call over several speculative ones.

Anything that changes something — writing a file, running a command —
pauses for %s's approval before it happens. That is normal, not an
error. Say plainly what you intend to do and why; don't pretend an action
already succeeded, and don't ask for permission in prose when calling the
tool will ask properly.`, name, owner, owner, owner, owner)
}

// ChatRequest is one message from a person.
type ChatRequest struct {
	ConversationID int64  `json:"conversation_id"`
	Message        string `json:"message"`
	Provider       string `json:"provider"`
}

// ChatReply is what goes back to the interface.
type ChatReply struct {
	ConversationID int64   `json:"conversation_id"`
	Reply          string  `json:"reply"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	Recalled       []int64 `json:"recalled"`

	// ActionsTaken and PendingApprovals are what the interface shows beneath a
	// reply: what the brain did, and what it is waiting to be allowed to do.
	ActionsTaken     []string        `json:"actions_taken"`
	PendingApprovals []agent.Pending `json:"pending_approvals"`
}

// Chat answers a message and records the exchange.
func (b *Brain) Chat(ctx context.Context, req ChatRequest) (ChatReply, error) {
	if strings.TrimSpace(req.Message) == "" {
		return ChatReply{}, fmt.Errorf("a message is required")
	}

	providerName := req.Provider
	if providerName == "" {
		providerName = b.Cfg.DefaultProvider
	}

	provider, err := b.Router.Provider(providerName)
	if err != nil {
		return ChatReply{}, err
	}

	conversationID := req.ConversationID
	isNew := conversationID == 0

	if isNew {
		if conversationID, err = b.DB.NewConversation(req.Message); err != nil {
			return ChatReply{}, err
		}

		if _, err := b.DB.AddMessage(conversationID, llm.RoleSystem, "", "", b.SystemPrompt()); err != nil {
			return ChatReply{}, err
		}
	} else {
		ok, err := b.DB.ConversationExists(conversationID)
		if err != nil {
			return ChatReply{}, err
		}

		if !ok {
			return ChatReply{}, fmt.Errorf("conversation %d does not exist", conversationID)
		}
	}

	if _, err := b.DB.AddMessage(conversationID, llm.RoleUser, "", "", req.Message); err != nil {
		return ChatReply{}, err
	}

	history, err := b.DB.History(conversationID)
	if err != nil {
		return ChatReply{}, err
	}

	messages := make([]llm.Message, 0, len(history)+1)

	for _, m := range history {
		messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
	}

	// Memory is only ever given to a local model. What the brain has learned is
	// assembled from this machine's disk — project paths, client names,
	// documents — and the user never composed it, so it is not ours to forward
	// to a third party. What they type is their choice; this is not.
	var recalled []store.Scored

	if llm.AllowsMemoryFor(provider.Name()) {
		recalled = b.recall(ctx, req.Message)
	}

	if preamble := formatRecall(recalled); preamble != "" {
		// Placed immediately before the newest user message, so it reads as
		// context for the question rather than as part of the conversation.
		messages = append(messages[:len(messages)-1],
			llm.Message{Role: llm.RoleSystem, Content: preamble},
			messages[len(messages)-1],
		)
	}

	result, err := b.Agent.Run(ctx, conversationID, provider, messages)
	if err != nil {
		return ChatReply{}, err
	}

	// When the loop stopped for approval it has already written its own
	// message; writing again would duplicate it in the transcript.
	if !result.WaitingForApproval() {
		if _, err := b.DB.AddMessage(conversationID, llm.RoleAssistant,
			result.Provider, result.Model, result.Reply); err != nil {
			return ChatReply{}, err
		}
	}

	// Learning happens after the reply is on its way, never before it: the
	// user waits on the answer, not on the brain deciding what to remember.
	if b.Learner != nil {
		b.Learner.Learn(conversationID)
	}

	ids := make([]int64, 0, len(recalled))
	for _, f := range recalled {
		ids = append(ids, f.ID)
	}

	return ChatReply{
		ConversationID:   conversationID,
		Reply:            result.Reply,
		Provider:         result.Provider,
		Model:            result.Model,
		Recalled:         ids,
		ActionsTaken:     result.ActionsTaken,
		PendingApprovals: result.Pending,
	}, nil
}

// recall finds relevant memories.
//
// Failure is not fatal. Recall is a nice-to-have, not a precondition for
// replying: if the local embedding model is down, the brain should answer
// without memory rather than fail the whole request.
func (b *Brain) recall(ctx context.Context, query string) []store.Scored {
	embedder, err := b.Router.Embedder()
	if err != nil {
		b.Log.Warn("no embedder; answering without memory", "error", err)

		return nil
	}

	vec, err := embedder.Embed(ctx, query)
	if err != nil {
		b.Log.Warn("recall failed; answering without memory", "error", err)

		return nil
	}

	found, err := b.DB.Search(vec, b.Cfg.RecallLimit, b.Cfg.RecallFloor)
	if err != nil {
		b.Log.Warn("searching memory failed; answering without memory", "error", err)

		return nil
	}

	return found
}

func formatRecall(facts []store.Scored) string {
	if len(facts) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("Relevant things you already know, recalled from your own memory:\n")

	for _, f := range facts {
		b.WriteString("- " + f.Content + "\n")
	}

	b.WriteString("\nUse these if they help. Do not mention this list itself.")

	return b.String()
}

// Capabilities lists what this brain can currently do.
//
// Reported honestly: a capability appears only when the thing behind it is
// actually present. Listing an ability the brain does not have is worse than
// listing nothing, because the user will ask for it.
func (b *Brain) Capabilities() []string {
	caps := []string{"memory", "recall", "memory-map"}

	if b.Mode.AllowsWeb() {
		caps = append(caps, "web")
	}

	for _, a := range b.Router.Availabilities(context.Background()) {
		if a.Reachable {
			caps = append(caps, "model:"+a.Name)
		}
	}

	return caps
}

// Storage reports how much room the brain has left on the disk holding it.
func (b *Brain) Storage() storage.Report {
	return storage.Check(b.Root, b.DBPath)
}
