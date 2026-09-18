package bootstrap

import (
	"fmt"
	"os"
	"strings"
)

func homeDir() string {
	home, _ := os.UserHomeDir()

	return home
}

// Text is the proposal as a person reads it before saying yes.
func (p Proposal) Text() string {
	var b strings.Builder

	if p.Question != "" {
		b.WriteString(p.Question)

		for _, o := range p.Options {
			fmt.Fprintf(&b, "\n  - %s: %s", o.Title, o.Says)
		}

		return b.String()
	}

	if p.New {
		fmt.Fprintf(&b, "Proposed: a new project, %s, in %s\n", p.Name, p.Dir)
	} else {
		fmt.Fprintf(&b, "Proposed: work on the existing project in %s\n", p.Dir)
	}

	if p.Package != nil {
		fmt.Fprintf(&b, "Under: %s %s — %s (%s work)\n", p.Package.ID, p.Package.Version, p.Package.Title, p.Package.Work)
	}

	if p.Engine != "" {
		fmt.Fprintf(&b, "Engine: %s %s\n", p.Engine, p.EngineVersion)
	}

	section := func(title string, lines []string) {
		if len(lines) == 0 {
			return
		}

		b.WriteString("\n" + title + ":\n")

		for _, line := range lines {
			b.WriteString("  - " + line + "\n")
		}
	}

	section("What is there now", reportLines(p))

	var files []string

	for _, f := range p.Files {
		files = append(files, f)
	}

	if len(files) > 0 {
		files = append(files, ".pn-assistant/project.json")
	} else if p.Report.Config == nil {
		files = append(files, ".pn-assistant/project.json (only this: the project itself is left as it is)")
	}

	section("Files it would create (nothing existing is overwritten)", files)

	var who []string

	switch {
	case p.Hire != nil:
		who = append(who, fmt.Sprintf("%s (%s), hired for this project only and let go when it ends", p.Hire.Agent.Title, p.Hire.Agent.Name))

		if len(p.Hire.Tools) > 0 {
			who = append(who, "may use: "+strings.Join(p.Hire.Tools, ", "))
		}
	case p.Agent != "":
		who = append(who, p.Agent+", who already works this way")
	}

	section("Who does it", who)
	section("Chosen because", p.Chosen)
	section("How it will be done", p.Stack)

	if p.Minutes > 0 {
		section("Time it is given", []string{fmt.Sprintf("%d minutes — %s", p.Minutes, p.MinutesWhy)})
	}

	scope := []string{"write only inside " + p.Dir}

	if len(p.Config.Outputs) > 0 {
		scope = append(scope, "builds go only to "+strings.Join(p.Config.Outputs, ", "))
	}

	section("Confined to", scope)

	var needs []string

	for _, n := range p.Requires {
		s := n.Status
		line := s.Title

		switch {
		case s.Ready():
			line += " " + s.Version + " — here"
		case s.Present && s.Compatible && s.LicenceState != "":
			line += " " + s.Version + " — " + s.Problem
		case s.Installable && !n.Optional:
			line += fmt.Sprintf(" — will be installed first: %s, %s, licence %s, cost %s", orSize(s.Size), s.Source, s.Licence, s.Cost)
		case n.Optional:
			line += " — not here (optional: " + n.Why + ")"
		default:
			line += " — not here, and not installable from here: " + s.Manual
		}

		needs = append(needs, line)
	}

	section("Needs on this machine", needs)

	var steps []string

	for i, s := range p.Steps {
		steps = append(steps, fmt.Sprintf("%d. %s", i+1, s.Instruction))
	}

	section("Plan", steps)
	section("Finished when there is", p.Done)
	section("Must not", p.Limits)
	section("Needs a qualified professional", p.Professional)
	section("Cannot start until", p.Blockers)

	return strings.TrimSpace(b.String())
}

func reportLines(p Proposal) []string {
	r := p.Report

	var out []string

	switch {
	case !r.Exists:
		out = append(out, "the folder does not exist yet and will be created")
	case r.Empty:
		out = append(out, "the folder is empty")
	case r.Project != nil:
		out = append(out, fmt.Sprintf("a %s project, %s", r.Engine, r.Project.Name))
	default:
		out = append(out, fmt.Sprintf("%d things are in it", r.Entries))
	}

	if r.Git {
		out = append(out, fmt.Sprintf("git branch %s, %d uncommitted changes", r.Branch, r.Dirty))
	}

	return append(out, r.Risks...)
}
