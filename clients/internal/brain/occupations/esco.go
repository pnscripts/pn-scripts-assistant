package occupations

import (
	"context"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * ESCO: the European classification of occupations and skills.
 *
 * The spine of the catalogue, for three reasons that are about this program
 * rather than about classifications. It ships the links between occupations
 * and skills already made — the "what a job needs" nobody should write by
 * hand. It is built on ISCO-08, so every job arrives knowing where it sits.
 * And it comes in Bulgarian as well as English, which is the other language
 * this assistant is spoken to in.
 *
 * Licensed CC BY 4.0, and every row imported says it came from ESCO.
 *
 * Read in four passes over the CSV bundle — skills, occupations, the other
 * languages' names for them, the links — and written in batches, each one a
 * transaction of its own. So a run stopped halfway keeps what it wrote, and
 * a second run finds those rows already there, changes nothing about them and
 * carries on: stopping is safe, and starting again is resuming.
 */

// ESCO is the source name an import is recorded under.
const ESCO = "esco"

// batch is how many rows go in one transaction: small enough that stopping
// loses seconds, large enough that the disk is not asked to sync every row.
const batch = 250

// ImportESCO reads an ESCO CSV bundle into the catalogue.
func ImportESCO(ctx context.Context, db *store.DB, src *Source, report func(Progress)) (Progress, error) {
	var p Progress

	say := func(stage string, done, of int) {
		p.Stage, p.Done, p.Of = stage, done, of

		if report != nil {
			report(p)
		}
	}

	occupationsFile, ok := src.Find("occupations_en.csv")
	if !ok {
		all := src.All(func(base string) bool {
			return strings.HasPrefix(base, "occupations_") && strings.HasSuffix(base, ".csv")
		})

		if len(all) == 0 {
			return p, fmt.Errorf("this does not look like ESCO: there is no occupations_en.csv in it")
		}

		occupationsFile = all[0]
	}

	knownJobs, knownCapabilities, err := db.Labels()
	if err != nil {
		return p, err
	}

	jobIndex, capabilityIndex := newIndex(knownJobs), newIndex(knownCapabilities)

	taken := map[string]bool{}

	for _, j := range knownJobs {
		taken[j.ID] = true
	}

	// Pass one: what people are good at.
	skillID := map[string]string{}

	if skillsFile, ok := src.Find("skills_en.csv"); ok {
		var pending []store.Capability

		flush := func() error {
			if len(pending) == 0 {
				return nil
			}

			done, err := db.Import(nil, pending)
			p.add(done)
			pending = pending[:0]

			return err
		}

		read := 0

		err := src.table(ctx, skillsFile, ',', []string{"conceptUri", "preferredLabel"}, func(get func(string) string) error {
			name := get("preferredLabel")
			uri := get("conceptUri")

			if name == "" || uri == "" {
				return nil
			}

			aliases := lines(get("altLabels"))

			id := capabilityIndex.named(name)

			if id == "" {
				id = MakeID(name)
			}

			skillID[uri] = id

			kind := store.CapabilitySkill

			if strings.Contains(strings.ToLower(get("skillType")), "knowledge") {
				kind = store.CapabilityKnowledge
			}

			pending = append(pending, store.Capability{
				ID: id, Name: capitalised(name), Aliases: aliases, Kind: kind,
				Description: firstNonEmpty(get("description"), get("definition")),
				CameFrom:    store.FromESCO,
			})

			read++

			if len(pending) >= batch*4 {
				say("skills", read, 0)

				return flush()
			}

			return nil
		})
		if err != nil {
			return p, err
		}

		if err := flush(); err != nil {
			return p, err
		}

		say("skills", read, read)
	}

	// Pass two: the occupations themselves.
	jobs := []*store.Occupation{}
	byURI := map[string]*store.Occupation{}

	err = src.table(ctx, occupationsFile, ',', []string{"conceptUri", "preferredLabel", "iscoGroup"}, func(get func(string) string) error {
		title := get("preferredLabel")
		uri := get("conceptUri")

		if title == "" || uri == "" {
			return nil
		}

		aliases := append(lines(get("altLabels")), lines(get("hiddenLabels"))...)
		isco := get("iscoGroup")
		category := categoryOfISCO(isco)
		risk, oversight := riskOfISCO(isco)

		job := &store.Occupation{
			Title:       capitalised(title),
			Aliases:     aliases,
			Category:    category,
			ISCO:        isco,
			Description: firstNonEmpty(get("description"), get("definition")),
			Status:      store.Established,
			Risk:        risk,
			Oversight:   oversight,
			CameFrom:    store.FromESCO,
			Sources:     sourcesOf("ESCO "+get("code"), "ISCO-08 "+isco),
		}

		source := "ESCO " + get("code")

		id, clash := jobIndex.match(source, title, aliases)

		if clash != "" {
			p.note(fmt.Sprintf("%q matched more than one job already here, so it was added on its own", clash))
		}

		if id == "" {
			id = freeID(taken, category+"."+MakeID(title), get("code"))
		}

		job.ID = id
		taken[id] = true
		jobIndex.remember(source, id)

		jobs = append(jobs, job)
		byURI[uri] = job

		if len(jobs)%batch == 0 {
			say("occupations", len(jobs), 0)
		}

		return nil
	})
	if err != nil {
		return p, err
	}

	say("occupations", len(jobs), len(jobs))

	// Pass three: every other language's names, as more ways to find a job.
	for _, other := range src.All(func(base string) bool {
		return strings.HasPrefix(base, "occupations_") && strings.HasSuffix(base, ".csv") &&
			base != "occupations_en.csv"
	}) {
		if other == occupationsFile {
			continue
		}

		err := src.table(ctx, other, ',', []string{"conceptUri", "preferredLabel"}, func(get func(string) string) error {
			if job := byURI[get("conceptUri")]; job != nil {
				job.Aliases = append(append(job.Aliases, get("preferredLabel")), lines(get("altLabels"))...)
			}

			return nil
		})
		if err != nil {
			p.note("could not read " + other + ": " + err.Error())
		}
	}

	// And every other language's names for the skills, so "Who is good at"
	// can be asked in Bulgarian too.
	for _, other := range src.All(func(base string) bool {
		return strings.HasPrefix(base, "skills_") && strings.HasSuffix(base, ".csv") &&
			base != "skills_en.csv"
	}) {
		var more []store.Capability

		err := src.table(ctx, other, ',', []string{"conceptUri", "preferredLabel"}, func(get func(string) string) error {
			if id, known := skillID[get("conceptUri")]; known {
				more = append(more, store.Capability{ID: id,
					Aliases:  append([]string{get("preferredLabel")}, lines(get("altLabels"))...),
					CameFrom: store.FromESCO})
			}

			if len(more) >= batch*4 {
				done, err := db.Import(nil, more)
				p.add(done)
				more = more[:0]

				return err
			}

			return nil
		})
		if err == nil && len(more) > 0 {
			done, importErr := db.Import(nil, more)
			p.add(done)
			err = importErr
		}

		if err != nil {
			p.note("could not read " + other + ": " + err.Error())
		}
	}

	// Pass four: what each job needs.
	if relations, ok := src.Find("occupationskillrelations_en.csv", "occupationskillrelations.csv"); ok {
		links := 0

		err := src.table(ctx, relations, ',', []string{"occupationUri", "skillUri"}, func(get func(string) string) error {
			job := byURI[get("occupationUri")]
			skill, known := skillID[get("skillUri")]

			if job == nil || !known {
				return nil
			}

			job.Needs = append(job.Needs, store.Need{
				ID: skill, Essential: strings.EqualFold(get("relationType"), "essential"),
			})

			links++

			if links%5000 == 0 {
				say("what each job needs", links, 0)
			}

			return nil
		})
		if err != nil {
			return p, err
		}
	} else if len(skillID) > 0 {
		p.note("there was no occupationSkillRelations file, so no job was told what it needs")
	}

	// Written first and returned after: a return list is evaluated left to
	// right, and returning p beside the call would hand back the counts from
	// before the writing happened.
	err = writeJobs(ctx, db, jobs, &p, say)

	return p, err
}

// writeJobs writes the occupations in batches, stopping between batches
// when asked to.
func writeJobs(ctx context.Context, db *store.DB, jobs []*store.Occupation, p *Progress, say func(string, int, int)) error {
	for start := 0; start < len(jobs); start += batch {
		if err := ctx.Err(); err != nil {
			return err
		}

		end := start + batch

		if end > len(jobs) {
			end = len(jobs)
		}

		chunk := make([]store.Occupation, 0, end-start)

		for _, job := range jobs[start:end] {
			job.Aliases = uniqueNames(job.Aliases, job.Title)
			chunk = append(chunk, *job)
		}

		done, err := db.Import(chunk, nil)
		p.add(done)

		if err != nil {
			return err
		}

		say("writing", end, len(jobs))
	}

	return nil
}

// freeID is an id nothing else has, adding the source's own code when two
// jobs would otherwise share one.
func freeID(taken map[string]bool, id, code string) string {
	if len(id) > 90 {
		id = strings.TrimRight(id[:90], "_")
	}

	if !taken[id] {
		return id
	}

	if code = MakeID(code); code != "" && !taken[id+"_"+code] {
		return id + "_" + code
	}

	for i := 2; ; i++ {
		if next := fmt.Sprintf("%s_%d", id, i); !taken[next] {
			return next
		}
	}
}

// capitalised is a label as the rest of the catalogue writes one. ESCO keeps
// every label lower-case, which read as "Take on a ship pilot" beside
// "Backend engineer" and looked like two different programs.
func capitalised(label string) string {
	r := []rune(strings.TrimSpace(label))

	if len(r) == 0 {
		return ""
	}

	return strings.ToUpper(string(r[0])) + string(r[1:])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}

	return ""
}

func sourcesOf(values ...string) []string {
	out := []string{}

	for _, v := range values {
		if parts := strings.Fields(v); len(parts) > 1 {
			out = append(out, v)
		}
	}

	return out
}

// uniqueNames drops repeats and the title itself from a list of aliases,
// case aside — every language repeating an English loan word would otherwise
// be the same alias five times.
func uniqueNames(names []string, title string) []string {
	seen := map[string]bool{strings.ToLower(title): true}
	out := []string{}

	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))

		if key == "" || seen[key] {
			continue
		}

		seen[key] = true
		out = append(out, strings.TrimSpace(name))
	}

	return out
}

/*
 * categoryOfISCO files an occupation on the catalogue's own shelves by its
 * ISCO-08 group.
 *
 * By the most specific prefix that decides it, because the major groups cut
 * across the shelves: ISCO's professionals are doctors, lawyers and
 * programmers alike, and it is the sub-major and minor groups that say which.
 */
func categoryOfISCO(code string) string {
	code = strings.TrimSpace(code)

	for _, rule := range []struct {
		prefix, category string
	}{
		{"211", Science}, {"212", Science}, {"213", Science},
		{"214", Engineering}, {"215", Engineering}, {"216", Engineering},
		{"241", Finance}, {"242", Operations}, {"243", Marketing},
		{"261", Legal}, {"262", Media}, {"263", Science}, {"264", Media}, {"265", Media},
		{"331", Finance}, {"332", Sales}, {"333", Operations}, {"334", Operations}, {"335", Government},
		{"341", Legal}, {"342", Hospitality}, {"3434", Hospitality}, {"343", Media},
		{"422", Support},
		{"0", Safety},
		{"11", Leadership}, {"12", Leadership}, {"13", Operations}, {"14", Hospitality},
		{"22", Health}, {"23", Education}, {"25", Software},
		{"315", Transport}, {"31", Engineering}, {"32", Health}, {"35", Software},
		{"4", Operations},
		{"51", Hospitality}, {"52", Sales}, {"53", Health}, {"54", Safety},
		{"6", Land}, {"7", Trades}, {"81", Trades}, {"82", Trades}, {"83", Transport},
		{"9", General},
	} {
		// Longer prefixes are listed before the shorter ones they sit inside,
		// so the first match is the most specific.
		if strings.HasPrefix(code, rule.prefix) {
			return rule.category
		}
	}

	return General
}

/*
 * riskOfISCO is the care a group of occupations needs, where ISCO says enough
 * to know: medicine and law, where the decision is somebody's own to make;
 * money; and the forces and emergency services.
 *
 * Caution only ever added. Anything not named is low, and a job imported as
 * low that somebody has marked higher by hand stays higher.
 */
func riskOfISCO(code string) (string, bool) {
	switch {
	case strings.HasPrefix(code, "221"), strings.HasPrefix(code, "2261"), strings.HasPrefix(code, "261"):
		return store.RiskCritical, true
	case strings.HasPrefix(code, "22"), strings.HasPrefix(code, "32"):
		return store.RiskHigh, true
	case strings.HasPrefix(code, "241"), strings.HasPrefix(code, "0"), strings.HasPrefix(code, "54"):
		return store.RiskHigh, false
	case strings.HasPrefix(code, "83"):
		return store.RiskMedium, false
	}

	return store.RiskLow, false
}
