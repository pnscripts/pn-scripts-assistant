package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/sandbox"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/workspace"
)

/*
 * Handing work on a project to a coding agent, and reading back what it did.
 *
 * Claude Code and Codex are programs their owner signed in to; they are run
 * as themselves, in the project's folder, with nothing of this program's in
 * their environment — no keys, no tokens — and their own limits set as tight
 * as the work allows. Claude Code is given the file tools and not a shell:
 * this program checks, runs and builds the project itself afterwards, and a
 * shell is the one tool that could reach outside the folder. Codex runs in
 * its own workspace-write sandbox, which on Linux is the kernel's.
 *
 * What they did is read from their own event stream: every file they wrote,
 * every command, how much of their allowance is left when they say. A file
 * written outside the project stops the work and is reported, whatever the
 * agent says about it. Nothing an agent says it did counts until this
 * program has checked it.
 */

// Job is one piece of work for an agent.
type Job struct {
	Dir     string
	Prompt  string
	Model   string
	Timeout time.Duration
}

// Failures, for failing over.
const (
	FailAuth    = "auth"
	FailLimit   = "limit"
	FailOffline = "offline"
	FailOutside = "outside"
	FailTimeout = "timeout"
	FailError   = "error"
)

// Outcome is what came of a job.
type Outcome struct {
	OK      bool     `json:"ok"`
	Said    string   `json:"said"`
	Files   []string `json:"files,omitempty"`
	Outside []string `json:"outside,omitempty"`

	Commands []string `json:"commands,omitempty"`

	Failure string `json:"failure,omitempty"`
	Detail  string `json:"detail,omitempty"`

	Model string        `json:"model,omitempty"`
	Took  time.Duration `json:"took"`

	// Usage is what the agent said about its allowance while it worked,
	// when it said anything.
	Usage *store.Usage `json:"usage,omitempty"`

	// CostEquivalent is what the agent reported the work would have cost at
	// API prices; on a subscription nothing was paid.
	CostEquivalent float64 `json:"cost_equivalent_usd,omitempty"`
}

// Executor is a coding agent this program can hand work to.
type Executor interface {
	ID() string
	Run(ctx context.Context, job Job) Outcome
}

// ExecutorFor is the adapter for an agent resource, when there is one.
func ExecutorFor(r Resource) (Executor, bool) {
	switch r.ID {
	case "claude-code":
		return ClaudeCodeExecutor{Path: r.Path}, r.Path != ""
	case "codex":
		return CodexExecutor{Path: r.Path}, r.Path != ""
	}

	return nil, false
}

// ClaudeCodeExecutor runs Claude Code headless.
type ClaudeCodeExecutor struct{ Path string }

func (ClaudeCodeExecutor) ID() string { return "claude-code" }

// claudeTools is what Claude Code is given: files, nothing else.
var (
	claudeTools  = "Read,Write,Edit,MultiEdit,Glob,Grep,LS,TodoWrite"
	claudeRefuse = "Bash,WebFetch,WebSearch,Task,NotebookEdit,CronCreate,CronDelete,RemoteTrigger," +
		"ScheduleWakeup,PushNotification,SendMessage,Monitor,EnterWorktree,ExitWorktree,Skill"
)

func (e ClaudeCodeExecutor) Run(ctx context.Context, job Job) Outcome {
	argv := []string{e.Path, "-p", job.Prompt, "--output-format", "stream-json", "--verbose",
		"--permission-mode", "acceptEdits", "--allowedTools", claudeTools, "--disallowedTools", claudeRefuse,
		"--strict-mcp-config", "--setting-sources", "project"}

	if job.Model != "" {
		argv = append(argv, "--model", job.Model)
	}

	return run(ctx, job, argv, parseClaude)
}

// CodexExecutor runs Codex headless in its own sandbox.
type CodexExecutor struct{ Path string }

func (CodexExecutor) ID() string { return "codex" }

func (e CodexExecutor) Run(ctx context.Context, job Job) Outcome {
	argv := []string{e.Path, "exec", "--json", "--sandbox", "workspace-write", "--skip-git-repo-check",
		"-C", job.Dir}

	if job.Model != "" {
		argv = append(argv, "-m", job.Model)
	}

	return run(ctx, job, append(argv, job.Prompt), parseCodex)
}

// parser reads one event line into the outcome.
type parser func(line []byte, o *Outcome, dir string)

func run(ctx context.Context, job Job, argv []string, parse parser) Outcome {
	o := Outcome{Model: job.Model}

	limit := job.Timeout
	if limit <= 0 {
		limit = 45 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	cmd, err := sandbox.Command(ctx, sandbox.Spec{Dir: job.Dir}, argv...)
	if err != nil {
		o.Failure, o.Detail = FailError, err.Error()

		return o
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		o.Failure, o.Detail = FailError, err.Error()

		return o
	}

	var stderr strings.Builder

	cmd.Stderr = &stderr

	started := time.Now()

	if err := cmd.Start(); err != nil {
		o.Failure, o.Detail = FailError, "could not start it: "+err.Error()

		return o
	}

	lines := bufio.NewScanner(out)
	lines.Buffer(make([]byte, 256*1024), 64<<20)

	for lines.Scan() {
		parse(lines.Bytes(), &o, job.Dir)

		// Outside the project is the end of it, whatever it meant to do next.
		if len(o.Outside) > 0 && o.Failure == "" {
			o.Failure = FailOutside
			o.Detail = "it wrote outside the project: " + strings.Join(o.Outside, ", ")

			cancel()
		}
	}

	waitErr := cmd.Wait()
	o.Took = time.Since(started).Round(time.Second)

	if ctx.Err() == context.DeadlineExceeded && o.Failure == "" {
		o.Failure, o.Detail = FailTimeout, fmt.Sprintf("it was still working after %s and was stopped", limit)
	}

	if o.Failure == "" && waitErr != nil && !o.OK {
		o.Failure = classify(stderr.String() + " " + o.Detail + " " + o.Said)
		o.Detail = orElse(o.Detail, lastLines(stderr.String(), 3))
	}

	sort.Strings(o.Files)
	o.Files = uniq(o.Files)

	return o
}

var (
	authWords    = regexp.MustCompile(`(?i)failed to authenticate|not logged in|please run /login|oauth|unauthori[sz]ed|401|authentication_failed|invalid api key`)
	limitWords   = regexp.MustCompile(`(?i)usage limit|rate limit|limit reached|quota|too many requests|429|out of credits`)
	offlineWords = regexp.MustCompile(`(?i)enotfound|econnrefused|network|could not resolve|fetch failed|timed out connecting|offline`)
)

// classify is what kind of failure some words describe.
func classify(text string) string {
	switch {
	case authWords.MatchString(text):
		return FailAuth
	case limitWords.MatchString(text):
		return FailLimit
	case offlineWords.MatchString(text):
		return FailOffline
	}

	return FailError
}

// written notes a file an agent wrote, inside the project or not.
func written(o *Outcome, dir, path string) {
	if path == "" {
		return
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	path = filepath.Clean(path)

	scope := workspace.ScopeOf(dir, workspace.Config{})

	if !scope.Allows(path) {
		o.Outside = append(o.Outside, path)

		return
	}

	o.Files = append(o.Files, path)
}

func parseClaude(line []byte, o *Outcome, dir string) {
	var event map[string]any

	if json.Unmarshal(line, &event) != nil {
		return
	}

	switch event["type"] {
	case "system":
		if m, ok := event["model"].(string); ok && o.Model == "" {
			o.Model = m
		}
	case "assistant":
		if errText, _ := event["error"].(string); errText != "" {
			o.Failure = classify(errText + " " + textOf(event))
			o.Detail = orElse(textOf(event), errText)
		}

		message, _ := event["message"].(map[string]any)
		content, _ := message["content"].([]any)

		for _, c := range content {
			item, _ := c.(map[string]any)

			if item["type"] != "tool_use" {
				continue
			}

			input, _ := item["input"].(map[string]any)

			switch item["name"] {
			case "Write", "Edit", "MultiEdit", "NotebookEdit":
				path, _ := input["file_path"].(string)
				written(o, dir, path)
			case "Bash":
				command, _ := input["command"].(string)
				o.Commands = append(o.Commands, command)
			}
		}
	case "rate_limit_event":
		o.Usage = claudeUsage(event)
	case "result":
		o.Said, _ = event["result"].(string)
		isError, _ := event["is_error"].(bool)
		o.OK = !isError && o.Failure == ""
		o.CostEquivalent, _ = event["total_cost_usd"].(float64)

		if isError && o.Failure == "" {
			o.Failure = classify(o.Said)
			o.Detail = o.Said
		}
	}
}

func textOf(event map[string]any) string {
	message, _ := event["message"].(map[string]any)
	content, _ := message["content"].([]any)

	var parts []string

	for _, c := range content {
		if item, ok := c.(map[string]any); ok && item["type"] == "text" {
			if t, ok := item["text"].(string); ok {
				parts = append(parts, t)
			}
		}
	}

	return strings.Join(parts, " ")
}

/*
 * claudeUsage is a rate-limit event as a reading: which window, how much of
 * it is used when that is said, and when it resets. Read by its fields'
 * names wherever they sit, since the event's shape is Claude Code's own and
 * this program should not break when it moves a field.
 */
func claudeUsage(event map[string]any) *store.Usage {
	u := &store.Usage{Resource: "claude-code", State: string(Available), Source: "Claude Code's rate-limit event"}

	var walk func(v any)

	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				switch strings.ToLower(k) {
				case "status":
					if s, ok := val.(string); ok {
						switch s {
						case "rejected":
							u.State = string(LimitReached)
						case "allowed_warning":
							u.State = string(LowCapacity)
						}
					}
				case "utilization":
					if f, ok := val.(float64); ok {
						if f <= 1 {
							f *= 100
						}

						left := 100 - f
						u.Remaining = &left
					}
				case "resetsat", "resets_at":
					switch t := val.(type) {
					case float64:
						u.ResetsAt = time.Unix(int64(t), 0)
					case string:
						u.ResetsAt, _ = time.Parse(time.RFC3339, t)
					}
				case "ratelimittype", "rate_limit_type":
					if s, ok := val.(string); ok {
						u.Window = s
					}
				}

				walk(val)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}

	walk(event)

	return u
}

func parseCodex(line []byte, o *Outcome, dir string) {
	var event struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
		Item *struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Command string `json:"command"`
			Changes []struct {
				Path string `json:"path"`
			} `json:"changes"`
		} `json:"item"`
	}

	if json.Unmarshal(line, &event) != nil {
		return
	}

	switch event.Type {
	case "item.completed":
		if event.Item == nil {
			return
		}

		switch event.Item.Type {
		case "file_change":
			for _, c := range event.Item.Changes {
				written(o, dir, c.Path)
			}
		case "command_execution":
			o.Commands = append(o.Commands, event.Item.Command)
		case "agent_message":
			o.Said = event.Item.Text
		}
	case "turn.completed":
		o.OK = o.Failure == ""
	case "error", "turn.failed":
		message := event.Message
		if event.Error != nil {
			message = event.Error.Message
		}

		o.OK = false
		o.Failure = classify(message)
		o.Detail = message

		if o.Failure == FailLimit {
			zero := 0.0
			u := &store.Usage{Resource: "codex", State: string(LimitReached), Remaining: &zero,
				Source: "Codex's answer", Detail: "its usage limit was reached"}

			if m := codexLimit.FindStringSubmatch(message); m != nil {
				u.ResetsAt, _ = time.ParseInLocation("Jan 2, 2006 3:04 PM", ordinal.ReplaceAllString(m[1], "$1"), time.Local)
			}

			o.Usage = u
		}
	}
}

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, " | ")
}

func uniq(list []string) []string {
	out := list[:0]

	for i, v := range list {
		if i == 0 || v != list[i-1] {
			out = append(out, v)
		}
	}

	return out
}

// StateAfter is what a failure says about the resource that failed.
func StateAfter(failure string) State {
	switch failure {
	case FailAuth:
		return AuthRequired
	case FailLimit:
		return LimitReached
	case FailOffline:
		return Offline
	}

	return Failed
}

// Evidence is what an outcome proves, as rows: the agent and what it
// changed. What it said it did is not among them.
func (o Outcome) Evidence(agent string) []store.Evidence {
	var ev []store.Evidence

	detail := fmt.Sprintf("%s in %s, %d files changed", orElse(o.Model, "its default model"), o.Took, len(o.Files))
	if o.Failure != "" {
		detail += " — stopped: " + o.Failure + ": " + o.Detail
	}

	ev = append(ev, store.Evidence{Kind: store.EvidenceAgent, Subject: agent, Detail: detail, OK: o.OK})

	for _, f := range o.Files {
		info, err := os.Stat(f)

		size := "not found afterwards"
		if err == nil {
			size = fmt.Sprintf("%d bytes", info.Size())
		}

		ev = append(ev, store.Evidence{Kind: store.EvidenceFile, Subject: f, Detail: "written by " + agent + ", " + size,
			OK: err == nil})
	}

	for _, f := range o.Outside {
		ev = append(ev, store.Evidence{Kind: store.EvidenceWarning, Subject: f, Detail: agent + " wrote outside the project"})
	}

	return ev
}

/*
 * ModelFor is which of an agent's models a kind of work is given: the one
 * that thinks hardest for planning, architecture and debugging, the working
 * one for writing code. A model is a choice per piece of work, not per task.
 * Empty leaves the agent's own default.
 */
func ModelFor(r Resource, work string) string {
	if r.ID != "claude-code" {
		return ""
	}

	switch work {
	case Plan, Reason:
		return "opus"
	case Code, Edit, Write:
		return "sonnet"
	}

	return ""
}
