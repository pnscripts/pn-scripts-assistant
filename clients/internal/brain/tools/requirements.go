package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/risk"
)

/*
 * What a piece of work needs on this machine, and putting it there.
 *
 * Checking is safe and quick. Installing is only ever one of the recipes the
 * program ships — the argument is a recipe's name, never an address or a
 * command — and it is always put to its owner, with what it is, where it comes
 * from, how big it is and under what licence, because putting software on
 * somebody's machine is theirs to decide whatever else they have allowed.
 */

// Provisioning is what the tools need from the brain.
type Provisioning interface {
	Recipes() *provision.Book
	Packages() *capability.Set

	// StartInstall installs one recipe behind the conversation and says so.
	StartInstall(id, versions string) (string, error)
}

// CheckRequirements says what a package or a recipe needs and what is here.
type CheckRequirements struct {
	Provisioning Provisioning
}

func (CheckRequirements) Name() string { return "check_requirements" }

func (CheckRequirements) Description() string {
	return "Say what a capability package needs on this machine — engines, runtimes, tools — " +
		"and whether each is here in a version that will do. Give a package id " +
		"(software.game.godot) or one recipe (godot, node)."
}

func (CheckRequirements) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"package": {"type": "string", "description": "A capability package id, like software.game.threejs."},
			"recipe": {"type": "string", "description": "One recipe, like godot or node."}
		},
		"additionalProperties": false
	}`)
}

func (CheckRequirements) Risk() Risk { return Safe }

func (CheckRequirements) Summarize(args json.RawMessage) string {
	var a struct{ Package, Recipe string }

	json.Unmarshal(args, &a)

	return "Check what " + firstNonEmpty(a.Package, a.Recipe, "the work") + " needs"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}

	return ""
}

func (t CheckRequirements) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Package string `json:"package"`
		Recipe  string `json:"recipe"`
	}

	if err := argsOf(args, &a); err != nil {
		return "", err
	}

	book := t.Provisioning.Recipes()

	if a.Recipe != "" {
		return StatusLine(book.Check(a.Recipe, ""), false, ""), nil
	}

	p, ok := t.Provisioning.Packages().Get(a.Package)
	if !ok {
		return "", fmt.Errorf("there is no capability package called %q", a.Package)
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%s %s needs:\n", p.Title, p.Version)

	if len(p.Requires) == 0 {
		b.WriteString("  nothing beyond this program")
	}

	for _, r := range p.Requires {
		b.WriteString("  " + StatusLine(book.Check(r.ID, r.Versions), r.Optional, r.Why) + "\n")
	}

	return strings.TrimSpace(b.String()), nil
}

// StatusLine is one requirement in a sentence a person can act on.
func StatusLine(s provision.Status, optional bool, why string) string {
	line := s.Title

	if s.Wanted != "" {
		line += " " + s.Wanted
	}

	switch {
	case s.Ready():
		line += ": here"
		if s.Version != "" {
			line += " (" + s.Version + ")"
		}
	case s.Present:
		line += ": here, but " + s.Problem
	default:
		line += ": missing"
	}

	if why != "" {
		line += " — " + why
	}

	if optional {
		line += " (optional)"
	}

	if !s.Ready() {
		if s.Installable {
			line += fmt.Sprintf("; can be installed from here (%s, %s, %s)", orSize(s.Size), s.Source, s.Licence)
		} else if s.Manual != "" {
			line += "; not installed from here: " + s.Manual
		}
	}

	return line
}

func orSize(size string) string {
	if size == "" {
		return "size not known"
	}

	return size
}

// InstallRequirement installs one shipped recipe, with its owner's yes.
type InstallRequirement struct {
	Provisioning Provisioning
}

func (InstallRequirement) Name() string { return "install_requirement" }

func (InstallRequirement) Description() string {
	return "Install one thing a piece of work needs, from the recipes this program ships " +
		"(godot, godot-templates, node, git, pandoc…). Always asks the owner first, and " +
		"says the size, the source and the licence. Runs in the background."
}

func (InstallRequirement) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"recipe": {"type": "string", "description": "The recipe's id, from check_requirements."},
			"versions": {"type": "string", "description": "Optional version rule the result must meet, like >=4.2 <5."}
		},
		"required": ["recipe"],
		"additionalProperties": false
	}`)
}

func (InstallRequirement) Risk() Risk { return Mutating }

type installArgs struct {
	Recipe   string `json:"recipe"`
	Versions string `json:"versions"`
}

// Consent: putting software on the machine is always its owner's call.
func (InstallRequirement) Consent(json.RawMessage) string { return "installing software" }

func (t InstallRequirement) Weight(args json.RawMessage) risk.Level {
	var a installArgs

	argsOf(args, &a)

	if t.Provisioning != nil {
		if r, ok := t.Provisioning.Recipes().Get(a.Recipe); ok && r.NeedsRoot {
			return risk.High
		}
	}

	return risk.Medium
}

// Summarize is what would be installed, from where, how big, under what
// terms and where it would go — the things somebody is agreeing to.
func (t InstallRequirement) Summarize(args json.RawMessage) string {
	var a installArgs

	argsOf(args, &a)

	if t.Provisioning == nil {
		return "Install " + a.Recipe
	}

	r, ok := t.Provisioning.Recipes().Get(a.Recipe)
	if !ok {
		return "Install " + a.Recipe + " (no such recipe — this will fail)"
	}

	s := r.Check(a.Versions)

	parts := []string{"Install " + r.Title}

	for _, detail := range []string{s.Size, s.Source, "licence: " + s.Licence, "cost: " + s.Cost} {
		if strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(detail, "licence: "), "cost: ")) != "" {
			parts = append(parts, detail)
		}
	}

	if s.InstallTo != "" {
		parts = append(parts, "into "+s.InstallTo)
	}

	if r.NeedsRoot {
		parts = append(parts, "asks for your password")
	}

	return strings.Join(parts, " — ")
}

func (t InstallRequirement) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var a installArgs

	if err := argsOf(args, &a); err != nil {
		return "", err
	}

	if t.Provisioning == nil {
		return "", fmt.Errorf("installing is not available here")
	}

	r, ok := t.Provisioning.Recipes().Get(a.Recipe)
	if !ok {
		return "", fmt.Errorf("there is no recipe called %q; check_requirements names them", a.Recipe)
	}

	if s := r.Check(a.Versions); s.Ready() {
		return fmt.Sprintf("%s is already here (%s).", r.Title, s.Version), nil
	}

	if !r.Installable() {
		return "", fmt.Errorf("%s cannot be installed from here: %s", r.Title, r.Manual)
	}

	return t.Provisioning.StartInstall(a.Recipe, a.Versions)
}
