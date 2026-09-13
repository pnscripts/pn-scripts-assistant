package store

import "testing"

func someJobs() ([]Occupation, []Capability) {
	return []Occupation{{
			ID: "software.backend_engineer", Title: "Backend engineer",
			Aliases: []string{"backend developer"}, Category: "software",
			Description: "Builds the part that runs on a server.",
			Status:      Established, Risk: RiskLow, CameFrom: FromSeed,
			Needs: []Need{
				{ID: "api_design", Essential: true},
				{ID: "performance", Essential: false},
			},
		}, {
			ID: "trades.welder", Title: "Welder", Aliases: []string{"Заварчик"},
			Category: "trades", Status: Established, Risk: RiskHigh,
			Oversight: true, CameFrom: FromSeed,
			Needs: []Need{{ID: "welding", Essential: true}},
		}},
		[]Capability{
			{ID: "api_design", Name: "API design", CameFrom: FromSeed},
			{ID: "performance", Name: "Performance tuning", CameFrom: FromSeed},
			{ID: "welding", Name: "Welding", CameFrom: FromSeed},
		}
}

/*
 * Importing the same thing twice changes nothing the second time.
 *
 * This is what lets the built-in seed be written on every start rather than
 * once, which is in turn what makes a job added in a later version simply
 * appear. If the second run touched anything, that would become a write to
 * the database on every launch for no reason.
 */
func TestImportingTheSameThingTwiceChangesNothing(t *testing.T) {
	db := open(t)

	jobs, capabilities := someJobs()

	first, err := db.Import(jobs, capabilities)
	if err != nil {
		t.Fatal(err)
	}

	if first.Jobs != 2 || first.Capabilities != 3 || first.Links != 3 {
		t.Fatalf("the first run wrote %+v", first)
	}

	again, err := db.Import(jobs, capabilities)
	if err != nil {
		t.Fatal(err)
	}

	if again.Jobs != 0 || again.Capabilities != 0 || again.Filled != 0 || again.Links != 0 {
		t.Errorf("the second run wrote %+v", again)
	}

	if again.Left != 5 {
		t.Errorf("left %d rows alone, want 5", again.Left)
	}
}

/*
 * What somebody wrote by hand survives an import.
 *
 * The whole risk of importing three thousand rows over a taxonomy somebody
 * has been editing is that it quietly undoes the editing. A title corrected
 * on purpose must still be there afterwards, and an import that disagrees
 * must lose.
 */
func TestAnImportDoesNotUndoWhatSomebodyWrote(t *testing.T) {
	db := open(t)

	mine := Occupation{
		ID: "software.backend_engineer", Title: "The one who does the server bits",
		Category: "software", CameFrom: FromHand,
	}

	if _, err := db.Import([]Occupation{mine}, nil); err != nil {
		t.Fatal(err)
	}

	jobs, capabilities := someJobs()

	if _, err := db.Import(jobs, capabilities); err != nil {
		t.Fatal(err)
	}

	job, err := db.Occupation("software.backend_engineer")
	if err != nil || job == nil {
		t.Fatalf("reading it back: %v %v", job, err)
	}

	if job.Title != "The one who does the server bits" {
		t.Errorf("the import renamed it to %q", job.Title)
	}

	// But it may fill in what was never there. An import is an enrichment.
	if job.Description == "" {
		t.Error("the import did not add the description that was missing")
	}

	if len(job.Aliases) != 1 || job.Aliases[0] != "backend developer" {
		t.Errorf("aliases are %v, want the imported one added", job.Aliases)
	}
}

/*
 * Caution is only ever added.
 *
 * One source saying a job needs a person watching is enough. A second source
 * silent about it is not a disagreement, and treating it as one would mean an
 * import could quietly declassify work that somebody had marked as needing
 * care.
 */
func TestCautionIsOnlyEverAdded(t *testing.T) {
	db := open(t)

	watched := Occupation{
		ID: "trades.welder", Title: "Welder", Category: "trades",
		Oversight: true, Risk: RiskHigh, CameFrom: FromESCO,
	}

	if _, err := db.Import([]Occupation{watched}, nil); err != nil {
		t.Fatal(err)
	}

	// A second source, ranking higher, that says nothing about oversight.
	quiet := Occupation{
		ID: "trades.welder", Title: "Welder", Category: "trades", CameFrom: FromSeed,
	}

	if _, err := db.Import([]Occupation{quiet}, nil); err != nil {
		t.Fatal(err)
	}

	job, err := db.Occupation("trades.welder")
	if err != nil || job == nil {
		t.Fatalf("reading it back: %v %v", job, err)
	}

	if !job.Oversight {
		t.Error("a source that said nothing about oversight removed it")
	}
}

/*
 * Search works in Bulgarian, which is why it is not written in SQL.
 *
 * SQLite's LOWER and LIKE only know the English alphabet, so "Заварчик" and
 * "заварчик" would be different words to a query written the obvious way —
 * and this assistant is spoken to in two languages.
 */
func TestSearchDoesNotMindCaseInEitherLanguage(t *testing.T) {
	db := open(t)

	jobs, capabilities := someJobs()

	if _, err := db.Import(jobs, capabilities); err != nil {
		t.Fatal(err)
	}

	for _, looking := range []string{"заварчик", "ЗАВАРЧИК", "Заварчик", "welder", "WELDER"} {
		found, err := db.FindOccupations(looking, 10)
		if err != nil {
			t.Fatal(err)
		}

		if len(found) == 0 || found[0].ID != "trades.welder" {
			t.Errorf("looking for %q found %v", looking, found)
		}
	}
}

// The name it is actually called comes before a name it merely contains, and
// both come before a word buried in somebody else's description.
func TestTheClosestNameComesFirst(t *testing.T) {
	db := open(t)

	if _, err := db.Import([]Occupation{
		{ID: "a.one", Title: "Engineer", Category: "a", CameFrom: FromSeed},
		{ID: "a.two", Title: "Backend engineer", Category: "a", CameFrom: FromSeed},
		{ID: "a.three", Title: "Chef", Category: "a",
			Description: "Nothing to do with an engineer at all.", CameFrom: FromSeed},
	}, nil); err != nil {
		t.Fatal(err)
	}

	found, err := db.FindOccupations("engineer", 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 3 {
		t.Fatalf("found %d", len(found))
	}

	if found[0].ID != "a.one" || found[2].ID != "a.three" {
		t.Errorf("the order was %s, %s, %s", found[0].ID, found[1].ID, found[2].ID)
	}
}

// What a job is comes before what it often also involves.
func TestWhatAJobIsComesBeforeWhatItAlsoInvolves(t *testing.T) {
	db := open(t)

	jobs, capabilities := someJobs()

	if _, err := db.Import(jobs, capabilities); err != nil {
		t.Fatal(err)
	}

	needs, err := db.CapabilitiesOf("software.backend_engineer")
	if err != nil {
		t.Fatal(err)
	}

	if len(needs) != 2 {
		t.Fatalf("%d capabilities", len(needs))
	}

	if !needs[0].Essential || needs[0].ID != "api_design" {
		t.Errorf("the first is %+v", needs[0])
	}

	if needs[0].Name != "API design" {
		t.Errorf("the capability was not named: %q", needs[0].Name)
	}
}

// Which jobs call for a capability is a query, not a guess. It is half the
// answer to "who here could do this" — the roster is the other half.
func TestJobsCanBeFoundByWhatTheyCallFor(t *testing.T) {
	db := open(t)

	jobs, capabilities := someJobs()

	if _, err := db.Import(jobs, capabilities); err != nil {
		t.Fatal(err)
	}

	found, err := db.OccupationsNeeding("api_design", 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 || found[0].ID != "software.backend_engineer" {
		t.Errorf("found %v", found)
	}
}
