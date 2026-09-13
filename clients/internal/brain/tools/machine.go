package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

/*
 * Looking after the machine, from inside the conversation.
 *
 * Everything here could already be done by pressing a button, and that was
 * held to be enough: installing writes to the machine, some of it asks for a
 * password, so it was "a deliberate press on a named thing, never something
 * the brain decides to do".
 *
 * That reasoning was about who authorises the change, and it was answered by
 * building somewhere for authorisation to live. Permissions decide whether a
 * change may happen; they are asked before any of this runs, and the answer is
 * remembered. With that in place, refusing to let somebody say "install the
 * bigger model" — and making them find the panel instead — is not caution, it
 * is the assistant declining to be useful about the one subject it knows most
 * about.
 *
 * So: the same lists and the same installers the panels use, reached by
 * talking. Not a second copy of either. A parallel list is a list that goes
 * stale.
 */

// MachinePart is one of the pieces the assistant runs on, as the tools see it.
type MachinePart struct {
	Name        string
	Why         string
	Consequence string
	State       string
	Detail      string
	Size        string
	Where       string
	Optional    bool
	Installable bool

	// Removable is whether this program installed it and can take it away.
	// False for the system's own packages, which are shared.
	Removable bool
}

/*
 * WhatItNeeds answers "what is missing", which is asked far more often than
 * anything is installed.
 *
 * Safe: it reads the same checks the setup screen reads and changes nothing.
 */
type WhatItNeeds struct {
	Parts func() []MachinePart
}

func (WhatItNeeds) Name() string { return "what_this_machine_needs" }

func (WhatItNeeds) Description() string {
	return "List every piece this assistant runs on — Ollama, the models, the voice, " +
		"the recogniser, Godot — with whether each is installed, what it is for, what " +
		"stops working without it and how large the download is. Use when asked what is " +
		"missing, what is installed, why something does not work, or before installing " +
		"anything, so the answer names what is actually on this machine."
}

func (WhatItNeeds) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (WhatItNeeds) Risk() Risk { return Safe }

func (WhatItNeeds) Summarize(json.RawMessage) string {
	return "look at what this machine has and is missing"
}

func (t WhatItNeeds) Execute(context.Context, json.RawMessage) (string, error) {
	if t.Parts == nil {
		return "", fmt.Errorf("I cannot see this machine's parts from here")
	}

	var b strings.Builder

	var missing int

	for _, p := range t.Parts() {
		if p.State != "ok" {
			missing++
		}

		fmt.Fprintf(&b, "%s — %s\n", p.Name, p.State)
		fmt.Fprintf(&b, "  what it is for: %s\n", p.Why)

		if p.Detail != "" {
			fmt.Fprintf(&b, "  now: %s\n", p.Detail)
		}

		if p.State != "ok" {
			if p.Consequence != "" {
				fmt.Fprintf(&b, "  without it: %s\n", p.Consequence)
			}

			if p.Size != "" {
				fmt.Fprintf(&b, "  download: %s\n", p.Size)
			}

			if p.Where != "" {
				fmt.Fprintf(&b, "  would go to: %s\n", p.Where)
			}

			if !p.Installable {
				fmt.Fprint(&b, "  cannot be installed from here\n")
			}
		}
	}

	if missing == 0 {
		b.WriteString("\nNothing is missing.\n")
	} else {
		fmt.Fprintf(&b, "\n%d not installed.\n", missing)
	}

	return b.String(), nil
}

/*
 * InstallPart installs one named piece.
 *
 * Mutating, and it stays mutating even once permissions allow it: the summary
 * is what somebody reads when deciding, so it names the piece and the size
 * rather than saying that something will be installed.
 */
type InstallPart struct {
	Parts func() []MachinePart

	// Start begins the install in the background and returns what to say about
	// it. Background because a model is gigabytes and a recogniser is minutes
	// of compiling, and a turn that waits for either is a turn that looks hung.
	Start func(name string) (string, error)
}

func (InstallPart) Name() string { return "install_a_part" }

func (InstallPart) Description() string {
	return "Install one of the pieces this assistant runs on, by its exact name from " +
		"what_this_machine_needs — for example \"Voice (listening)\" or \"Godot (making " +
		"games)\". It downloads in the background and appears under Activity. Check " +
		"what_this_machine_needs first, so you install the right name and can say what " +
		"it costs."
}

func (InstallPart) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","description":"The exact name of the piece to install."}
		},
		"required":["name"],
		"additionalProperties":false
	}`)
}

func (InstallPart) Risk() Risk { return Mutating }

func (t InstallPart) Summarize(raw json.RawMessage) string {
	var a struct {
		Name string `json:"name"`
	}

	json.Unmarshal(raw, &a)

	// The size belongs in the question. "Install Voice (listening)?" and
	// "Install Voice (listening), 488MB?" are different questions.
	if t.Parts != nil {
		for _, p := range t.Parts() {
			if p.Name == a.Name && p.Size != "" {
				return "install " + a.Name + " (" + p.Size + ")"
			}
		}
	}

	return "install " + a.Name
}

func (t InstallPart) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("I could not read that: %w", err)
	}

	name := strings.TrimSpace(a.Name)
	if name == "" {
		return "", fmt.Errorf("say which piece to install")
	}

	if t.Start == nil {
		return "", fmt.Errorf("I cannot install anything from here")
	}

	/*
	 * A wrong name is answered with the right ones.
	 *
	 * The names have brackets in them — "Voice (listening)" — and a model
	 * asked to install "whisper" would otherwise get nothing back but a
	 * refusal, and try again with another guess.
	 */
	if t.Parts != nil {
		var known []string

		for _, p := range t.Parts() {
			if p.Name == name {
				known = nil

				break
			}

			if p.Installable {
				known = append(known, p.Name)
			}
		}

		if len(known) > 0 {
			return "", fmt.Errorf("there is no piece called %q. The ones that can be "+
				"installed are: %s", name, strings.Join(known, ", "))
		}
	}

	return t.Start(name)
}

/*
 * InstallModel downloads a language model by name.
 *
 * Separate from InstallPart because the parts list holds "a chat model" as one
 * requirement, and this is about a particular one — somebody who has read the
 * list of what their machine can run and wants that one.
 */
type InstallModel struct {
	Start func(name string) (string, error)
}

func (InstallModel) Name() string { return "install_a_model" }

func (InstallModel) Description() string {
	return "Download a language model onto this machine by its exact Ollama name, for " +
		"example \"qwen2.5-coder:7b\" or \"llama3.2:3b\". Use list_models first to see " +
		"what is already here. Large: say the size before installing one."
}

func (InstallModel) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","description":"The model's name, as Ollama spells it."}
		},
		"required":["name"],
		"additionalProperties":false
	}`)
}

func (InstallModel) Risk() Risk { return Mutating }

func (InstallModel) Summarize(raw json.RawMessage) string {
	var a struct {
		Name string `json:"name"`
	}

	json.Unmarshal(raw, &a)

	return "download the model " + a.Name
}

func (t InstallModel) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("I could not read that: %w", err)
	}

	name := strings.TrimSpace(a.Name)
	if name == "" {
		return "", fmt.Errorf("say which model to download")
	}

	if t.Start == nil {
		return "", fmt.Errorf("I cannot download models from here")
	}

	return t.Start(name)
}

/*
 * CheckUpdates reports what is out of date, without changing anything.
 *
 * Safe, and it is worth saying why given what it touches: it asks whether
 * newer versions exist. Looking is gated by the separate "may it look things
 * up" permission, which is where that decision belongs — it is not a privacy
 * setting and it is not this tool's business.
 */
type CheckUpdates struct {
	Look func(ctx context.Context) (string, error)
}

func (CheckUpdates) Name() string { return "check_for_updates" }

func (CheckUpdates) Description() string {
	return "Check whether the program itself, or any of the pieces it runs on, has a " +
		"newer version. Reports what is out of date and does not install anything. Use " +
		"when asked whether it is up to date, or before offering to update."
}

func (CheckUpdates) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (CheckUpdates) Risk() Risk { return Safe }

func (CheckUpdates) Summarize(json.RawMessage) string { return "check for updates" }

func (t CheckUpdates) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.Look == nil {
		return "", fmt.Errorf("I cannot check for updates from here")
	}

	return t.Look(ctx)
}

/*
 * RemovePart takes one piece back off the machine.
 *
 * Mutating, and the summary says the name rather than "remove something",
 * because this is the tool whose approval question matters most: the cost of
 * getting it wrong is a gigabyte download to undo.
 *
 * Only the pieces this program installed. The refusal for the others is worth
 * reading rather than a plain error — "it came with your system" tells
 * somebody why the answer is no and what to do instead, where "cannot remove"
 * invites trying again.
 */
type RemovePart struct {
	Parts  func() []MachinePart
	Remove func(name string) (string, error)
}

func (RemovePart) Name() string { return "remove_a_part" }

func (RemovePart) Description() string {
	return "Take one of the pieces this assistant installed back off the machine, by " +
		"its exact name from what_this_machine_needs — for example \"Godot (making " +
		"games)\". Only works for pieces this program installed; the ones that came " +
		"with the system are not ours to remove. Say what will stop working first."
}

func (RemovePart) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","description":"The exact name of the piece to remove."}
		},
		"required":["name"],
		"additionalProperties":false
	}`)
}

func (RemovePart) Risk() Risk { return Mutating }

func (t RemovePart) Summarize(raw json.RawMessage) string {
	var a struct {
		Name string `json:"name"`
	}

	json.Unmarshal(raw, &a)

	// What breaks, in the question. Somebody deciding whether to allow this
	// needs the consequence more than the name.
	if t.Parts != nil {
		for _, p := range t.Parts() {
			if p.Name == a.Name && p.Consequence != "" {
				return "remove " + a.Name + " — then " + p.Consequence
			}
		}
	}

	return "remove " + a.Name + " from this machine"
}

func (t RemovePart) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("I could not read that: %w", err)
	}

	name := strings.TrimSpace(a.Name)
	if name == "" {
		return "", fmt.Errorf("say which piece to remove")
	}

	if t.Remove == nil {
		return "", fmt.Errorf("I cannot remove anything from here")
	}

	// A wrong name is answered with the right ones, as with installing.
	if t.Parts != nil {
		var known []string

		found := false

		for _, p := range t.Parts() {
			if p.Name == name {
				found = true
			}

			if p.Removable {
				known = append(known, p.Name)
			}
		}

		if !found && len(known) > 0 {
			return "", fmt.Errorf("there is no piece called %q. The ones I can remove "+
				"are: %s", name, strings.Join(known, ", "))
		}
	}

	return t.Remove(name)
}
