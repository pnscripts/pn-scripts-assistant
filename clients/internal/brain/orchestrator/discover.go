package orchestrator

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/catalogue"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The one list of what is here, and the one place a choice is made.
 *
 * Discovery reads the machine, the local models, the coding agents and
 * editors, and whatever services are configured — each from the part of the
 * program that already knows it — and keeps the answer for a minute: asking
 * Claude Code whether it is signed in is a program start, and a proposal, a
 * plan and a step all want to know within seconds of each other.
 */

// Sources is where discovery reads from; any may be nil.
type Sources struct {
	Root      string
	DB        *store.DB
	OllamaURL string

	// Agents is whether to look for coding agents: a test does not start
	// somebody's Claude Code.
	Agents bool

	// Services is the configured services that are paid per use, as
	// resources; nil is none.
	Services func() []Resource

	// Library is what local models could be installed.
	Library func(ctx context.Context) ([]catalogue.Model, error)

	// Integrations is the MCP servers known here, as resources.
	Integrations func() []Resource

	// Extra is resources a test supplies, and Executors their adapters.
	Extra     []Resource
	Executors map[string]Executor
}

// Orchestrator is the decider, with its cache.
type Orchestrator struct {
	Sources

	mu      sync.Mutex
	list    []Resource
	machine Machine
	at      time.Time
}

// New is an orchestrator reading from sources.
func New(s Sources) *Orchestrator { return &Orchestrator{Sources: s} }

// Fresh is how long a discovery is kept.
const Fresh = time.Minute

// Resources is everything here, and the machine it is on.
func (o *Orchestrator) Resources(ctx context.Context) ([]Resource, Machine) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.list != nil && time.Since(o.at) < Fresh {
		return append([]Resource{}, o.list...), o.machine
	}

	pol := o.Policy()

	o.machine = ReadMachine(o.Root)

	var list []Resource

	// A test binary does not start somebody's coding agents to ask them
	// anything, unless the test says so.
	if o.Agents && (!testing.Testing() || os.Getenv("PN_TEST_AGENTS") != "") {
		list = append(list, ClaudeCode(o.DB), Codex(o.DB), CursorAgent())
		list = append(list, Editors()...)
	}

	if o.OllamaURL != "" {
		if local, err := LocalModels(ctx, o.OllamaURL, o.machine); err == nil {
			list = append(list, local...)
		} else {
			list = append(list, Resource{ID: "ollama", Kind: Runtime, Title: "Ollama", Local: true,
				State: Offline, Why: "it did not answer: " + err.Error(), Evidence: Verified, Observed: time.Now()})
		}
	}

	if o.Services != nil {
		list = append(list, o.Services()...)
	}

	if o.Integrations != nil {
		list = append(list, o.Integrations()...)
	}

	list = append(list, o.Extra...)

	for i := range list {
		thresholds(&list[i], pol)
	}

	o.list, o.at = list, time.Now()

	return append([]Resource{}, list...), o.machine
}

// Forget drops what was discovered, for after something changed.
func (o *Orchestrator) Forget() {
	o.mu.Lock()
	o.list = nil
	o.mu.Unlock()
}

// Policy is the owner's policy as it is now.
func (o *Orchestrator) Policy() Policy {
	if o.Root == "" {
		return Default()
	}

	p, err := LoadPolicy(o.Root)
	if err != nil {
		return Default()
	}

	return p
}

// thresholds is a known reading turned into a state by the owner's lines.
func thresholds(r *Resource, pol Policy) {
	left, known := r.Remaining()
	if !known || !r.State.Usable() {
		return
	}

	switch {
	case left <= 0:
		r.State, r.Why = LimitReached, "none of its allowance is left"
	case left < pol.Thresholds.Critical:
		r.State, r.Why = LimitNear, fmt.Sprintf("only %.0f%% of its allowance is left", left)
	case left < pol.Thresholds.Low:
		r.State, r.Why = LowCapacity, fmt.Sprintf("%.0f%% of its allowance is left", left)
	}
}

// History is every resource's latest runs.
func (o *Orchestrator) History() History {
	h := History{}

	if o.DB == nil {
		return h
	}

	runs, err := o.DB.Runs("", 500)
	if err != nil {
		return h
	}

	for _, r := range runs {
		if len(h[r.Resource]) < 20 {
			h[r.Resource] = append(h[r.Resource], r)
		}
	}

	return h
}

/*
 * Decide is who does a piece of work, under the owner's policy narrowed by
 * the project's and the task's. When nothing here will do and a local model
 * could — and the need allows this machine — the model to install is worked
 * out and put on the decision, allowed or to be asked about.
 */
func (o *Orchestrator) Decide(ctx context.Context, need Need, over Override) Decision {
	resources, machine := o.Resources(ctx)

	pol := o.Policy().Within(over)
	if over.Only != "" && need.Only == "" {
		need.Only = over.Only
	}

	d := Select(resources, need, pol, o.History())

	if o.Library == nil || localAmong(d) || !ollamaAnswers(resources) {
		return d
	}

	// A test binary does not fetch the model library from the internet.
	if testing.Testing() && os.Getenv("PN_TEST_LIBRARY") == "" {
		return d
	}

	// Nothing local will do this, as the choice or as a fallback: see
	// whether something local could, once installed.
	if library, err := o.Library(ctx); err == nil {
		if plan, err := PlanModel(ctx, need, library, machine, pol); err == nil {
			d.Install = plan
		}
	}

	return d
}

// Note records what came of giving a resource work, and what it says about
// the resource: a failure that is its sign-in or its allowance changes its
// state until it resets.
func (o *Orchestrator) Note(r Resource, work string, taskID, stepID int64, out Outcome, verified bool) {
	if o.DB == nil {
		return
	}

	result := store.RunOK
	if !out.OK {
		result = store.RunFailed
	}

	o.DB.RecordRun(store.ResourceRun{Resource: r.ID, Kind: string(r.Kind), Work: work, Model: out.Model,
		TaskID: taskID, StepID: stepID, Result: result, Failure: out.Failure, Detail: out.Detail,
		Millis: out.Took.Milliseconds(), Verified: verified})

	switch {
	case out.Usage != nil:
		u := *out.Usage
		u.Resource = r.ID
		u.Plan = orElse(u.Plan, r.Plan)

		if out.Failure == FailAuth {
			u.State, u.Detail = string(AuthRequired), "its sign-in has expired"
		}

		o.DB.NoteUsage(u)
	case out.Failure == FailAuth || out.Failure == FailLimit || out.Failure == FailOffline:
		o.DB.NoteUsage(store.Usage{Resource: r.ID, State: string(StateAfter(out.Failure)), Plan: r.Plan,
			Source: "what it answered", Detail: out.Detail})
	}

	if out.Failure != "" || out.Usage != nil {
		o.Forget()
	}
}

// localAmong is whether a decision has something on this machine to do the
// work, chosen or in reserve.
func localAmong(d Decision) bool {
	if d.Primary != nil && d.Primary.Local {
		return true
	}

	for _, f := range d.Fallbacks {
		if f.Local {
			return true
		}
	}

	return false
}

// ollamaAnswers is whether Ollama is there to install into: a model cannot
// be put where nothing is listening.
func ollamaAnswers(list []Resource) bool {
	for _, r := range list {
		if r.ID == "ollama" && r.State == Offline {
			return false
		}
	}

	return true
}

// ExecutorFor is the adapter for an agent: one supplied, or the real one.
func (o *Orchestrator) ExecutorFor(r Resource) (Executor, bool) {
	if e, ok := o.Executors[r.ID]; ok {
		return e, true
	}

	return ExecutorFor(r)
}
