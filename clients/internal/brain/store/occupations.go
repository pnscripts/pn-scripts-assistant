package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

/*
 * What work there is, as against who does it.
 *
 * An occupation is a definition — what a backend engineer is for, what one is
 * expected to know — and it exists whether or not anybody here holds it. An
 * agent is somebody who holds one. Keeping them apart is what allows two
 * backend engineers who are not the same person, and a specialist that can be
 * described before it is hired.
 *
 * A capability is something somebody is good at. It is deliberately neither a
 * job nor a tool. Not a job, because "API design" belongs to a backend
 * engineer and to a technical product manager both, and copying it into each
 * of them is how a list of skills turns into a list of spellings of skills.
 * Not a tool, because most capabilities map to no tool at all — judgement,
 * domain knowledge and taste are things somebody has, not things this program
 * can do.
 *
 * Matching is done in Go rather than in SQL, which looks like the lazy choice
 * and is not. SQLite's LIKE and LOWER are ASCII-only, so "Заварчик" and
 * "заварчик" are different words to them — and this assistant is spoken to in
 * two languages. Go's ToLower knows better, and at a few thousand rows the
 * whole table is cheaper to read than an index would be to maintain. The same
 * reasoning the memory map already uses for vectors.
 */

// Where a row came from. Also which of two rows describing the same thing
// wins: what somebody wrote by hand beats what shipped, and both beat
// anything imported.
const (
	FromHand = "hand"
	FromSeed = "seed"
	FromESCO = "esco"
	FromONET = "onet"
)

/*
 * How settled an occupation is.
 *
 * Worth recording because the answer changes what it is honest to say. An
 * established job has decades of agreement behind it; an experimental one is
 * a description somebody wrote last year, and an assistant that presents the
 * two with equal confidence is wrong about one of them.
 */
const (
	Established  = "established"
	Emerging     = "emerging"
	Specialised  = "specialised"
	Experimental = "experimental"
)

/*
 * How much care work of this kind needs.
 *
 * This is not the gate — permits is — and it never makes anything possible.
 * It only ever adds caution: work that is critical stops and asks however
 * permissive the settings are that morning.
 */
const (
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

// What kind of thing a capability is.
const (
	CapabilitySkill     = "skill"
	CapabilityKnowledge = "knowledge"
	CapabilityLanguage  = "language"
)

// Occupation is one job, as a definition rather than as a person.
type Occupation struct {
	// ID is hierarchical text — engineering.backend.api_engineer — because it
	// is written into agent files that people read and edit. Titles change;
	// this does not.
	ID string `json:"id"`

	Title string `json:"title"`

	// Aliases are the other names for the same job, which is most of what
	// makes it findable. "SRE" and "site reliability engineer" are one job.
	Aliases []string `json:"aliases,omitempty"`

	Category    string `json:"category"`
	ISCO        string `json:"isco,omitempty"`
	Description string `json:"description,omitempty"`

	Status string `json:"status"`
	Risk   string `json:"risk"`

	// Oversight marks work a person has to stay in the loop on whatever the
	// settings say — medicine, law, money.
	Oversight bool `json:"oversight"`

	CameFrom string `json:"came_from"`

	// Does is what somebody in this job is actually responsible for. This is
	// the part a brief is built from: the title tells a model nothing it did
	// not already know, and these tell it what the work consists of.
	Does []string `json:"does,omitempty"`

	// Knows is the subjects this job is expected to know about, as against
	// the things it does. Tax law is known; filing a return is done.
	Knows []string `json:"knows,omitempty"`

	// Makes is what comes out: a pull request, a balance sheet, a floor plan.
	// The best single answer to "what does done look like".
	Makes []string `json:"makes,omitempty"`

	// MeasuredBy is what good work in this job is judged on.
	MeasuredBy []string `json:"measured_by,omitempty"`

	/*
	 * Ladder is this job's own career steps, in order.
	 *
	 * Its own, because ladders are not shared. "Intern, junior, mid, senior,
	 * staff, principal" belongs to software and almost nowhere else — a
	 * surgeon is a resident and then a consultant, and an accountant is
	 * qualified or is not. One ladder applied to everything would be wrong
	 * for most jobs while looking authoritative.
	 */
	Ladder []string `json:"ladder,omitempty"`

	// Near is other jobs worth considering instead of this one, which is what
	// makes "nobody here does that" answerable with something better than no.
	Near []string `json:"near,omitempty"`

	// Sources is which classification this came out of — an ISCO unit group,
	// an O*NET-SOC code. A row that cannot say where its authority came from
	// should say that rather than imply one.
	Sources []string `json:"sources,omitempty"`

	// Needs is what this job is expected to be good at, filled by the reads
	// that ask for it.
	Needs []Need `json:"needs,omitempty"`
}

/*
 * CommonLadder is the ladder for a job that does not name its own.
 *
 * Deliberately short and deliberately generic. A job with a real ladder says
 * so; this is the answer for the rest, and inventing six rungs for a welder
 * would be worse than admitting there are three.
 */
func CommonLadder() []string { return []string{"junior", "mid", "senior", "lead"} }

// Steps is this job's ladder, or the common one.
func (o Occupation) Steps() []string {
	if len(o.Ladder) > 0 {
		return o.Ladder
	}

	return CommonLadder()
}

// Need is one capability a job calls for.
type Need struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`

	// Essential separates what the job is from what it often also involves.
	Essential bool `json:"essential"`
}

// Capability is something somebody is good at.
type Capability struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Kind        string   `json:"kind"`
	Description string   `json:"description,omitempty"`
	CameFrom    string   `json:"came_from"`
}

// Imported is what one run of an import actually did.
type Imported struct {
	Jobs         int `json:"jobs"`
	Capabilities int `json:"capabilities"`
	Links        int `json:"links"`

	// Filled counts rows that already existed and gained something they were
	// missing. Left counts rows that were already better than what arrived.
	Filled int `json:"filled"`
	Left   int `json:"left"`
}

const occupationColumns = `id, title, aliases, category, isco, description,
	status, risk, oversight, came_from, COALESCE(responsibilities,''),
	COALESCE(knows,''), COALESCE(makes,''), COALESCE(measured_by,''),
	COALESCE(ladder,''), COALESCE(near,''), COALESCE(sources,'')`

// The same columns as names alone, for writing. COALESCE is for reading a row
// that predates the columns; an INSERT names them plainly.
const occupationFields = `id, title, aliases, category, isco, description,
	status, risk, oversight, came_from, responsibilities, knows, makes,
	measured_by, ladder, near, sources`

const capabilityColumns = `id, name, aliases, kind, description, came_from`

/*
 * One place that knows the column order.
 *
 * Seventeen columns read in three places was three chances to put one in the
 * wrong order, and the failure would be a job whose description was its
 * category — plausible enough to survive a glance.
 */
func scanOccupation(row interface{ Scan(...any) error }) (Occupation, error) {
	var (
		o                                                         Occupation
		aliases, does, knows, makes, measured, ladder, near, from string
	)

	err := row.Scan(&o.ID, &o.Title, &aliases, &o.Category, &o.ISCO, &o.Description,
		&o.Status, &o.Risk, &o.Oversight, &o.CameFrom,
		&does, &knows, &makes, &measured, &ladder, &near, &from)
	if err != nil {
		return o, err
	}

	o.Aliases = splitList(aliases)
	o.Does = splitList(does)
	o.Knows = splitList(knows)
	o.Makes = splitList(makes)
	o.MeasuredBy = splitList(measured)
	o.Ladder = splitList(ladder)
	o.Near = splitList(near)
	o.Sources = splitList(from)

	return o, nil
}

// valuesOf is the same order, going the other way.
func valuesOf(o Occupation) []any {
	return []any{
		o.ID, o.Title, joinList(o.Aliases), o.Category, o.ISCO, o.Description,
		orElse(o.Status, Established), orElse(o.Risk, RiskLow),
		boolInt(o.Oversight), orElse(o.CameFrom, FromSeed),
		joinList(o.Does), joinList(o.Knows), joinList(o.Makes),
		joinList(o.MeasuredBy), joinList(o.Ladder), joinList(o.Near),
		joinList(o.Sources),
	}
}

/*
 * rank decides which of two descriptions of the same job is kept.
 *
 * The two imports rank equally on purpose. Neither is a correction of the
 * other, so when both describe a job the second one to arrive fills in what
 * the first left empty and changes nothing else. An import is an enrichment,
 * never an overwrite — otherwise running one twice, or running the second
 * source, would quietly undo a title somebody had fixed.
 */
func rank(cameFrom string) int {
	switch cameFrom {
	case FromHand:
		return 3
	case FromSeed:
		return 2
	default:
		return 1
	}
}

/*
 * Import writes a batch of jobs and capabilities, and never loses anything.
 *
 * One transaction, because an import that fails halfway through and leaves a
 * taxonomy with jobs pointing at capabilities that were never written is
 * worse than one that did not run.
 */
func (d *DB) Import(jobs []Occupation, capabilities []Capability) (Imported, error) {
	var done Imported

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := d.sql().Begin()
	if err != nil {
		return done, err
	}

	defer tx.Rollback()

	for _, c := range capabilities {
		written, err := saveCapability(tx, c, now)
		if err != nil {
			return done, fmt.Errorf("writing the capability %q: %w", c.ID, err)
		}

		switch written {
		case wroteNew:
			done.Capabilities++
		case wroteFill:
			done.Filled++
		default:
			done.Left++
		}
	}

	for _, o := range jobs {
		written, err := saveOccupation(tx, o, now)
		if err != nil {
			return done, fmt.Errorf("writing the job %q: %w", o.ID, err)
		}

		switch written {
		case wroteNew:
			done.Jobs++
		case wroteFill:
			done.Filled++
		default:
			done.Left++
		}

		for _, need := range o.Needs {
			/*
			 * The WHERE is what makes running an import twice free.
			 *
			 * Without it the conflict clause updates the row to the value it
			 * already had, which SQLite counts as a change — so every start
			 * would report having rewritten every link, and the count that
			 * says what an import did would say it did everything, always.
			 *
			 * And it only ever promotes: one source calling a capability
			 * essential settles it, because the other source is describing
			 * the same job less precisely rather than contradicting it.
			 */
			res, err := tx.Exec(`
				INSERT INTO job_capabilities (job_id, capability_id, essential)
				VALUES (?,?,?)
				ON CONFLICT (job_id, capability_id) DO UPDATE SET essential = 1
				WHERE job_capabilities.essential = 0 AND excluded.essential = 1`,
				o.ID, need.ID, boolInt(need.Essential))
			if err != nil {
				return done, fmt.Errorf("linking %q to %q: %w", o.ID, need.ID, err)
			}

			if changed, _ := res.RowsAffected(); changed > 0 {
				done.Links++
			}
		}
	}

	return done, tx.Commit()
}

// What one write did, so an import can say what it actually changed.
const (
	wroteNew = iota
	wroteFill
	wroteNothing
)

func saveOccupation(tx *sql.Tx, o Occupation, now string) (int, error) {
	have, err := scanOccupation(tx.QueryRow(
		`SELECT `+occupationColumns+` FROM occupations WHERE id = ?`, o.ID))

	if err == sql.ErrNoRows {
		_, err = tx.Exec(`
			INSERT INTO occupations (`+occupationFields+`, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			append(valuesOf(o), now, now)...)

		return wroteNew, err
	}

	if err != nil {
		return wroteNothing, err
	}

	merged, changed := mergeOccupation(have, o)
	if !changed {
		return wroteNothing, nil
	}

	_, err = tx.Exec(`
		UPDATE occupations SET title = ?, aliases = ?, category = ?, isco = ?,
			description = ?, status = ?, risk = ?, oversight = ?, came_from = ?,
			responsibilities = ?, knows = ?, makes = ?, measured_by = ?,
			ladder = ?, near = ?, sources = ?, updated_at = ?
		WHERE id = ?`,
		append(valuesOf(merged)[1:], now, merged.ID)...)

	return wroteFill, err
}

/*
 * mergeOccupation keeps the better of two descriptions, field by field.
 *
 * An arriving row may replace a field only when it outranks what is there.
 * Otherwise it may fill a field that is empty and nothing more — so an import
 * can add the description a hand-written job never had without touching the
 * title somebody chose. Aliases are the exception and always union: another
 * name for the same job is never a contradiction, it is one more way to find
 * it.
 */
func mergeOccupation(have, arriving Occupation) (Occupation, bool) {
	may := rank(orElse(arriving.CameFrom, FromSeed)) > rank(have.CameFrom)

	out, changed := have, false

	take := func(into *string, from string, always bool) {
		if from == "" || from == *into {
			return
		}

		if *into == "" || (may && always) {
			*into, changed = from, true
		}
	}

	take(&out.Title, arriving.Title, true)
	take(&out.Category, arriving.Category, true)
	take(&out.ISCO, arriving.ISCO, true)
	take(&out.Description, arriving.Description, true)
	take(&out.Status, arriving.Status, true)
	take(&out.Risk, arriving.Risk, true)

	if arriving.Oversight && !out.Oversight {
		// Caution is only ever added. One source saying a job needs a person
		// watching is enough; a second source silent about it is not a
		// disagreement.
		out.Oversight, changed = true, true
	}

	/*
	 * The lists all union rather than replace.
	 *
	 * Another name for the same job is never a contradiction, and neither is
	 * another responsibility, another thing it produces, or another
	 * classification it appears in. Two sources describing one occupation are
	 * describing one occupation; keeping both descriptions is more true than
	 * picking the one that arrived second.
	 */
	for _, list := range []struct {
		into     *[]string
		arriving []string
	}{
		{&out.Aliases, arriving.Aliases},
		{&out.Does, arriving.Does},
		{&out.Knows, arriving.Knows},
		{&out.Makes, arriving.Makes},
		{&out.MeasuredBy, arriving.MeasuredBy},
		{&out.Near, arriving.Near},
		{&out.Sources, arriving.Sources},
	} {
		if union := mergeLists(*list.into, list.arriving); len(union) != len(*list.into) {
			*list.into, changed = union, true
		}
	}

	/*
	 * The ladder is the exception, and replaces rather than unions.
	 *
	 * It is an ordered thing. Merging "junior, senior" into "resident,
	 * consultant" produces a four-rung ladder that nobody climbs, so the
	 * better-ranked source wins outright or nothing happens.
	 */
	if len(arriving.Ladder) > 0 && (len(out.Ladder) == 0 || may) {
		if !sameList(out.Ladder, arriving.Ladder) {
			out.Ladder, changed = arriving.Ladder, true
		}
	}

	if may && arriving.CameFrom != "" && arriving.CameFrom != out.CameFrom {
		out.CameFrom, changed = arriving.CameFrom, true
	}

	return out, changed
}

func saveCapability(tx *sql.Tx, c Capability, now string) (int, error) {
	var (
		have Capability
		raw  string
	)

	err := tx.QueryRow(`SELECT `+capabilityColumns+` FROM capabilities WHERE id = ?`, c.ID).
		Scan(&have.ID, &have.Name, &raw, &have.Kind, &have.Description, &have.CameFrom)

	if err == sql.ErrNoRows {
		_, err = tx.Exec(`
			INSERT INTO capabilities (`+capabilityColumns+`, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			c.ID, c.Name, joinList(c.Aliases), orElse(c.Kind, CapabilitySkill),
			c.Description, orElse(c.CameFrom, FromSeed), now, now)

		return wroteNew, err
	}

	if err != nil {
		return wroteNothing, err
	}

	have.Aliases = splitList(raw)

	may := rank(orElse(c.CameFrom, FromSeed)) > rank(have.CameFrom)
	changed := false

	if c.Name != "" && (have.Name == "" || may) && c.Name != have.Name {
		have.Name, changed = c.Name, true
	}

	if c.Description != "" && have.Description == "" {
		have.Description, changed = c.Description, true
	}

	if c.Kind != "" && (have.Kind == "" || may) && c.Kind != have.Kind {
		have.Kind, changed = c.Kind, true
	}

	if union := mergeLists(have.Aliases, c.Aliases); len(union) != len(have.Aliases) {
		have.Aliases, changed = union, true
	}

	if !changed {
		return wroteNothing, nil
	}

	_, err = tx.Exec(`
		UPDATE capabilities SET name = ?, aliases = ?, kind = ?, description = ?,
			updated_at = ?
		WHERE id = ?`,
		have.Name, joinList(have.Aliases), have.Kind, have.Description, now, have.ID)

	return wroteFill, err
}

// Occupation reads one job, with what it is expected to be good at.
func (d *DB) Occupation(id string) (*Occupation, error) {
	o, err := scanOccupation(d.sql().QueryRow(
		`SELECT `+occupationColumns+` FROM occupations WHERE id = ?`, id))

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if o.Needs, err = d.CapabilitiesOf(id); err != nil {
		return nil, err
	}

	return &o, nil
}

// CapabilitiesOf is what a job is expected to be good at, the essential ones
// first because that is the order somebody reads them in.
func (d *DB) CapabilitiesOf(jobID string) ([]Need, error) {
	rows, err := d.sql().Query(`
		SELECT j.capability_id, COALESCE(c.name, ''), j.essential
		FROM job_capabilities j
		LEFT JOIN capabilities c ON c.id = j.capability_id
		WHERE j.job_id = ?
		ORDER BY j.essential DESC, COALESCE(c.name, j.capability_id)`, jobID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Need{}

	for rows.Next() {
		var need Need

		if err := rows.Scan(&need.ID, &need.Name, &need.Essential); err != nil {
			return nil, err
		}

		out = append(out, need)
	}

	return out, rows.Err()
}

// Occupations lists a category, or all of them when category is empty.
func (d *DB) Occupations(category string, limit int) ([]Occupation, error) {
	where, args := "", []any{}

	if category != "" {
		where, args = " WHERE category = ?", []any{category}
	}

	if limit <= 0 {
		limit = 200
	}

	args = append(args, limit)

	return d.readOccupations(`SELECT `+occupationColumns+` FROM occupations`+where+
		` ORDER BY title LIMIT ?`, args...)
}

/*
 * FindOccupations is the search, and it reads the whole table on purpose.
 *
 * Three thousand rows is a few milliseconds of memory bandwidth, and doing the
 * comparison here rather than in SQL is what makes it work in Bulgarian:
 * SQLite would treat "Заварчик" and "заварчик" as different words. Scored
 * rather than filtered, so an exact title beats a word buried in somebody
 * else's description.
 */
func (d *DB) FindOccupations(text string, limit int) ([]Occupation, error) {
	looking := fold(text)

	if looking == "" {
		return d.Occupations("", limit)
	}

	all, err := d.readOccupations(`SELECT ` + occupationColumns + ` FROM occupations`)
	if err != nil {
		return nil, err
	}

	type scored struct {
		o     Occupation
		score int
	}

	found := []scored{}

	for _, o := range all {
		if score := match(looking, o.Title, o.Aliases, o.Description); score > 0 {
			found = append(found, scored{o: o, score: score})
		}
	}

	sort.SliceStable(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score > found[j].score
		}

		return found[i].o.Title < found[j].o.Title
	})

	if limit <= 0 {
		limit = 50
	}

	out := make([]Occupation, 0, limit)

	for _, s := range found {
		if len(out) >= limit {
			break
		}

		out = append(out, s.o)
	}

	return out, nil
}

// FindCapabilities is the same search over what people are good at.
func (d *DB) FindCapabilities(text string, limit int) ([]Capability, error) {
	looking := fold(text)

	all, err := d.readCapabilities(`SELECT ` + capabilityColumns + ` FROM capabilities`)
	if err != nil {
		return nil, err
	}

	type scored struct {
		c     Capability
		score int
	}

	found := []scored{}

	for _, c := range all {
		score := 1

		if looking != "" {
			score = match(looking, c.Name, c.Aliases, c.Description)
		}

		if score > 0 {
			found = append(found, scored{c: c, score: score})
		}
	}

	sort.SliceStable(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score > found[j].score
		}

		return found[i].c.Name < found[j].c.Name
	})

	if limit <= 0 {
		limit = 50
	}

	out := make([]Capability, 0, limit)

	for _, s := range found {
		if len(out) >= limit {
			break
		}

		out = append(out, s.c)
	}

	return out, nil
}

// Capability reads one.
func (d *DB) Capability(id string) (*Capability, error) {
	list, err := d.readCapabilities(
		`SELECT `+capabilityColumns+` FROM capabilities WHERE id = ?`, id)

	if err != nil || len(list) == 0 {
		return nil, err
	}

	return &list[0], nil
}

/*
 * OccupationsNeeding answers "which jobs call for this", which is half of
 * "who here could do this" — the other half is which agents hold those jobs,
 * and that is the roster's question rather than the taxonomy's.
 */
func (d *DB) OccupationsNeeding(capabilityID string, limit int) ([]Occupation, error) {
	if limit <= 0 {
		limit = 50
	}

	return d.readOccupations(`
		SELECT `+occupationColumns+` FROM occupations o
		JOIN job_capabilities j ON j.job_id = o.id
		WHERE j.capability_id = ?
		ORDER BY j.essential DESC, o.title
		LIMIT ?`, capabilityID, limit)
}

// HowManyOccupations and HowManyCapabilities are for the interface, which says
// how much the program knows about the world of work before offering to look
// something up in it.
func (d *DB) HowManyOccupations() (int, error) { return d.count(`SELECT COUNT(*) FROM occupations`) }

func (d *DB) HowManyCapabilities() (int, error) {
	return d.count(`SELECT COUNT(*) FROM capabilities`)
}

func (d *DB) count(query string) (int, error) {
	var n int

	err := d.sql().QueryRow(query).Scan(&n)

	return n, err
}

func (d *DB) readOccupations(query string, args ...any) ([]Occupation, error) {
	rows, err := d.sql().Query(query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Occupation{}

	for rows.Next() {
		o, err := scanOccupation(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, o)
	}

	return out, rows.Err()
}

func (d *DB) readCapabilities(query string, args ...any) ([]Capability, error) {
	rows, err := d.sql().Query(query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Capability{}

	for rows.Next() {
		var (
			c   Capability
			raw string
		)

		if err := rows.Scan(&c.ID, &c.Name, &raw, &c.Kind, &c.Description, &c.CameFrom); err != nil {
			return nil, err
		}

		c.Aliases = splitList(raw)
		out = append(out, c)
	}

	return out, rows.Err()
}

/*
 * match scores one row against what somebody typed.
 *
 * The numbers matter less than their order: a name that is the thing asked
 * for comes before a name that merely contains it, and both come a long way
 * before a word appearing in a paragraph of description.
 */
func match(looking, name string, aliases []string, description string) int {
	folded := fold(name)

	switch {
	case folded == looking:
		return 100
	case strings.HasPrefix(folded, looking):
		return 60
	case strings.Contains(folded, looking):
		return 40
	}

	for _, alias := range aliases {
		alias = fold(alias)

		if alias == looking {
			return 50
		}

		if strings.Contains(alias, looking) {
			return 30
		}
	}

	if strings.Contains(fold(description), looking) {
		return 10
	}

	return 0
}

// fold is the one place case and spacing stop mattering. Go's ToLower rather
// than SQLite's, because SQLite's only knows the English alphabet.
func fold(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

// Aliases are kept one to a line: a comma is a character that turns up inside
// job titles, and a separator that can appear in the thing it separates is a
// bug waiting for the right job title.
func joinList(list []string) string { return strings.Join(list, "\n") }

func splitList(raw string) []string {
	out := []string{}

	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// mergeLists unions two lists of names without minding case, keeping the
// spelling that was there first.
func mergeLists(have, arriving []string) []string {
	seen := map[string]bool{}

	for _, one := range have {
		seen[fold(one)] = true
	}

	out := have

	for _, one := range arriving {
		if one = strings.TrimSpace(one); one == "" || seen[fold(one)] {
			continue
		}

		seen[fold(one)] = true
		out = append(out, one)
	}

	return out
}

// sameList reports whether two ordered lists say the same thing.
func sameList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func boolInt(yes bool) int {
	if yes {
		return 1
	}

	return 0
}
