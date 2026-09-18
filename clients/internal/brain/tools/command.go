package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/sandbox"
)

// RunCommand runs a program on the machine.
//
// This is the most dangerous capability the brain has, and it is shaped so the
// dangerous parts are the ones a person actually sees.
//
// The command is an array — program first, then arguments — never a shell
// string. There is no shell, so there is nothing to inject into: a semicolon is
// an argument, not a second command. It also makes the approval meaningful,
// because what is shown is exactly what will run, with no expansion or
// substitution happening afterwards.
//
// The brain runs on the machine it is asked about, so a command it is given
// simply runs. There is nothing between it and the computer.
type RunCommand struct {
	// Timeout bounds one command. Zero means DefaultCommandTimeout.
	Timeout time.Duration
}

// DefaultCommandTimeout is long enough for a build and short enough that a
// command waiting on input does not hang the brain forever.
const DefaultCommandTimeout = 5 * time.Minute

func (RunCommand) Name() string { return "run_command" }

func (RunCommand) Description() string {
	return "Run a command on the computer. Give the program and its arguments as separate " +
		"array items, not a single shell string. There is no shell, so pipes, redirects " +
		"and wildcards will be passed through as literal text. Requires the owner's approval."
}

func (RunCommand) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"argv": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Program then arguments, e.g. [\"git\",\"status\"]"
			},
			"dir": {"type": "string", "description": "Optional working directory"}
		},
		"required": ["argv"]
	}`)
}

func (RunCommand) Risk() Risk { return Mutating }

type commandArgs struct {
	Argv []string `json:"argv"`
	Dir  string   `json:"dir"`
}

// Summarize shows the exact command. A person approving this is vouching for
// something that will run on their machine, so paraphrasing would make the
// approval worthless.
func (RunCommand) Summarize(raw json.RawMessage) string {
	var a commandArgs
	argsOf(raw, &a)

	quoted := make([]string, 0, len(a.Argv))

	for _, s := range a.Argv {
		if strings.ContainsAny(s, " \t\n\"'") {
			quoted = append(quoted, fmt.Sprintf("%q", s))

			continue
		}

		quoted = append(quoted, s)
	}

	out := "Run: " + strings.Join(quoted, " ")

	if a.Dir != "" {
		out += "  (in " + a.Dir + ")"
	}

	return out
}

func (c RunCommand) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a commandArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if len(a.Argv) == 0 {
		return "", fmt.Errorf("no command was given")
	}

	timeout := c.Timeout
	if timeout == 0 {
		timeout = DefaultCommandTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	/*
	 * Started with nothing of this program's: no keys in its environment, in
	 * a process group of its own so stopping it stops what it started, and —
	 * on a project — held by the kernel to writing in the project.
	 */
	spec := sandbox.Spec{Dir: a.Dir, Inherit: true}

	if writable, confined := sandbox.WritableFrom(ctx); confined {
		spec = sandbox.Spec{Dir: a.Dir, Writable: writable}
	}

	cmd, err := sandbox.Command(ctx, spec, a.Argv...)
	if err != nil {
		return "", err
	}

	// A command that reads stdin would block forever waiting for a person who
	// is not there. Closing it makes such a command fail immediately instead.
	cmd.Stdin = nil

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	text := strings.TrimSpace(out.String())

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("the command was still running after %s and was stopped:\n\n%s", timeout, text)
	}

	if text == "" {
		text = "(no output)"
	}

	// A non-zero exit is reported rather than raised. It is usually the answer
	// — a failing test, a dirty working tree — and the model needs to read it
	// to say anything useful.
	if exit, ok := err.(*exec.ExitError); ok {
		return fmt.Sprintf("Exited with code %d.\n\n%s", exit.ExitCode(), text), nil
	}

	if err != nil {
		return "", fmt.Errorf("could not run %s: %w", a.Argv[0], err)
	}

	return text, nil
}
