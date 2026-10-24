package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

/*
 * Goal mode: one consent, then everything on its own except four things.
 *
 * Owner 2026-10-23/24: "всичко трябва да е автоматично" — a goal is planned,
 * staffed, provisioned (models, user-level tools from trusted sources with
 * checksums), worked in its own folder, checked, fixed and reported without
 * questions. What stays a question every time, whatever else is allowed, is
 * what reaches outside or cannot be taken back:
 *
 *   1. a payment or a paid service;
 *   2. sending or publishing anything in the owner's name;
 *   3. deleting files outside the goal's folder;
 *   4. a change to the whole system (administrator rights, services, disks).
 *
 * The four sentences are also what the setup screen shows, so what was
 * agreed to and what the program enforces are the same words.
 */

// The four questions goal mode still asks.
const (
	AskPaying     = "a payment or a paid service"
	AskSending    = "sending or publishing in your name"
	AskDeleting   = "deleting files outside the goal's folder"
	AskSystemWide = "a change to the whole system"
)

// GoalModeConsent is the one sentence agreed to in setup.
func GoalModeConsent(bulgarian bool) string {
	if bulgarian {
		return "Работя по целите ви сам — планирам, избирам специалисти и модели, изтеглям нужните модели и " +
			"инструменти от доверени източници с проверка, работя в отделна папка за всяка цел и ви давам отчет. " +
			"Питам ви само преди плащане или платена услуга, изпращане или публикуване от ваше име, изтриване " +
			"на файлове извън папката на целта и промяна на цялата система."
	}

	return "I work on your goals on my own — I plan, choose specialists and models, download the models and " +
		"tools I need from trusted sources with checks, work in a separate folder for each goal and report " +
		"to you. I ask you only before a payment or a paid service, sending or publishing in your name, " +
		"deleting files outside the goal's folder, and changing the whole system."
}

// Spending is a tool that can cost money; Spends says how, or "".
type Spending interface {
	Spends(args json.RawMessage) string
}

// SystemWide is a tool whose call may change the whole system.
type SystemWide interface {
	SystemWide(args json.RawMessage) bool
}

/*
 * GoalModeAsks is which of the four a call is, or "" when goal mode does it
 * without asking. inside says whether a path is in the goal's folder.
 */
func GoalModeAsks(t Tool, args json.RawMessage, inside func(path string) bool) string {
	if s, ok := t.(Spending); ok {
		if why := s.Spends(args); why != "" {
			return AskPaying + " (" + why + ")"
		}
	}

	if s, ok := t.(SystemWide); ok && s.SystemWide(args) {
		return AskSystemWide
	}

	switch t.Name() {
	case "run_as_admin":
		return AskSystemWide
	case "remove_a_part":
		// Software on the machine is not the goal's to take away.
		return AskDeleting
	}

	if why := ConsentFor(t, args); strings.Contains(why, "in your name") || strings.Contains(why, "publish") {
		return AskSending
	}

	if _, ok := t.(RunCommand); ok {
		var a commandArgs

		_ = json.Unmarshal(args, &a)

		return commandAsks(a, inside)
	}

	return ""
}

func commandAsks(a commandArgs, inside func(string) bool) string {
	why := Destructive(a.Argv)
	if why == "" {
		return ""
	}

	switch {
	case strings.Contains(why, "administrator"), strings.Contains(why, "service"), strings.Contains(why, "machine off"),
		strings.Contains(why, "formats"), strings.Contains(why, "overwrites data"), strings.Contains(why, "stops programs"):
		return AskSystemWide
	}

	// Deleting, discarding or rewriting: only inside the goal's folder.
	if inside == nil {
		return AskDeleting
	}

	dir := a.Dir
	if dir == "" || !inside(dir) {
		return AskDeleting
	}

	for _, arg := range a.Argv[1:] {
		if strings.HasPrefix(arg, "-") || !looksLikePath(arg) {
			continue
		}

		p := arg
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}

		if !inside(filepath.Clean(p)) {
			return AskDeleting
		}
	}

	return ""
}

// looksLikePath leaves out git words and the like ("reset", "HEAD").
func looksLikePath(arg string) bool {
	if strings.ContainsAny(arg, "/\\.~*") {
		return true
	}

	switch arg {
	case "reset", "clean", "push", "branch", "stash", "drop", "clear", "checkout", "restore", "HEAD", "origin":
		return false
	}

	return true
}

// SystemWide: a recipe that installs with administrator rights.
func (t InstallRequirement) SystemWide(args json.RawMessage) bool {
	var a installArgs

	if json.Unmarshal(args, &a) != nil || t.Provisioning == nil {
		return true
	}

	r, ok := t.Provisioning.Recipes().Get(a.Recipe)

	return !ok || r.NeedsRoot
}

// SystemWide: one of the machine's own packages rather than this program's.
func (t InstallPart) SystemWide(args json.RawMessage) bool {
	var a struct {
		Name string `json:"name"`
	}

	if json.Unmarshal(args, &a) != nil || t.Parts == nil {
		return true
	}

	for _, p := range t.Parts() {
		if strings.EqualFold(p.Name, a.Name) {
			return !p.Removable
		}
	}

	return true
}
