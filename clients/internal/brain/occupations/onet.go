package occupations

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * O*NET: the American occupational database, as enrichment.
 *
 * Where ESCO says what a job is and what it needs, O*NET says what somebody
 * in it actually spends the day doing — task statements written by people who
 * hold the job, the subjects it is important to know, the other titles it
 * goes by, and the technologies it is in demand for. Those are exactly the
 * fields a brief is built from and the ones ESCO leaves thinnest.
 *
 * So it enriches before it adds. A job whose title or other names match one
 * the catalogue already has gains what it was missing and changes nothing
 * else — the same merge every import uses, where what somebody wrote by hand
 * and what shipped always win. Only a job nothing matches is added as new.
 *
 * Reads the text release: tab-separated files, one per table, found by name
 * wherever they sit inside the folder or the zip.
 */

// ONET is the source name an import is recorded under.
const ONET = "onet"

// How much of each list one job keeps. O*NET has thirty task statements for
// some jobs; a brief built from thirty is a brief nobody reads.
const (
	mostTasks   = 10
	mostAliases = 15
	mostKnows   = 8

	// importantEnough is O*NET's own importance scale, one to five. Three and
	// a half is "important" and above in its own wording.
	importantEnough = 3.5
)

// ImportONET reads an O*NET text release into the catalogue.
func ImportONET(ctx context.Context, db *store.DB, src *Source, report func(Progress)) (Progress, error) {
	var p Progress

	say := func(stage string, done, of int) {
		p.Stage, p.Done, p.Of = stage, done, of

		if report != nil {
			report(p)
		}
	}

	occupationsFile, ok := src.Find("occupation data.txt")
	if !ok {
		return p, fmt.Errorf("this does not look like O*NET: there is no \"Occupation Data.txt\" in it")
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

	const code = "O*NET-SOC Code"

	jobs := []*store.Occupation{}
	byCode := map[string]*store.Occupation{}

	// Which jobs were given a new id rather than matched, so their other
	// titles can be tried against the catalogue once they are known.
	fresh := map[*store.Occupation]bool{}

	err = src.table(ctx, occupationsFile, '\t', []string{code, "Title"}, func(get func(string) string) error {
		soc, title := get(code), get("Title")

		if soc == "" || title == "" {
			return nil
		}

		category := categoryOfSOC(soc)
		risk, oversight := riskOfSOC(soc)

		job := &store.Occupation{
			Title:       title,
			Category:    category,
			Description: get("Description"),
			Status:      store.Established,
			Risk:        risk,
			Oversight:   oversight,
			CameFrom:    store.FromONET,
			Sources:     []string{"O*NET-SOC " + soc},
		}

		id, clash := jobIndex.match(title)

		if clash != "" {
			p.note(fmt.Sprintf("%q matched more than one job already here, so it was added on its own", clash))
		}

		if id == "" {
			id = freeID(taken, category+"."+MakeID(title), soc)
			fresh[job] = true
		}

		job.ID = id
		taken[id] = true

		jobs = append(jobs, job)
		byCode[soc] = job

		return nil
	})
	if err != nil {
		return p, err
	}

	say("occupations", len(jobs), len(jobs))

	// Other titles — and a job the first pass added as new may turn out to be
	// one the catalogue has under one of them.
	if file, ok := src.Find("alternate titles.txt"); ok {
		err := src.table(ctx, file, '\t', []string{code, "Alternate Title"}, func(get func(string) string) error {
			job := byCode[get(code)]

			if job == nil || len(job.Aliases) >= mostAliases {
				return nil
			}

			for _, alias := range []string{get("Alternate Title"), get("Short Title")} {
				if alias != "" {
					job.Aliases = append(job.Aliases, alias)
				}
			}

			return nil
		})
		if err != nil {
			p.note("could not read the alternate titles: " + err.Error())
		}
	}

	for job := range fresh {
		if id, clash := jobIndex.match(job.Aliases...); id != "" && clash == "" {
			delete(taken, job.ID)
			job.ID = id
		}
	}

	say("other titles", len(jobs), len(jobs))

	// What the day consists of: the core tasks, not the supplemental ones.
	if file, ok := src.Find("task statements.txt"); ok {
		err := src.table(ctx, file, '\t', []string{code, "Task"}, func(get func(string) string) error {
			job := byCode[get(code)]

			if job == nil || len(job.Does) >= mostTasks {
				return nil
			}

			if kind := get("Task Type"); kind != "" && !strings.EqualFold(kind, "Core") {
				return nil
			}

			if task := get("Task"); task != "" {
				job.Does = append(job.Does, task)
			}

			return nil
		})
		if err != nil {
			p.note("could not read the task statements: " + err.Error())
		}
	}

	say("tasks", len(jobs), len(jobs))

	// What it is important to know about, on O*NET's own importance scale.
	if file, ok := src.Find("knowledge.txt"); ok {
		err := src.table(ctx, file, '\t', []string{code, "Element Name", "Scale ID", "Data Value"}, func(get func(string) string) error {
			job := byCode[get(code)]

			if job == nil || len(job.Knows) >= mostKnows || get("Scale ID") != "IM" {
				return nil
			}

			if value, err := strconv.ParseFloat(get("Data Value"), 64); err == nil && value >= importantEnough {
				job.Knows = append(job.Knows, get("Element Name"))
			}

			return nil
		})
		if err != nil {
			p.note("could not read the knowledge ratings: " + err.Error())
		}
	}

	say("knowledge", len(jobs), len(jobs))

	/*
	 * The technologies a job is in demand for, as capabilities.
	 *
	 * Only the ones O*NET marks as hot. The full list is tens of thousands of
	 * product names — every accounting package in every version — and a
	 * catalogue of capabilities that is mostly brand names is one nobody can
	 * search. A technology the catalogue already knows by that name is
	 * linked rather than added again.
	 */
	var capabilities []store.Capability

	if file, ok := src.Find("technology skills.txt"); ok {
		added := map[string]bool{}

		err := src.table(ctx, file, '\t', []string{code, "Example", "Hot Technology"}, func(get func(string) string) error {
			job := byCode[get(code)]
			name := get("Example")

			if job == nil || name == "" || !strings.EqualFold(get("Hot Technology"), "Y") {
				return nil
			}

			id, _ := capabilityIndex.match(name)

			if id == "" {
				id = MakeID(name)

				if !added[id] {
					added[id] = true
					capabilities = append(capabilities, store.Capability{
						ID: id, Name: name, Kind: store.CapabilityKnowledge, CameFrom: store.FromONET,
						Description: get("Commodity Title"),
					})
				}
			}

			job.Needs = append(job.Needs, store.Need{ID: id})

			return nil
		})
		if err != nil {
			p.note("could not read the technology skills: " + err.Error())
		}
	}

	if len(capabilities) > 0 {
		done, err := db.Import(nil, capabilities)
		p.add(done)

		if err != nil {
			return p, err
		}
	}

	say("technologies", len(jobs), len(jobs))

	err = writeJobs(ctx, db, jobs, &p, say)

	return p, err
}

/*
 * categoryOfSOC files an O*NET occupation on the catalogue's shelves by its
 * SOC major group, and by the minor group where the major one spans two.
 */
func categoryOfSOC(soc string) string {
	for _, rule := range []struct {
		prefix, category string
	}{
		{"13-2", Finance}, {"15-2", Data}, {"27-1", Design}, {"43-4051", Support},
		{"11-", Leadership}, {"13-", Operations}, {"15-", Software}, {"17-", Engineering},
		{"19-", Science}, {"21-", General}, {"23-", Legal}, {"25-", Education},
		{"27-", Media}, {"29-", Health}, {"31-", Health}, {"33-", Safety},
		{"35-", Hospitality}, {"37-", Trades}, {"39-", Hospitality}, {"41-", Sales},
		{"43-", Operations}, {"45-", Land}, {"47-", Trades}, {"49-", Trades},
		{"51-", Trades}, {"53-", Transport}, {"55-", Safety},
	} {
		if strings.HasPrefix(soc, rule.prefix) {
			return rule.category
		}
	}

	return General
}

// riskOfSOC is the care a group needs where SOC says enough to know, on the
// same terms as riskOfISCO.
func riskOfSOC(soc string) (string, bool) {
	switch {
	case strings.HasPrefix(soc, "29-12"), strings.HasPrefix(soc, "23-1011"):
		return store.RiskCritical, true
	case strings.HasPrefix(soc, "29-"), strings.HasPrefix(soc, "31-"):
		return store.RiskHigh, true
	case strings.HasPrefix(soc, "23-"):
		return store.RiskHigh, true
	case strings.HasPrefix(soc, "13-2"), strings.HasPrefix(soc, "33-"), strings.HasPrefix(soc, "55-"):
		return store.RiskHigh, false
	case strings.HasPrefix(soc, "53-3"):
		return store.RiskMedium, false
	}

	return store.RiskLow, false
}
