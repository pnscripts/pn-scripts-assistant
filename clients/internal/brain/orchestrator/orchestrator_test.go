package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

func pct(v float64) *float64 { return &v }

func agent(id string, code int, tier Tier, state State, left *float64) Resource {
	r := Resource{ID: id, Title: id, Kind: Agent, Tier: tier, State: state, Executes: true,
		Can: map[string]int{Code: code, Plan: code, "tools": 1}}

	if left != nil {
		r.Usage = &store.Usage{Resource: id, Remaining: left}
	}

	return r
}

func local(id string, code int) Resource {
	return Resource{ID: id, Title: id, Kind: Model, Tier: Local, Local: true, State: Available, Executes: true,
		Can: map[string]int{Code: code, Plan: code, "tools": 1}}
}

func chosen(t *testing.T, d Decision) string {
	t.Helper()

	if d.Primary == nil {
		return ""
	}

	return d.Primary.ID
}

func TestASuitableSubscriptionIsChosenFirst(t *testing.T) {
	d := Select([]Resource{local("qwen", 5), agent("claude", 9, Subscription, Available, pct(70))},
		Need{Work: Code, Files: true}, Default(), nil)

	if chosen(t, d) != "claude" || len(d.Fallbacks) != 1 || d.Fallbacks[0].ID != "qwen" {
		t.Fatalf("chose %s, then %+v", chosen(t, d), d.Fallbacks)
	}

	text := d.Explain()
	for _, want := range []string{"Selected claude", "subscription", "70% of its capacity", "Local fallback: qwen"} {
		if !strings.Contains(text, want) {
			t.Errorf("the explanation does not say %q:\n%s", want, text)
		}
	}
}

// Suitability first: a subscription does not win over something clearly
// better at the work.
func TestSuitabilityComesBeforeHowItIsPaid(t *testing.T) {
	d := Select([]Resource{agent("weak-sub", 3, Subscription, Available, nil), local("strong-local", 8)},
		Need{Work: Code, Files: true}, Default(), nil)

	if chosen(t, d) != "strong-local" {
		t.Errorf("chose %s", chosen(t, d))
	}
}

func TestExhaustedSignedOutAndUnsuitableAreSkipped(t *testing.T) {
	d := Select([]Resource{
		agent("exhausted", 9, Subscription, LimitReached, pct(0)),
		agent("signed-out", 9, Subscription, AuthRequired, nil),
		agent("cannot-code", 0, Subscription, Available, nil),
		local("qwen", 5),
	}, Need{Work: Code, Files: true}, Default(), nil)

	if chosen(t, d) != "qwen" || len(d.Rejected) != 3 {
		t.Fatalf("chose %s, rejected %+v", chosen(t, d), d.Rejected)
	}
}

// Before the limit, not at it: 18% left and work expected to take 12% would
// end below the critical line, so something else is given the work.
func TestItSwitchesBeforeTheLimitIsHit(t *testing.T) {
	near := agent("claude", 9, Subscription, Available, pct(18))
	other := agent("codex", 8, Subscription, Available, pct(80))

	d := Select([]Resource{near, other, local("qwen", 5)}, Need{Work: Code, Files: true, Estimate: 12}, Default(), nil)

	if chosen(t, d) != "codex" {
		t.Fatalf("chose %s", chosen(t, d))
	}

	found := false

	for _, r := range d.Rejected {
		if r.ID == "claude" && strings.Contains(r.Why, "18%") {
			found = true
		}
	}

	if !found {
		t.Errorf("the reason claude was passed over is not said: %+v", d.Rejected)
	}
}

func TestLocalOnlyNeverChoosesTheCloud(t *testing.T) {
	pol := Default()
	pol.Privacy = LocalOnly

	d := Select([]Resource{agent("claude", 9, Subscription, Available, pct(90))}, Need{Work: Code, Files: true}, pol, nil)

	if d.Primary != nil {
		t.Fatalf("a local-only policy chose %s", d.Primary.ID)
	}

	// And a project may make it local only for itself, never the other way.
	project := Default().Within(Override{Privacy: LocalOnly})
	if project.Privacy != LocalOnly {
		t.Error("a project could not narrow to local only")
	}

	if owner := pol.Within(Override{Privacy: CloudPreferred}); owner.Privacy != LocalOnly {
		t.Error("a project opened the cloud to an owner who keeps things local")
	}
}

func TestNothingPaidPerUseWithoutABudget(t *testing.T) {
	api := agent("api:anthropic", 9, Metered, Available, nil)
	api.Kind = Model

	d := Select([]Resource{api}, Need{Work: Code, Files: true}, Default(), nil)
	if d.Primary != nil || !strings.Contains(d.Blocked, "budget") {
		t.Errorf("a paid-per-use service was chosen with no budget: %+v", d)
	}

	// Spending cannot be counted, so a budget does not make it a choice made
	// on the owner's behalf — only one they make themselves.
	pol := Default()
	pol.Budget.MonthlyAPI = 20

	if d := Select([]Resource{api}, Need{Work: Code, Files: true}, pol, nil); d.Primary != nil {
		t.Errorf("uncounted spending was chosen without being asked for: %+v", d)
	}

	if d := Select([]Resource{api}, Need{Work: Code, Files: true, Only: "api:anthropic"}, pol, nil); chosen(t, d) != "api:anthropic" {
		t.Error("named by the task, with a budget, it still could not be used")
	}
}

// A decision says why the others were passed over, not only why the one
// chosen was.
func TestTheDecisionSaysWhyNotTheOthers(t *testing.T) {
	embed := local("nomic-embed-text", 0)

	d := Select([]Resource{agent("claude", 9, Subscription, AuthRequired, nil), local("qwen", 7), embed},
		Need{Work: Code, Files: true}, Default(), nil)

	said := d.Explain()
	if !strings.Contains(said, "Not used: claude — auth_required") || strings.Contains(said, "nomic-embed-text") {
		t.Errorf("the rejections were not said, or the irrelevant ones were:\n%s", said)
	}
}

func TestUnknownCapacityIsNotInvented(t *testing.T) {
	d := Select([]Resource{agent("claude", 9, Subscription, Available, nil)}, Need{Work: Code, Files: true}, Default(), nil)

	if d.Primary.Usage != nil || !strings.Contains(d.Explain(), "not known") {
		t.Errorf("unknown capacity became known, or went unsaid:\n%s", d.Explain())
	}
}

func TestBlockedAndTheTasksOwnChoice(t *testing.T) {
	pol := Default()
	pol.Blocked = []string{"claude"}

	all := []Resource{agent("claude", 9, Subscription, Available, nil), agent("codex", 8, Subscription, Available, nil)}

	if d := Select(all, Need{Work: Code, Files: true}, pol, nil); chosen(t, d) != "codex" {
		t.Errorf("a blocked resource was chosen: %s", chosen(t, d))
	}

	if d := Select(all, Need{Work: Code, Files: true, Only: "codex"}, Default(), nil); chosen(t, d) != "codex" || len(d.Fallbacks) != 0 {
		t.Errorf("the task's own choice was not kept to: %s %+v", chosen(t, d), d.Fallbacks)
	}
}

// Integrations are listed with everything else and never chosen to do work:
// they are used through their grants, not handed a task.
func TestIntegrationsAreShownNotChosen(t *testing.T) {
	o := New(Sources{Root: t.TempDir(), Integrations: func() []Resource {
		return []Resource{{ID: "mcp:files", Kind: Integration, Title: "files", State: Available, Local: true,
			Can: map[string]int{Code: 10, "tools": 1}}}
	}})

	list, _ := o.Resources(t.Context())
	if _, ok := Find(list, "mcp:files"); !ok {
		t.Fatalf("the integration was not listed: %+v", list)
	}

	if d := Select(list, Need{Work: Code, Files: true}, Default(), nil); d.Primary != nil {
		t.Errorf("an integration was chosen to write code: %s", d.Primary.ID)
	}
}

// Failing over moves to the next in line and keeps the rest.
func TestNextIsTheFirstFallback(t *testing.T) {
	d := Select([]Resource{agent("claude", 9, Subscription, Available, nil), agent("codex", 8, Subscription, Available, nil),
		local("qwen", 5)}, Need{Work: Code, Files: true}, Default(), nil)

	next, ok := d.Next()
	if !ok || next.Primary.ID != "codex" || len(next.Fallbacks) != 1 {
		t.Errorf("failing over went to %+v", next.Primary)
	}

	// And says why for what it moved to. Seen on this machine: a failover to
	// the local model explained it as "the max subscription already paid for".
	last, _ := next.Next()
	if said := last.Explain(); !strings.Contains(said, "Selected qwen") || strings.Contains(said, "subscription") ||
		!strings.Contains(said, "runs on this machine") {
		t.Errorf("the failover gave the reasons for what failed:\n%s", said)
	}
}

func TestHistoryBreaksTiesOnly(t *testing.T) {
	a := agent("a", 9, Subscription, Available, nil)
	b := agent("b", 9, Subscription, Available, nil)

	h := History{"b": {{Result: store.RunOK, Verified: true}, {Result: store.RunOK, Verified: true},
		{Result: store.RunOK, Verified: true}},
		"a": {{Result: store.RunFailed}, {Result: store.RunFailed}, {Result: store.RunOK, Verified: true}}}

	if d := Select([]Resource{a, b}, Need{Work: Code, Files: true}, Default(), h); chosen(t, d) != "b" {
		t.Errorf("history did not break the tie: %s", chosen(t, d))
	}

	// But it never outweighs being better at the work.
	c := agent("c", 5, Subscription, Available, nil)
	if d := Select([]Resource{a, c}, Need{Work: Code, Files: true}, Default(),
		History{"c": h["b"], "a": h["a"]}); chosen(t, d) != "a" {
		t.Errorf("history outweighed suitability: %s", chosen(t, d))
	}
}

// Finishing is not a record: only work that passed this program's checks
// counts, and a decision never calls unchecked work verified.
func TestOnlyVerifiedWorkCounts(t *testing.T) {
	h := History{"qwen": {{Result: store.RunOK}, {Result: store.RunOK}, {Result: store.RunOK}}}

	if h.Rate("qwen") != 0 {
		t.Errorf("unchecked work counted: %.2f", h.Rate("qwen"))
	}

	d := Select([]Resource{local("qwen", 7)}, Need{Work: Code, Files: true}, Default(), h)
	if strings.Contains(d.Explain(), "passed") || strings.Contains(d.Explain(), "verified") {
		t.Errorf("unchecked work was called checked:\n%s", d.Explain())
	}
}

func TestThresholdsTurnReadingsIntoStates(t *testing.T) {
	pol := Default()

	for left, want := range map[float64]State{80: Available, 15: LowCapacity, 5: LimitNear, 0: LimitReached} {
		r := agent("x", 9, Subscription, Available, pct(left))
		thresholds(&r, pol)

		if r.State != want {
			t.Errorf("%.0f%% left is %s, not %s", left, r.State, want)
		}
	}
}

// What the real agents said today, read back as what it means.
func TestTheAgentsFailuresAreRecognised(t *testing.T) {
	dir := t.TempDir()

	var o Outcome
	for _, line := range []string{
		`{"type":"system","subtype":"init","model":"claude-haiku-4-5-20251001"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Failed to authenticate: OAuth session expired and could not be refreshed"}]},"error":"authentication_failed"}`,
		`{"type":"result","subtype":"success","is_error":true,"result":"Failed to authenticate: OAuth session expired and could not be refreshed","total_cost_usd":0}`,
	} {
		parseClaude([]byte(line), &o, dir)
	}

	if o.OK || o.Failure != FailAuth || StateAfter(o.Failure) != AuthRequired {
		t.Errorf("an expired sign-in read as %+v", o)
	}

	var c Outcome
	parseCodex([]byte(`{"type":"turn.failed","error":{"message":"You've hit your usage limit. To continue using Codex and get access to GPT-5.3-Codex, start a free trial of Plus today (https://chatgpt.com/explore/plus), or try again at Oct 15th, 2026 3:31 PM."}}`), &c, dir)

	if c.Failure != FailLimit || c.Usage == nil || c.Usage.ResetsAt.Month() != time.October || c.Usage.ResetsAt.Day() != 15 {
		t.Errorf("Codex's limit read as %+v %+v", c, c.Usage)
	}
}

func TestAnAgentWritingOutsideTheProjectIsStopped(t *testing.T) {
	dir := t.TempDir()

	var o Outcome
	parseClaude([]byte(`{"type":"assistant","message":{"content":[`+
		`{"type":"tool_use","name":"Write","input":{"file_path":"`+filepath.Join(dir, "main.js")+`"}},`+
		`{"type":"tool_use","name":"Edit","input":{"file_path":"/home/somebody/.bashrc"}},`+
		`{"type":"tool_use","name":"Write","input":{"file_path":"`+filepath.Join(dir, ".pn-assistant", "project.json")+`"}}]}}`), &o, dir)

	if len(o.Files) != 1 || len(o.Outside) != 2 {
		t.Errorf("inside %v, outside %v", o.Files, o.Outside)
	}
}

func TestCodexsOwnLogSaysWhatIsLeft(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "09", "18")
	os.MkdirAll(day, 0o755)

	os.WriteFile(filepath.Join(day, "rollout-1.jsonl"), []byte(
		`{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"limit_id":"codex","primary":{"used_percent":97.0,"window_minutes":43200},"plan_type":"free"}}}`+"\n"), 0o644)

	u := CodexUsage(home)
	if u == nil || u.Remaining == nil || *u.Remaining != 3 || u.Plan != "free" {
		t.Fatalf("read %+v", u)
	}
}

func TestEnginesAreChosenFromTheRequestAndTheMachine(t *testing.T) {
	options := []EngineOption{
		{Package: "software.game.threejs", Engine: "threejs", Title: "three.js", Installed: true, Version: "r169",
			CanCreate: true, Picture: true, Platforms: []string{"web"}, Dims: []string{"2d", "3d"}, Language: "javascript"},
		{Package: "software.game.godot", Engine: "godot", Title: "Godot", Installed: true, Version: "4.7.1",
			CanCreate: true, Picture: true, Platforms: []string{"desktop", "web"}, Dims: []string{"2d", "3d"}, Language: "gdscript"},
		{Package: "software.game.unity", Engine: "unity", Title: "Unity", Installed: true, Licence: "inactive",
			CanCreate: true, Platforms: []string{"desktop", "web", "mobile"}, Dims: []string{"2d", "3d"}, Language: "csharp"},
		{Package: "software.game.unreal", Engine: "unreal", Title: "Unreal Engine", CanCreate: true,
			Platforms: []string{"desktop"}, Dims: []string{"3d"}, Language: "cpp", HighEnd: true},
	}

	for request, want := range map[string]string{
		"Make me a Tetris game":           "threejs",
		"Make a 3D racing game for my PC": "godot",
		"Make a browser game":             "threejs",
	} {
		c := ChooseEngine(options, request, false)
		if c.Chosen == nil || c.Chosen.Engine != want {
			t.Errorf("%q chose %+v (asked: %s)", request, c.Chosen, c.Ask)
		}
	}

	c := ChooseEngine(options, "Make me a mobile game", false)
	if c.Chosen != nil || c.Ask == "" {
		t.Errorf("a phone game was promised: %+v", c.Chosen)
	}

	for _, r := range ChooseEngine(options, "Make me a Tetris game", false).Rejected {
		if r.ID == "unity" && !strings.Contains(r.Why, "licensed") {
			t.Errorf("unity's rejection does not say why: %s", r.Why)
		}
	}
}

// The machine as it is, when asked for: every resource with its state and
// why, and what would be chosen to write code. Set PN_TEST_AGENTS=1 — it
// asks the installed coding agents whether they are signed in.
func TestTheMachineAsItIs(t *testing.T) {
	if os.Getenv("PN_TEST_AGENTS") == "" {
		t.Skip("set PN_TEST_AGENTS=1 to look at this machine's agents")
	}

	o := New(Sources{OllamaURL: "http://127.0.0.1:11434", Agents: true})

	list, m := o.Resources(t.Context())

	t.Logf("machine: %s, %s, %d cores, %.0f GB RAM, GPU %v, display %s, virtual %s, landlock %d, network: %s",
		m.OS, m.CPU, m.Cores, float64(m.RAM)/(1<<30), m.GPU, m.Display, m.Virtual, m.Landlock, m.Network)

	for _, r := range list {
		left := "unknown"
		if v, ok := r.Remaining(); ok {
			left = strings.TrimRight(strings.TrimRight(strconvF(v), "0"), ".") + "%"
		}

		t.Logf("%-28s %-8s %-14s v%-24s capacity %-8s %s", r.ID, r.Kind, r.State, r.Version, left, r.Why)
	}

	d := o.Decide(t.Context(), Need{Work: Code, Files: true}, Override{})
	t.Logf("to write code:\n%s", d.Explain())

	for _, r := range d.Rejected {
		t.Logf("not: %s — %s", r.Title, r.Why)
	}
}

func strconvF(v float64) string { return strings.TrimSpace(fmt.Sprintf("%6.1f", v)) }
