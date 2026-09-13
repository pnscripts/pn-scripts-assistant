// Package tools is what the brain can do besides talk.
//
// Every tool declares a Risk, and that declaration is the whole basis of the
// approval system. Safe tools observe; Mutating tools change something. A
// Mutating tool never runs without a person saying yes, and the person is shown
// exactly what will happen — not a paraphrase, because an approval you cannot
// verify is not an approval.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Risk decides whether a person has to approve a call.
type Risk string

const (
	// Safe tools only observe. They run without asking, because stopping to
	// approve every directory listing would train the user to click yes.
	Safe Risk = "safe"

	// Mutating tools change something on the machine, or spend money, or reach
	// out in the user's name. These always stop and ask.
	Mutating Risk = "mutating"
)

// Tool is one capability.
type Tool interface {
	Name() string
	Description() string

	// Parameters is a JSON Schema object describing the arguments.
	Parameters() json.RawMessage

	Risk() Risk

	// Summarize renders the call for a human deciding whether to allow it. It
	// must describe exactly what will happen; a summary that omits detail makes
	// the approval worthless.
	Summarize(args json.RawMessage) string

	// Execute performs the call and returns what the model should see.
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

/*
 * Cued is a tool that should only be offered when it is plainly wanted.
 *
 * Optional, and nothing built in implements it. It exists for skills, which
 * are added by their owner and could otherwise be added without limit: how
 * well a model chooses among tools falls off sharply with its size, and the
 * small model here already picks wrongly among thirty-one. A dozen skills
 * offered on every turn would not make it more capable, it would make it worse
 * at everything.
 *
 * A tool that says nothing here is offered as it always was.
 */
type Cued interface {
	// Cues are words that mean this is worth offering. Lower case.
	Cues() []string
}

/*
 * Registry holds the tools available to a brain.
 *
 * Locked, because it is no longer built once and read forever: a skill can be
 * taught in the middle of a conversation, and the turn that teaches it is
 * running on one goroutine while a background task reads the list on another.
 *
 * Version counts changes so that anything caching a view of this — the agent
 * loop caches the schemas, which are expensive to build — can tell that its
 * copy is stale without comparing the lists.
 */
type Registry struct {
	mu      sync.RWMutex
	byName  map[string]Tool
	version int64
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{byName: map[string]Tool{}}

	for _, t := range tools {
		if t != nil {
			r.byName[t.Name()] = t
		}
	}

	return r
}

/*
 * Register adds a tool, or replaces one of the same name.
 *
 * It refuses to replace something it did not add. A skill called run_command
 * would not be a skill, it would be a way to redefine what running a command
 * means — and the approval gate is written against tool names, so the summary
 * somebody reads and the thing that happens could then be different.
 */
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return errors.New("no tool given")
	}

	name := t.Name()

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byName[name]; ok {
		if _, replaceable := existing.(Replaceable); !replaceable {
			return fmt.Errorf("%q is already the name of something built in", name)
		}
	}

	r.byName[name] = t
	r.version++

	return nil
}

// Unregister removes a tool. Removing one that is not there is not an error:
// deleting a skill twice should not fail the second time.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byName[name]; !ok {
		return
	}

	delete(r.byName, name)
	r.version++
}

// Replaceable marks a tool that was added at runtime and may be replaced by
// another of the same name. Nothing built in implements it.
type Replaceable interface {
	AddedAtRunTime()
}

// Version changes whenever the set of tools does.
func (r *Registry) Version() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.version
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.byName[name]

	return t, ok
}

// All returns the tools in a stable order, so the model sees the same list
// every time and its behaviour does not drift with map iteration.
func (r *Registry) All() []Tool {
	r.mu.RLock()

	out := make([]Tool, 0, len(r.byName))

	for _, t := range r.byName {
		out = append(out, t)
	}

	r.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })

	return out
}

// Result is the outcome of one call.
type Result struct {
	Tool    string `json:"tool"`
	Output  string `json:"output"`
	Err     string `json:"error,omitempty"`
	Refused bool   `json:"refused,omitempty"`
}

// argsOf decodes a tool's arguments into v, reporting a readable error.
//
// Small models produce malformed arguments regularly, and a decode failure has
// to come back as something the model can correct rather than as a crash.
func argsOf(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("no arguments were given")
	}

	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("those arguments could not be read: %w", err)
	}

	return nil
}

/*
 * Root is the brain's own folder, for the few tools that need to know it.
 *
 * A package-level value rather than a field on each tool, because it is one
 * value for the life of the program and threading it through every tool that
 * writes a file would be five constructors changed to carry a constant.
 *
 * It is where the previous version of anything overwritten is kept — inside
 * the brain's folder rather than beside the original, since a .bak next to
 * somebody's source file turns up in their editor and their git status and is
 * one more thing to clean up after an assistant.
 */
var Root string
