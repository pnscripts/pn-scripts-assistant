package brain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/askclaude"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/sandbox"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Asking the user's own Claude (tools.AskClaude), the brain's side: which
 * ways there are, and one hand-off through one of them.
 *
 * Claude Code is the user's own program, signed in by them: it is started
 * as itself, in an empty folder, with nothing of this program's in its
 * environment (no key, no token; sandbox.Clean), no tools, and the question
 * and files on its standard input. The key way is the Anthropic key its
 * person typed into this assistant and nothing else (config.Chosen-
 * AnthropicKey): a key that is only in the environment is never used.
 */

// ClaudeTimeout is the longest one question may take.
const ClaudeTimeout = 5 * time.Minute

// mostClaudeOutput bounds what is read from Claude Code: the answer is at
// most askclaude.MostAnswer, the JSON around it a little more.
const mostClaudeOutput = 4 << 20

// AnthropicTestEnv points the key way at a stand-in for the Anthropic API,
// for tests and the CI demo. Honoured only for a loopback address: a
// variable that could send the user's key anywhere would be a way to steal
// it.
const AnthropicTestEnv = "PN_SCRIPTS_ASSISTANT_ANTHROPIC_URL"

// claudeRun is one question at a time: two at once spend the user's
// allowance twice for what is almost always one question asked twice.
var claudeRun sync.Mutex

// ClaudeWays is how the user's own Claude can be reached here, and whether
// each can now.
func (b *Brain) ClaudeWays() []tools.ClaudeWay {
	code := tools.ClaudeWay{ID: tools.ClaudeCodeWay, Title: "Claude Code, signed in as you",
		Why: "Claude Code is not installed on this computer"}

	if r, ok := b.claudeCode(context.Background()); ok {
		switch {
		case r.State == orchestrator.Available:
			code.Ready, code.Why = true, ""

			if r.Tier == orchestrator.Metered {
				code.Title = "Claude Code, with the API key set up in it"
			}
		case r.Why != "":
			code.Why = r.Why
		default:
			code.Why = "it cannot be used now (" + string(r.State) + ")"
		}
	}

	key := tools.ClaudeWay{ID: tools.ClaudeKeyWay, Title: "your own Anthropic key",
		Why: "no Anthropic key was typed into this assistant (a key that is only in the environment is not used)"}

	b.mu.Lock()
	chosen := b.Cfg.ChosenAnthropicKey()
	b.mu.Unlock()

	if chosen != "" {
		key.Ready, key.Why = true, ""
	}

	return []tools.ClaudeWay{code, key}
}

// claudeCode is Claude Code as discovery found it, when it is installed.
func (b *Brain) claudeCode(ctx context.Context) (orchestrator.Resource, bool) {
	list, _ := b.Orchestrator().Resources(ctx)

	for _, r := range list {
		if r.ID == "claude-code" && r.State != orchestrator.NotInstalled && r.Path != "" {
			return r, true
		}
	}

	return orchestrator.Resource{}, false
}

// AskClaude hands one question and its files to the user's own Claude.
func (b *Brain) AskClaude(ctx context.Context, via, question string, docs []askclaude.Doc) (tools.ClaudeAnswer, error) {
	if !claudeRun.TryLock() {
		return tools.ClaudeAnswer{}, errors.New("Claude is already answering a question; ask again when it has finished")
	}
	defer claudeRun.Unlock()

	nonce := askclaude.NewNonce()
	for askclaude.Collides(docs, nonce) {
		nonce = askclaude.NewNonce()
	}

	prompt := askclaude.Prompt(question, docs, nonce)

	ctx, cancel := context.WithTimeout(ctx, ClaudeTimeout)
	defer cancel()

	switch via {
	case tools.ClaudeCodeWay:
		return b.askClaudeCode(ctx, prompt)
	case tools.ClaudeKeyWay:
		return b.askWithChosenKey(ctx, prompt)
	}

	return tools.ClaudeAnswer{}, fmt.Errorf("there is no way to Claude called %q", via)
}

func (b *Brain) askClaudeCode(ctx context.Context, prompt string) (tools.ClaudeAnswer, error) {
	r, ok := b.claudeCode(ctx)
	if !ok || r.State != orchestrator.Available {
		return tools.ClaudeAnswer{}, errors.New("Claude Code is not signed in on this computer any more")
	}

	// An empty folder of its own: no project settings, no CLAUDE.md, no
	// files it could be pointed at.
	dir, err := os.MkdirTemp("", "pn-scripts-assistant-ask-claude-*")
	if err != nil {
		return tools.ClaudeAnswer{}, err
	}
	defer os.RemoveAll(dir)

	cmd, err := sandbox.Command(ctx, sandbox.Spec{Dir: dir}, askclaude.ClaudeCodeArgv(r.Path, "")...)
	if err != nil {
		return tools.ClaudeAnswer{}, err
	}

	cmd.Stdin = strings.NewReader(prompt)

	var out, stderr bytes.Buffer

	cmd.Stdout = &limited{w: &out, left: mostClaudeOutput}
	cmd.Stderr = &limited{w: &stderr, left: 64 << 10}

	progress.Detail("Claude Code is answering")

	runErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return tools.ClaudeAnswer{}, fmt.Errorf("Claude Code was still answering after %s and was stopped", ClaudeTimeout)
	}

	answer, err := askclaude.ReadClaudeCode(out.Bytes())
	if err != nil {
		if runErr != nil && stderr.Len() > 0 {
			return tools.ClaudeAnswer{}, fmt.Errorf("%v (%s)", err, lastLine(stderr.String()))
		}

		return tools.ClaudeAnswer{}, err
	}

	who := "Claude Code, signed in as you"
	if r.Tier == orchestrator.Metered {
		who = "Claude Code, with the API key set up in it"
	}

	return tools.ClaudeAnswer{Who: who, Model: answer.Model, Text: answer.Text, Cut: answer.Cut}, nil
}

func (b *Brain) askWithChosenKey(ctx context.Context, prompt string) (tools.ClaudeAnswer, error) {
	b.mu.Lock()
	key, model := b.Cfg.ChosenAnthropicKey(), b.Cfg.AnthropicModel
	b.mu.Unlock()

	if key == "" {
		return tools.ClaudeAnswer{}, errors.New("no Anthropic key was typed into this assistant")
	}

	client := llm.NewAnthropic(key, model)

	if raw := os.Getenv(AnthropicTestEnv); raw != "" {
		if u, err := url.Parse(raw); err == nil && loopbackHost(u.Hostname()) {
			client.BaseURL = strings.TrimRight(raw, "/")
		}
	}

	progress.Detail("Claude is answering")

	reply, err := client.Chat(ctx, llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: askclaude.System()},
			{Role: llm.RoleUser, Content: prompt},
		},
		MaxTokens: 4096,
	})
	if err != nil {
		return tools.ClaudeAnswer{}, err
	}

	if strings.TrimSpace(reply.Content) == "" {
		return tools.ClaudeAnswer{}, errors.New("Claude gave an empty answer")
	}

	text, cut := askclaude.Bounded(reply.Content)

	return tools.ClaudeAnswer{Who: "your own Anthropic key", Model: reply.Model, Text: text, Cut: cut}, nil
}

func loopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")

	return strings.TrimSpace(lines[len(lines)-1])
}

// limited keeps at most left bytes and quietly drops the rest, so a program
// that prints without end cannot fill the memory.
type limited struct {
	w    io.Writer
	left int
}

func (l *limited) Write(p []byte) (int, error) {
	n := len(p)

	if l.left <= 0 {
		return n, nil
	}

	if len(p) > l.left {
		p = p[:l.left]
	}

	l.left -= len(p)

	if _, err := l.w.Write(p); err != nil {
		return 0, err
	}

	return n, nil
}
