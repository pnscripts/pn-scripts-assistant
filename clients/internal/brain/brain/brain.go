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

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/appearance"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/jobs"
	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/machine"
	"pn-scripts-assistant/internal/brain/mail"
	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/places"
	"pn-scripts-assistant/internal/brain/profile"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/protect"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/smarthome"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/brain/storage"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tasks"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
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

	// Whether the pause for a full review queue has already been explained.
	// Checked every ten minutes for as long as it stays full, and a program
	// that repeats itself every ten minutes is one somebody turns off.
	saidQueueIsFull bool

	// Jobs is work happening behind the conversation, so that "read through
	// that folder" does not mean sitting in silence for four minutes.
	Jobs *jobs.Runner

	/*
	 * Permits is what the brain has been allowed to do on this machine.
	 *
	 * Separate from Cfg.Privacy on purpose, and separate from the registry
	 * too: a capability existing, being permitted to leave the machine, and
	 * being permitted to act are three questions, and this program used to
	 * have one answer for all of them.
	 */
	Permits *permits.Book

	// Learner runs the Extractor/Validator/Curator pipeline in the background.
	// Nil when no local model is available, since extraction must stay local.
	Learner *learning.Worker

	// Agent runs tools. It stops rather than acting when a tool would change
	// something, so approval stays a real gate rather than a notification.
	Agent *agent.Loop

	/*
	 * TaskAgent is a second loop, for work nobody is sitting in front of.
	 *
	 * Its own instance rather than the conversation's, because that one has
	 * fields set and unset around a spoken turn and a task running beside a
	 * conversation would read them mid-change. It is also quiet: the panel
	 * saying what the brain is doing belongs to whoever is waiting for an
	 * answer, and a task in the background must not take the line.
	 */
	TaskAgent *agent.Loop

	// Tasks is work that outlives the sentence that asked for it.
	Tasks *tasks.Conductor

	// taught is which skills are currently in the registry, so a reload can
	// remove the ones whose files have gone.
	taught taught
}

// New assembles a brain from settings.
func New(db *store.DB, cfg config.Config, root, dbPath string, logger *slog.Logger) *Brain {
	/*
	 * What may leave this machine follows how much it asks, which is one
	 * setting rather than two. See llm.ModeFor: the second setting was the
	 * one that actually stood in the way, and it refused rather than asked.
	 */
	mode := llm.ModeFor(cfg.Asking())

	// What the interface looks like, so that being asked to change it is
	// something the brain can do rather than something it agrees to.
	look := appearance.Open(root)

	ollama := llm.NewOllama(cfg.OllamaURL, cfg.OllamaModel, cfg.EmbedModel)

	providers := providersFor(cfg, ollama)

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

	/*
	 * The web tools are registered whatever the privacy setting, and hidden
	 * from the model when it forbids them.
	 *
	 * They used to be left out entirely, for a good reason: a tool the model
	 * can see is a tool it will try, and an assistant that keeps proposing
	 * something it may never do is worse than one that simply cannot. The
	 * reason still holds and the mechanism has moved — they are filtered out
	 * of what the model is shown, by the live setting, on every turn.
	 *
	 * Left out at startup, privacy could only change by restarting: somebody
	 * switching to research saw the file change and nothing else, because the
	 * tools that make research mean anything were decided minutes earlier.
	 */
	available = append(available,
		tools.FetchURL{},
		/*
		 * And the same page opened properly, when the plain fetch is not
		 * enough.
		 *
		 * Hidden by the same privacy rule as the other two, because it reaches
		 * the web exactly as they do — more thoroughly, in fact, since it runs
		 * whatever the page is made of.
		 */
		tools.ReadAPage{},

		tools.WebSearch{BraveKey: cfg.BraveKey},
	)

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
		/*
		 * Registered as pointers so the connection can be replaced.
		 *
		 * By value they were copies, and ReloadHome's type switch would never
		 * have matched one — so an address typed into the running program
		 * would have looked saved and changed nothing, which is precisely the
		 * bug this reload exists to fix.
		 */
		available = append(available, &tools.ListDevices{Home: home}, &tools.SetDevice{Home: home})
	}

	// Added after the brain exists, because they change the brain's own
	// settings or act on its own queue, and so need a handle to it.
	b.Jobs = &jobs.Runner{Announce: b.announce}

	/*
	 * What it has been allowed to do, read once at the start.
	 *
	 * A failure here is reported and not fatal, because the failure mode is
	 * safe by construction: an empty book grants nothing and everything falls
	 * back to asking. Refusing to start would refuse somebody their assistant
	 * over a file that only ever widens what it can do.
	 */
	book, err := permits.Load(root)
	if err != nil {
		logger.Warn("permissions could not be read, so nothing is granted", "error", err)
	}

	b.Permits = book

	available = append(available,
		/*
		 * Teaching it how a job is done here.
		 *
		 * Writing one stops for approval and the summary is the whole skill,
		 * word for word — because what is written is followed on later turns,
		 * so it shapes what the assistant does for weeks. Reading one back is
		 * Safe, because it returns text: every action in a skill is still
		 * carried out by the ordinary tools with the gates they always had.
		 */
		/*
		 * The diary, which is a table here rather than an account somewhere.
		 *
		 * Reading it is Safe. Putting something in and changing something are
		 * not: an appointment invented at the wrong hour is worse than one
		 * never made, because its owner acts on it — so the summary shows the
		 * day, the time and the name, which is a thing they can check.
		 */
		/*
		 * Changing a document rather than rebuilding it.
		 *
		 * The program could read eight formats and create four, and could
		 * change none of them — so "put the new name in that contract" meant
		 * rebuilding the contract from what could be read out of it, losing
		 * the letterhead, the table and everything else that made it one.
		 */
		tools.EditDocument{Root: root},

		/*
		 * Pictures, and films made out of them.
		 *
		 * Which way a picture is made is decided in one place — see
		 * studioOf.Painter — because a description of what somebody wants a
		 * picture of is often the most revealing sentence they will write all
		 * week, and whether it stays here must not depend on which tool asked.
		 */
		tools.MakeAPicture{Studio: studioOf{b}},
		tools.MakeAVideo{Studio: studioOf{b}},

		tools.WhatIsOn{DB: db},
		tools.PutInTheDiary{DB: db},
		tools.ChangeTheDiary{DB: db},

		tools.RememberHowToDoThis{Brain: teachingOf{b}},
		tools.ForgetHowToDoThis{Brain: teachingOf{b}},
		tools.WhatIveBeenTaught{Brain: teachingOf{b}},

		tools.InBackground{Jobs: backgroundOf{b}, Registry: func() *tools.Registry {
			return b.Agent.Registry
		}},
		tools.ListBackground{Jobs: backgroundOf{b}},
		tools.StopBackground{Jobs: backgroundOf{b}},
		tools.SetWakeWord{Brain: b},
		tools.ListWaiting{Queue: queueOf{b}},
		tools.DecideWaiting{Queue: queueOf{b}},
		tools.ListModels{Machine: b},

		/*
		 * Looking after the machine, by talking to it.
		 *
		 * The panels could already do all of this. What they could not do is
		 * answer "what is missing?" — which is the question somebody actually
		 * has, and which they had to translate into knowing that a Parts panel
		 * exists and where. Installing is Mutating and goes through
		 * permissions like every other change; looking is Safe.
		 */
		tools.WhatItNeeds{Parts: machineParts},
		tools.InstallPart{Parts: machineParts, Start: b.startInstalling},
		tools.InstallModel{Start: b.startPulling},
		tools.RemovePart{Parts: machineParts, Remove: b.startRemoving},

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
		/*
		 * Asking, rather than guessing or ploughing on.
		 *
		 * Registered like any other capability so it is in the list the model
		 * reads every turn. A convention in the prompt is something a model
		 * does when it remembers to; a tool is something it can see.
		 */
		tools.Ask{Owner: b.Cfg.Owner},

		/*
		 * Making games with Godot.
		 *
		 * The reference lookup is the one that earns its place: a model asked
		 * to write GDScript writes it whether or not it knows the engine, and
		 * what comes out is fluent and half invented. Looking a class up is
		 * authoritative, costs a second, and is checkable.
		 */
		tools.GodotStatus{
			Places: func() []string {
				list, err := places.List(b.Root)
				if err != nil {
					return nil
				}

				out := make([]string, 0, len(list))

				for _, p := range list {
					out = append(out, p.Path)
				}

				return out
			},
			// Reaching out is privacy's question, asked fresh: an update check
			// is a small thing to leak and it still says this machine exists.
			Online: func() bool { return b.Cfg.LookOnline },
		},
		tools.GodotDocs{Online: func() bool { return b.Cfg.LookOnline }},
		tools.GodotBuild{},

		tools.Remind{Diary: diaryOf{b}},
		tools.ListReminders{Diary: diaryOf{b}},
		tools.ForgetReminder{Diary: diaryOf{b}},

		/*
		 * Subtitles for a film that has none.
		 *
		 * Asked for by name, one at a time, because a feature this expensive
		 * has to be chosen rather than triggered. Listing what has none is
		 * separate and safe: it is the question somebody asks before deciding,
		 * and answering it must not start an hour of work.
		 */
		tools.FilmsWithoutSubtitles{Places: func() []string {
			// Only where the brain has been given permission to look.
			list, err := places.List(b.Root)
			if err != nil {
				return nil
			}

			out := make([]string, 0, len(list))

			for _, p := range list {
				out = append(out, p.Path)
			}

			return out
		}},
		tools.MakeSubtitles{Recogniser: func() (string, string, string) {
			// Asked at call time, like the learner above: the recogniser is
			// found on disk and can appear or vanish while this runs.
			r, _ := speech.FindRecogniser()
			if r == nil {
				return "", "", ""
			}

			return r.Command, r.Model, speech.Language()
		}},
	)

	b.Agent = &agent.Loop{
		DB:       db,
		Log:      logger,
		Registry: tools.NewRegistry(available...),

		/*
		 * What the privacy setting forbids, asked fresh on every turn.
		 *
		 * The reason it is a function rather than a list: privacy can change
		 * while the program is running, and it used to be able to change only
		 * by restarting — which meant somebody switching to research watched
		 * the file change and nothing else happen.
		 */
		/*
		 * What may be done on this machine, which is not what may leave it.
		 *
		 * Asked fresh on every call for the same reason OffLimits is: somebody
		 * can grant a permission in the approval they are looking at, and the
		 * next call in the same turn should already know about it.
		 */
		MayI: func(who, tool string, changesSomething bool, level risk.Level) permits.Answer {
			if b.Permits == nil {
				if changesSomething {
					return permits.Ask
				}

				return permits.Allow
			}

			return b.Permits.DecideAt(who, tool, changesSomething, b.Freedom(), level)
		},

		NothingAsks: func() bool { return b.Freedom() == permits.Everything },

		OffLimits: func(tool string) bool {
			switch tool {
			// read_a_page belongs here for a sharper reason than the other
			// two: it does not merely request a page, it runs whatever the
			// page is made of.
			case "fetch_url", "web_search", "read_a_page":
				return !b.Mode.AllowsWeb()
			}

			return false
		},

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
	 * The same registry, the same gate, the same privacy rule — and quiet.
	 *
	 * Sharing MayI and OffLimits is not a convenience: a task must not be a
	 * way round the approval gate, and the surest way to guarantee that is for
	 * there to be one gate rather than two that are meant to agree.
	 */
	b.TaskAgent = &agent.Loop{
		DB:          db,
		Log:         logger,
		Registry:    b.Agent.Registry,
		MayI:        b.Agent.MayI,
		NothingAsks: b.Agent.NothingAsks,
		OffLimits:   b.Agent.OffLimits,
		Quietly:     true,
	}

	/*
	 * And whatever its owner has taught it, before anything can be asked.
	 *
	 * A failure here is reported and not fatal, in the same spirit as the
	 * permissions book: a folder that cannot be read means no skills, which is
	 * every brain until somebody writes one. Refusing to start would refuse
	 * somebody their assistant over a file that only ever adds to it.
	 */
	if count, err := b.ReloadSkills(); err != nil {
		logger.Warn("skills could not be read", "error", err)
	} else if count > 0 {
		logger.Info("loaded what it has been taught", "skills", count)
	}

	/*
	 * And what work there is in the world, as against who here does it.
	 *
	 * Written on every start rather than once. It is a no-op after the first
	 * — the writer compares before it writes, and reports nothing changed —
	 * and doing it this way is what makes a job added in a later version of
	 * the program simply appear rather than needing a migration of its own.
	 *
	 * Nothing here can undo an edit: what shipped loses to what somebody
	 * wrote by hand, decided in the store rather than trusted to the caller.
	 */
	if done, err := occupations.Ensure(db); err != nil {
		logger.Warn("the list of jobs could not be written", "error", err)
	} else if done.Jobs > 0 || done.Capabilities > 0 {
		logger.Info("learned what work there is",
			"jobs", done.Jobs, "capabilities", done.Capabilities, "links", done.Links)
	}

	/*
	 * Work that outlives the sentence that asked for it.
	 *
	 * Built here rather than beside the Runner, because it holds the quiet
	 * loop and that does not exist until the conversation's does. Built too
	 * early it captured a nil, and the first task on a real machine took the
	 * whole program down with it — in a background goroutine, where a panic is
	 * not something a request can survive.
	 *
	 * Provider is a function rather than a provider, for the same reason
	 * OffLimits is: privacy can change while a task is running, and a task
	 * holding a hosted provider it was given twenty minutes ago would keep
	 * sending to it after somebody switched to private. Asked at every step,
	 * the task stops at the next one instead.
	 */
	b.Tasks = &tasks.Conductor{
		DB:       db,
		Log:      logger,
		Jobs:     b.Jobs,
		Agent:    b.TaskAgent,
		Provider: func(name string) (llm.Provider, error) { return b.Router.Provider(name) },
		Sizes:    b.modelRoles,
		Roster:   func() []team.Agent { return team.Roster(b.Root) },

		/*
		 * What the organisation looks like, what the jobs are, and what tools
		 * exist — read afresh for every step rather than captured once.
		 *
		 * All three are things somebody may change while a task is running:
		 * a unit file edited, a job imported, a skill taught. A task holding
		 * who somebody was twenty minutes ago is the same mistake as one
		 * holding a provider from twenty minutes ago.
		 */
		Chart:       func() []org.Unit { return org.Chart(b.Root) },
		Occupations: db,
		Toolbox:     b.Agent.Registry,
		Prompt:      func(provider string) string { return b.personaFor(provider, true) },
		Say:         b.SayInto,
		Budget:      tasks.Sensible(),
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

	// And goals that come round on their own — only the ones told to. See
	// watchGoals.
	go b.watchGoals(ctx)

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
		name = config.DefaultName
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

When a request has two readings that lead to different work, call
ask_first and put the choice to %s. Not "shall I go on" — the
actual question, with the two readings named. One question, then stop;
their next message is the answer. Guessing well is worse than asking,
because a confident wrong reading is indistinguishable from a right one
until the work is done.

Ask while you are working too, not only at the start. If what you find
changes what was asked for — the file is not what its name says, the
folder holds ten thousand things rather than ten, the thing you were
told to change has already been changed — stop and say so. Carrying on
around a surprise is how a small misunderstanding becomes an hour of
the wrong work.

Ask once and about something that matters. Two readings that lead to
the same work is not a question, and neither is checking that you were
listened to. If you can find the answer with a tool, use the tool
instead: asking somebody what is in their own folder is not diligence.

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
		name, owner, owner, owner, owner, owner, owner, owner,
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

/*
 * personaFor is who the assistant is, for this turn and this model.
 *
 * Two decisions in one place. How much of the persona — the long one for
 * something being read, the short one for something being heard, where every
 * token of prompt is a second of silence before anybody hears a word. And
 * whether what its owner wrote about themselves goes with it.
 *
 * The profile is held back from a paid service by default, and it is a
 * separate decision from privacy rather than a consequence of it. Opening
 * privacy says this conversation may be answered elsewhere. It does not say
 * that a standing description of somebody's business should be attached to
 * every single turn, including the ones with nothing to do with it. The model
 * on this machine is given it in every mode, because nothing leaves.
 */
func (b *Brain) personaFor(provider string, spoken bool) string {
	persona := b.SystemPrompt()

	most := 0

	if spoken {
		persona = b.spokenSystemPrompt()
		most = profile.SpokenRunes
	}

	if !b.mayReadProfileTo(provider) {
		return persona
	}

	block := profile.Block(b.Root, most)
	if block == "" {
		return persona
	}

	return persona + "\n\n" + block
}

// mayReadProfileTo says whether this model may be told about its owner.
func (b *Brain) mayReadProfileTo(provider string) bool {
	if provider == llm.Local {
		return true
	}

	return b.Cfg.ProfileToHosted
}

// spokenSystemPrompt is who the assistant is, said briefly.
func (b *Brain) spokenSystemPrompt() string {
	name := b.Cfg.Name
	if name == "" {
		name = config.DefaultName
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

	// Steps is the same work with its detail kept, so the interface can let
	// somebody open one and see what was actually run and what came back.
	Steps []agent.Step `json:"steps,omitempty"`

	// AlreadySpoken is true when the answer was said aloud as it was written,
	// so the page must not send it to be spoken a second time.
	AlreadySpoken bool `json:"already_spoken"`

	// TaskID is set when the turn started a piece of work rather than
	// answering, so the page can offer to go and watch it.
	TaskID int64 `json:"task_id,omitempty"`
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

	/*
	 * Asked for as a job, so it becomes one and the turn ends here.
	 *
	 * The turn ends immediately, which is the whole point: a task takes
	 * minutes, and somebody who has just asked for one should be able to carry
	 * on talking rather than watch a spinner. What it is going to do is
	 * written down before any of it runs, so it can be read — and stopped —
	 * from the Tasks view first.
	 */
	if request, forced, worth := b.worthPlanning(req); worth {
		task, started, err := b.Tasks.Take(ctx, conversationID, request, provider.Name(), forced)

		switch {
		case err != nil:
			b.Log.Warn("could not start a task", "error", err)
		case started:
			answer := "Started: " + task.Name +
				". I'll work through it and tell you when it's done."

			if _, err := b.DB.AddMessage(conversationID, llm.RoleAssistant, "", "", answer); err != nil {
				return ChatReply{}, err
			}

			answered = true

			return ChatReply{
				ConversationID: conversationID,
				Reply:          answer,
				Provider:       provider.Name(),
				TaskID:         task.ID,
			}, nil
		}
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

	if llm.AllowsMemoryFor(provider.Name(), b.Router.Mode()) {
		recalled = b.recall(ctx, req.Message)

		if req.Spoken && len(recalled) > SpokenRecallLimit {
			recalled = recalled[:SpokenRecallLimit]
		}
	}

	/*
	 * The stored persona row is the record; what is sent is built now.
	 *
	 * It used to be written once, when the conversation was created, and never
	 * looked at again — so the parts of it that are computed went stale the
	 * moment anything changed. A thread open since the morning told the model
	 * how many things were remembered that morning, and listed the drives that
	 * were plugged in then. Anything its owner wrote about themselves since
	 * would never have been read at all.
	 *
	 * Rebuilt per turn it also picks the right length: the short persona for a
	 * spoken turn, where prompt size is the whole of the wait, and the full one
	 * for a typed one.
	 */
	for i := range messages {
		if messages[i].Role == llm.RoleSystem {
			messages[i].Content = b.personaFor(provider.Name(), req.Spoken)

			break
		}
	}

	factLimit := RecalledFactLimit
	if req.Spoken {
		factLimit = SpokenFactLimit
	}

	/*
	 * Noted as used, because they are about to be.
	 *
	 * The only feedback available without asking somebody to rate their own
	 * assistant, and a real one: a fact that keeps coming up when the subject
	 * comes up is a fact about something that keeps coming up. It moves recall
	 * by a few hundredths — see store.worth — which is enough to break a tie
	 * between two facts that both fit and not enough to lift one that does not.
	 *
	 * Written here rather than in recall(), because the interface searches
	 * memory through the same function and somebody looking something up is
	 * not the assistant finding it useful.
	 */
	if len(recalled) > 0 {
		used := make([]int64, 0, len(recalled))

		for _, r := range recalled {
			used = append(used, r.ID)
		}

		if err := b.DB.Recalled(used); err != nil {
			b.Log.Warn("could not note what was recalled", "error", err)
		}
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
		Steps:            result.Steps,
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

/*
 * UsePrivacy changes what is allowed to leave this machine, while it runs.
 *
 * It used to be read once at startup, so changing it in the panel wrote the
 * file, said "Saved", and did nothing — while the panel went on reporting the
 * old setting, because the brain reports what it is using rather than what the
 * file says. A saved change that looks like a failed one is worse than a
 * refusal: somebody sets it three times, sees no effect, and concludes the
 * program is lying about its privacy.
 *
 * Both places that hold the mode are set, and they are the only two: the brain
 * answers questions about it and the router enforces it. Everything that
 * obtains a provider goes through the router, so there is no path that
 * captured the old mode and could go on using it.
 */
func (b *Brain) UsePrivacy(mode string) (llm.Mode, error) {
	parsed := llm.ParseMode(mode)

	/*
	 * An unrecognised value is refused rather than quietly read as private.
	 *
	 * ParseMode is deliberately strict for configuration, where the safe
	 * reading of a typo is the strict one. Here it would mean somebody asking
	 * for "reserch" and being told they now have privacy they did not ask for
	 * — the right answer either way, and it has to be said rather than done
	 * silently.
	 */
	if string(parsed) != strings.ToLower(strings.TrimSpace(mode)) {
		return b.Mode, fmt.Errorf("there is no privacy setting called %q", mode)
	}

	/*
	 * And it moves the one switch rather than a second one beside it.
	 *
	 * There is a single setting now — how much it asks — and what may leave
	 * this machine is read off it. Anything still setting privacy by name is
	 * setting that, which is why this maps back rather than storing a value
	 * of its own: two settings that are meant to agree eventually do not.
	 */
	freedom := permits.AskEveryTime

	switch parsed {
	case llm.ModeOpen:
		freedom = permits.Everything
	case llm.ModeResearch:
		freedom = permits.WhatIveAllowed
	}

	if _, err := b.UseFreedom(string(freedom)); err != nil {
		return b.Mode, err
	}

	return parsed, nil
}

/*
 * Freedom is how much the brain may do on this machine without asking.
 *
 * Read from the configuration on every call rather than held, so that changing
 * it takes effect in the next thing the brain does rather than at the next
 * launch — which is the bug privacy had, and it was not obvious then either:
 * the file changed, the program did not, and the person watching concluded the
 * setting was broken.
 *
 * Anything unrecognised is the careful one. A typo in a permission setting
 * must not be read as more permission.
 */
func (b *Brain) Freedom() permits.Freedom {
	b.mu.Lock()
	f := permits.Freedom(strings.TrimSpace(strings.ToLower(b.Cfg.Asking())))
	b.mu.Unlock()

	if !permits.Known(f) {
		return permits.AskEveryTime
	}

	return f
}

/*
 * UseFreedom changes how much the brain may do, and says what that means.
 *
 * Set from the Permissions panel only, never from a conversation — the same
 * rule privacy has, and for a stronger reason. A model that can widen its own
 * permissions by being asked to has no permissions: anything that can talk to
 * it, including a web page it was told to read, can ask.
 */
func (b *Brain) UseFreedom(level string) (permits.Freedom, error) {
	f := permits.Freedom(strings.TrimSpace(strings.ToLower(level)))

	if !permits.Known(f) {
		return b.Freedom(), fmt.Errorf(
			"%q is not something I understand. It is ask, granted, or everything", level)
	}

	/*
	 * Both written down together, before the file is saved.
	 *
	 * They were written in two steps once, and the file ended up saying
	 * private while the program was open — which is the exact disagreement
	 * between two settings that having one switch was meant to end.
	 */
	mode := llm.ModeFor(string(f))

	b.mu.Lock()
	b.Cfg.Freedom = string(f)
	b.Cfg.Privacy = string(mode)
	b.Mode = mode
	cfg := b.Cfg
	b.mu.Unlock()

	if err := cfg.Save(b.Root); err != nil {
		return b.Freedom(), fmt.Errorf("could not write it down: %w", err)
	}

	/*
	 * And what may leave this machine follows it, at once.
	 *
	 * The whole point of one switch is that there is nothing else to
	 * remember: somebody who has just said "allow everything" and then finds
	 * the program still refusing to reach a model has been given a setting
	 * that does not do what it says. Applied to the running router rather
	 * than only written down, because a task in flight asks the router afresh
	 * at every step.
	 */
	if b.Router != nil {
		b.Router.UseMode(mode)
	}

	b.Log.Info("freedom changed", "level", f, "leaving this machine", mode)

	return f, nil
}

/*
 * providersFor is which model providers exist, given the keys.
 *
 * Its own function because it is now needed twice: once when the brain is
 * built, and again when somebody types a key into the Models page. It used to
 * happen only in New, so a key entered into a running program did nothing at
 * all until the next launch — the same shape of bug privacy had, and it
 * presents identically: the setting saves, the page says so, nothing works.
 *
 * A provider with no key is not registered. Registering one anyway would show
 * it in the interface and fail the moment it was chosen.
 */
func providersFor(cfg config.Config, ollama llm.Provider) []llm.Provider {
	providers := []llm.Provider{ollama}

	/*
	 * One pass over the list of companies, rather than a branch each.
	 *
	 * This used to name three of them by hand, which is the reason there were
	 * three: a fourth meant editing here as well as the settings file, the
	 * setup picker and the key check, and whoever added the third had already
	 * missed one of those. Now a company exists in llm.Services or it does not
	 * exist at all.
	 */
	for _, svc := range llm.Services() {
		key := cfg.ProviderKeys[svc.ID]

		// A company with no key is not registered. Registering one anyway
		// would show it in the interface and fail the moment it was chosen.
		if svc.NeedsKey && key == "" {
			continue
		}

		model := cfg.ProviderModels[svc.ID]
		if model == "" {
			model = svc.Model
		}

		if svc.Native {
			providers = append(providers, llm.NewAnthropic(key, model))

			continue
		}

		base := svc.BaseURL

		// The custom entry carries no address of its own — it is whatever
		// somebody typed, and without one there is nothing to talk to.
		if given := cfg.ProviderURLs[svc.ID]; given != "" {
			base = given
		}

		if base == "" {
			continue
		}

		providers = append(providers, &llm.OpenAICompatible{
			ProviderName: svc.ID,
			BaseURL:      base,
			APIKey:       key,
			Model:        model,
		})
	}

	return providers
}

/*
 * ReloadProviders rebuilds the router from the current keys.
 *
 * Called when somebody connects or disconnects a provider, so it takes effect
 * in the next thing the brain does rather than at the next launch. The privacy
 * mode is carried across rather than re-read: connecting a model is not a
 * privacy decision and must not quietly become one.
 */
func (b *Brain) ReloadProviders() {
	b.mu.Lock()
	cfg := b.Cfg
	b.mu.Unlock()

	ollama := llm.NewOllama(cfg.OllamaURL, cfg.OllamaModel, cfg.EmbedModel)

	router := llm.NewRouter(b.Router.Mode(), cfg.DefaultProvider, providersFor(cfg, ollama)...)

	b.mu.Lock()
	b.ollama = ollama
	b.Router = router
	b.mu.Unlock()

	b.Log.Info("providers rebuilt", "count", len(providersFor(cfg, ollama)))
}

/*
 * ReloadHome rebuilds the connection to the house from the current settings.
 *
 * The same reason as ReloadProviders: the tools hold a client built at startup,
 * so an address typed into a running program changed the file and nothing
 * else. Three settings in this program have had that bug now — privacy, the
 * model keys, and this — which is enough for it to be the first thing to check
 * whenever a setting appears not to work.
 */
func (b *Brain) ReloadHome() {
	b.mu.Lock()
	cfg := b.Cfg
	b.mu.Unlock()

	home := smarthome.New(cfg.HomeAssistantURL, cfg.HomeAssistantToken)

	for _, t := range b.Agent.Registry.All() {
		switch tool := t.(type) {
		case *tools.ListDevices:
			tool.Home = home

		case *tools.SetDevice:
			tool.Home = home
		}
	}

	b.Log.Info("home connection rebuilt", "configured", home.Configured())
}
