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
func (p Proposal) Text() string { return p.TextIn("en") }

// say is the sentence in the language asked for: Bulgarian for "bg".
func say(lang, en, bg string) string {
	if lang == "bg" {
		return bg
	}

	return en
}

// TextIn is the proposal in a language ("en" or "bg"), for the approval
// card. Only the fixed words are translated: names, folders, the model's
// lines and what the machine reported stay as they are.
func (p Proposal) TextIn(lang string) string {
	var b strings.Builder

	if p.Question != "" {
		b.WriteString(p.Question)

		for _, o := range p.Options {
			fmt.Fprintf(&b, "\n  - %s: %s", o.Title, o.Says)
		}

		return b.String()
	}

	if p.New {
		fmt.Fprintf(&b, say(lang, "Proposed: a new project, %s, in %s\n", "Предложение: нов проект, %s, в %s\n"), p.Name, p.Dir)
	} else {
		fmt.Fprintf(&b, say(lang, "Proposed: work on the existing project in %s\n",
			"Предложение: работа по съществуващия проект в %s\n"), p.Dir)
	}

	if p.Package != nil {
		fmt.Fprintf(&b, say(lang, "Under: %s %s, %s (%s work)\n", "По пакет: %s %s, %s (работа: %s)\n"),
			p.Package.ID, p.Package.Version, p.Package.Title, p.Package.Work)
	}

	if p.Engine != "" {
		fmt.Fprintf(&b, say(lang, "Engine: %s %s\n", "Енджин: %s %s\n"), p.Engine, p.EngineVersion)
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

	section(say(lang, "What is there now", "Какво има сега"), reportLinesIn(p, lang))

	var files []string

	for _, f := range p.Files {
		files = append(files, f)
	}

	if len(files) > 0 {
		files = append(files, ".pn-assistant/project.json")
	} else if p.Report.Config == nil {
		files = append(files, ".pn-assistant/project.json"+say(lang, " (only this: the project itself is left as it is)",
			" (само този: самият проект остава непроменен)"))
	}

	section(say(lang, "Files it would create (nothing existing is overwritten)",
		"Файлове, които ще създаде (нищо съществуващо не се презаписва)"), files)

	var who []string

	switch {
	case p.Hire != nil:
		who = append(who, fmt.Sprintf(say(lang, "%s (%s), hired for this project only and let go when it ends",
			"%s (%s), нает само за този проект и освободен, когато той приключи"), p.Hire.Agent.Title, p.Hire.Agent.Name))

		if len(p.Hire.Tools) > 0 {
			who = append(who, say(lang, "may use: ", "може да използва: ")+strings.Join(p.Hire.Tools, ", "))
		}
	case p.Agent != "":
		who = append(who, p.Agent+say(lang, ", who already works this way", ", който вече работи така"))
	}

	section(say(lang, "Who does it", "Кой го прави"), who)
	section(say(lang, "Chosen because", "Избрано, защото"), p.Chosen)
	section(say(lang, "How it will be done", "Как ще се направи"), p.Stack)

	if p.Minutes > 0 {
		section(say(lang, "Time it is given", "Отделено време"),
			[]string{fmt.Sprintf(say(lang, "%d minutes: %s", "%d мин.: %s"), p.Minutes, p.MinutesWhy)})
	}

	scope := []string{say(lang, "write only inside ", "пише само в ") + p.Dir}

	if len(p.Config.Outputs) > 0 {
		scope = append(scope, say(lang, "builds go only to ", "компилациите отиват само в ")+strings.Join(p.Config.Outputs, ", "))
	}

	section(say(lang, "Confined to", "Ограничено до"), scope)

	var needs []string

	for _, n := range p.Requires {
		s := n.Status
		line := s.Title

		switch {
		case s.Ready():
			line += " " + s.Version + say(lang, ": here", ": има го")
		case s.Present && s.Compatible && s.LicenceState != "":
			line += " " + s.Version + ": " + s.Problem
		case s.Blocked:
			line += ": " + s.Problem
		case s.Installable && !n.Optional:
			line += fmt.Sprintf(say(lang, ": will be installed first (%s, %s, licence %s, cost %s)",
				": ще бъде инсталирано първо (%s, %s, лиценз %s, цена %s)"), orSizeIn(s.Size, lang), s.Source, s.Licence, s.Cost)
		case n.Optional:
			line += say(lang, ": not here (optional: ", ": липсва (по избор: ") + n.Why + ")"
		default:
			line += say(lang, ": not here, and not installable from here. ", ": липсва и не може да се инсталира оттук. ") + s.Manual
		}

		needs = append(needs, line)
	}

	section(say(lang, "Needs on this machine", "Нужно на този компютър"), needs)

	var steps []string

	for i, s := range p.Steps {
		steps = append(steps, fmt.Sprintf("%d. %s", i+1, s.Instruction))
	}

	section(say(lang, "Plan", "План"), steps)
	section(say(lang, "Finished when there is", "Готово е, когато има"), p.Done)
	section(say(lang, "Must not", "Не бива"), p.Limits)
	section(say(lang, "Needs a qualified professional", "Нужен е квалифициран специалист"), p.Professional)
	section(say(lang, "Cannot start until", "Не може да започне, докато"), p.Blockers)

	return strings.TrimSpace(b.String())
}

func reportLinesIn(p Proposal, lang string) []string {
	r := p.Report

	var out []string

	switch {
	case !r.Exists:
		out = append(out, say(lang, "the folder does not exist yet and will be created", "папката още не съществува и ще бъде създадена"))
	case r.Empty:
		out = append(out, say(lang, "the folder is empty", "папката е празна"))
	case r.Project != nil:
		out = append(out, fmt.Sprintf(say(lang, "a %s project, %s", "проект на %s, %s"), r.Engine, r.Project.Name))
	default:
		out = append(out, fmt.Sprintf(say(lang, "%d things are in it", "елементи в нея: %d"), r.Entries))
	}

	if r.Git {
		out = append(out, fmt.Sprintf(say(lang, "git branch %s, %d uncommitted changes", "клон в git %s, некъмитнати промени: %d"), r.Branch, r.Dirty))
	}

	return append(out, r.Risks...)
}
