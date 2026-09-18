package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Privacy is where work may be done.
type Privacy string

const (
	CloudPreferred Privacy = "cloud_preferred"
	Balanced       Privacy = "balanced"
	LocalPreferred Privacy = "local_preferred"
	LocalOnly      Privacy = "local_only"
)

// Thresholds are how much capacity left counts as what, in percent.
type Thresholds struct {
	Warning  float64 `json:"warning"`  // below this, still used, and said
	Low      float64 `json:"low"`      // below this, others are preferred
	Critical float64 `json:"critical"` // below this, not given new work
}

// Budget is what may be spent and fetched without asking.
type Budget struct {
	MonthlyAPI float64 `json:"monthly_api_usd"`
	TaskAPI    float64 `json:"task_api_usd"`

	MaxDownload  int64 `json:"max_download_bytes"`
	MaxModelSize int64 `json:"max_model_bytes"`
	MaxModelDisk int64 `json:"max_model_disk_bytes"`
	MinFreeDisk  int64 `json:"min_free_disk_bytes"`
}

/*
 * Autonomy is what its owner has said may happen without asking, class by
 * class. Choosing among what is already here never needed asking. Installing
 * a local model, or a pinned tool this program ships a recipe for, may be
 * allowed here once — within the budget's sizes — and then happens without a
 * question each time. Spending, signing in, switching an integration on and
 * anything else outward are not classes here at all: they are always asked.
 */
type Autonomy struct {
	InstallLocalModels bool   `json:"install_local_models"`
	InstallTools       bool   `json:"install_tools"`
	ApprovedAt         string `json:"approved_at,omitempty"`
}

// Policy is its owner's rules for how work is done.
type Policy struct {
	Privacy    Privacy    `json:"privacy"`
	Thresholds Thresholds `json:"thresholds"`
	Budget     Budget     `json:"budget"`
	Autonomy   Autonomy   `json:"autonomy"`

	// Order is how resources are preferred by how they are paid for, first
	// first; empty is the default.
	Order []Tier `json:"order,omitempty"`

	Allowed []string `json:"allowed,omitempty"`
	Blocked []string `json:"blocked,omitempty"`
	Prefer  []string `json:"prefer,omitempty"`

	// Families is the local model families that may be installed; empty is
	// any from the Ollama library.
	Families []string `json:"families,omitempty"`

	// Notify is how much a switch of resource is said: verbose, normal or
	// minimal.
	Notify string `json:"notify"`
}

const gb = int64(1) << 30

/*
 * Default is the policy its owner approved: subscriptions first, balanced
 * privacy, local models and pinned tools installed without asking within
 * ten gigabytes — and twenty left free on the disk — and nothing paid per
 * use without asking, since there is no budget for it until one is set.
 */
func Default() Policy {
	return Policy{
		Privacy:    Balanced,
		Thresholds: Thresholds{Warning: 40, Low: 20, Critical: 10},
		Budget: Budget{MonthlyAPI: 0, TaskAPI: 0, MaxDownload: 10 * gb, MaxModelSize: 10 * gb,
			MaxModelDisk: 40 * gb, MinFreeDisk: 20 * gb},
		Autonomy: Autonomy{InstallLocalModels: true, InstallTools: true, ApprovedAt: "2026-09-18"},
		Notify:   "normal",
	}
}

// PolicyFile is where the policy is kept in the brain's folder.
func PolicyFile(root string) string { return filepath.Join(root, "resources", "policy.json") }

// LoadPolicy reads the owner's policy, or the default when there is none.
func LoadPolicy(root string) (Policy, error) {
	raw, err := os.ReadFile(PolicyFile(root))
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}

	if err != nil {
		return Default(), err
	}

	p := Default()

	if err := json.Unmarshal(raw, &p); err != nil {
		return Default(), fmt.Errorf("%s could not be read: %w", PolicyFile(root), err)
	}

	return p, p.Check()
}

// SavePolicy writes the owner's policy.
func SavePolicy(root string, p Policy) error {
	if err := p.Check(); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	path := PolicyFile(root)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	if err := os.WriteFile(path+".new", append(raw, '\n'), 0o600); err != nil {
		return err
	}

	return os.Rename(path+".new", path)
}

// Check refuses a policy that does not make sense.
func (p Policy) Check() error {
	switch p.Privacy {
	case CloudPreferred, Balanced, LocalPreferred, LocalOnly:
	default:
		return fmt.Errorf("privacy %q is not cloud_preferred, balanced, local_preferred or local_only", p.Privacy)
	}

	t := p.Thresholds
	if !(t.Warning > t.Low && t.Low > t.Critical && t.Critical >= 0 && t.Warning <= 100) {
		return fmt.Errorf("the thresholds must fall: warning > low > critical ≥ 0, and they are %v > %v > %v",
			t.Warning, t.Low, t.Critical)
	}

	switch p.Notify {
	case "verbose", "normal", "minimal":
	default:
		return fmt.Errorf("notify %q is not verbose, normal or minimal", p.Notify)
	}

	return nil
}

func (p Policy) rank(t Tier) int {
	for i, o := range p.Order {
		if o == t {
			return i
		}
	}

	return len(p.Order) + int(t)
}

func (p Policy) blocks(id string) bool {
	if listed(p.Blocked, id) {
		return true
	}

	return len(p.Allowed) > 0 && !listed(p.Allowed, id)
}

/*
 * Within is this policy narrowed by another — a project's or a task's.
 *
 * Only ever narrowed. A project that says local only makes it local only; a
 * project that says cloud preferred cannot open the cloud to an owner who
 * keeps things local. Budgets take the smaller, lists add refusals, an allow
 * list becomes the overlap, and autonomy is allowed only where both allow it.
 */
func (p Policy) Within(o Override) Policy {
	out := p

	if o.Privacy != "" && strictness(o.Privacy) > strictness(out.Privacy) {
		out.Privacy = o.Privacy
	}

	out.Blocked = append(append([]string{}, out.Blocked...), o.Blocked...)

	if len(o.Allowed) > 0 {
		if len(out.Allowed) == 0 {
			out.Allowed = append([]string{}, o.Allowed...)
		} else {
			var both []string

			for _, a := range o.Allowed {
				if listed(out.Allowed, a) {
					both = append(both, a)
				}
			}

			out.Allowed = both

			if len(both) == 0 {
				out.Allowed = []string{"(nothing)"}
			}
		}
	}

	if o.MaxCost != nil && *o.MaxCost < out.Budget.TaskAPI {
		out.Budget.TaskAPI = *o.MaxCost
	}

	if o.MaxDownload != nil && *o.MaxDownload < out.Budget.MaxDownload {
		out.Budget.MaxDownload = *o.MaxDownload
		out.Budget.MaxModelSize = min(out.Budget.MaxModelSize, *o.MaxDownload)
	}

	if o.NoInstalls {
		out.Autonomy.InstallLocalModels, out.Autonomy.InstallTools = false, false
	}

	return out
}

func strictness(p Privacy) int {
	return map[Privacy]int{CloudPreferred: 0, Balanced: 1, LocalPreferred: 2, LocalOnly: 3}[p]
}

/*
 * Override is what a project or a task says about how its work may be done:
 * kept in the project's settings or on the task, and only ever narrowing the
 * owner's policy. Only is not a narrowing of the policy but of the choice:
 * "use Codex for this" — still subject to everything else.
 */
type Override struct {
	Privacy     Privacy  `json:"privacy,omitempty"`
	Allowed     []string `json:"allowed,omitempty"`
	Blocked     []string `json:"blocked,omitempty"`
	Only        string   `json:"only,omitempty"`
	Engine      string   `json:"engine,omitempty"`
	MaxCost     *float64 `json:"max_cost_usd,omitempty"`
	MaxDownload *int64   `json:"max_download_bytes,omitempty"`
	NoInstalls  bool     `json:"no_installs,omitempty"`

	// LocalOnly is paths in a project that never go to a service elsewhere.
	LocalOnly []string `json:"local_only_paths,omitempty"`
}

// ParseOverride reads an override, "" being none.
func ParseOverride(text string) (Override, error) {
	var o Override

	if strings.TrimSpace(text) == "" {
		return o, nil
	}

	err := json.Unmarshal([]byte(text), &o)

	return o, err
}

// ApprovedAt is when the autonomy classes were approved, for showing.
func (a Autonomy) Approved() time.Time {
	t, _ := time.Parse("2006-01-02", a.ApprovedAt)

	return t
}

// Merge is two overrides at once — a project's and a task's — the stricter
// of each where they differ, and the task's own choice of resource.
func Merge(a, b Override) Override {
	out := a

	if strictness(b.Privacy) > strictness(out.Privacy) || out.Privacy == "" {
		if b.Privacy != "" {
			out.Privacy = b.Privacy
		}
	}

	out.Blocked = append(append([]string{}, a.Blocked...), b.Blocked...)

	switch {
	case len(a.Allowed) == 0:
		out.Allowed = b.Allowed
	case len(b.Allowed) > 0:
		var both []string

		for _, x := range a.Allowed {
			if listed(b.Allowed, x) {
				both = append(both, x)
			}
		}

		out.Allowed = both
	}

	if b.Only != "" {
		out.Only = b.Only
	}

	if b.Engine != "" {
		out.Engine = b.Engine
	}

	if b.MaxCost != nil && (a.MaxCost == nil || *b.MaxCost < *a.MaxCost) {
		out.MaxCost = b.MaxCost
	}

	if b.MaxDownload != nil && (a.MaxDownload == nil || *b.MaxDownload < *a.MaxDownload) {
		out.MaxDownload = b.MaxDownload
	}

	out.NoInstalls = a.NoInstalls || b.NoInstalls
	out.LocalOnly = append(append([]string{}, a.LocalOnly...), b.LocalOnly...)

	return out
}
