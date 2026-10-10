package orchestrator

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Everything allowed (ADR 0007): the coding agents run with their own
// permission checks off; otherwise exactly as ADR 0005 started them.
func TestTheAgentsRunWithFullPermissionsWhenEverythingIsAllowed(t *testing.T) {
	job := Job{Dir: "/p", Prompt: "make it", Unbounded: true}

	claude := strings.Join(ClaudeCodeExecutor{Path: "claude"}.Argv(job), " ")
	if !strings.Contains(claude, "--dangerously-skip-permissions") || strings.Contains(claude, "--disallowedTools") {
		t.Errorf("Claude Code is not given everything: %s", claude)
	}

	codex := strings.Join(CodexExecutor{Path: "codex"}.Argv(job), " ")
	if !strings.Contains(codex, "--dangerously-bypass-approvals-and-sandbox") || strings.Contains(codex, "workspace-write") {
		t.Errorf("Codex is still sandboxed: %s", codex)
	}

	job.Unbounded = false

	claude = strings.Join(ClaudeCodeExecutor{Path: "claude"}.Argv(job), " ")
	if strings.Contains(claude, "dangerously") || !strings.Contains(claude, "acceptEdits") {
		t.Errorf("confined Claude Code lost its limits: %s", claude)
	}

	codex = strings.Join(CodexExecutor{Path: "codex"}.Argv(job), " ")
	if strings.Contains(codex, "dangerously") || !strings.Contains(codex, "workspace-write") {
		t.Errorf("confined Codex lost its sandbox: %s", codex)
	}
}

// The default is cloud preferred, and a policy written before ADR 0007 is
// moved there once — after which its owner's choice is kept.
func TestWorkPrivacyOpensUpOnce(t *testing.T) {
	if Default().Privacy != CloudPreferred {
		t.Errorf("work privacy starts at %q", Default().Privacy)
	}

	root := t.TempDir()

	old := Default()
	old.Privacy, old.Opened = LocalOnly, false

	raw, _ := json.Marshal(old)

	if err := os.MkdirAll(root+"/resources", 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(PolicyFile(root), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPolicy(root)
	if err != nil || p.Privacy != CloudPreferred {
		t.Fatalf("an older policy was not opened up: %q, %v", p.Privacy, err)
	}

	p.Privacy = LocalOnly

	if err := SavePolicy(root, p); err != nil {
		t.Fatal(err)
	}

	if again, _ := LoadPolicy(root); again.Privacy != LocalOnly {
		t.Errorf("a choice made afterwards was moved again: %q", again.Privacy)
	}
}
