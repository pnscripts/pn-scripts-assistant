/*
 * Package orchestrator decides how a piece of work is done: which engine,
 * which coding agent, which model — and what to fall back to — so that its
 * owner says what they want and not which programs to use.
 *
 * One decision-maker, built on what the program already knows rather than a
 * second copy of it. What is installed and in which version is the provision
 * recipes'; what an engine can do is the engines'; which services exist and
 * whether privacy allows them is llm's; what a project may use is its
 * settings'. This package gathers those into one list of resources with a
 * state each — available, near its limit, signed out, not licensed — and
 * chooses among them for a stated need, saying why in words.
 *
 * Choosing is not permission. Nothing here installs, signs in, spends, or
 * runs anything; a resource that needs any of those is chosen with that
 * written on the decision, and the approval gate decides as it always has.
 * And nothing here invents a number: a subscription whose service does not
 * say how much is left is "unknown", never a percentage.
 */
package orchestrator

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

// Kind is what sort of thing a resource is. An editor is not a model, and
// choosing one must never imply it can do the work by itself.
type Kind string

const (
	// Agent is a coding agent that works on files itself: Claude Code, Codex.
	Agent Kind = "agent"

	// Model is a model this program drives through its own loop and tools.
	Model Kind = "model"

	IDE     Kind = "ide"
	Engine  Kind = "engine"
	Runtime Kind = "runtime"

	// Integration is an MCP server: used through its grants, shown here so
	// what exists is all in one place.
	Integration Kind = "integration"
)

// State is how usable a resource is right now.
type State string

const (
	Available    State = "available"
	Degraded     State = "degraded"
	LowCapacity  State = "low_capacity"
	LimitNear    State = "limit_near"
	LimitReached State = "limit_reached"
	AuthRequired State = "auth_required"
	NotInstalled State = "not_installed"
	NotLicensed  State = "not_licensed"
	NotSupported State = "not_supported"
	Offline      State = "offline"
	Failed       State = "failed"
	Unknown      State = "unknown"
)

// Usable is a state work can be given to.
func (s State) Usable() bool { return s == Available || s == Degraded || s == LowCapacity }

// Level is how a resource's state is known.
type Level string

const (
	Verified Level = "verified" // tried on this machine
	Reported Level = "reported" // said by the resource or its own files
	Inferred Level = "inferred" // worked out, not seen
)

// Tier is how a resource is paid for, in the order they are preferred.
type Tier int

const (
	Subscription Tier = iota + 1 // already paid for, with capacity included
	Included                     // free or included capacity elsewhere
	Local                        // on this machine, installed
	Installable                  // on this machine once installed
	Metered                      // paid per use
)

func (t Tier) String() string {
	return map[Tier]string{Subscription: "an existing subscription", Included: "included capacity",
		Local: "this machine", Installable: "this machine, once installed", Metered: "paid per use"}[t]
}

// Work is what a piece of work asks of whoever does it.
const (
	Code   = "code"   // repository-level coding
	Edit   = "edit"   // small, repetitive changes
	Plan   = "plan"   // planning
	Reason = "reason" // architecture and debugging
	Write  = "write"  // documentation and prose
	Vision = "vision" // looking at pictures
	Chat   = "chat"
)

// Resource is one thing work can be given to, and how it stands.
type Resource struct {
	ID      string `json:"id"`
	Kind    Kind   `json:"kind"`
	Title   string `json:"title"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`

	State    State     `json:"state"`
	Why      string    `json:"why,omitempty"`
	Evidence Level     `json:"evidence"`
	Observed time.Time `json:"observed_at"`

	// Local is whether it runs on this machine, so nothing it is given
	// leaves it.
	Local bool `json:"local"`

	// Can is how well it does each kind of work, 0 to 10.
	Can map[string]int `json:"can,omitempty"`

	Tier Tier   `json:"tier"`
	Plan string `json:"plan,omitempty"`

	// Usage is the last reading of what is left of it, nil when nobody has
	// ever said.
	Usage *store.Usage `json:"usage,omitempty"`

	// Model is the model it thinks with, where that is one thing; Models is
	// what else it may be asked to use.
	Model  string   `json:"model,omitempty"`
	Models []string `json:"models,omitempty"`

	// Executes is whether this program can hand it work on a project and
	// read back what it did. An editor, and an agent without an adapter
	// here, cannot.
	Executes bool `json:"executes"`

	Licence string   `json:"licence,omitempty"`
	Size    int64    `json:"size,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

// Remaining is the share of capacity left, and whether anybody said.
func (r Resource) Remaining() (float64, bool) {
	if r.Usage == nil || r.Usage.Remaining == nil {
		return 0, false
	}

	return *r.Usage.Remaining, true
}

// Find is a resource by id.
func Find(list []Resource, id string) (Resource, bool) {
	for _, r := range list {
		if strings.EqualFold(r.ID, id) {
			return r, true
		}
	}

	return Resource{}, false
}

// Need is what a piece of work requires of whoever does it.
type Need struct {
	Work string `json:"work"`

	// Files is work on a project's files: an agent that edits them itself,
	// or a model with this program's file tools.
	Files bool `json:"files,omitempty"`

	// Local keeps it on this machine whatever else allows.
	Local bool `json:"local,omitempty"`

	// Only is the one resource the task was told to use, when it was.
	Only string `json:"only,omitempty"`

	// Estimate is how much of a subscription's window it is expected to use,
	// as a percentage; 0 when nobody can say.
	Estimate float64 `json:"estimate,omitempty"`

	// Without is resources already tried and failed for this work.
	Without []string `json:"without,omitempty"`
}

// Rejection is a resource that was not chosen, and why.
type Rejection struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Why   string `json:"why"`
}

// Decision is what to use, what next if it fails, and why.
type Decision struct {
	Need      Need        `json:"need"`
	Primary   *Resource   `json:"primary,omitempty"`
	Fallbacks []Resource  `json:"fallbacks,omitempty"`
	Rejected  []Rejection `json:"rejected,omitempty"`
	Reasons   []string    `json:"reasons,omitempty"`

	// FallbackReasons is why each fallback would be used, in the same order,
	// so a failover says the reasons for what it moved to — not the ones for
	// what failed.
	FallbackReasons [][]string `json:"fallback_reasons,omitempty"`

	// Install is a local model to put on this machine because nothing here
	// will do, when policy allows it.
	Install *ModelPlan `json:"install,omitempty"`

	// Blocked is why nothing can do this, when nothing can.
	Blocked string    `json:"blocked,omitempty"`
	At      time.Time `json:"at"`
}

// Next is the decision with its primary gone, the first fallback in its
// place: what a failover moves to.
func (d Decision) Next() (Decision, bool) {
	if len(d.Fallbacks) == 0 {
		return d, false
	}

	next := d
	primary := d.Fallbacks[0]
	next.Primary = &primary
	next.Fallbacks = append([]Resource{}, d.Fallbacks[1:]...)
	next.Reasons = nil

	if len(d.FallbackReasons) > 0 {
		next.Reasons = d.FallbackReasons[0]
		next.FallbackReasons = append([][]string{}, d.FallbackReasons[1:]...)
	}

	return next, true
}

/*
 * Explain is the decision in words, the way the owner reads it: what was
 * chosen and the reasons, what comes next if it fails, and the local
 * fallback named apart — the one that works when every service is down.
 */
func (d Decision) Explain() string {
	var b strings.Builder

	if d.Primary == nil {
		fmt.Fprintf(&b, "Nothing here can do this: %s", d.Blocked)

		if d.Install != nil {
			fmt.Fprintf(&b, "\nIt would take installing %s (%s).", d.Install.Name, sizeWords(d.Install.Size))
		}

		return b.String()
	}

	fmt.Fprintf(&b, "Selected %s because:", d.Primary.Title)

	for _, r := range d.Reasons {
		b.WriteString("\n  - " + r)
	}

	local := ""

	for i, f := range d.Fallbacks {
		if i < 3 {
			fmt.Fprintf(&b, "\nFallback: %s", f.Title)
		}

		if f.Local && local == "" && !d.Primary.Local {
			local = f.Title
		}
	}

	if local != "" {
		fmt.Fprintf(&b, "\nLocal fallback: %s", local)
	}

	if d.Install != nil {
		fmt.Fprintf(&b, "\nWould install first: %s (%s, %s)", d.Install.Name, sizeWords(d.Install.Size), d.Install.Licence)
	}

	// Why the others were not used, which is most of what a decision is:
	// signed out, out of allowance, kept here by privacy. What was never the
	// kind of thing for this work — an embedding model for code — is left out.
	for _, r := range d.Rejected {
		if !strings.HasPrefix(r.Why, "it is not suited to") {
			fmt.Fprintf(&b, "\nNot used: %s — %s", r.Title, r.Why)
		}
	}

	return b.String()
}

func sizeWords(bytes int64) string {
	switch {
	case bytes <= 0:
		return "size not known"
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	default:
		return fmt.Sprintf("%d MB", bytes>>20)
	}
}

/*
 * Select chooses who does a piece of work.
 *
 * In two passes, the way the owner's rule reads. First what cannot be used
 * at all is set aside, each with its reason: not able to do this kind of
 * work, not usable right now, not allowed by privacy or the owner's lists,
 * would cost money the budget does not have, would run out part-way. Then
 * what is left is ordered: by how well it does the work, in bands, so a
 * subscription never wins over something clearly better suited; within a
 * band by how it is paid for — subscriptions first — then by how much it has
 * left, then by its record here.
 */
func Select(resources []Resource, need Need, pol Policy, history History) Decision {
	d := Decision{Need: need, At: time.Now()}

	var usable []Resource

	for _, r := range resources {
		if r.Kind != Agent && r.Kind != Model {
			continue
		}

		if why := unusable(r, need, pol); why != "" {
			d.Rejected = append(d.Rejected, Rejection{ID: r.ID, Title: r.Title, Why: why})

			continue
		}

		usable = append(usable, r)
	}

	if pol.Privacy == LocalPreferred || pol.Privacy == LocalOnly {
		// Local first whenever something local does the work adequately.
		if hasLocal(usable, need) {
			sort.SliceStable(usable, func(i, j int) bool { return usable[i].Local && !usable[j].Local })
		}
	}

	sort.SliceStable(usable, func(i, j int) bool {
		a, b := usable[i], usable[j]

		if pol.Privacy == LocalPreferred && a.Local != b.Local && hasLocal(usable, need) {
			return a.Local
		}

		if ba, bb := band(a.Can[need.Work]), band(b.Can[need.Work]); ba != bb {
			return ba > bb
		}

		if pol.Privacy == CloudPreferred && a.Local != b.Local {
			return !a.Local
		}

		if a.Tier != b.Tier {
			return pol.rank(a.Tier) < pol.rank(b.Tier)
		}

		if ha, hb := health(a, pol), health(b, pol); ha != hb {
			return ha > hb
		}

		if ra, rb := history.Rate(a.ID), history.Rate(b.ID); ra != rb {
			return ra > rb
		}

		if a.Can[need.Work] != b.Can[need.Work] {
			return a.Can[need.Work] > b.Can[need.Work]
		}

		return a.ID < b.ID
	})

	if len(usable) == 0 {
		d.Blocked = "nothing that can do " + workWords(need.Work) + " is usable now"

		if len(d.Rejected) > 0 {
			d.Blocked += ": " + summary(d.Rejected)
		}

		return d
	}

	primary := usable[0]
	d.Primary = &primary
	d.Fallbacks = usable[1:]
	d.Reasons = reasonsFor(primary, need, pol, history)

	for _, f := range d.Fallbacks {
		d.FallbackReasons = append(d.FallbackReasons, reasonsFor(f, need, pol, history))
	}

	return d
}

func hasLocal(list []Resource, need Need) bool {
	for _, r := range list {
		if r.Local && band(r.Can[need.Work]) >= 1 {
			return true
		}
	}

	return false
}

// band is how well something does a kind of work, coarsely: suitability
// decides first, and within a band the cheaper way wins.
func band(score int) int {
	switch {
	case score >= 8:
		return 3
	case score >= 5:
		return 2
	case score >= 3:
		return 1
	}

	return 0
}

// health is how far from its limit a resource is: known and healthy first,
// unknown next, then lower, then near the end.
func health(r Resource, pol Policy) int {
	left, known := r.Remaining()

	switch {
	case !known:
		return 2
	case left > pol.Thresholds.Warning:
		return 3
	case left > pol.Thresholds.Low:
		return 2
	case left > pol.Thresholds.Critical:
		return 1
	}

	return 0
}

// unusable is why a resource cannot be given this work, or empty.
func unusable(r Resource, need Need, pol Policy) string {
	switch {
	case need.Only != "" && !strings.EqualFold(need.Only, r.ID):
		return "the task names " + need.Only
	case listed(need.Without, r.ID):
		return "it already failed at this"
	case band(r.Can[need.Work]) == 0:
		return "it is not suited to " + workWords(need.Work)
	case need.Files && r.Kind == Agent && !r.Executes:
		return "this program cannot hand it work and read back what it did"
	case need.Files && r.Kind == Model && r.Can["tools"] == 0:
		return "it cannot use tools, so it cannot work on files"
	case !r.State.Usable():
		why := string(r.State)
		if r.Why != "" {
			why += ": " + r.Why
		}

		return why
	case (need.Local || pol.Privacy == LocalOnly) && !r.Local:
		return "privacy keeps this work on this machine"
	case pol.blocks(r.ID):
		return "your resource policy blocks it"
	case r.Tier == Metered && pol.Budget.MonthlyAPI <= 0:
		return "it is paid per use, and the budget for that is nothing"
	case r.Tier == Metered && need.Only == "":
		// No service here says what a call cost, and a price list typed in
		// would be invented; a budget nobody counts against is not one.
		return "it is paid per use, and what it spends cannot be counted here yet, so it is used only when a task names it"
	}

	if left, known := r.Remaining(); known && left-need.Estimate < pol.Thresholds.Critical {
		return fmt.Sprintf("only %.0f%% of its capacity is left, which this work could use up", left)
	}

	return ""
}

func reasonsFor(r Resource, need Need, pol Policy, history History) []string {
	quality := map[int]string{3: "very well suited", 2: "well suited", 1: "adequately suited"}[band(r.Can[need.Work])]

	out := []string{fmt.Sprintf("%s to %s", quality, workWords(need.Work))}

	switch r.Tier {
	case Subscription:
		out = append(out, "it uses "+orPlan(r.Plan, "a subscription already paid for"))
	case Local:
		out = append(out, "it runs on this machine: nothing leaves it, and it costs nothing")
	default:
		out = append(out, "it is "+r.Tier.String())
	}

	if left, known := r.Remaining(); known {
		out = append(out, fmt.Sprintf("%.0f%% of its capacity is left", left))
	} else if r.Tier == Subscription {
		out = append(out, "how much of it is left is not known — its service does not say, so nothing is assumed")
	}

	if rate := history.Rate(r.ID); rate > 0 && history.Count(r.ID) >= 3 {
		out = append(out, fmt.Sprintf("%.0f%% of its work here passed this program's own checks", rate*100))
	}

	if pol.Privacy == LocalPreferred && r.Local {
		out = append(out, "privacy prefers this machine")
	}

	return out
}

func orPlan(plan, fallback string) string {
	if plan == "" {
		return fallback
	}

	return "the " + plan + " subscription already paid for"
}

func workWords(work string) string {
	return map[string]string{Code: "coding on a project", Edit: "small changes", Plan: "planning",
		Reason: "architecture and debugging", Write: "writing", Vision: "looking at pictures", Chat: "conversation"}[work]
}

func summary(list []Rejection) string {
	parts := make([]string, 0, len(list))

	for i, r := range list {
		if i == 4 {
			parts = append(parts, fmt.Sprintf("and %d more", len(list)-4))

			break
		}

		parts = append(parts, r.Title+" — "+r.Why)
	}

	return strings.Join(parts, "; ")
}

func listed(list []string, id string) bool {
	for _, v := range list {
		if strings.EqualFold(v, id) {
			return true
		}
	}

	return false
}

// History is what came of giving resources work here before.
type History map[string][]store.ResourceRun

/*
 * Rate is the share of a resource's recent runs whose work this program then
 * verified: a check that passed on what it wrote, an engine run that passed.
 * Finishing is not the same thing. A model that answered every time and
 * whose code failed every check had a perfect record by finishing — and a
 * proposal said so.
 */
func (h History) Rate(id string) float64 {
	runs := h[id]
	if len(runs) == 0 {
		return 0
	}

	ok := 0

	for _, r := range runs {
		if r.Result == store.RunOK && r.Verified {
			ok++
		}
	}

	return float64(ok) / float64(len(runs))
}

// Count is how many runs there are to judge by.
func (h History) Count(id string) int { return len(h[id]) }
