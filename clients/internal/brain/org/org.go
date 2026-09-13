/*
 * Package org is the shape of the organisation: what parts there are, what
 * seats are in them, and who answers to whom.
 *
 * Kept apart from the roster on purpose, because a seat and the person in it
 * are different things and conflating them is what stops an organisation
 * growing. A position exists whether or not anybody holds it — which is how
 * the program can say "there is a place for somebody who does SEO here, and
 * it is empty" rather than being silent about everything it has not got.
 *
 * Several agents may hold the same position, and one agent holds exactly one.
 * That asymmetry is deliberate: two backend engineers with different models
 * and different temperaments are a real thing to want, and an agent with two
 * jobs is usually a sign that the second job should have been its own seat.
 *
 * Files rather than rows, like the roster and the skills, and for the same
 * reason: this is something somebody maintains rather than something that
 * happened. One file per unit, with its seats in it, so the answer to "what
 * is in engineering" is one file rather than a query.
 */
package org

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FolderName is where a chart somebody has changed lives, inside the brain's
// own folder, so it travels with the brain and can be edited by hand.
const FolderName = "organisation"

var validName = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)

/*
 * What kind of part of the organisation a unit is.
 *
 * A closed set of names but not a closed set of shapes: nothing here requires
 * a company to contain divisions or a department to contain teams, and a unit
 * may sit under any other. The names are what somebody reads; the nesting is
 * whatever they build. A personal organisation that is one company and five
 * departments should not have to invent a division to be well formed.
 */
const (
	Company    = "company"
	Division   = "division"
	Department = "department"
	Team       = "team"
)

// How senior a seat is. Used for reading and for choosing between two people
// who could both do something, never for deciding what either may do — that
// is permissions, and it is kept separate on purpose.
const (
	Junior = "junior"
	Mid    = "mid"
	Senior = "senior"
	Lead   = "lead"
	Head   = "head"
	Chief  = "chief"
)

// Unit is one part of the organisation.
type Unit struct {
	// Name is the identifier, and the filename. Lower case, as everywhere.
	Name string `json:"name"`

	// Title is what a person reads. "Engineering", not "engineering".
	Title string `json:"title"`

	Kind string `json:"kind"`

	// Parent is the unit this one sits inside. Empty means the top.
	Parent string `json:"parent,omitempty"`

	// Purpose is what this part of the organisation is for, in a line or two.
	// It reaches a model when it is deciding who should do something, so it
	// should say what work belongs here rather than describe a box.
	Purpose string `json:"purpose,omitempty"`

	Seats []Position `json:"seats"`

	// BuiltIn marks one that shipped rather than being written by its owner.
	BuiltIn bool `json:"built_in,omitempty"`
}

/*
 * Position is a seat: a job, held with a certain amount of responsibility,
 * answerable to somebody.
 *
 * Deliberately thin. What a seat adds to its job is responsibility and
 * limits — everything about what the work *is* lives in the job definition,
 * which is shared by every seat that names it. A position that restated its
 * job would be a second copy of it, out of date by the second week.
 */
type Position struct {
	Name  string `json:"name"`
	Title string `json:"title"`

	// Job is an occupation id — software.backend_engineer. What the seat is
	// expected to know and be good at comes from there.
	Job string `json:"job"`

	Seniority string `json:"seniority,omitempty"`

	// ReportsTo is another position's name. Empty means it answers to the
	// owner, which on a personal organisation is most of them and is the
	// honest default: the person at the top of this chart is a person.
	ReportsTo string `json:"reports_to,omitempty"`

	// Needs is capabilities this seat wants beyond what its job implies —
	// the PostgreSQL on top of "backend engineer".
	Needs []string `json:"needs,omitempty"`

	/*
	 * Tools and Never are the seat's limits, and both only ever narrow.
	 *
	 * Tools, when set, replaces what the job's capabilities would have
	 * offered. Never is subtracted whatever else is decided, and survives
	 * every later widening — which is what makes it the right place to put
	 * "this seat does not send email", where a short Tools list would be
	 * undone by anybody adding a capability.
	 */
	Tools []string `json:"tools,omitempty"`
	Never []string `json:"never,omitempty"`
}

// Folder is where a changed chart lives.
func Folder(root string) string { return filepath.Join(root, FolderName) }

// Path is where one unit lives.
func Path(root, name string) string {
	return filepath.Join(Folder(root), name+".md")
}

/*
 * Chart is the organisation as it stands: what shipped, plus whatever its
 * owner has changed or added.
 *
 * A file naming an existing unit replaces it, so somebody who wants
 * engineering to have a different seat edits one file rather than rebuilding
 * the chart. A file with an unknown name adds one. The same rule the roster
 * uses, because somebody who has learned it once should not have to learn it
 * again.
 */
func Chart(root string) []Unit {
	byName := map[string]Unit{}
	order := []string{}

	for _, u := range BuiltIn() {
		byName[u.Name] = u
		order = append(order, u.Name)
	}

	for _, u := range fromFolder(root) {
		if _, known := byName[u.Name]; !known {
			order = append(order, u.Name)
		}

		u.BuiltIn = false
		byName[u.Name] = u
	}

	out := make([]Unit, 0, len(order))

	for _, name := range order {
		out = append(out, byName[name])
	}

	return out
}

/*
 * fromFolder reads units somebody wrote.
 *
 * A file that will not parse is skipped rather than failing the lot, for the
 * same reason skills and agents are: these are hand-editable by design, so one
 * of them being mid-edit is an ordinary Tuesday and not a reason for the
 * organisation to disappear.
 */
func fromFolder(root string) []Unit {
	entries, err := os.ReadDir(Folder(root))
	if err != nil {
		return nil
	}

	var out []Unit

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(Folder(root), entry.Name()))
		if err != nil {
			continue
		}

		unit := parse(string(raw), strings.TrimSuffix(entry.Name(), ".md"))

		if !validName.MatchString(unit.Name) {
			continue
		}

		out = append(out, unit)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

/*
 * Separator is what divides a unit from its seats in a file.
 *
 * Three dashes and a name, on its own line. The header-then-dashes shape is
 * the one the agents and the skills folders already use; this only adds that
 * the dashes may be followed by a name, and that there may be several.
 */
const Separator = "\n--- "

func parse(raw, filename string) Unit {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")

	parts := strings.Split("\n"+strings.TrimSpace(raw), Separator)

	unit := Unit{Name: filename, Kind: Department}

	head, purpose := fields(parts[0])

	for key, value := range head {
		switch key {
		case "name":
			if value != "" {
				unit.Name = value
			}
		case "title":
			unit.Title = value
		case "kind":
			unit.Kind = orElse(strings.ToLower(value), Department)
		case "parent":
			unit.Parent = strings.ToLower(value)
		case "purpose":
			unit.Purpose = value
		}
	}

	if purpose != "" {
		unit.Purpose = purpose
	}

	for _, part := range parts[1:] {
		name, rest, _ := strings.Cut(part, "\n")

		seat := Position{Name: strings.ToLower(strings.TrimSpace(name))}

		if !validName.MatchString(seat.Name) {
			continue
		}

		written, _ := fields(rest)

		for key, value := range written {
			switch key {
			case "title":
				seat.Title = value
			case "job":
				seat.Job = strings.ToLower(value)
			case "seniority":
				seat.Seniority = strings.ToLower(value)
			case "reports_to":
				seat.ReportsTo = strings.ToLower(value)
			case "needs":
				seat.Needs = splitList(value)
			case "tools":
				seat.Tools = splitList(value)
			case "never":
				seat.Never = splitList(value)
			}
		}

		seat.Title = orElse(seat.Title, readable(seat.Name))
		unit.Seats = append(unit.Seats, seat)
	}

	unit.Title = orElse(unit.Title, readable(unit.Name))

	return unit
}

/*
 * fields reads the key: value lines at the top of a block, and returns
 * whatever follows them as prose.
 *
 * It stops at the first line that is not a field rather than picking fields
 * out of the whole block, and that is what makes the prose safe to write: a
 * purpose that happens to contain a colon — "Being found: and understood
 * once found" — would otherwise be read as a setting called "Being found".
 * A key with a space in it is not a key.
 */
func fields(block string) (map[string]string, string) {
	out := map[string]string{}

	lines := strings.Split(block, "\n")

	at := 0

	for ; at < len(lines); at++ {
		line := strings.TrimSpace(lines[at])

		if line == "" {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		key = strings.ToLower(strings.TrimSpace(key))

		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			break
		}

		out[key] = strings.TrimSpace(value)
	}

	return out, strings.TrimSpace(strings.Join(lines[at:], "\n"))
}

func splitList(value string) []string {
	var out []string

	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}

	return out
}

func readable(name string) string {
	if name == "" {
		return ""
	}

	return strings.ToUpper(name[:1]) + strings.ReplaceAll(name[1:], "_", " ")
}

func orElse(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}

/*
 * Save writes a unit to the folder, so a change made in the program and a
 * change made in an editor are the same change.
 *
 * The whole unit is written, not a patch, because the file is the record: a
 * chart that lived half in a file and half in the code that shipped would be
 * a chart nobody could read the truth of.
 */
func Save(root string, unit Unit) error {
	if !validName.MatchString(unit.Name) {
		return fmt.Errorf("a unit's name must be lower-case letters, digits or underscores")
	}

	if unit.Name == unit.Parent {
		return fmt.Errorf("%s cannot be inside itself", unit.Title)
	}

	for _, seat := range unit.Seats {
		if !validName.MatchString(seat.Name) {
			return fmt.Errorf("%q is not a name for a position", seat.Name)
		}

		if strings.TrimSpace(seat.Job) == "" {
			return fmt.Errorf("%s has no job, so nobody could be found for it", seat.Title)
		}
	}

	if err := os.MkdirAll(Folder(root), 0o700); err != nil {
		return fmt.Errorf("making the organisation folder: %w", err)
	}

	var b strings.Builder

	b.WriteString("name: " + unit.Name + "\n")
	b.WriteString("title: " + unit.Title + "\n")
	b.WriteString("kind: " + orElse(unit.Kind, Department) + "\n")

	if unit.Parent != "" {
		b.WriteString("parent: " + unit.Parent + "\n")
	}

	if unit.Purpose != "" {
		b.WriteString("\n" + strings.TrimSpace(unit.Purpose) + "\n")
	}

	for _, seat := range unit.Seats {
		b.WriteString("\n--- " + seat.Name + "\n")
		b.WriteString("title: " + orElse(seat.Title, readable(seat.Name)) + "\n")
		b.WriteString("job: " + seat.Job + "\n")

		for _, line := range [][2]string{
			{"seniority", seat.Seniority},
			{"reports_to", seat.ReportsTo},
			{"needs", strings.Join(seat.Needs, ", ")},
			{"tools", strings.Join(seat.Tools, ", ")},
			{"never", strings.Join(seat.Never, ", ")},
		} {
			if line[1] != "" {
				b.WriteString(line[0] + ": " + line[1] + "\n")
			}
		}
	}

	// Written whole and moved into place, like everything else in this folder.
	path := Path(root, unit.Name)

	if err := os.WriteFile(path+".new", []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing the unit: %w", err)
	}

	return os.Rename(path+".new", path)
}

/*
 * Reset puts a built-in unit back to what it shipped as, by deleting the file
 * rather than rewriting it — so the answer to "what is engineering now" is
 * always one of two things, and never a third that merely resembles the first.
 */
func Reset(root, name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("that is not a unit's name")
	}

	err := os.Remove(Path(root, name))

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// Find returns the named unit, or false.
func Find(chart []Unit, name string) (Unit, bool) {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, u := range chart {
		if u.Name == name {
			return u, true
		}
	}

	return Unit{}, false
}

// Seat finds one position anywhere in the chart, and the unit it sits in.
// Positions are named across the whole organisation rather than within a
// unit, because reports_to has to be able to point across one.
func Seat(chart []Unit, name string) (Unit, Position, bool) {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, u := range chart {
		for _, s := range u.Seats {
			if s.Name == name {
				return u, s, true
			}
		}
	}

	return Unit{}, Position{}, false
}

// Seats is every position in the organisation, in reading order.
func Seats(chart []Unit) []Position {
	out := []Position{}

	for _, u := range Shape(chart) {
		out = append(out, u.Seats...)
	}

	return out
}
