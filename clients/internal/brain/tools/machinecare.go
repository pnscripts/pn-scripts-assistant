package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/machinecare"
)

// MachineCaring is what the machine-care tools need from the brain.
type MachineCaring interface {
	CareLook(ctx context.Context) (last machinecare.LastStop, restart machinecare.Readiness, leftovers []machinecare.Leftover, here bool)
	StopLeftover(pid int, command string) error
}

// CanIRestart answers "can I restart now?" from the machine.
type CanIRestart struct{ Care MachineCaring }

func (CanIRestart) Name() string { return "can_i_restart" }

func (CanIRestart) Description() string {
	return "Say whether this computer can be restarted now: a newer kernel waiting and whether its " +
		"drivers (DKMS) are built for it, software being installed, the assistant's own work still " +
		"running, project folders with work not committed or not pushed, and how the last session ended."
}

func (CanIRestart) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (CanIRestart) Risk() Risk { return Safe }

func (CanIRestart) Cues() []string {
	return []string{"restart", "reboot", "рестарт", "рестартирам", "рестартирам ли", "kernel", "ядро"}
}

func (CanIRestart) Summarize(json.RawMessage) string { return "Check whether a restart is safe now" }

func (t CanIRestart) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.Care == nil {
		return "", errors.New("the machine cannot be looked at from here")
	}

	last, r, _, _ := t.Care.CareLook(ctx)

	if !r.Known {
		return "Whether a restart is safe is only looked at on Linux so far.", nil
	}

	var b strings.Builder

	if r.Ready {
		b.WriteString("Yes, a restart is fine now.\n")
	} else {
		b.WriteString("Not yet.\n")
	}

	for _, i := range r.Items {
		mark := "- "
		if i.Blocking {
			mark = "- In the way: "
		}

		b.WriteString(mark + i.Say + "\n")
	}

	if len(r.Items) == 0 {
		b.WriteString("Nothing is waiting or running that a restart would cut off.\n")
	}

	if note := lastStopSentence(last); note != "" {
		b.WriteString("\n" + note + "\n")
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// lastStopSentence says how the last session ended, when it ended badly.
func lastStopSentence(last machinecare.LastStop) string {
	if !last.Unclean {
		return ""
	}

	return "The last session was not closed: it stopped (a power cut or a crash) after " +
		last.LastSeen.Local().Format("15:04 on 2 January") + ". Anything it was doing was parked and waits under Tasks."
}

// LeftoverProcesses lists what was left running whose job looks over.
type LeftoverProcesses struct{ Care MachineCaring }

func (LeftoverProcesses) Name() string { return "leftover_processes" }

func (LeftoverProcesses) Description() string {
	return "List processes left running whose job looks over (a headless browser, a virtual display, " +
		"a waiting sleep or timeout, a log follower, a small web server) that this assistant started or " +
		"whose starter has gone. The user's own programs are never listed."
}

func (LeftoverProcesses) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (LeftoverProcesses) Risk() Risk { return Safe }

func (LeftoverProcesses) Cues() []string {
	return []string{"leftover", "processes", "процеси", "останали"}
}

func (LeftoverProcesses) Summarize(json.RawMessage) string { return "Look for leftover processes" }

func (t LeftoverProcesses) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.Care == nil {
		return "", errors.New("the machine cannot be looked at from here")
	}

	_, _, list, here := t.Care.CareLook(ctx)

	if !here {
		return "Leftover processes are only looked for on Linux so far.", nil
	}

	if len(list) == 0 {
		return "Nothing is left running whose job looks over.", nil
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d left running (stop one with stop_leftover, giving its pid and command):\n", len(list))

	for _, l := range list {
		fmt.Fprintf(&b, "- pid %d, %s, running %s, %s: %s\n", l.PID, l.Kind, l.Age, l.Why, l.Command)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

/*
 * StopLeftover stops one process from that list, after its user says yes.
 * Only one the list still shows with the same command: a process id that
 * has been reused since is never touched, and nothing else can be stopped
 * this way.
 */
type StopLeftover struct{ Care MachineCaring }

func (StopLeftover) Name() string { return "stop_leftover" }

func (StopLeftover) Description() string {
	return "Stop one leftover process listed by leftover_processes, by its pid and exact command. " +
		"Nothing that is not on that list can be stopped."
}

func (StopLeftover) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pid": {"type": "integer", "description": "The process id from the list."},
			"command": {"type": "string", "description": "Its command, exactly as the list showed it."}
		},
		"required": ["pid", "command"]
	}`)
}

func (StopLeftover) Risk() Risk { return Mutating }

// AlwaysAsks: a process is stopped only with its user's yes, however much
// else they have allowed.
func (StopLeftover) AlwaysAsks(json.RawMessage) string { return "stopping a program" }

type stopArgs struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

func (StopLeftover) Summarize(raw json.RawMessage) string {
	var a stopArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Stop the leftover process %d: %s", a.PID, a.Command)
}

func (t StopLeftover) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	if t.Care == nil {
		return "", errors.New("the machine cannot be looked at from here")
	}

	var a stopArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := t.Care.StopLeftover(a.PID, a.Command); err != nil {
		return "", err
	}

	return fmt.Sprintf("Stopped process %d (%s) at %s.", a.PID, a.Command, time.Now().Format("15:04")), nil
}
