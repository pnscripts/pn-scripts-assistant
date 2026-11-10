package main

import (
	"bytes"
	"net"
	"os"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
)

/*
 * The terminal says the program's command, never "brain".
 *
 * A drive that had gone away was explained with "brain start-again", a
 * command that does not exist since the program became PN Scripts Assistant
 * (owner, 10.11.2026: commands shown as "brain …" must say the program's
 * name). These read what a person sees in the terminal.
 */

// aCommand is a line that tells somebody what to type: indented, then a word
// that is not a sentence.
var aCommand = regexp.MustCompile(`(?m)^\s{2,}([a-z][a-z-]*) [a-z-<\[]`)

func TestEveryCommandInTheHelpIsTheProgramsOwn(t *testing.T) {
	var b bytes.Buffer
	usageTo(&b)

	help := b.String()

	lines := 0

	for _, m := range aCommand.FindAllStringSubmatch(help, -1) {
		lines++

		if m[1] != "pn-scripts-assistant" {
			t.Errorf("the help shows a command as %q: %q", m[1], strings.TrimSpace(m[0]))
		}
	}

	if lines < 20 {
		t.Fatalf("only %d command lines found in the help:\n%s", lines, help)
	}

	// Putting it in the applications menu puts the program there, not its
	// knowledge.
	if !strings.Contains(help, "put PN Scripts Assistant in the applications menu") {
		t.Errorf("the menu line does not name the program:\n%s", help)
	}
}

func TestADriveThatIsAwaySaysTheRealCommand(t *testing.T) {
	text := awayText("/media/drive/PN-SCRIPTS-ASSISTANT-DATA")

	if strings.Contains(text, "brain start-again") {
		t.Errorf("it still says brain start-again:\n%s", text)
	}

	if !strings.Contains(text, "    pn-scripts-assistant start-again\n") {
		t.Errorf("it does not say the command that starts again:\n%s", text)
	}

	if !strings.Contains(text, "/media/drive/PN-SCRIPTS-ASSISTANT-DATA") {
		t.Errorf("it does not say where the knowledge was:\n%s", text)
	}
}

// A taken port is answered with the variable under the program's name.
func TestATakenPortSaysTheVariableUnderTheProgramsName(t *testing.T) {
	cfg := config.Default()
	cfg.Addr = "127.0.0.1:1" // nothing of ours answers there

	taken := &net.OpError{Op: "listen", Net: "tcp", Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}

	msg := bindProblem(cfg, taken).Error()

	if !strings.Contains(msg, "PN_SCRIPTS_ASSISTANT_ADDR=127.0.0.1:<port>") {
		t.Errorf("a taken port does not say PN_SCRIPTS_ASSISTANT_ADDR: %s", msg)
	}

	if strings.Contains(msg, "BRAIN_ADDR") {
		t.Errorf("a taken port still says BRAIN_ADDR: %s", msg)
	}
}
