package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/sandbox"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The coding agents and editors on this machine, found where they are really
 * installed and asked — the official way each offers — whether they are
 * signed in. Never by opening their credential files: a token is not read,
 * copied or passed on, and an agent is only ever used as itself, signed in
 * as its owner signed it in.
 */

// ask runs a status command with nothing of this program's in its
// environment, and gives up after a few seconds.
func ask(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd, err := sandbox.Command(ctx, sandbox.Spec{Dir: os.TempDir()}, append([]string{path}, args...)...)
	if err != nil {
		return "", err
	}

	out, err := cmd.CombinedOutput()

	return strings.TrimSpace(string(out)), err
}

func firstFound(candidates ...string) string {
	for _, c := range candidates {
		if c == "" {
			continue
		}

		if !strings.Contains(c, "/") {
			if at, err := exec.LookPath(c); err == nil {
				return at
			}

			continue
		}

		if info, err := os.Stat(c); err == nil && info.Mode()&0o111 != 0 {
			return c
		}
	}

	return ""
}

var semver = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:[-.][0-9A-Za-z.]+)?`)

func versionOf(path string, args ...string) string {
	if len(args) == 0 {
		args = []string{"--version"}
	}

	out, _ := ask(path, args...)

	return semver.FindString(out)
}

// ClaudeCode is Claude Code as installed for the terminal, and whether it is
// signed in — which `claude auth status` says, as JSON.
func ClaudeCode(db *store.DB) Resource {
	home, _ := os.UserHomeDir()

	r := Resource{ID: "claude-code", Kind: Agent, Title: "Claude Code", Tier: Subscription,
		Can:      map[string]int{Code: 9, Edit: 9, Plan: 9, Reason: 9, Write: 8, Vision: 7, "tools": 1},
		Executes: true, Evidence: Reported, Observed: time.Now(), Models: []string{"opus", "sonnet", "haiku"}}

	r.Path = firstFound("claude", filepath.Join(home, ".local", "bin", "claude"))
	if r.Path == "" {
		r.State, r.Why = NotInstalled, "the claude command is not on this machine"

		return r
	}

	r.Version = versionOf(r.Path)

	out, _ := ask(r.Path, "auth", "status")

	var status struct {
		LoggedIn         bool   `json:"loggedIn"`
		AuthMethod       string `json:"authMethod"`
		SubscriptionType string `json:"subscriptionType"`
	}

	if json.Unmarshal([]byte(out), &status) != nil {
		r.State, r.Why = Unknown, "it would not say whether it is signed in"

		return r
	}

	if !status.LoggedIn {
		r.State, r.Why = AuthRequired, "not signed in — its owner runs claude and signs in (it asks in the browser)"
		r.Evidence = Verified

		return r
	}

	r.State, r.Evidence = Available, Verified
	r.Plan = status.SubscriptionType

	if r.Plan == "" {
		r.Plan = claudePlan(home)
	}

	if status.AuthMethod == "api_key" || status.AuthMethod == "apiKey" {
		// Signed in with a key: every call is paid for.
		r.Tier, r.Plan = Metered, "API key"
	}

	withUsage(&r, db)

	return r
}

// claudePlan is the plan Claude Code's own settings name — its kind, never
// who is signed in.
func claudePlan(home string) string {
	raw, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return ""
	}

	var settings struct {
		Account struct {
			OrganizationType string `json:"organizationType"`
		} `json:"oauthAccount"`
	}

	json.Unmarshal(raw, &settings)

	return settings.Account.OrganizationType
}

// Codex is OpenAI's Codex, which may be a command of its own or ride inside
// the ChatGPT desktop app, and how much of its allowance is left — which its
// own session log records.
func Codex(db *store.DB) Resource {
	home, _ := os.UserHomeDir()

	r := Resource{ID: "codex", Kind: Agent, Title: "Codex", Tier: Subscription,
		Can:      map[string]int{Code: 8, Edit: 8, Plan: 8, Reason: 8, Write: 7, "tools": 1},
		Executes: true, Evidence: Reported, Observed: time.Now()}

	r.Path = firstFound("codex", "/usr/lib/chatgpt/resources/codex", filepath.Join(home, ".local", "bin", "codex"))
	if r.Path == "" {
		r.State, r.Why = NotInstalled, "Codex is not on this machine"

		return r
	}

	r.Version = versionOf(r.Path)

	out, _ := ask(r.Path, "login", "status")

	switch {
	case strings.Contains(out, "Logged in"):
		r.State, r.Evidence = Available, Verified
	case strings.Contains(strings.ToLower(out), "not logged in"):
		r.State, r.Why, r.Evidence = AuthRequired, "not signed in — its owner signs in to Codex", Verified

		return r
	default:
		r.State, r.Why = Unknown, "it would not say whether it is signed in"

		return r
	}

	// What Codex's own log last said, kept when there is somewhere to keep
	// it, and applied either way.
	if u := CodexUsage(home); u != nil {
		if db != nil {
			db.NoteUsage(*u)
		}

		applyUsage(&r, u)
	}

	withUsage(&r, db)

	return r
}

// CursorAgent is Cursor's command-line agent. Its sign-in is not asked:
// asking it starts a login and rewrites its settings. And this program has
// no adapter to hand it work yet, which is said rather than hidden.
func CursorAgent() Resource {
	home, _ := os.UserHomeDir()

	r := Resource{ID: "cursor-agent", Kind: Agent, Title: "Cursor agent", Tier: Subscription,
		Can: map[string]int{Code: 8, Edit: 8, Plan: 7, Reason: 7, Write: 7}, Evidence: Reported, Observed: time.Now()}

	r.Path = firstFound("cursor-agent", filepath.Join(home, ".local", "bin", "cursor-agent"))
	if r.Path == "" {
		r.State, r.Why = NotInstalled, "Cursor's agent is not on this machine"

		return r
	}

	r.Version = versionOf(r.Path)
	r.State = NotSupported
	r.Why = "this program cannot hand work to Cursor's agent yet; whether it is signed in is not checked, " +
		"because checking starts its login"

	return r
}

// Editors are the editors installed here. They are where the owner looks,
// never who does the work.
func Editors() []Resource {
	var out []Resource

	for _, e := range []struct{ id, title, path, version string }{
		{"vscode", "Visual Studio Code", firstFound("code", "/snap/bin/code", "/usr/bin/code"), ""},
		{"cursor", "Cursor", firstFound("cursor", "/usr/bin/cursor"), readJSONVersion("/usr/share/cursor/resources/app/package.json")},
		{"intellij", "IntelliJ IDEA", firstFound("intellij-idea-community", "idea", "/snap/bin/intellij-idea-community"), ""},
	} {
		if e.path == "" {
			continue
		}

		out = append(out, Resource{ID: e.id, Kind: IDE, Title: e.title, Path: e.path, Version: e.version,
			State: Available, Evidence: Reported, Local: true, Observed: time.Now(),
			Why: "an editor to open a project in; it does not do the work"})
	}

	return out
}

func readJSONVersion(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var v struct {
		Version string `json:"version"`
	}

	json.Unmarshal(raw, &v)

	return v.Version
}

// withUsage puts the last reading of a subscription on it, and lets it
// decide the state: known and low is low, known and gone is gone.
func withUsage(r *Resource, db *store.DB) {
	if db == nil {
		return
	}

	u, err := db.UsageOf(r.ID)
	if err != nil || u == nil {
		return
	}

	applyUsage(r, u)
}

// applyUsage is a reading put on a resource: its numbers, and — while it
// holds — what it says about being signed out or out of allowance.
func applyUsage(r *Resource, u *store.Usage) {

	// A reading about being signed out, or out of allowance, stands until
	// the time it said it would reset — or a day, when it said none.
	stale := time.Since(u.At) > 24*time.Hour
	if !u.ResetsAt.IsZero() {
		stale = time.Now().After(u.ResetsAt)
	}

	if stale {
		return
	}

	r.Usage = u
	r.Plan = orElse(r.Plan, u.Plan)

	switch State(u.State) {
	case LimitReached, AuthRequired, Offline:
		r.State, r.Why = State(u.State), u.Detail

		if !u.ResetsAt.IsZero() {
			r.Why += " — until " + u.ResetsAt.Local().Format("2 Jan 15:04")
		}
	}
}

func orElse(v, fallback string) string {
	if v == "" {
		return fallback
	}

	return v
}

var (
	codexLimit = regexp.MustCompile(`(?i)(?:hit your usage limit|usage limit)[^"]*?try again at ([A-Z][a-z]{2} \d{1,2}(?:st|nd|rd|th)?, \d{4} \d{1,2}:\d{2} ?[AP]M)`)
	ordinal    = regexp.MustCompile(`(\d)(st|nd|rd|th)`)
)

/*
 * CodexUsage is what Codex's own session log last said about its allowance:
 * how much of its window is used, its plan, and — when it refused — until
 * when. Only those numbers are read from the log; nothing else in it.
 */
func CodexUsage(home string) *store.Usage {
	var files []string

	filepath.WalkDir(filepath.Join(home, ".codex", "sessions"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			files = append(files, path)
		}

		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		a, _ := os.Stat(files[i])
		b, _ := os.Stat(files[j])

		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})

	for _, path := range files {
		if u := usageIn(path); u != nil {
			return u
		}
	}

	return nil
}

func usageIn(path string) *store.Usage {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}

	defer f.Close()

	// A reading is as old as the log it was last written into.
	at := time.Now()
	if info, err := f.Stat(); err == nil {
		at = info.ModTime()
	}

	var found *store.Usage

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 16<<20)

	for s.Scan() {
		line := s.Text()

		if m := codexLimit.FindStringSubmatch(line); m != nil {
			u := &store.Usage{Resource: "codex", State: string(LimitReached), Source: "Codex's session log",
				Detail: "its usage limit was reached"}
			zero := 0.0
			u.Remaining = &zero

			if at, err := time.ParseInLocation("Jan 2, 2006 3:04 PM", ordinal.ReplaceAllString(m[1], "$1"), time.Local); err == nil {
				u.ResetsAt = at
			}

			found = u

			continue
		}

		if !strings.Contains(line, `"rate_limits"`) {
			continue
		}

		var event struct {
			Payload struct {
				RateLimits *codexLimits `json:"rate_limits"`
			} `json:"payload"`
			RateLimits *codexLimits `json:"rate_limits"`
		}

		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}

		limits := event.RateLimits
		if limits == nil {
			limits = event.Payload.RateLimits
		}

		if limits == nil || limits.Primary == nil {
			continue
		}

		left := 100 - limits.Primary.UsedPercent
		u := &store.Usage{Resource: "codex", State: string(Available), Remaining: &left,
			Window: strconv.Itoa(limits.Primary.WindowMinutes) + " minutes", Plan: limits.PlanType,
			Source: "Codex's session log"}

		if limits.Primary.ResetsAt > 0 {
			u.ResetsAt = time.Unix(limits.Primary.ResetsAt, 0)
		}

		found = u
	}

	if found != nil {
		found.At = at
	}

	return found
}

type codexLimits struct {
	PlanType string `json:"plan_type"`
	Primary  *struct {
		UsedPercent   float64 `json:"used_percent"`
		WindowMinutes int     `json:"window_minutes"`
		ResetsAt      int64   `json:"resets_at"`
	} `json:"primary"`
}
