/*
 * Package environs is what is on this machine, in one list.
 *
 * There were four answers to that question and none of them was the whole
 * answer. The orchestrator knew about coding agents, editors and models,
 * because it has to choose between them. provision knew about engines and
 * runtimes, because it can install them. preflight knew what setup needs.
 * engines knew which game engines this program can drive. Nothing knew about
 * git, docker, python or a browser, and — the part that mattered — none of it
 * was ever told to the model. Asked "can you make a Godot game", the assistant
 * answered from its own tool descriptions rather than from the Godot sitting
 * at a known path.
 *
 * So: one list, one type, one state vocabulary, one cache. What is here, what
 * is not, and what is here but wants something before it can be used — which
 * are three different answers and were previously two, because a thing that
 * needed configuring was simply left out and could not be spoken about.
 *
 * This package looks at the machine and nothing else. What the assistant can
 * do with a model, an integration or a mailbox is known further up, and is
 * added through Sources rather than imported here: a package that discovers
 * the machine should not depend on the program that runs on it.
 */
package environs

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Kind groups what was found, for a reader rather than for the code.
type Kind string

const (
	// Runtime is a language or platform something is written for.
	Runtime Kind = "runtime"

	// Tool is a program used from a terminal.
	Tool Kind = "tool"

	// Engine is a game engine.
	Engine Kind = "engine"

	// Editor is where a person writes code.
	Editor Kind = "editor"

	// Coder is an AI agent that writes code on somebody's subscription.
	Coder Kind = "coder"

	// Model is something that thinks, local or hosted.
	Model Kind = "model"

	// Integration is an MCP server or anything else added to reach further.
	Integration Kind = "integration"

	// Service is a paid API this program can call.
	Service Kind = "service"

	// Ability is something this program itself can do — its own tools.
	Ability Kind = "ability"

	// Browser is a web browser on this machine.
	Browser Kind = "browser"
)

/*
 * State is how usable something is, and the whole point of this package.
 *
 * Four answers where the program used to have two. "Not configured" is not
 * "not permitted" and neither is "not installed": each wants a different
 * sentence from the assistant and a different action from its owner, and
 * collapsing them is how somebody ends up being told to install what they
 * already have.
 */
type State string

const (
	// Here and usable now.
	Here State = "available"

	// WantsSetting is installed and needs something first — a key, a
	// mailbox, an address, a sign-in.
	WantsSetting State = "needs_configuration"

	// WantsSignIn is installed and configured and nobody is signed in.
	WantsSignIn State = "needs_sign_in"

	// WantsLicence is installed and not licensed for this use.
	WantsLicence State = "needs_licence"

	// WantsInstalling is not on the machine, and could be.
	WantsInstalling State = "not_installed"

	// Blocked is here and cannot be used: a permission, a folder nobody may
	// read. Different from absent, because the remedy is different.
	Blocked State = "blocked"

	// Spent is here and has nothing left — an allowance used up.
	Spent State = "no_capacity"

	// Unknown is what could not be established. Said rather than guessed.
	Unknown State = "unknown"
)

// Usable reports whether something can be used right now.
func (s State) Usable() bool { return s == Here }

/*
 * Thing is one capability, however it was found.
 *
 * Deliberately flat and small. This is read by three things that disagree
 * about everything else — the model's prompt, the interface, and the code
 * that decides which tools to offer — and a shape that suits all three is a
 * shape with no cleverness in it.
 */
type Thing struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Title string `json:"title"`

	State State `json:"state"`

	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`

	// Why is one sentence about the state, when the state is not Here.
	Why string `json:"why,omitempty"`

	// Needs is what would make it usable, in words somebody can act on.
	Needs string `json:"needs,omitempty"`

	// Local is whether using it keeps everything on this machine.
	Local bool `json:"local"`

	// Observed is when this was last established, so a stale answer can be
	// told from a fresh one rather than presented as timeless.
	Observed time.Time `json:"observed_at"`
}

/*
 * Sources are where the world is read from.
 *
 * Programs is this package's own business — what is installed on the
 * machine. Everything else is supplied by whoever builds the world, because
 * models, integrations, services and the assistant's own abilities are known
 * further up and this package must not reach for them.
 */
type Sources struct {
	// Programs to look for. Nil means Known, which is the ordinary case; a
	// test supplies its own.
	Programs []Program

	// More are the things this package cannot see for itself. Each is asked
	// on every refresh and may be slow; all of them run together.
	More []func(context.Context) []Thing

	// Hidden is asked about every thing found, so that a decision made by
	// somebody's settings is applied in one place rather than in each reader.
	// Nil hides nothing.
	Hidden func(id string) bool
}

/*
 * World is what is on this machine, kept for a while.
 *
 * Kept because the question is asked constantly — every turn's prompt, every
 * time somebody opens the interface, every proposal — and answered by running
 * thirty programs, which is a second of the machine's time and not something
 * to spend on every message. Long enough that it is not noticed, short enough
 * that installing something and asking about it a minute later gives the
 * right answer.
 */
type World struct {
	Sources

	// Fresh is how long a reading is kept. Zero means HowLongFresh.
	Fresh time.Duration

	mu   sync.Mutex
	list []Thing
	at   time.Time
}

// HowLongFresh is the default life of a reading.
const HowLongFresh = 10 * time.Minute

// New is a world that reads from these sources.
func New(s Sources) *World { return &World{Sources: s} }

// All is everything known, reading the machine again if the last reading is
// old.
func (w *World) All(ctx context.Context) []Thing {
	w.mu.Lock()
	defer w.mu.Unlock()

	fresh := w.Fresh
	if fresh <= 0 {
		fresh = HowLongFresh
	}

	if w.list != nil && time.Since(w.at) < fresh {
		return append([]Thing(nil), w.list...)
	}

	w.list = w.read(ctx)
	w.at = time.Now()

	return append([]Thing(nil), w.list...)
}

/*
 * Again reads the machine now, whatever the cache says.
 *
 * For the moment after something is installed: the program that installed it
 * knows, and waiting ten minutes to believe it would make the assistant deny
 * having what it has just put there.
 */
func (w *World) Again(ctx context.Context) []Thing {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.list = w.read(ctx)
	w.at = time.Now()

	return append([]Thing(nil), w.list...)
}

// Find is one thing by its id, and whether it is known at all.
func (w *World) Find(ctx context.Context, id string) (Thing, bool) {
	for _, thing := range w.All(ctx) {
		if thing.ID == id {
			return thing, true
		}
	}

	return Thing{}, false
}

// Usable reports whether something can be used right now. Unknown ids are
// not usable, which is the safe reading and the honest one.
func (w *World) Usable(ctx context.Context, id string) bool {
	thing, known := w.Find(ctx, id)

	return known && thing.State.Usable()
}

// OfKind is everything of one kind, in the order All returns.
func (w *World) OfKind(ctx context.Context, kind Kind) []Thing {
	var out []Thing

	for _, thing := range w.All(ctx) {
		if thing.Kind == kind {
			out = append(out, thing)
		}
	}

	return out
}

/*
 * read does the work, with the sources running together.
 *
 * Together because the slow ones are slow for unrelated reasons — a program
 * that takes a second to say its version, a service that has to be asked over
 * the network — and one after another they would add up to a wait somebody
 * notices.
 */
func (w *World) read(ctx context.Context) []Thing {
	programs := w.Programs
	if programs == nil {
		programs = Known()
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		found []Thing
	)

	add := func(things []Thing) {
		mu.Lock()
		found = append(found, things...)
		mu.Unlock()
	}

	wg.Add(1)

	go func() {
		defer wg.Done()

		add(Look(ctx, programs))
	}()

	for _, source := range w.More {
		if source == nil {
			continue
		}

		wg.Add(1)

		go func(ask func(context.Context) []Thing) {
			defer wg.Done()

			add(ask(ctx))
		}(source)
	}

	wg.Wait()

	return w.tidy(found)
}

/*
 * tidy settles duplicates and puts the list in a fixed order.
 *
 * Duplicates happen honestly: Ollama is a program on the PATH and also the
 * thing that holds the models, and two sources may both describe it. The
 * better-informed answer wins — something usable beats something absent —
 * because a source that found it running knows more than one that looked for
 * a file.
 *
 * The order is fixed so that the block given to the model is the same from
 * one turn to the next. A list that reshuffles itself is a prompt that cannot
 * be cached, and on a machine without a graphics card that is minutes.
 */
func (w *World) tidy(found []Thing) []Thing {
	best := make(map[string]Thing, len(found))

	for _, thing := range found {
		if w.Hidden != nil && w.Hidden(thing.ID) {
			continue
		}

		if thing.Observed.IsZero() {
			thing.Observed = time.Now()
		}

		was, seen := best[thing.ID]
		if !seen || better(thing, was) {
			best[thing.ID] = thing
		}
	}

	out := make([]Thing, 0, len(best))

	for _, thing := range best {
		out = append(out, thing)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return kindOrder(out[i].Kind) < kindOrder(out[j].Kind)
		}

		if out[i].State != out[j].State {
			return stateOrder(out[i].State) < stateOrder(out[j].State)
		}

		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})

	return out
}

// better says which of two answers about the same thing to keep.
func better(now, was Thing) bool {
	if stateOrder(now.State) != stateOrder(was.State) {
		return stateOrder(now.State) < stateOrder(was.State)
	}

	// Same state: the one that says more.
	return len(now.Version)+len(now.Path)+len(now.Why) >
		len(was.Version)+len(was.Path)+len(was.Why)
}

func kindOrder(k Kind) int {
	for i, each := range []Kind{Model, Coder, Runtime, Engine, Editor, Tool, Browser, Integration, Service, Ability} {
		if each == k {
			return i
		}
	}

	return 99
}

func stateOrder(s State) int {
	for i, each := range []State{Here, WantsSignIn, WantsSetting, WantsLicence, Spent, Blocked, WantsInstalling, Unknown} {
		if each == s {
			return i
		}
	}

	return 99
}
