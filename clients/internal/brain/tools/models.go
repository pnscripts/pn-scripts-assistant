package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

/*
 * What the brain is running on.
 *
 * Asked "what models do I have?", it answered "I don't know. Can you provide
 * more context?" — which is true and useless: the answer was three inches away
 * on the same screen and it had no way to reach it. A brain that cannot say
 * what it is made of is oddly incurious about itself.
 */

// Installed is what these tools need from the machine.
type Installed interface {
	Models(ctx context.Context) ([]InstalledModel, error)
	Roles() (work, talk, reason string)
}

// InstalledModel is one model on this machine.
type InstalledModel struct {
	Name string

	// Size as the machine already describes it — "4.4GB" — rather than a
	// number to be formatted again. It is shown to a person, and the place it
	// comes from has already made that decision.
	Size string
}

// ListModels reports the models available and which does what.
type ListModels struct{ Machine Installed }

func (ListModels) Name() string { return "list_models" }

func (ListModels) Description() string {
	return "List the language models installed on this machine and say which " +
		"one is used for talking, for doing things and for working things out. " +
		"Use when asked what models are available or which one is answering."
}

func (ListModels) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (ListModels) Risk() Risk { return Safe }

func (ListModels) Summarize(json.RawMessage) string { return "List the models on this machine" }

func (t ListModels) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.Machine == nil {
		return "", fmt.Errorf("there is nothing to ask about the models")
	}

	installed, err := t.Machine.Models(ctx)
	if err != nil {
		return "", err
	}

	if len(installed) == 0 {
		return "No models are installed.", nil
	}

	work, talk, reason := t.Machine.Roles()

	job := map[string]string{}

	// Written per model rather than listed separately, because "which one
	// answers me" is the actual question behind "what models do I have".
	for name, role := range map[string]string{
		work: "used for doing things", talk: "used for talking",
		reason: "used for working things out",
	} {
		if name != "" {
			job[name] = role
		}
	}

	var b strings.Builder

	b.WriteString("Models on this machine:\n")

	for _, m := range installed {
		fmt.Fprintf(&b, "  %s (%s)", m.Name, m.Size)

		if role, ok := job[m.Name]; ok {
			fmt.Fprintf(&b, " — %s", role)
		}

		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n"), nil
}
