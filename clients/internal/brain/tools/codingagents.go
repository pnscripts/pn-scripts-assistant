package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

/*
 * Coding agents as tools: Claude Code, Codex, Cursor's agent — whichever are
 * on this machine and signed in by their owner — asked for one piece of
 * work from a conversation, in a folder its user chooses, as far as they
 * allow for that one task:
 *
 *   - read   — it only reads the folder and answers;
 *   - folder — it may change files in the folder and runs no commands;
 *   - full   — it may do anything its user could, anywhere: asked about
 *              every time, whatever else has been allowed.
 *
 * It runs on its owner's own subscription, signed in as they signed it in:
 * nothing here reads, keeps or passes on a token, and the environment it
 * starts in carries no API key. What it changed is read from the folder
 * itself afterwards, not taken from what it says.
 */

// CodingAgent is one agent that can be asked, as the brain finds it.
type CodingAgent struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Ready bool   `json:"ready"`
	Why   string `json:"why,omitempty"`

	// Pace is how its allowance is being spent, in a sentence, when the
	// agent has said: see orchestrator.PaceOf.
	Pace string `json:"pace,omitempty"`

	// Held says, when it is, that work for it waits for its user's quiet
	// hours (its allowance is ahead of its pace and the window is shut),
	// and until when: see the quiethours package. A held call is always
	// asked, and the yes starts it anyway.
	Held string `json:"held,omitempty"`
}

// AgentChange is one file an agent added, changed or removed.
type AgentChange struct {
	Path string `json:"path"`
	What string `json:"what"`
}

// AgentAnswer is what came of asking.
type AgentAnswer struct {
	Agent    string
	Model    string
	Said     string
	Changed  []AgentChange
	Commands []string
	Took     string

	// Partial is whether the folder was too big to read whole afterwards.
	Partial bool
}

// CodingAgents is what the tool needs from the brain.
type CodingAgents interface {
	CodingAgents() []CodingAgent
	AskCodingAgent(ctx context.Context, agent, folder, task, access string) (AgentAnswer, error)
}

/*
 * AlwaysAsking is a tool with calls that are put to its user every time,
 * whatever else has been allowed: "never stop", goal mode and everything
 * allowed included. AlwaysAsks says why, or is empty for a call that is not
 * one of them.
 */
type AlwaysAsking interface {
	AlwaysAsks(args json.RawMessage) string
}

/*
 * QuietHolding is a tool whose call can be held for its user's quiet hours.
 *
 * QuietHold says whether this call is held now, and whether being held is
 * all its tool would ask about (the rest of the gate is the loop's to say).
 * A call held for nothing else would have run without a question at any
 * other time, so it answers itself when the window opens; the second answer
 * is read again then, when the call is no longer held.
 */
type QuietHolding interface {
	QuietHold(args json.RawMessage) (held, only bool)
}

// The ways a task may reach, as the tool takes them.
const (
	ReachRead   = "read"
	ReachFolder = "folder"
	ReachFull   = "full"
)

// AskCodingAgent hands one task to a coding agent on this machine.
type AskCodingAgent struct{ Agents CodingAgents }

func (AskCodingAgent) Name() string { return "ask_coding_agent" }

func (AskCodingAgent) Description() string {
	return "Hand one task to a coding agent on this computer (Claude Code, Codex or Cursor's agent, " +
		"signed in by the user) working in a folder the user chose, and bring back its answer and every " +
		"file it changed. access: read (only reads and answers; the default), folder (may change files " +
		"in the folder, runs no commands), full (may do anything; only when the user asked for it). " +
		"Give the whole task and what it needs to know; it does not see this conversation."
}

func (AskCodingAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {"type": "string", "description": "The task, in full, with what the agent needs to know."},
			"folder": {"type": "string", "description": "The folder it works in, as a full path."},
			"access": {"type": "string", "enum": ["read", "folder", "full"], "description": "How far it may reach. Default read."},
			"agent": {"type": "string", "description": "claude-code, codex or cursor-agent. Default: the first one ready."}
		},
		"required": ["task", "folder"]
	}`)
}

// Mutating: it runs a program that works on its own, and with folder or
// full access changes things. Asked unless its user has allowed it.
func (AskCodingAgent) Risk() Risk { return Mutating }

func (AskCodingAgent) Cues() []string {
	return []string{"claude", "codex", "coding agent", "агент за код"}
}

// LeavesTheMachine: the task and what it reads go to the agent's own service.
func (AskCodingAgent) LeavesTheMachine() bool { return true }

type agentArgs struct {
	Task   string `json:"task"`
	Folder string `json:"folder"`
	Access string `json:"access"`
	Agent  string `json:"agent"`
}

func readAgentArgs(raw json.RawMessage) agentArgs {
	var a agentArgs

	_ = json.Unmarshal(raw, &a)

	a.Access = strings.ToLower(strings.TrimSpace(a.Access))
	if a.Access == "" {
		a.Access = ReachRead
	}

	return a
}

// chosen is the agent a call would ask: the one named, or the first ready.
func (t AskCodingAgent) chosen(name string) (CodingAgent, bool) {
	if t.Agents == nil {
		return CodingAgent{}, false
	}

	all := t.Agents.CodingAgents()

	for _, a := range all {
		if name != "" && (a.ID == name || strings.EqualFold(a.Title, name)) {
			return a, true
		}
	}

	if name == "" {
		for _, a := range all {
			if a.Ready {
				return a, true
			}
		}
	}

	return CodingAgent{}, false
}

// Summarize says which agent, how far it may reach, where, and the task
// word for word: the person saying yes is agreeing to all four.
func (t AskCodingAgent) Summarize(raw json.RawMessage) string {
	a := readAgentArgs(raw)

	title, pace, held := a.Agent, "", ""
	if agent, ok := t.chosen(a.Agent); ok {
		title, pace, held = agent.Title, agent.Pace, agent.Held
	}

	if title == "" {
		title = "a coding agent"
	}

	var reach string

	switch a.Access {
	case ReachRead:
		reach = "only reads " + a.Folder + " and answers"
	case ReachFolder:
		reach = "may change files in " + a.Folder + "; runs no commands"
	case ReachFull:
		reach = "FULL ACCESS: may run any command and change any file you can, starting in " + a.Folder
	default:
		reach = "unknown access " + a.Access
	}

	out := fmt.Sprintf("Ask %s (%s):\n%s", title, reach, strings.TrimSpace(a.Task))

	// Its allowance, so whoever says yes knows what the work spends.
	if pace != "" {
		out += "\n\n" + title + ": " + pace + "."
	}

	// And, when the work waits for the quiet hours, until when: the yes is
	// what starts it anyway.
	if held != "" {
		out += "\n\n" + held
	}

	return out
}

/*
 * AlwaysAsks: full access is put to its user every time, whatever else
 * they have allowed: "never stop", goal mode and everything allowed
 * included. It is the one reach that is not bounded by the folder. So is
 * any call to an agent held for the quiet hours.
 */
func (t AskCodingAgent) AlwaysAsks(raw json.RawMessage) string {
	a := readAgentArgs(raw)

	var why []string

	if a.Access == ReachFull {
		why = append(why, "full access for a coding agent")
	}

	// Held for the quiet hours: whatever its reach, the work waits for a
	// yes, which starts it now anyway.
	if agent, ok := t.chosen(a.Agent); ok && agent.Held != "" {
		why = append(why, "held for your quiet hours")
	}

	return strings.Join(why, "; ")
}

// QuietHold: held when its agent is; only that, unless it has full access,
// which is always asked whatever the hour.
func (t AskCodingAgent) QuietHold(raw json.RawMessage) (held, only bool) {
	a := readAgentArgs(raw)

	if agent, ok := t.chosen(a.Agent); ok && agent.Held != "" {
		held = true
	}

	return held, a.Access != ReachFull
}

// Outward: with folder or full access it changes files.
func (AskCodingAgent) Outward(raw json.RawMessage) []string {
	if readAgentArgs(raw).Access == ReachRead {
		return nil
	}

	return []string{"files"}
}

func (t AskCodingAgent) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	a := readAgentArgs(raw)

	if strings.TrimSpace(a.Task) == "" {
		return "", errors.New("say what the agent should do")
	}

	switch a.Access {
	case ReachRead, ReachFolder, ReachFull:
	default:
		return "", fmt.Errorf("access is read, folder or full, not %q", a.Access)
	}

	agent, ok := t.chosen(a.Agent)
	if !ok {
		return "", errors.New(noAgent(t.Agents, a.Agent))
	}

	if !agent.Ready {
		return "", fmt.Errorf("%s cannot be asked now: %s", agent.Title, agent.Why)
	}

	answer, err := t.Agents.AskCodingAgent(ctx, agent.ID, a.Folder, a.Task, a.Access)
	if err != nil {
		return "", err
	}

	return answer.Text(), nil
}

func noAgent(agents CodingAgents, name string) string {
	if agents == nil {
		return "no coding agent can be asked from here"
	}

	var titles []string

	for _, a := range agents.CodingAgents() {
		state := "ready"
		if !a.Ready {
			state = a.Why
		}

		titles = append(titles, a.Title+" ("+state+")")
	}

	if len(titles) == 0 {
		return "no coding agent is installed on this computer: Claude Code, Codex or Cursor's agent can be " +
			"installed and signed in by you"
	}

	if name != "" {
		return fmt.Sprintf("there is no coding agent called %q here; there is %s", name, strings.Join(titles, ", "))
	}

	return "no coding agent is ready: " + strings.Join(titles, ", ")
}

// Text is the answer as the model and the record read it: what the agent
// said, then every file it changed and every command it ran.
func (a AgentAnswer) Text() string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s answered", a.Agent)

	if a.Model != "" {
		b.WriteString(" (" + a.Model + ")")
	}

	if a.Took != "" {
		b.WriteString(" in " + a.Took)
	}

	b.WriteString(":\n" + strings.TrimSpace(a.Said) + "\n")

	if len(a.Changed) == 0 {
		b.WriteString("\nNo file in the folder changed.\n")
	} else {
		fmt.Fprintf(&b, "\nFiles it changed (%d):\n", len(a.Changed))

		for _, c := range a.Changed {
			b.WriteString("- " + c.What + ": " + c.Path + "\n")
		}
	}

	if a.Partial {
		b.WriteString("(The folder is too big to read whole; files beyond the first ones read were not compared.)\n")
	}

	if len(a.Commands) > 0 {
		b.WriteString("\nCommands it ran:\n")

		for _, c := range a.Commands {
			b.WriteString("- " + c + "\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}
