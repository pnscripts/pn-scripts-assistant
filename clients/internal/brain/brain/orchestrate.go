package brain

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/bootstrap"
	"pn-scripts-assistant/internal/brain/browse"
	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/catalogue"
	"pn-scripts-assistant/internal/brain/display"
	"pn-scripts-assistant/internal/brain/engines"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/workspace"
)

/*
 * The orchestrator, fed from what this brain already knows: its settings for
 * where Ollama is and which services have keys, its router for what privacy
 * lets conversations reach, its engines, its recipes, and its record of what
 * has worked here before.
 */

type orchestration struct {
	once sync.Once
	o    *orchestrator.Orchestrator
}

// Orchestrator is the one decider for how work is done.
func (b *Brain) Orchestrator() *orchestrator.Orchestrator {
	b.orch.once.Do(func() {
		b.orch.o = orchestrator.New(orchestrator.Sources{
			Root: b.Root, DB: b.DB, OllamaURL: b.Cfg.OllamaURL, Agents: true,
			Services:     b.meteredServices,
			Integrations: b.integrationResources,
			Library: func(ctx context.Context) ([]catalogue.Model, error) {
				return catalogue.Fetch(ctx, &http.Client{Timeout: 30 * time.Second})
			},
		})
	})

	return b.orch.o
}

// meteredServices is every hosted service with a key here, as a resource
// paid for by the call — and held back where privacy keeps conversations
// on this machine, since this program's own loop reaches them through the
// same router.
func (b *Brain) meteredServices() []orchestrator.Resource {
	var out []orchestrator.Resource

	for _, s := range llm.Services() {
		if s.ID == "custom" || strings.TrimSpace(b.Cfg.ProviderKeys[s.ID]) == "" {
			continue
		}

		r := orchestrator.Resource{ID: "api:" + s.ID, Kind: orchestrator.Model, Title: s.Name + " (API)",
			Tier: orchestrator.Metered, State: orchestrator.Available, Evidence: orchestrator.Reported,
			Model: b.Cfg.ProviderModels[s.ID], Executes: true, Observed: time.Now(),
			Can: map[string]int{orchestrator.Code: 8, orchestrator.Edit: 8, orchestrator.Plan: 8,
				orchestrator.Reason: 8, orchestrator.Write: 8, orchestrator.Chat: 8, "tools": 1}}

		if b.Router != nil && !b.Router.Mode().AllowsProvider(s.ID) {
			r.State, r.Why = orchestrator.NotSupported, "privacy keeps this program's own model calls on this machine"
		}

		out = append(out, r)
	}

	return out
}

// engineProfiles is what each engine makes: where it runs, in how many
// dimensions, in which language — facts about the engine, not about this
// machine, which the options add.
var engineProfiles = map[string]struct {
	platforms, dims []string
	language        string
	highEnd         bool
}{
	"threejs":    {[]string{"web"}, []string{"2d", "3d"}, "javascript", false},
	"phaser":     {[]string{"web"}, []string{"2d"}, "javascript", false},
	"babylonjs":  {[]string{"web"}, []string{"3d"}, "javascript", false},
	"playcanvas": {[]string{"web"}, []string{"3d"}, "javascript", false},
	"godot":      {[]string{"desktop", "web"}, []string{"2d", "3d"}, "gdscript", false},
	"unity":      {[]string{"desktop", "web", "mobile"}, []string{"2d", "3d"}, "csharp", false},
	"unreal":     {[]string{"desktop"}, []string{"3d"}, "cpp", true},
	"bevy":       {[]string{"desktop"}, []string{"2d", "3d"}, "rust", false},
	"defold":     {[]string{"desktop", "web", "mobile"}, []string{"2d"}, "lua", false},
	"gamemaker":  {[]string{"desktop"}, []string{"2d"}, "gml", true},
}

// engineOptions is each package's engine as it stands on this machine.
func (b *Brain) engineOptions(pkgs []capability.Package) []orchestrator.EngineOption {
	var out []orchestrator.EngineOption

	for _, p := range pkgs {
		if p.Engine == "" || b.Engines == nil {
			continue
		}

		a, ok := b.Engines.Get(p.Engine)
		if !ok {
			continue
		}

		profile := engineProfiles[p.Engine]

		o := orchestrator.EngineOption{Package: p.ID, Engine: p.Engine, Title: a.Title(),
			Platforms: profile.platforms, Dims: profile.dims, Language: profile.language, HighEnd: profile.highEnd}

		installed, found := a.Engine()
		o.Installed, o.Version = found, installed.Version

		if a.Recipe() != "" {
			st := b.Recipes().Check(a.Recipe(), "")

			if found && a.Licensed() {
				o.Licence = st.LicenceState
			}

			if !found {
				o.Installable = st.Installable && !st.NeedsRoot
				o.Size = st.Size
			}
		}

		if _, err := a.Scaffold(engines.Scaffold{Name: "Probe"}); err == nil {
			o.CanCreate = true
		}

		switch p.Engine {
		case "threejs", "phaser", "babylonjs", "playcanvas":
			o.Picture = browse.Possible()
		case "godot":
			_, err := display.Available()
			o.Picture = err == nil
		}

		o.Proven = b.proven("engine:" + p.Engine)

		out = append(out, o)
	}

	return out
}

// proven is what has worked with a resource on this machine, by the name of
// the work.
func (b *Brain) proven(resource string) []string {
	runs, err := b.DB.Runs(resource, 50)
	if err != nil {
		return nil
	}

	seen := map[string]bool{}

	var out []string

	for _, r := range runs {
		if r.Result == store.RunOK && r.Work != "" && !seen[r.Work] {
			seen[r.Work] = true
			out = append(out, r.Work)
		}
	}

	return out
}

// chooseEngine is bootstrap's Choose: the orchestrator's engine choice among
// the packages that could make the game.
func (b *Brain) chooseEngine(request string, candidates []capability.Package) bootstrap.Chosen {
	coder := b.Orchestrator().Decide(context.Background(), orchestrator.Need{Work: orchestrator.Code, Files: true}, orchestrator.Override{})
	strong := coder.Primary != nil && coder.Primary.Can[orchestrator.Code] >= 8

	choice := orchestrator.ChooseEngine(b.engineOptions(candidates), request, strong)

	if choice.Chosen == nil {
		return bootstrap.Chosen{Ask: choice.Ask}
	}

	for _, p := range candidates {
		if p.ID == choice.Chosen.Package {
			reasons := choice.Reasons

			for _, r := range choice.Rejected {
				reasons = append(reasons, "not "+r.Title+": "+r.Why)
			}

			return bootstrap.Chosen{Package: p, Reasons: reasons}
		}
	}

	return bootstrap.Chosen{Ask: "the engine chosen belongs to no package here"}
}

// projectOverride is what a project's settings say about how its work may
// be done, or nothing.
func projectOverride(dir string) orchestrator.Override {
	cfg, err := workspace.Load(dir)
	if err != nil || len(cfg.Resources) == 0 {
		return orchestrator.Override{}
	}

	o, _ := orchestrator.ParseOverride(string(cfg.Resources))

	return o
}

/*
 * stack is bootstrap's Stack: how a project's work will be done, in lines —
 * who writes it and why, what takes over if that fails, what runs here if
 * every service is out, and that checking it is this program's own work
 * rather than anybody's word.
 */
func (b *Brain) stack(dir string, pkg capability.Package) []string {
	if pkg.Work != capability.Project {
		return nil
	}

	d := b.Orchestrator().Decide(context.Background(), orchestrator.Need{Work: orchestrator.Code, Files: true}, projectOverride(dir))

	return stackLines(d, pkg.Engine)
}

func stackLines(d orchestrator.Decision, engine string) []string {
	var lines []string

	switch {
	case d.Primary == nil && d.Install != nil && d.Install.Auto:
		lines = append(lines, "Writes it: "+d.Install.Name+" (local), installed first — nothing here can do it now")
	case d.Primary == nil && d.Install != nil:
		lines = append(lines, "Nobody here can write it now")
	case d.Primary == nil:
		lines = append(lines, "Nobody can write it now: "+d.Blocked)
	default:
		who := d.Primary.Title
		if d.Primary.Model != "" && d.Primary.Kind == orchestrator.Agent {
			who += " (" + d.Primary.Model + ")"
		}

		lines = append(lines, "Writes it: "+who+" — "+strings.Join(d.Reasons, "; "))

		var next []string

		local := ""

		for _, f := range d.Fallbacks {
			next = append(next, f.Title)

			if f.Local && local == "" && !d.Primary.Local {
				local = f.Title
			}
		}

		if len(next) > 0 {
			lines = append(lines, "If that fails or runs low: "+strings.Join(next, ", then "))
		}

		if local != "" {
			lines = append(lines, "On this machine, if every service is out: "+local)
		}
	}

	if d.Install != nil {
		how := "installed without asking, within your policy"
		if !d.Install.Auto {
			how = "asked first: " + d.Install.Ask
		}

		lines = append(lines, fmt.Sprintf("Would install %s (%s, licence %q) — %s", d.Install.Name,
			sizeText(d.Install.Size), d.Install.Licence, how))
	}

	said := 0

	for _, r := range d.Rejected {
		// What was never the kind of thing for this work — an embedding
		// model, for code — is not a reason worth reading.
		if said == 4 || strings.HasPrefix(r.Why, "it is not suited to") {
			continue
		}

		lines = append(lines, "Not used: "+r.Title+" — "+r.Why)
		said++
	}

	if engine != "" {
		lines = append(lines, "Checked, run and built by this program itself with "+engine+" — what the writer says it did counts only once that passes")
	}

	return lines
}

func sizeText(bytes int64) string {
	if bytes >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	}

	return fmt.Sprintf("%d MB", bytes>>20)
}

/*
 * MayInstallModel is whether a local model may be fetched without asking,
 * under its owner's policy: installing models without asking allowed, its
 * size from the registry within the download limit, and the disk left with
 * the room the policy keeps free. For the models this program fetches for
 * itself at start as much as for the ones a task needs.
 */
func (b *Brain) MayInstallModel(ctx context.Context) func(name string) (bool, string) {
	return func(name string) (bool, string) {
		pol := b.Orchestrator().Policy()

		if !pol.Autonomy.InstallLocalModels {
			return false, "your policy asks before installing models"
		}

		size, _, _, err := orchestrator.Manifest(ctx, name)
		if err != nil {
			return false, "its size could not be checked: " + err.Error()
		}

		if size > pol.Budget.MaxDownload {
			return false, fmt.Sprintf("it is %s, more than the %s your policy allows without asking",
				sizeText(size), sizeText(pol.Budget.MaxDownload))
		}

		if free := orchestrator.ReadMachine(b.Root).FreeFor("home"); free >= 0 && free-size < pol.Budget.MinFreeDisk {
			return false, fmt.Sprintf("it would leave %s free, less than the %s your policy keeps",
				sizeText(free-size), sizeText(pol.Budget.MinFreeDisk))
		}

		return true, ""
	}
}

// integrationResources is the MCP servers known here, as they stand: not
// approved, approved and stopped, running — and whether what they are sent
// leaves this machine.
func (b *Brain) integrationResources() []orchestrator.Resource {
	if b.Integrations == nil {
		return nil
	}

	var out []orchestrator.Resource

	for _, v := range b.Integrations.List() {
		r := orchestrator.Resource{ID: "mcp:" + v.ID, Kind: orchestrator.Integration, Title: v.Title,
			Version: v.Version, Local: !v.Leaves, Evidence: orchestrator.Reported, Observed: time.Now(),
			Licence: v.Licence}

		switch {
		case !v.Approved:
			r.State, r.Why = orchestrator.NotSupported, "not approved — nothing runs until you approve it"
		case v.Missing != "":
			r.State, r.Why = orchestrator.NotInstalled, v.Missing
		case v.Running:
			r.State, r.Why = orchestrator.Available, "running, for "+strings.Join(v.Agents, ", ")
		default:
			r.State, r.Why = orchestrator.Available, "approved, and started when switched on"
		}

		if v.Leaves {
			r.Why += "; what it is sent leaves this machine"
		}

		out = append(out, r)
	}

	return out
}

/*
 * projectTime is bootstrap's Time: how long a project's work is given. The
 * usual half hour, unless the one writing it is a model on this processor
 * with no graphics card — where a single step of a game took ten minutes on
 * this machine, and the half hour ran out while it was fixing what the
 * check found. Said in the proposal, so the time is approved with the plan.
 */
func (b *Brain) projectTime(dir string, pkg capability.Package) (int, string) {
	if pkg.Work != capability.Project {
		return 0, ""
	}

	b.mu.Lock()
	tier := b.tier
	b.mu.Unlock()

	d := b.Orchestrator().Decide(context.Background(), orchestrator.Need{Work: orchestrator.Code, Files: true}, projectOverride(dir))

	const slow = " on this processor, with no graphics card to speed it, where one step takes minutes"

	if tier == "modest" && d.Primary == nil && d.Install != nil {
		return 90, "it is written by " + d.Install.Name + ", installed first," + slow
	}

	if d.Primary == nil || tier != "modest" {
		return 30, "the usual time for a task"
	}

	if d.Primary.Local {
		return 90, "it is written by " + d.Primary.Title + slow
	}

	// A limit is a ceiling, not a target: work that falls to the model here
	// needs the time, and work that does not finishes long before it.
	for _, f := range d.Fallbacks {
		if f.Local {
			return 90, "if it falls to " + f.Title + ", that runs" + slow
		}
	}

	return 30, "the usual time for a task"
}
