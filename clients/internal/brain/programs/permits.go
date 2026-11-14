package programs

import (
	"context"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

/*
 * Permits: what the assistant may do with each program, decided by the
 * person whose computer it is (design "every program", section 3 point 1,
 * and the removal rules of 2.9).
 *
 * A permit is one program and one kind of use (a verb) and one answer: ask
 * each time, always, or never. Nothing here asks or runs anything; the loop
 * that is about to act asks Rule, and Rule says what the person decided,
 * or that nothing was decided here and the general setting answers.
 *
 * Removal is the first case Rule looks at: a program that is gone is never
 * used, whatever was allowed, and the answer says since when. Its permits
 * are kept, dormant, for when it comes back.
 */

// Verb is a kind of use of a program.
type Verb string

const (
	// VerbStart is opening the program, as from the menu.
	VerbStart Verb = "start"
	// VerbInput is typing and clicking in its windows.
	VerbInput Verb = "control-input"
	// VerbChange is running it as a command, with arguments, which can change
	// anything the program can.
	VerbChange Verb = "change"

	// VerbRead, VerbExport, VerbSend and VerbUninstall are the design's other uses; no tool
	// of this version does them yet (recipes, 1.3.0; uninstalling, 1.6.0).
	VerbRead      Verb = "read"
	VerbExport    Verb = "export"
	VerbSend      Verb = "send"
	VerbUninstall Verb = "uninstall"
)

// InUse is every verb a tool of this version uses, in the order a card
// shows them. A verb no tool uses would be a switch that changes nothing.
var InUse = []Verb{VerbStart, VerbInput, VerbChange}

// Verbs is every verb a permit may name.
var Verbs = []Verb{VerbStart, VerbInput, VerbChange, VerbRead, VerbExport, VerbSend, VerbUninstall}

// KnownVerb is whether v is a verb at all.
func KnownVerb(v Verb) bool {
	for _, k := range Verbs {
		if k == v {
			return true
		}
	}

	return false
}

// Answer is what the person decided for one program and one verb.
type Answer string

const (
	// AskEach is the default: no standing decision, so asking (or the
	// general setting) decides each time. It is never stored.
	AskEach Answer = "ask"
	// Always runs it without asking, for this program and this verb.
	Always Answer = "allow"
	// Never refuses it without asking, whatever else is allowed.
	Never Answer = "never"
)

// KnownAnswer is whether a is an answer at all.
func KnownAnswer(a Answer) bool { return a == AskEach || a == Always || a == Never }

// Permit is one standing decision.
type Permit struct {
	Verb   Verb   `json:"verb"`
	Answer Answer `json:"answer"`

	// Version is the program's version when "always" was given. A newer
	// version can do things the old one could not, so "always" sleeps
	// after an update until it is given again (Rule).
	Version string    `json:"version,omitempty"`
	At      time.Time `json:"at"`
}

// Asleep is whether an "always" was given to another version than v: kept,
// shown, and not acted on.
func (p Permit) Asleep(version string) bool {
	return p.Answer == Always && p.Verb != VerbRead && p.Version != version
}

/*
 * Use is what a tool is about to do with a program, and every clue it has
 * about which program that is. Match turns the clues into records.
 */
type Use struct {
	Verb Verb `json:"verb"`

	// Desktop is a menu entry's desktop id (open_app found it).
	Desktop string `json:"desktop,omitempty"`
	// Path is the program file that runs, absolute.
	Path string `json:"path,omitempty"`
	// Flatpak and Snap are application ids a process or a command named.
	Flatpak string `json:"flatpak,omitempty"`
	Snap    string `json:"snap,omitempty"`
	// Package is the system package that owns Path, when it was asked.
	Package string `json:"package,omitempty"`
	// Name is what the person or the model called it, tried only when no
	// other clue matched (so a removed program can still be named).
	Name string `json:"name,omitempty"`
}

// Empty is a use with no clue at all.
func (u Use) Empty() bool {
	return u.Desktop == "" && u.Path == "" && u.Flatpak == "" && u.Snap == "" && u.Package == "" && u.Name == ""
}

/*
 * Match is every remembered record the use is about, present or removed,
 * at most one per source and id, in the order the records came. One program
 * often has several records (Firefox: a menu entry and a snap), and each
 * has its own card; Rule weighs them together.
 */
func Match(records []Record, u Use) []Record {
	var out []Record

	seen := map[string]bool{}
	add := func(r Record) {
		if !seen[r.Key()] {
			seen[r.Key()] = true
			out = append(out, r)
		}
	}

	desktop := strings.TrimSuffix(u.Desktop, ".desktop")
	snap, flatpak := u.Snap, u.Flatpak

	if u.Path != "" {
		if s := snapOfPath(u.Path); s != "" && snap == "" {
			snap = s
		}

		if f := flatpakOfPath(u.Path); f != "" && flatpak == "" {
			flatpak = f
		}
	}

	for _, r := range records {
		switch r.Kind {
		case App:
			id := strings.TrimSuffix(r.ID, ".desktop")

			switch {
			case desktop != "" && strings.EqualFold(id, desktop):
				add(r)
			case u.Path != "" && r.Exec != "" && sameProgram(r.Exec, u.Path):
				add(r)
			case flatpak != "" && strings.EqualFold(id, flatpak):
				add(r)
			}
		case Snap:
			if snap != "" && r.ID == snap {
				add(r)
			}
		case Flatpak:
			if flatpak != "" && r.ID == flatpak {
				add(r)
			}
		case Deb:
			if u.Package != "" && r.ID == u.Package {
				add(r)
			}
		case Win, Msix:
			if u.Path != "" && r.Path != "" && within(u.Path, r.Path) {
				add(r)
			}
		}
	}

	// A menu entry found leads to what it starts (a snap's entry runs
	// /snap/bin/<name>), so a decision on either card counts.
	for _, r := range append([]Record(nil), out...) {
		if r.Kind != App || r.Exec == "" {
			continue
		}

		if s := snapOfPath(r.Exec); s != "" {
			for _, o := range records {
				if o.Kind == Snap && o.ID == s {
					add(o)
				}
			}
		}
	}

	if len(out) > 0 || u.Name == "" {
		return out
	}

	name := strings.ToLower(strings.TrimSpace(u.Name))

	for _, r := range records {
		id := strings.ToLower(strings.TrimSuffix(r.ID, ".desktop"))

		if id == name || strings.ToLower(r.Name) == name || strings.ToLower(filepath.Base(r.Exec)) == name {
			add(r)
		}
	}

	return out
}

// sameProgram is whether a menu entry's Exec (absolute or a bare name)
// names the program file path.
func sameProgram(exec, path string) bool {
	if filepath.IsAbs(exec) || strings.ContainsAny(exec, `/\`) {
		return samePath(exec, path)
	}

	base := filepath.Base(path)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(strings.TrimSuffix(strings.ToLower(base), ".exe"), strings.TrimSuffix(strings.ToLower(exec), ".exe"))
	}

	return base == exec
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" || strings.Contains(a, `\`) {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}

	return filepath.Clean(a) == filepath.Clean(b)
}

// within is whether path is dir or inside it (Windows: by letters, any
// case; the install folder of an uninstall entry).
func within(path, dir string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
	d := strings.TrimRight(strings.ToLower(strings.ReplaceAll(dir, `\`, "/")), "/")

	if d == "" || len(d) < 3 {
		return false
	}

	return p == d || strings.HasPrefix(p, d+"/")
}

// snapOfPath is the snap a program file belongs to: /snap/bin/<name>
// (or <name>.<app>) and /snap/<name>/<revision>/...
func snapOfPath(path string) string {
	p := filepath.ToSlash(filepath.Clean(path))

	if rest, ok := strings.CutPrefix(p, "/snap/bin/"); ok && rest != "" && !strings.Contains(rest, "/") {
		name, _, _ := strings.Cut(rest, ".")
		return name
	}

	if rest, ok := strings.CutPrefix(p, "/snap/"); ok {
		name, more, _ := strings.Cut(rest, "/")
		if name != "" && name != "bin" && more != "" {
			return name
		}
	}

	return ""
}

// flatpakOfPath is the Flatpak application a program file belongs to:
// .../exports/bin/<id> or .../app/<id>/<arch>/...
func flatpakOfPath(path string) string {
	p := filepath.ToSlash(filepath.Clean(path))

	if i := strings.Index(p, "/flatpak/exports/bin/"); i >= 0 {
		id := p[i+len("/flatpak/exports/bin/"):]
		if id != "" && !strings.Contains(id, "/") {
			return id
		}
	}

	if i := strings.Index(p, "/flatpak/app/"); i >= 0 {
		id, more, _ := strings.Cut(p[i+len("/flatpak/app/"):], "/")
		if id != "" && more != "" {
			return id
		}
	}

	return ""
}

// Why is the reason of a Ruling, as a code the page and the tests know.
type Why string

const (
	// WhyUndecided: no record, or no standing decision: the general setting
	// answers.
	WhyUndecided Why = ""
	// WhyGone: every matching record is removed.
	WhyGone Why = "removed"
	// WhyRefused: a "never" on one of the records.
	WhyRefused Why = "never"
	// WhyAllowed: an "always" for the version that is here.
	WhyAllowed Why = "allowed"
	// WhyUpdated: an "always" given to an older version, asleep.
	WhyUpdated Why = "updated"
	// WhyStopped: its person stopped every use of other programs.
	WhyStopped Why = "stopped"
)

/*
 * Ruling is what the person's decisions say about one use. Answer is
 * AskEach for "ask" and for "nothing decided"; Why tells them apart.
 */
type Ruling struct {
	Answer  Answer
	Why     Why
	Program *Record

	// Said is one English sentence for the model and the approval card
	// (the loop's other refusals are English too), empty when undecided.
	Said string
}

/*
 * Rule weighs a use's records (Match) and their permits, keyed by RowID.
 *
 * In this order: all removed = refused with the time, never used, whatever
 * else (2.9); a "never" on any present record = refused; an "always" for
 * the version that is here on any present record = allowed; an "always"
 * for another version = asked again (asleep until given again); nothing =
 * undecided. A removed record's permits are dormant: they count only when
 * it is back.
 */
func Rule(matches []Record, permits map[int64][]Permit, verb Verb) Ruling {
	if len(matches) == 0 {
		return Ruling{Answer: AskEach}
	}

	var present []Record

	for _, r := range matches {
		if r.Present {
			present = append(present, r)
		}
	}

	if len(present) == 0 {
		// The most recently removed one says when.
		gone := matches[0]

		for _, r := range matches[1:] {
			if removedAt(r).After(removedAt(gone)) {
				gone = r
			}
		}

		return Ruling{Answer: Never, Why: WhyGone, Program: &gone, Said: GoneSentence(gone)}
	}

	var allowed, asleep *Record

	for i := range present {
		r := present[i]

		for _, p := range permits[r.RowID] {
			if p.Verb != verb {
				continue
			}

			switch {
			case p.Answer == Never:
				return Ruling{Answer: Never, Why: WhyRefused, Program: &r,
					Said: "Refused: the owner has set " + nameOf(r) + " to never " + VerbWords(verb) +
						". Do not try it again; say so and offer something else. They can change it on its card on the Programs page."}
			case p.Answer == Always && p.Asleep(r.Version):
				if asleep == nil {
					asleep = &r
				}
			case p.Answer == Always:
				if allowed == nil {
					allowed = &r
				}
			}
		}
	}

	if allowed != nil {
		return Ruling{Answer: Always, Why: WhyAllowed, Program: allowed,
			Said: "Allowed always for " + nameOf(*allowed) + " (" + VerbWords(verb) + ")."}
	}

	if asleep != nil {
		return Ruling{Answer: AskEach, Why: WhyUpdated, Program: asleep,
			Said: nameOf(*asleep) + " was updated since it was allowed to " + VerbWords(verb) + ", so this is asked again."}
	}

	first := present[0]

	return Ruling{Answer: AskEach, Program: &first}
}

func removedAt(r Record) time.Time {
	if r.RemovedAt != nil {
		return *r.RemovedAt
	}

	return r.ChangedAt
}

func nameOf(r Record) string {
	if r.Name != "" {
		return r.Name
	}

	return r.ID
}

// GoneSentence is what the model is told about a removed program, with
// the day and time it went (local time).
func GoneSentence(r Record) string {
	at := removedAt(r).Local().Format("2 Jan 2006 at 15:04")

	return "Refused: " + nameOf(r) + " was removed from this computer on " + at +
		", so it cannot be used. Say so; it will be used again if it comes back."
}

/*
 * StoppedSentence is what the model is told when its person has stopped
 * every use of other programs ("Stop using other programs" on the Programs
 * page): since when, and that only they can start it again.
 */
func StoppedSentence(since time.Time) string {
	return "Refused: the owner stopped all use of other programs on " + since.Local().Format("2 Jan 2006 at 15:04") +
		", so nothing is opened, typed, clicked, scrolled or run in another program. Do not try it again; say so. " +
		"They can turn it back on with \"Use other programs again\" on the Programs page."
}

/*
 * Choices is everything that decides how a program may be used, for
 * saying it (list_programs): the standing decisions by program row, how
 * the general setting answers what is not decided, and the switch that
 * stops all use of other programs.
 */
type Choices struct {
	Permits map[int64][]Permit

	// Honoured: a standing "always" is acted on (not on "ask every time").
	Honoured bool
	// Unasked: what is not decided runs without asking (everything is
	// allowed); typing and clicking in another program is asked anyway.
	Unasked bool

	// Stopped since Since: nothing uses another program.
	Stopped bool
	Since   time.Time
}

// VerbWords is a verb as the sentences above say it.
func VerbWords(v Verb) string {
	switch v {
	case VerbStart:
		return "start it"
	case VerbInput:
		return "type and click in its windows"
	case VerbChange:
		return "run it as a command"
	case VerbRead:
		return "read from it"
	case VerbExport:
		return "export from it"
	case VerbSend:
		return "send with it"
	case VerbUninstall:
		return "uninstall it"
	}

	return string(v)
}

// SortPermits orders one program's permits as a card shows them.
func SortPermits(ps []Permit) {
	order := map[Verb]int{}
	for i, v := range Verbs {
		order[v] = i
	}

	sort.SliceStable(ps, func(i, j int) bool { return order[ps[i].Verb] < order[ps[j].Verb] })
}

/*
 * Weigh is Match and Rule together, with what the use itself shows: a menu
 * entry or a program file that is there right now means the program is
 * here, whatever an older record says (a program reinstalled from another
 * source, or a change the inventory has not read yet, about two seconds).
 * Only a program known by name alone, with nothing of it left on the
 * computer, is refused as removed.
 */
func Weigh(records []Record, permits map[int64][]Permit, u Use) Ruling {
	if u.Empty() {
		return Ruling{Answer: AskEach}
	}

	r := Rule(Match(records, u), permits, u.Verb)

	if r.Why == WhyGone && (u.Desktop != "" || u.Path != "") {
		return Ruling{Answer: AskEach}
	}

	return r
}

// NeedsPackage is whether a use's package is worth asking the system
// for: only when a package has a standing decision, since asking is a
// process.
func NeedsPackage(records []Record, permits map[int64][]Permit, u Use) bool {
	if u.Path == "" || u.Package != "" {
		return false
	}

	for _, r := range records {
		if r.Kind == Deb && len(permits[r.RowID]) > 0 {
			return true
		}
	}

	return false
}

/*
 * PackageOf is the system package that owns a program file, asked of
 * dpkg-query -S (dpkg's interface; its database is dpkg's own business);
 * empty when there is none or no dpkg.
 */
func (s Scanner) PackageOf(ctx context.Context, path string) string {
	// dpkg's paths are Unix paths, whatever system reads this.
	if path == "" || !pathpkg.IsAbs(path) {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	for _, p := range []string{path, resolved(path)} {
		if p == "" {
			continue
		}

		out, err := s.run(ctx, "dpkg-query", "-S", p)
		if err != nil {
			continue
		}

		if pkg := ownerOf(string(out), p); pkg != "" {
			return pkg
		}
	}

	return ""
}

func resolved(path string) string {
	r, err := filepath.EvalSymlinks(path)
	if err != nil || r == path {
		return ""
	}

	return r
}

// ownerOf reads `dpkg-query -S` ("pkg[:arch][, pkg2]: /path"), for the
// line that is exactly the path (diversions say "diversion by").
func ownerOf(out, path string) string {
	for _, line := range strings.Split(out, "\n") {
		pkgs, file, ok := strings.Cut(line, ": ")
		if !ok || strings.TrimSpace(file) != path || strings.HasPrefix(pkgs, "diversion by") {
			continue
		}

		first, _, _ := strings.Cut(pkgs, ",")
		name, _, _ := strings.Cut(strings.TrimSpace(first), ":")

		return name
	}

	return ""
}
