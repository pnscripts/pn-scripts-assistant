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
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"pn-brain/internal/brain/agent"
	"pn-brain/internal/brain/appearance"
	"pn-brain/internal/brain/config"
	"pn-brain/internal/brain/jobs"
	"pn-brain/internal/brain/learning"
	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/machine"
	"pn-brain/internal/brain/mail"
	"pn-brain/internal/brain/models"
	"pn-brain/internal/brain/places"
	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/protect"
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

	// drive notices the disk the brain lives on going away. See drivewatch.go.
	drive driveState

	// cutShort is how many turns the last run left unfinished. See NoteCutShort.
	cutShort int

	// journeyFrom is where this brain was the last time it was opened, when
	// that is somewhere else — the drive has been carried to another machine.
	// See travelled.go.
	journeyFrom string

	// currentConversation is the one this turn belongs to, so a tool asked to
	// change "this conversation" changes the right one.
	currentConversation int64

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

	// When the last turn was held out loud, which decides whether finished
	// background work is announced or only written down.
	lastSpokenAt time.Time

	// Jobs is work happening behind the conversation, so that "read through
	// that folder" does not mean sitting in silence for four minutes.
	Jobs *jobs.Runner

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
	if cfg.OpenAIKey != "" {
		providers = append(providers, llm.NewOpenAI(cfg.OpenAIKey, cfg.OpenAIModel))
	}

	/*
	 * OpenRouter is one key reaching models from every major company.
	 *
	 * Worth having for its own sake, and it is also the honest answer to "why
	 * only one paid provider": adding two more of them would still be a short
	 * list, where this is most of them behind a single account.
	 */
	if cfg.OpenRouterKey != "" {
		providers = append(providers,
			llm.NewOpenRouter(cfg.OpenRouterKey, cfg.OpenRouterModel))
	}

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
		tools.ListWindows{},
		tools.OpenApp{},
		tools.Click{},
		tools.TypeText{},
		tools.Scroll{},
		tools.WriteDocument{},
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
	b.Jobs = &jobs.Runner{Announce: b.announce}

	available = append(available,
		tools.InBackground{Jobs: backgroundOf{b}, Registry: func() *tools.Registry {
			return b.Agent.Registry
		}},
		tools.ListBackground{Jobs: backgroundOf{b}},
		tools.StopBackground{Jobs: backgroundOf{b}},
		tools.SetWakeWord{Brain: b},
		tools.ListWaiting{Queue: queueOf{b}},
		tools.DecideWaiting{Queue: queueOf{b}},
		tools.ListModels{Machine: b},

		// So "my external drive" can be looked up rather than asked about.
		tools.ListDrives{Root: b.Root},

		// And whether there is a copy of itself anywhere, which is the
		// question somebody asks with their hand on the drive.
		tools.Copies{Root: b.Root, Source: b.DB},

		/*
		 * Changing what has just happened, which is what makes it a
		 * conversation rather than a transcript.
		 *
		 * "Say that again", "forget that", "throw this away" — asked out loud,
		 * in whatever language somebody happens to be speaking, which is why
		 * it is a tool and not a button.
		 */
		tools.Revise{Talk: talkingTo{b}, Now: b.CurrentConversation},

		// Putting a file back the way it was, which is the one word in that
		// vocabulary that used to mean nothing.
		tools.PutBack{Root: root},

		/*
		 * What it is hearing and why it acted or did not, asked out loud.
		 *
		 * "It answered the television" and "it ignored me" are the two
		 * complaints, and from a chair they look the same as each other and as
		 * a broken microphone. Everything that tells them apart is measured;
		 * this is how somebody gets at it without opening a panel in the
		 * middle of the problem.
		 */
		tools.Hearing{Recent: b.recentlyHeard, State: b.howItListens},

		/*
		 * And how long all of that took, which is the other half of the same
		 * complaint.
		 *
		 * "It did not hear me" and "it is too slow" are asked in the same
		 * tone, and on a machine where an answer takes a minute the second is
		 * often mistaken for the first.
		 */
		tools.HowFast{Reading: b.howFast},

		/*
		 * And changing how it behaves, by saying so.
		 *
		 * Most of what somebody wants to adjust about an assistant they adjust
		 * while talking to it. Those all lived in panels, which made the
		 * answer to "how do I change this" be "stop talking to me and go and
		 * find a page" — from the program whose whole argument is that
		 * everything works from inside it.
		 */
		tools.Settings{
			Read:  b.settingsNow,
			Set:   b.changeSetting,
			Voice: b.changeVoice,
			Loud:  b.changeLoudness,
		},

		/*
		 * And saying what all of that is, in the words to ask for it.
		 *
		 * Everything here is reachable by saying a sentence and none of it was
		 * discoverable by saying a sentence. Answered from the registry rather
		 * than from the model's idea of what an assistant is, so it cannot
		 * promise something that was never built.
		 */
		tools.Introduce{Loaded: b.loadedTools, Owner: b.Cfg.Owner},

		// And the one setting that fixes the commonest cause of the confusion.
		tools.Quieten{
			Reroute: func(ctx context.Context, on bool) error {
				if !on {
					speech.PutTheSoundBack(ctx)
					b.Cfg.CancelRoom = false

					return b.Cfg.Save(b.Root)
				}

				if err := speech.CancelWhatThisMachinePlays(ctx); err != nil {
					return err
				}

				b.Cfg.CancelRoom = true

				return b.Cfg.Save(b.Root)
			},
			Rerouting: speech.Rerouting,
		},

		// And the drives and folders it reads from, which is the other half
		// of the same question.
		tools.Places{
			Root:  b.Root,
			Owner: b.Cfg.Owner,
			Learn: func() places.Reads {
				if b.Learner == nil {
					return nil
				}

				return b.Learner
			},
			Seen: b.DB,

			// What it can see to start from, assembled by looking at the
			// machine rather than by anybody typing a path. See Candidates.
			Candidates: b.somewhereToStart,
		},

		/*
		 * What is in the memory, which recall cannot answer.
		 *
		 * Recall is a similarity search with a floor, and "what do you know
		 * about me" is the question least like anything stored — so a brain
		 * holding a thousand facts recalled none and said it had none.
		 */
		tools.WhatYouKnow{Memory: rememberedBy{b.DB}},

		// The thing this program is for, which only the command line could reach.
		tools.LearnFolder{
			// Asked at call time: the learner does not exist yet.
			Learn: func() tools.Ingests {
				if b.Learner == nil {
					return nil
				}

				return b.Learner
			},
			Owner: b.Cfg.Owner,
		},
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
			/*
			 * Switching the automatic choice off pins one model for
			 * everything — but only the one actually settled on.
			 *
			 * Saying "chosen by you" about the value this program shipped
			 * with is a small lie with a real cost: it appears in the
			 * interface beside the model name, so somebody wondering why
			 * every answer takes minutes reads that they picked it, and stops
			 * looking there.
			 */
			if !b.Cfg.AutoModel {
				roles := b.modelRoles()

				why := "chosen by you"
				if !b.Cfg.ModelChosen {
					why = "the best this machine can run"
				}

				return llm.Choice{Model: roles.Work, Why: why}
			}

			return llm.ChooseModel(message, b.modelRoles())
		},
	}

	/*
	 * What to stop and ask about before reading, as its owner set it.
	 *
	 * Loaded before any tool can run. The list itself is only reachable from
	 * setup and the privacy panel — never from a conversation — because a page
	 * that could talk the brain into unprotecting the keys is a page that
	 * could read them.
	 */
	protect.OwnFolder(root)
	protect.Use(protect.Load(root))

	// Where the previous version of anything overwritten is kept, so a change
	// can be undone.
	tools.Root = root

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

	/*
	 * The recogniser is told which names this person uses.
	 *
	 * It knows the language and not the speaker, so a name it has never seen
	 * comes back as the nearest thing in its vocabulary, confidently and
	 * without any mark of doubt: "pnscripts.com" arrived as "pncryptz.com" and
	 * a considered opinion followed about a website that does not exist. The
	 * brain has been reading this machine's files and history for weeks and
	 * already holds the right spellings — the most visited site here is
	 * pnscripts.local, at eleven hundred visits — and none of it was reaching
	 * the recogniser.
	 *
	 * A function rather than a list, read fresh each turn, so a name learned
	 * this morning is heard correctly this afternoon without a restart.
	 */
	speech.SetVocabulary(func() []string {
		facts, err := db.AllFacts()
		if err != nil {
			return nil
		}

		out := make([]string, 0, len(facts))
		for _, f := range facts {
			out = append(out, f.Content)
		}

		return out
	})

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
// ModelRoles is which model does which job on this machine, worked out once.
//
// Exported for the interface, which has to say why an answer was quick or slow
// and cannot do that without naming the model that gave it.
func (b *Brain) ModelRoles() llm.Sizes { return b.modelRoles() }

func (b *Brain) modelRoles() llm.Sizes {
	b.rolesOnce.Do(func() {
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

		/*
		 * Which models to prefer depends on what this machine is.
		 *
		 * Everything here was chosen against four processor cores, where the
		 * size of the model is the whole of the wait. On a machine with a
		 * graphics card that reasoning inverts — a larger model costs memory
		 * rather than minutes — and this program is meant to run on both.
		 */
		power := models.WhatItCanRun(ctx, client)
		workOrder, talkOrder, reasonOrder := llm.ForMachine(string(power.Tier()))

		b.roles.Talk = llm.PickFor(talkOrder, names)
		b.roles.Reason = llm.PickFor(reasonOrder, names)

		// The quick, tool-capable model that answers ordinary turns.
		b.roles.Quick = llm.PickFor(workOrder, names)
		// The reserve is chosen by capability alone, never by speed: it exists
		// precisely to be the thing worth waiting for. Picking it from the same
		// list as the working model made them identical on a machine without a
		// card, leaving nothing to escalate to.
		b.roles.Best = llm.PickFor(llm.SmartestOrder(string(power.Tier())), names)

		/*
		 * And how much may run at once, which is the same question.
		 *
		 * It was a constant, so a workstation with a graphics card ran exactly
		 * as many background jobs as a laptop with four cores and none. On the
		 * laptop that is one too many — every job is a model call on the
		 * processor, and a second one steals from the answer somebody is
		 * waiting for. On the workstation it is four too few.
		 */
		if b.Jobs != nil {
			b.Jobs.AtMost = jobs.HowManyAtOnce(string(power.Tier()))

			b.Log.Info("how much can run at once",
				"tier", power.Tier(), "background jobs", b.Jobs.AtMost)
		}

		b.Log.Info("what this machine can run",
			"tier", power.Tier(), "hardware", power.Describe(),
			"best installed for work", b.roles.Best)

		/*
		 * The effective working model, not the configured one.
		 *
		 * They differ whenever nobody has chosen, which is the ordinary case,
		 * and logging the setting instead of the choice is how the tier came
		 * to be computed and ignored without anybody noticing.
		 *
		 * Worked out here rather than by calling modelRoles, which is the
		 * function this block is inside: sync.Once deadlocks if its own Do
		 * re-enters, so that call hung every turn that needed a model, and it
		 * hung silently.
		 */
		work := b.Cfg.OllamaModel
		if !b.Cfg.ModelChosen && b.roles.Quick != "" {
			work = b.roles.Quick
		}

		// Both, because they are different models now and the difference is
		// minutes: the quick one answers ordinary turns and the reserve is
		// what a demanding request escalates to.
		b.Log.Info("models chosen for each kind of turn",
			"work", work, "talk", b.roles.Talk, "reason", b.roles.Reason,
			"in reserve", b.roles.Best)
	})

	/*
	 * The model that does things is read fresh every time.
	 *
	 * Only what had to be discovered — which small and which reasoning model
	 * are installed — is worked out once. The working model is whatever its
	 * owner has chosen, and they can change that from the Models page while
	 * the brain is running; remembering it here meant the page said one thing
	 * and the brain went on using another until it was restarted.
	 */
	roles := b.roles
	roles.Work = b.Cfg.OllamaModel

	/*
	 * Nobody has chosen, so the machine decides.
	 *
	 * The value shipped with this program was picked against four processor
	 * cores and no graphics card. It was then used unchanged everywhere, which
	 * makes the tier a decoration: the right model for the hardware was worked
	 * out at startup, written into the log, and read by nothing. A computer
	 * with a card ran the model chosen for one without, and — the direction
	 * that actually hurt here — a computer without one ran a
	 * seven-billion-parameter model that takes minutes per round, when a
	 * smaller one it also has answers in a fraction of that and asks for its
	 * tools more reliably.
	 *
	 * A choice made from the Models page still wins, on any machine. This only
	 * fills in for somebody who has never made one, which is everybody until
	 * they do.
	 */
	if !b.Cfg.ModelChosen && roles.Quick != "" {
		roles.Work = roles.Quick
	}

	return roles
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
/*
 * ChooseAutomatically hands the choice of model back to the brain.
 *
 * Picking one by hand pins it for every turn, which is the right answer when
 * somebody wants a particular model and the wrong one the rest of the time:
 * small talk then waits on the big model, and a hard question is answered by
 * whichever one happened to be pinned for a greeting. Auto is what the tiering
 * was built for and what most turns should use.
 */
func (b *Brain) ChooseAutomatically() error {
	b.Cfg.AutoModel = true
	b.Cfg.ModelChosen = false

	if err := b.Cfg.Save(b.Root); err != nil {
		return fmt.Errorf("could not save the choice: %w", err)
	}

	b.Log.Info("the model is chosen per turn again")

	return nil
}

func (b *Brain) UseModel(name string) error {
	if b.ollama == nil {
		return fmt.Errorf("there is no local provider to change")
	}

	b.ollama.ChatModel = name
	b.Cfg.OllamaModel = name
	b.Cfg.ModelChosen = true

	// Pinned by hand means pinned: leaving auto on would let the brain move off
	// the model somebody just asked for, which reads as the setting not working.
	b.Cfg.AutoModel = false

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

	/*
	 * Recordings a previous run could not clear up.
	 *
	 * Each turn removes its own, which works for every turn that finishes and
	 * not for the ones interrupted by the program being killed.
	 */
	if removed, freed := speech.SweepOldRecordings(); removed > 0 {
		b.Log.Info("cleared recordings left by an earlier run",
			"files", removed, "megabytes", freed/(1<<20))
	}

	// The only thing here that speaks without being spoken to first.
	go b.watchReminders(ctx)

	// Both models loaded and kept loaded, so choosing between them costs
	// nothing at the moment of choosing.
	go b.keepModelsWarm(ctx)
	go b.watchTheDrive(ctx)

	// A reading of the machine every second, kept in memory, so the operations
	// view has the shape of the last few minutes rather than one number.
	go machine.Watch(ctx)

	/*
	 * And the machine's own sound through the canceller, if that was asked
	 * for.
	 *
	 * After the canceller exists, which is why it waits: the sink has to be
	 * there before anything can be pointed at it. Put back when the brain
	 * stops — see Stop.
	 */
	// What the room hears while it talks, which is a setting and so has to be
	// applied rather than left at the package default.
	speech.DuckOthersWhileTalking(b.Cfg.KeepQuiet)

	// And how loud its own voice is, which is a setting and so has to be
	// applied rather than left at whatever the package starts with.
	if b.Cfg.VoiceLoudness > 0 {
		speech.SetLoudness(b.Cfg.VoiceLoudness)
	}

	// And anything a previous run left turned down goes back up. The note only
	// exists if that run was killed between lowering a level and restoring it.
	go speech.PutBackAnythingLeftDown()

	if b.Cfg.CancelRoom {
		go func() {
			for i := 0; i < 20; i++ {
				if err := speech.CancelWhatThisMachinePlays(ctx); err == nil {
					b.Log.Info("this machine's sound now goes through the echo canceller")

					return
				}

				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
			}

			b.Log.Warn("could not route this machine's sound through the echo canceller")
		}()
	}

	// And the copies of itself on other drives, refreshed whenever one of them
	// is plugged in and the brain has learned something since.
	go b.keepCopies(ctx)

	// And the drives and folders it looks after, read a bite at a time
	// whenever one of them is attached and holds something it has not seen.
	go b.keepPlaces(ctx)

	// A resident recogniser, started in the background because loading its
	// model takes seconds and nothing should wait on it. Without one, every
	// spoken turn reloads 141MB of weights before looking at any audio — which
	// is bearable once and ruinous in a conversation.
	go func() {
		if err := speech.StartResident(ctx); err != nil {
			b.Log.Info("no resident recogniser; transcription will be slower", "reason", err)
		}
	}()

	/*
	 * And a synthesiser, waiting for the first thing to be said.
	 *
	 * The same argument as the recogniser above, for the same reason at the
	 * other end of the turn: starting piper and loading its voice is 0.94
	 * seconds measured here, and started when there is something to say it
	 * lands entirely in the silence between the answer being ready and being
	 * heard. Started now, it lands in the time between the program opening and
	 * somebody speaking to it, which nobody is waiting through.
	 */
	go speech.WarmTheVoice()
}

// Stop waits for in-flight learning to finish.
func (b *Brain) Stop() {
	if b.Learner != nil {
		b.Learner.Stop()
	}

	// Anything turned down while it was talking goes back up, including after
	// an answer that was interrupted by the program closing.
	speech.PutTheVolumeBack()

	// And the synthesiser kept waiting for the next thing to say.
	speech.StopWarmVoice()

	/*
	 * And the machine's sound goes back where it was.
	 *
	 * Its own context, because the one the brain ran on is already cancelled
	 * by the time anything gets here — and leaving somebody's default output
	 * pointed at a sink that is about to disappear is the kind of parting gift
	 * that gets a program uninstalled.
	 */
	putBack, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()

	speech.PutTheSoundBack(putBack)

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
tool will ask properly.

Some things are asked about before they are done, not after. Sending a
message to anybody, deleting or overwriting anything, spending money,
changing an account or its settings, and telling anyone something
private about %s — for those, say what you are about to do and wait to
be told to go ahead. Not a summary of it: the actual message, the actual
file, the actual amount. Somebody agreeing to "send an email to Anna"
has agreed to nothing.

Never state what is on this machine from memory. What is waiting, what
reminders exist, which models are installed, what a file contains, what
is in the mailbox — every one of those has a tool, and the tool is the
only thing that knows. You do not. Asked any of them, call the tool
first and answer from what it returns.

If you did not call it, say so and stop. A list you made up is worse
than saying nothing, because it cannot be told apart from a real one.

Once a tool has answered, answer from what it returned. Do not say you
are about to check something you have already checked, and do not
describe the checking — %s can see every tool you run and how long it
took, so narrating it says nothing they do not already have and buries
the answer they asked for.

Answer first, in a sentence or two. Then add what is worth adding.
Never open with what you are thinking, what you might do, or which
approach you are considering: that is deliberation, and it belongs
inside your head rather than at the top of a reply. If a tool came back
with nothing useful, say that plainly in one line and say what would
help — not a paragraph about what you tried.

Say what you actually did, by name. "I searched the web" when you
searched the web; never "I am checking the file" when you did nothing of
the kind. A wrong account of your own actions is the one mistake that
cannot be caught by looking at the answer.

You are running on this machine and can read it. Never say you have no
access to files or that you are a text-based model without it — you are
neither, and saying so is a false account of yourself that also refuses
work you were asked to do.

You have a memory and it persists. Never say you have none, that you
cannot remember earlier conversations, or that you are a fresh start:
that is false, it is about the one thing this program exists for, and
somebody told it has no reason to believe anything else you say. If you
have not been given anything to recall, that means nothing was relevant
to this question — not that there is nothing. Call what_you_know and
say what is actually there.

%s

%s

Never call a tool with an example path. "/absolute/path/to/your/projects"
and the like are placeholders out of documentation, not places, and
passing one to a tool produces a confident failure about a directory
nobody has. If you do not know where something is, list one of the places
above and look. Asking which folder is fair when looking has not settled
it; inventing the folder is not.`,
		name, owner, owner, owner, owner, owner, owner,
		b.whereThingsAre(), b.whatIsRemembered())
}

/*
 * whereThingsAre tells the assistant what this machine actually looks like.
 *
 * It was told it had tools and never told where anything is, which leaves
 * "check my projects" unanswerable: there is no path in the question and none
 * in its head. What happened then was worse than asking — it called the
 * directory tool with "/absolute/path/to/your/projects", the placeholder out
 * of a documentation example, and reported the resulting failure as though it
 * had looked somewhere real. Then it said it had no file access at all, having
 * just used the file tool.
 *
 * Real paths, gathered when the prompt is built rather than remembered, since
 * a drive can be plugged in between one question and the next.
 */
/*
 * whatIsRemembered puts the size and shape of the memory in the prompt itself.
 *
 * Asked "what do you know for me?", the brain answered "I know nothing about
 * you. I have no memory of our conversations. I am a fresh start" — while
 * holding one thousand and twenty-nine things about the person asking. Three
 * separate things had to go right for it to answer properly and none of them
 * is guaranteed: the question had to keep its tools, the model had to choose
 * what_you_know, and it had to call the tool rather than announce it. The last
 * one failed even after the first two were fixed.
 *
 * So the answer does not depend on any of them. The counts are in the prompt
 * before a word is generated, which no phrasing and no language can route
 * around, and a model cannot claim to have nothing while looking at how much
 * it has. The tool is still there for the detail; this is the floor.
 */
func (b *Brain) whatIsRemembered() string {
	// No store means nothing to say about it, rather than a crash while
	// building a prompt. Found by a test that builds a Brain without one.
	if b.DB == nil {
		return ""
	}

	total, err := b.DB.CountFacts()
	if err != nil {
		return ""
	}

	if total == 0 {
		return "You have learned nothing yet. Say so plainly if asked, and offer to " +
			"read a folder."
	}

	line := fmt.Sprintf("You remember %d things about %s.", total, b.Cfg.Owner)

	if counts, err := b.DB.FactsByCategory(); err == nil && len(counts) > 0 {
		kinds := make([]string, 0, len(counts))

		for name, n := range counts {
			if n > 0 {
				kinds = append(kinds, fmt.Sprintf("%d from %ss", n, name))
			}
		}

		sort.Strings(kinds)

		if len(kinds) > 0 {
			line += " Roughly " + strings.Join(kinds, ", ") + "."
		}
	}

	return line + " Never say you have no memory: you have this. Call what_you_know " +
		"for the detail, and never say you are about to call it — call it."
}

func (b *Brain) whereThingsAre() string {
	var lines []string

	if home, err := os.UserHomeDir(); err == nil {
		lines = append(lines, "Home folder: "+home)
	}

	if b.Root != "" {
		lines = append(lines, "Your own memory lives in: "+b.Root)
	}

	/*
	 * And the drives, because "my external drive" is a thing people say and
	 * the mount point is not something they know or should have to.
	 */
	if drives, err := storage.Drives(b.Root); err == nil {
		for _, d := range drives {
			where := d.MountPoint
			if where == "" {
				continue
			}

			note := where
			if d.Removable {
				note += " (a drive that was plugged in)"
			}

			lines = append(lines, "Drive: "+note)
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return "Where things are on this machine, as of now:\n" + strings.Join(lines, "\n")
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

	return fmt.Sprintf(spokenPersona, name, owner, owner)
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
/*
 * spokenPersona is who it is when it is being listened to rather than read.
 *
 * Written to sound like somebody competent who is busy, because that is what
 * this is for. The failure mode of an assistant that talks is not being too
 * terse, it is the opposite: the courtesies, the restating of the question, the
 * offer of further help at the end of every answer. Read on a screen that is
 * mildly annoying. Read aloud, on a machine that produces about seven words a
 * second, it is most of the wait.
 *
 * Short, because every word costs real time here. Every line in it is aimed at
 * a specific thing the model does otherwise.
 */
const spokenPersona = `You are %s, %s's assistant, speaking aloud.

Sound like a capable person who is busy: calm, direct, and brief. One or two
sentences, then stop. No lists, no headings, no paths or URLs read out — name
the thing instead.

Never open with "Certainly", "Of course", "Sure", "Great question", or by
repeating what was just asked. Do not end by offering further help; if more is
needed, %s will say so.

While a tool is running, say what you are doing in a few words — "checking the
file", "looking at your mail" — and nothing more. Do not narrate the steps.

When a request could mean two different things, ask which, in one short
question. Guessing wastes a minute here; asking costs three seconds.

Say plainly when you do not know or cannot do something. Never invent what is
on this machine.`

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

	/*
	 * Marked for the whole turn, whatever the microphone does meanwhile.
	 *
	 * The listening loop reopens the moment an answer starts being written, so
	 * the current step alternates between answering and listening several
	 * times a second — and anything reading that to decide whether the brain
	 * is busy flickers. On a machine where a turn takes a minute, "is it
	 * working on my question or not" then has no answer anywhere on screen.
	 */
	progress.StartedATurn()
	defer progress.FinishedATurn()

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

	/*
	 * A question never goes unanswered in the record.
	 *
	 * The reply is written when the turn finishes, which is right until the
	 * turn does not finish. Learning a folder takes over an hour here, and
	 * anything that ends the process in the meantime — a restart, a crash,
	 * closing the window — left the question stored with nothing after it. The
	 * transcript then says the brain was asked something and ignored it, which
	 * is both untrue and the single most damaging thing a record of a
	 * conversation can say.
	 *
	 * Written only if nothing else was: the ordinary paths save their own
	 * answer and clear this.
	 */
	// So a tool asked to change "this conversation" changes this one.
	b.inConversation(conversationID)

	answered := false

	defer func() {
		if answered {
			return
		}

		note := "That turn did not finish — the answer was lost before it could be " +
			"written. Ask again."

		/*
		 * Stopped on purpose reads differently from lost.
		 *
		 * A cancelled context here means somebody pressed stop or said the
		 * brain's name over the top of it, which is a thing they did rather
		 * than a thing that went wrong — and the transcript should not report
		 * a decision as a failure.
		 */
		if err := ctx.Err(); err != nil {
			note = "You stopped that one, so there is no answer to it."
		}

		// Its own context: the turn's is cancelled, which is the case this
		// exists for, and a write on a cancelled context writes nothing.
		if _, err := b.DB.AddMessage(conversationID, llm.RoleAssistant, "", "", note); err != nil {
			b.Log.Warn("could not record that a turn was cut short", "error", err)
		}
	}()

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

		answered = true

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

		answered = true

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
		b.mu.Lock()
		b.lastSpokenAt = time.Now()
		b.mu.Unlock()

		b.Agent.Aloud = func(ctx context.Context) agent.TalkAloud {
			return speech.NewAloud(ctx)
		}

		b.Agent.Interrupted = speech.Interrupted

		// A new turn starts listening again, whatever happened to the last one.
		speech.ClearInterrupt()

		defer func() {
			b.Agent.Aloud = nil
			b.Agent.Interrupted = nil
		}()
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

	/*
	 * Answered either way.
	 *
	 * Waiting for approval is a real end to a turn — the agent has written its
	 * own message saying what it wants to do — so it is not an unanswered
	 * question and must not be marked as one.
	 */
	answered = true

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

/*
 * rememberedBy is the store seen through the narrow hole the tool needs.
 *
 * RecentFacts returns the store's own type, which carries an id and a
 * timestamp the tool has no use for. Converting here keeps the tools package
 * from importing the store to read three fields.
 */
type rememberedBy struct{ db *store.DB }

func (r rememberedBy) CountFacts() (int, error) { return r.db.CountFacts() }

func (r rememberedBy) FactsByCategory() (map[string]int, error) { return r.db.FactsByCategory() }

func (r rememberedBy) RecentFacts(limit int) ([]tools.RecentFact, error) {
	found, err := r.db.RecentFacts(limit)
	if err != nil {
		return nil, err
	}

	out := make([]tools.RecentFact, 0, len(found))

	for _, f := range found {
		out = append(out, tools.RecentFact{Content: f.Content, Category: f.Category})
	}

	return out, nil
}

/*
 * CurrentConversation is the conversation this turn belongs to.
 *
 * Held on the brain rather than passed to the tool, because a tool is built
 * once at startup and a conversation is per turn — and the tool that changes a
 * conversation has to act on the one it is in, not on the one that existed
 * when the program started.
 */
func (b *Brain) CurrentConversation() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.currentConversation
}

func (b *Brain) inConversation(id int64) {
	b.mu.Lock()
	b.currentConversation = id
	b.mu.Unlock()
}

/*
 * talkingTo is the narrow view of the store the revise tool gets.
 *
 * An adapter rather than handing over the database, so a tool that exists to
 * change one conversation cannot reach anything else in it.
 */
type talkingTo struct{ b *Brain }

func (t talkingTo) ForgetLastExchange(id int64) (int, error) { return t.b.DB.ForgetLastExchange(id) }
func (t talkingTo) DeleteConversation(id int64) error        { return t.b.DB.DeleteConversation(id) }
func (t talkingTo) RenameConversation(id int64, title string) error {
	return t.b.DB.RenameConversation(id, title)
}

func (t talkingTo) History(id int64) ([]tools.Message, error) {
	stored, err := t.b.DB.History(id)
	if err != nil {
		return nil, err
	}

	out := make([]tools.Message, 0, len(stored))

	for _, m := range stored {
		out = append(out, tools.Message{Role: m.Role, Content: m.Content})
	}

	return out, nil
}
