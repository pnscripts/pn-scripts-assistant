package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

/*
 * Running something as the administrator, with the system asking its owner.
 *
 * Everything is allowed (ADR 0007, 0008), but root is not this program's to
 * hand itself: passwordless sudo, a sudoers line or a setuid helper would be
 * a change to the machine's own security that its owner makes or does not.
 * So this asks the operating system to ask him — polkit's password dialog on
 * Linux (pkexec), UAC on Windows, the administrator prompt on macOS — every
 * time, and runs the command only if he answers it. run_command cannot do
 * this: sudo there has no terminal to ask in.
 */
type RunAsAdmin struct {
	// Elevate is the program that asks; empty is the system's own. For tests.
	Elevate []string

	// Timeout bounds the whole thing, the question included.
	Timeout time.Duration
}

func (RunAsAdmin) Name() string { return "run_as_admin" }

func (RunAsAdmin) Description() string {
	return "Run a command as the administrator (root). The operating system shows its own password " +
		"dialog to the owner — polkit on Linux, UAC on Windows, the administrator prompt on macOS — " +
		"and the command runs only if he answers it. Give argv as program then arguments; use this " +
		"instead of sudo, which cannot ask from here."
}

func (RunAsAdmin) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"argv": {"type": "array", "items": {"type": "string"}, "description": "Program then arguments, e.g. [\"apt-get\",\"install\",\"-y\",\"ffmpeg\"]"},
			"why": {"type": "string", "description": "One line the owner can read in the log: why root is needed"}
		},
		"required": ["argv"]
	}`)
}

func (RunAsAdmin) Risk() Risk { return Mutating }

type adminArgs struct {
	Argv []string `json:"argv"`
	Why  string   `json:"why"`
}

func (RunAsAdmin) Summarize(raw json.RawMessage) string {
	var a adminArgs
	argsOf(raw, &a)

	return "As administrator (the system asks you): " + strings.Join(a.Argv, " ")
}

func (RunAsAdmin) Cues() []string {
	return []string{"root", "sudo", "admin", "administrator", "install", "apt", "system", "service",
		"systemctl", "permission denied"}
}

// Touches: root can write anywhere, and it says so.
func (RunAsAdmin) Touches(json.RawMessage) ([]string, []string) { return []string{"/"}, []string{"/"} }

// elevated is argv wrapped in the system's own way of asking.
func (t RunAsAdmin) elevated(argv []string) ([]string, error) {
	if len(t.Elevate) > 0 {
		return append(append([]string{}, t.Elevate...), argv...), nil
	}

	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("pkexec"); err != nil {
			return nil, errors.New("pkexec is not installed, so the system cannot be asked for the password " +
				"(install the policykit-1 or pkexec package)")
		}

		return append([]string{"pkexec"}, argv...), nil

	case "darwin":
		quoted := make([]string, len(argv))
		for i, a := range argv {
			quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}

		script := strings.Join(quoted, " ")
		script = strings.ReplaceAll(strings.ReplaceAll(script, `\`, `\\`), `"`, `\"`)

		return []string{"osascript", "-e", `do shell script "` + script + `" with administrator privileges`}, nil

	case "windows":
		quoted := make([]string, 0, len(argv)-1)
		for _, a := range argv[1:] {
			quoted = append(quoted, "'"+strings.ReplaceAll(a, "'", "''")+"'")
		}

		list := "@()"
		if len(quoted) > 0 {
			list = "@(" + strings.Join(quoted, ",") + ")"
		}

		ps := "$p = Start-Process -FilePath '" + strings.ReplaceAll(argv[0], "'", "''") + "' -ArgumentList " + list +
			" -Verb RunAs -Wait -PassThru; exit $p.ExitCode"

		return []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", ps}, nil
	}

	return nil, fmt.Errorf("running as administrator is not supported on %s", runtime.GOOS)
}

func (t RunAsAdmin) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a adminArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if len(a.Argv) == 0 || strings.TrimSpace(a.Argv[0]) == "" {
		return "", errors.New("which command?")
	}

	argv, err := t.elevated(a.Argv)
	if err != nil {
		return "", err
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = DefaultCommandTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var out bytes.Buffer

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = &out, &out

	err = cmd.Run()

	text := strings.TrimSpace(out.String())
	if len(text) > 20000 {
		text = text[len(text)-20000:]
	}

	if err != nil {
		var exit *exec.ExitError

		// pkexec's own codes: 126 is the dialog dismissed, 127 not authorised.
		if errors.As(err, &exit) && runtime.GOOS == "linux" && len(t.Elevate) == 0 &&
			(exit.ExitCode() == 126 || exit.ExitCode() == 127) {
			return "", fmt.Errorf("the owner did not give the administrator password, so nothing ran")
		}

		return "", fmt.Errorf("%w\n\n%s", err, text)
	}

	if text == "" {
		text = "done, with no output"
	}

	return text, nil
}
