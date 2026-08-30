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
	"sync"
	"time"

	"pn-brain/internal/brain/agent"
	"pn-brain/internal/brain/appearance"
	"pn-brain/internal/brain/config"
	"pn-brain/internal/brain/learning"
	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/mail"
	"pn-brain/internal/brain/models"
	"pn-brain/internal/brain/smarthome"
	"pn-brain/internal/brain/speech"
	"pn-brain/internal/brain/storage"
	"pn-brain/internal/brain/store"
	"pn-brain/internal/brain/tools"
)

// Brain is a running assistant.
type Brain struct {
	DB     *store.DB
	Router *llm.Router
	Cfg    config.Config

	// Look is what the core is coloured with, changeable by asking.
	Look *appearance.Store

	// ollama is kept so the chat model can be changed without a restart. The
	// choice of model is a decision somebody makes while using the brain,
	// having seen how the alternatives behave on their own machine, and
	// restarting to try one is enough friction to stop them trying.
	ollama *llm.Ollama
	Mode   llm.Mode
	Log    *slog.Logger

	// Root is the data root, and DBPath the file inside it. Held so the
	// interface can report free space on the disk that actually matters —
	// the one the brain grows on, not the one the binary happens to sit on.
	Root   string
	DBPath string

	// The small model used for small talk, looked up once. Empty means there
	// is none installed and everything goes to the usual one.
	rolesOnce sync.Once
	roles     llm.Sizes

	// modelsResident is whether both models are held in memory at once, which
	// is what makes switching between them free rather than a reload.
	mu             sync.Mutex
	modelsResident bool

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

	// What the interface looks like, so that being asked to change it is
	// something the brain can do rather than something it agrees to.
	look := appearance.Open(root)

	ollama := llm.NewOllama(cfg.OllamaURL, cfg.OllamaModel, cfg.EmbedModel)

	providers := []llm.Provider{ollama}

	// Anthropic is registered only when a key exists. Registering it without
	// one would show a provider in the interface that fails the moment it is
	// chosen.
	if cfg.AnthropicKey != "" {
		providers = append(providers, llm.NewAnthropic(cfg.AnthropicKey, cfg.AnthropicModel))
	}

	b := &Brain{
		Look:   look,
		ollama: ollama,
		DB:     db,
		Router: llm.NewRouter(mode, cfg.DefaultProvider, providers...),
		Cfg:    cfg,
		Mode:   mode,
		Log:    logger,
		Root:   root,
		DBPath: dbPath,
	}

	available := []tools.Tool{
		tools.SetAppearance{Look: look},
		tools.ReadFile{},
		tools.ListDirectory{},
		tools.WriteFile{},
		tools.RunCommand{},
		tools.EditFile{},
		tools.SearchFiles{},
		tools.ReadDocument{},
		tools.LookAtScreen{OllamaURL: cfg.OllamaURL},
	}

	// The web tool is not registered at all in private mode, rather than
	// registered and refused. A tool the model can see is a tool it will try,
	// and an assistant that keeps proposing something it may never do is worse
	// than one that simply cannot.
	if mode.AllowsWeb() {
		available = append(available,
			tools.FetchURL{},
			tools.WebSearch{BraveKey: cfg.BraveKey},
		)
	}

	/*
	 * Mail tools appear only when there is a mailbox.
	 *
	 * Same reason as the smart home: a tool the model can see is a tool it will
	 * try, and offering to read mail it cannot reach produces a brain that
	 * promises to check and then reports an error every time.
	 */
	if account := mailAccount(cfg); account.Configured() {
		read := func() mail.Account { return mailAccount(b.Cfg) }

		available = append(available,
			tools.ReadEmail{Account: read},
			tools.SendEmail{Account: read},
		)
	}

	// Smart-home tools appear only when there is a house to talk to. Offering
	// them unconfigured would have the model promise to turn on lights it
	// cannot reach.
	home := smarthome.New(cfg.HomeAssistantURL, cfg.HomeAssistantToken)

	if home.Configured() {
		available = append(available, tools.ListDevices{Home: home}, tools.SetDevice{Home: home})
	}

	// Added after the brain exists, because they change the brain's own
	// settings or act on its own queue, and so need a handle to it.
	available = append(available,
		tools.SetWakeWord{Brain: b},
		tools.ListWaiting{Queue: queueOf{b}},
		tools.DecideWaiting{Queue: queueOf{b}},
		tools.Remind{Diary: diaryOf{b}},
		tools.ListReminders{Diary: diaryOf{b}},
		tools.ForgetReminder{Diary: diaryOf{b}},
	)

	b.Agent = &agent.Loop{
		DB:       db,
		Log:      logger,
		Registry: tools.NewRegistry(available...),

		// Read from the live settings each turn, so switching it off in the
		// interface takes effect without a restart.
		Model: func(message string) llm.Choice {
			if !b.Cfg.AutoModel {
				return llm.Choice{Model: b.Cfg.OllamaModel, Why: "chosen by you"}
			}

			return llm.ChooseModel(message, b.modelRoles())
		},
	}

	// The owner's chosen voice and language, applied before anything speaks or
	// listens.
	speech.SetVoice(cfg.Voice)
	speech.SetLanguage(cfg.Language)

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

/*
 * modelRoles is which model does what, from whatever is installed.
 *
 * Looked up once and remembered, because it means asking ollama what is
 * installed and that is a round trip nobody should pay for on the way to
 * saying good morning. Anything missing is left empty, and that kind of turn
 * falls back to the model that does things — which every machine running this
 * has, because it is the one it was set up with.
 */
func (b *Brain) modelRoles() llm.Sizes {
	b.rolesOnce.Do(func() {
		b.roles = llm.Sizes{Work: b.Cfg.OllamaModel}

		client := models.New(b.Cfg.OllamaURL)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		installed, err := client.List(ctx)
		if err != nil {
			b.Log.Info("could not ask what models are installed", "error", err)

			return
		}

		names := make([]string, 0, len(installed))

		for _, m := range installed {
			names = append(names, m.Name)
		}

		b.roles.Talk = llm.PickTalk(names)
		b.roles.Reason = llm.PickReason(names)

		// The configured model stays the one that does things: it is what its
		// owner chose and tested, and second-guessing that is not this
		// function's job.
		b.Log.Info("models chosen for each kind of turn",
			"work", b.roles.Work, "talk", b.roles.Talk, "reason", b.roles.Reason)
	})

	return b.roles
}

// fastModel is the small model used for conversation.
func (b *Brain) fastModel() string { return b.modelRoles().Talk }

// mailAccount reads the mailbox out of the settings.
//
// Taken from the live config each time rather than captured once, so a mailbox
// filled in while the brain is running works without a restart.
func mailAccount(cfg config.Config) mail.Account {
	return mail.Account{
		Host:     cfg.MailHost,
		Port:     cfg.MailPort,
		User:     cfg.MailUser,
		Password: cfg.MailPassword,
		From:     cfg.MailFrom,
		SMTPHost: cfg.SMTPHost,
		SMTPPort: cfg.SMTPPort,
	}
}

// WakeWord is what has to be said before the brain answers, or empty.
func (b *Brain) WakeWord() string { return b.Cfg.WakeWord }

// SetWakeWord changes what has to be said before the brain answers.
//
// Empty turns it off. Saved immediately, because a setting that lasts until the
// next restart and then reverts is worse than one never offered.
func (b *Brain) SetWakeWord(word string) error {
	b.Cfg.WakeWord = strings.TrimSpace(word)

	if err := b.Cfg.Save(b.Root); err != nil {
		return err
	}

	b.Log.Info("wake word changed", "word", b.Cfg.WakeWord)

	return nil
}

// UseModel switches the model replies are generated with.
//
// Takes effect on the next reply, with no restart: choosing a model is a
// decision somebody makes while using the brain, having seen how the
// alternatives behave on their own machine, and having to restart to try one is
// enough friction to stop them trying.
//
// The embedding model is deliberately not switchable this way. Every memory in
// the database was embedded with the current one, and vectors from two
// different models cannot be compared — changing it would not give different
// recall, it would silently make recall meaningless.
func (b *Brain) UseModel(name string) error {
	if b.ollama == nil {
		return fmt.Errorf("there is no local provider to change")
	}

	b.ollama.ChatModel = name
	b.Cfg.OllamaModel = name

	// Written down, or the choice lasts until the next restart and then quietly
	// reverts. That is exactly what happened: a model was chosen in the panel,
	// used for the rest of the session, and the brain came back on the old one
	// with nothing saying it had changed back.
	if err := b.Cfg.Save(b.Root); err != nil {
		b.Log.Warn("the model was changed but could not be saved", "error", err)
	}

	b.Log.Info("chat model changed", "model", name)

	return nil
}

// Start begins background work. Stop must be called to let it finish cleanly.
func (b *Brain) Start(ctx context.Context) {
	if b.Learner != nil {
		b.Learner.Start(ctx)
	}

	// The only thing here that speaks without being spoken to first.
	go b.watchReminders(ctx)

	// Both models loaded and kept loaded, so choosing between them costs
	// nothing at the moment of choosing.
	go b.keepModelsWarm(ctx)

	// A resident recogniser, started in the background because loading its
	// model takes seconds and nothing should wait on it. Without one, every
	// spoken turn reloads 141MB of weights before looking at any audio — which
	// is bearable once and ruinous in a conversation.
	go func() {
		if err := speech.StartResident(ctx); err != nil {
			b.Log.Info("no resident recogniser; transcription will be slower", "reason", err)
		}
	}()
}

// Stop waits for in-flight learning to finish.
func (b *Brain) Stop() {
	if b.Learner != nil {
		b.Learner.Stop()
	}

	speech.StopResident()
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

Most things said to you are conversation and need no tool at all. A
greeting, a thank-you, a question about what you think — answer those in
words. Reach for a tool when the answer depends on something you would
otherwise have to guess at, not to demonstrate that you have tools.

Anything that changes something — writing a file, running a command —
pauses for %s's approval before it happens. That is normal, not an
error. Say plainly what you intend to do and why; don't pretend an action
already succeeded, and don't ask for permission in prose when calling the
tool will ask properly.`, name, owner, owner, owner, owner)
}

// spokenSystemPrompt is who the assistant is, said briefly.
func (b *Brain) spokenSystemPrompt() string {
	name := b.Cfg.Name
	if name == "" {
		name = "PN Brain"
	}

	owner := b.Cfg.Owner
	if owner == "" {
		owner = "your owner"
	}

	return fmt.Sprintf(spokenPersona, name, owner)
}

// ChatRequest is one message from a person.
type ChatRequest struct {
	ConversationID int64  `json:"conversation_id"`
	Message        string `json:"message"`
	Provider       string `json:"provider"`

	// Spoken marks a turn that will be heard rather than read.
	//
	// It changes two things. The model is asked for a couple of sentences, and
	// the reply is capped — which is what makes a spoken exchange bearable on a
	// CPU, since generation is the slow part and it scales with length. A page
	// of prose read aloud by a synthetic voice is nobody's idea of an answer.
	Spoken bool `json:"spoken"`
}

// Spoken turns are shaped by one measurement: on this machine the model
// processes about seven tokens a second, so the size of the prompt is the reply
// time almost exactly. 1727 tokens of context took 238 seconds; 113 took 13.
//
// Capping the answer barely helped, because generating a short answer was never
// the slow part. Cutting what is sent is the only lever that moves this, so a
// spoken turn gets fewer memories, shorter ones, and only the last few turns of
// conversation.
const (
	// SpokenReplyTokens caps the answer. Two or three sentences.
	SpokenReplyTokens = 90

	// SpokenRecallLimit is how many memories a spoken turn may use.
	SpokenRecallLimit = 4

	// SpokenFactLimit trims each of them harder than a typed turn would.
	SpokenFactLimit = 150

	// SpokenHistoryTurns is how much of the conversation is replayed. Enough to
	// follow a thread, far short of the whole transcript.
	SpokenHistoryTurns = 4
)

// spokenPersona replaces the full one for a spoken turn.
//
// The full persona is about 255 tokens and describes tool use and approval
// prompts at length — none of which applies when tools are not offered. At ten
// tokens a second that description costs half a minute per turn to say nothing
// relevant.
const spokenPersona = `You are %s, %s's personal assistant. You are speaking
aloud, so answer in one or two plain sentences and stop. Do not list or
enumerate. Do not read out paths or URLs — name the thing instead. Say when you
do not know.`

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

	// AlreadySpoken is true when the answer was said aloud as it was written,
	// so the page must not send it to be spoken a second time.
	AlreadySpoken bool `json:"already_spoken"`
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

	// An instruction about the review queue is answered here, before the model
	// is involved. It is deterministic, it takes no time, and until now the
	// model would say "I will remember these instructions" and do nothing —
	// which is the worst of the three possible outcomes.
	/*
	 * Start the chat model loading now, alongside everything else.
	 *
	 * The two model loads in a turn were serial: recall embeds the question
	 * first, which on a cold start means loading and running the embedding
	 * model, and only when that is done does the chat model begin loading.
	 * Measured here, that was twenty-one seconds before the chat model was even
	 * asked for, on a reply that took under two minutes in total.
	 *
	 * Neither load is waiting on the other — they are both waiting on disk — so
	 * they can happen at once. Started in the background and never waited for:
	 * if it fails, the ordinary path loads the model as it always did.
	 */
	if b.ollama != nil {
		go func() {
			if err := b.ollama.Warm(context.WithoutCancel(ctx)); err != nil {
				b.Log.Debug("could not warm the chat model", "error", err)
			}
		}()
	}

	// Being told to learn from something is carried out here rather than
	// described to the model, which would answer that it will and then not.
	if answer, handled := b.handleLearnInstruction(ctx, req.Message); handled {
		if _, err := b.DB.AddMessage(conversationID, llm.RoleAssistant, "", "", answer); err != nil {
			return ChatReply{}, err
		}

		return ChatReply{
			ConversationID: conversationID,
			Reply:          answer,
			Provider:       provider.Name(),
		}, nil
	}

	if answer, handled := b.handleLessonInstruction(ctx, req.Message); handled {
		if _, err := b.DB.AddMessage(conversationID, llm.RoleAssistant, "", "", answer); err != nil {
			return ChatReply{}, err
		}

		return ChatReply{
			ConversationID: conversationID,
			Reply:          answer,
			Provider:       provider.Name(),
		}, nil
	}

	history, err := b.DB.History(conversationID)
	if err != nil {
		return ChatReply{}, err
	}

	if req.Spoken {
		history = recentTurns(history, SpokenHistoryTurns)
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

		if req.Spoken && len(recalled) > SpokenRecallLimit {
			recalled = recalled[:SpokenRecallLimit]
		}
	}

	if req.Spoken {
		// Swap the stored persona for the short one rather than adding to it.
		// Appending would pay for both.
		for i := range messages {
			if messages[i].Role == llm.RoleSystem {
				messages[i].Content = b.spokenSystemPrompt()

				break
			}
		}
	}

	factLimit := RecalledFactLimit
	if req.Spoken {
		factLimit = SpokenFactLimit
	}

	if preamble := formatRecallLimited(recalled, factLimit); preamble != "" {
		// Placed immediately before the newest user message, so it reads as
		// context for the question rather than as part of the conversation.
		messages = append(messages[:len(messages)-1],
			llm.Message{Role: llm.RoleSystem, Content: preamble},
			messages[len(messages)-1],
		)
	}

	limit := 0
	if req.Spoken {
		limit = SpokenReplyTokens
	}

	/*
	 * Tools are offered on spoken turns too.
	 *
	 * They used to be withheld from anything said out loud, to save the time it
	 * costs to describe them — which meant that speaking to this brain could
	 * never make it do anything, only talk. "Approve everything", said aloud,
	 * could not have worked however well every other part of the chain behaved.
	 *
	 * What was said decides now, not how it arrived: small talk is answered
	 * without them either way, and anything that means doing something gets
	 * them whether it was typed or spoken.
	 */
	/*
	 * A spoken turn is said as it is written.
	 *
	 * Set only for a turn that will be heard. The wait before anybody hears
	 * anything is the whole of how a conversation feels, and on this machine
	 * an answer takes long enough that hearing the first sentence early is the
	 * difference between talking to something and submitting a form to it.
	 */
	if req.Spoken {
		b.Agent.Aloud = func(ctx context.Context) agent.TalkAloud {
			return speech.NewAloud(ctx)
		}

		defer func() { b.Agent.Aloud = nil }()
	}

	result, err := b.Agent.RunShaped(ctx, conversationID, provider, messages, limit, true)
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
		AlreadySpoken:    result.AlreadySpoken,
	}, nil
}

// recall finds relevant memories.
//
// Failure is not fatal. Recall is a nice-to-have, not a precondition for
// replying: if the local embedding model is down, the brain should answer
// without memory rather than fail the whole request.
// Search finds memories that mean something similar to the query.
//
// The same path a reply uses to remember things, exposed so the interface can
// offer a search box that searches what the brain actually knows rather than
// matching letters. Asking for "the hosting box" finds a memory that says
// "server" without either word appearing in the other.
func (b *Brain) Search(ctx context.Context, query string, limit int) []store.Scored {
	if strings.TrimSpace(query) == "" {
		return nil
	}

	embedder, err := b.Router.Embedder()
	if err != nil {
		return nil
	}

	vec, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil
	}

	found, err := b.DB.Search(vec, limit, b.Cfg.RecallFloor)
	if err != nil {
		return nil
	}

	return found
}

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

// RecalledFactLimit is how much of each fact goes into the prompt.
//
// Reply time on a CPU is dominated by prompt size, close to linearly: measured
// against qwen2.5-coder:7b, 113 tokens of context answered in 13.5s and 1727
// took 238s. Recall is the largest part of that prompt and the easiest to
// shrink without losing anything.
//
// What gets cut is the tail of a README excerpt. The first sentence of a
// scanned fact carries the name, the path and the date; past roughly this point
// it is build-status badges and headings. Trimming twelve worst-case facts here
// removes about two thirds of the context and none of the answer.
//
// The store keeps the full text. This is a prompt-shaping decision, not a
// storage one — the knowledge browser still shows everything, and embeddings
// were computed from the whole thing.
const RecalledFactLimit = 260

// recentTurns keeps the system prompt and the last few exchanges.
//
// The system prompt is not optional — it is who the assistant is — but the
// twentieth-most-recent message is, and on a CPU it costs as much to process as
// the question being asked.
func recentTurns(history []store.Message, turns int) []store.Message {
	var kept []store.Message

	for _, m := range history {
		if m.Role == llm.RoleSystem {
			kept = append(kept, m)
		}
	}

	var recent []store.Message

	for i := len(history) - 1; i >= 0 && len(recent) < turns*2; i-- {
		if history[i].Role == llm.RoleSystem {
			continue
		}

		recent = append([]store.Message{history[i]}, recent...)
	}

	return append(kept, recent...)
}

func formatRecall(facts []store.Scored) string {
	return formatRecallLimited(facts, RecalledFactLimit)
}

func formatRecallLimited(facts []store.Scored, limit int) string {
	if len(facts) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("Relevant things you already know, recalled from your own memory:\n")

	for _, f := range facts {
		b.WriteString("- " + trimForPrompt(f.Content, limit) + "\n")
	}

	b.WriteString("\nUse these if they help. Do not mention this list itself.")

	return b.String()
}

// trimForPrompt shortens a fact, preferring a sentence boundary so the model is
// never handed a claim that stops mid-word and reads as though it were complete.
func trimForPrompt(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")

	r := []rune(s)
	if len(r) <= limit {
		return s
	}

	cut := string(r[:limit])

	// Prefer to end where a sentence does, if one ends reasonably close to the
	// limit rather than right at the start.
	if i := strings.LastIndex(cut, ". "); i > limit/2 {
		return cut[:i+1]
	}

	return cut + "…"
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

	// Reported only when an engine is actually present. A capability listed
	// without the thing behind it is worse than none, because the user asks
	// for it.
	if speech.Available() != nil {
		caps = append(caps, "speech")
	}

	if listening, _ := speech.Listening(); listening {
		caps = append(caps, "listening")
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

// UseEmbeddingModel changes the model memories are indexed with, and rebuilds
// every vector to match.
//
// This is why it is not a setting like the others. A search compares the
// question's vector against every stored vector at once, and vectors from two
// different models are not comparable — a table holding both would rank results
// by which model happened to produce each row rather than by meaning. Changing
// the model without rebuilding would not give worse recall, it would give
// meaningless recall, and nothing would look wrong.
//
// So the rebuild is the operation, and the setting is a consequence of it. If
// it fails partway the old model stays in use, because a half-rebuilt table is
// the one state worse than either model.
func (b *Brain) ReEmbed(ctx context.Context, model string, note func(done, total int)) (int, error) {
	if b.ollama == nil {
		return 0, fmt.Errorf("there is no local provider to embed with")
	}

	facts, err := b.DB.AllFacts()
	if err != nil {
		return 0, err
	}

	previous := b.ollama.EmbedName
	b.ollama.EmbedName = model

	embedder, err := b.Router.Embedder()
	if err != nil {
		b.ollama.EmbedName = previous

		return 0, err
	}

	for i, fact := range facts {
		if err := ctx.Err(); err != nil {
			b.ollama.EmbedName = previous

			return i, err
		}

		if note != nil {
			note(i+1, len(facts))
		}

		vector, err := embedder.Embed(ctx, fact.Content)
		if err != nil {
			b.ollama.EmbedName = previous

			return i, fmt.Errorf("re-embedding stopped at %d of %d: %w", i+1, len(facts), err)
		}

		if err := b.DB.ReplaceEmbedding(fact.ID, vector); err != nil {
			b.ollama.EmbedName = previous

			return i, err
		}
	}

	b.Cfg.EmbedModel = model
	b.Log.Info("memories re-indexed", "model", model, "facts", len(facts))

	return len(facts), nil
}
