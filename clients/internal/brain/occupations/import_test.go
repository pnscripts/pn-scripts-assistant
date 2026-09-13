package occupations

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * A small ESCO bundle in the shape the real one has.
 *
 * The headers are the release's own, including columns this importer does not
 * read, in an order it does not assume — the point of reading by name is that
 * a release adding a column in the middle changes nothing. A label list is one
 * value a line inside quotes, which is how ESCO writes every list.
 */
var escoFiles = map[string]string{
	"ESCO dataset - v1.2.0 - classification - en - csv/skills_en.csv": "conceptType,conceptUri,skillType,reuseLevel,preferredLabel,altLabels,hiddenLabels,status,modifiedDate,scopeNote,definition,inScheme,description\n" +
		"KnowledgeSkillCompetence,http://data.europa.eu/esco/skill/s1,knowledge,sector-specific,navigation,\"piloting ships\nsea navigation\",,released,2023-01-01,,,x,The science of moving a ship safely.\n" +
		"KnowledgeSkillCompetence,http://data.europa.eu/esco/skill/s2,skill/competence,cross-sector,welding techniques,,,released,2023-01-01,,,x,Joining metal by melting it.\n",

	"ESCO dataset - v1.2.0 - classification - en - csv/occupations_en.csv": "conceptType,conceptUri,iscoGroup,preferredLabel,altLabels,hiddenLabels,status,modifiedDate,regulatedProfessionNote,scopeNote,definition,inScheme,description,code,naceCode\n" +
		"Occupation,http://data.europa.eu/esco/occupation/o1,3152,ship pilot,\"harbour pilot\nmarine pilot\",,released,2023-01-01,,,,x,\"Ship pilots guide vessels, through dangerous waters.\",3152.3,\n" +
		"Occupation,http://data.europa.eu/esco/occupation/o2,7212,welder,\"welding operator\",,released,2023-01-01,,,,x,Welders join metal parts.,7212.1,\n" +
		"Occupation,http://data.europa.eu/esco/occupation/o3,2211,general practitioner,family doctor,,released,2023-01-01,,,,x,Treats patients.,2211.1,\n",

	"ESCO dataset - v1.2.0 - classification - bg - csv/occupations_bg.csv": "conceptType,conceptUri,iscoGroup,preferredLabel,altLabels,hiddenLabels,status,modifiedDate,regulatedProfessionNote,scopeNote,definition,inScheme,description,code,naceCode\n" +
		"Occupation,http://data.europa.eu/esco/occupation/o1,3152,лоцман,морски лоцман,,released,2023-01-01,,,,x,,3152.3,\n",

	"ESCO dataset - v1.2.0 - classification - en - csv/occupationSkillRelations_en.csv": "occupationUri,occupationLabel,relationType,skillType,skillUri,skillLabel\n" +
		"http://data.europa.eu/esco/occupation/o1,ship pilot,essential,knowledge,http://data.europa.eu/esco/skill/s1,navigation\n" +
		"http://data.europa.eu/esco/occupation/o2,welder,optional,skill/competence,http://data.europa.eu/esco/skill/s2,welding techniques\n",
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()

	for name, body := range files {
		path := filepath.Join(dir, name)

		os.MkdirAll(filepath.Dir(path), 0o755)

		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func zipOf(t *testing.T, files map[string]string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "bundle.zip")
	out, _ := os.Create(path)
	w := zip.NewWriter(out)

	for name, body := range files {
		f, _ := w.Create(name)
		f.Write([]byte(body))
	}

	w.Close()
	out.Close()

	return path
}

func seeded(t *testing.T) *store.DB {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	if _, err := Ensure(db); err != nil {
		t.Fatal(err)
	}

	return db
}

/*
 * A bundle imports: jobs, skills, the links between them, and the Bulgarian
 * names — found by a search in either language.
 */
func TestAnESCOBundleImports(t *testing.T) {
	db := seeded(t)

	before, _ := db.HowManyOccupations()

	src, err := OpenSource(writeFiles(t, escoFiles))
	if err != nil {
		t.Fatal(err)
	}

	defer src.Close()

	p, err := ImportESCO(context.Background(), db, src, nil)
	if err != nil {
		t.Fatal(err)
	}

	after, _ := db.HowManyOccupations()

	if after <= before {
		t.Fatalf("nothing new arrived: %d jobs before, %d after (%+v)", before, after, p.So)
	}

	for _, looking := range []string{"ship pilot", "лоцман", "harbour pilot"} {
		found, _ := db.FindOccupations(looking, 3)

		if len(found) == 0 || found[0].Title != "Ship pilot" {
			t.Errorf("searching %q did not find the ship pilot: %+v", looking, found)
		}
	}

	pilot, _ := db.FindOccupations("ship pilot", 1)
	job, _ := db.Occupation(pilot[0].ID)

	if job.ISCO != "3152" || job.CameFrom != store.FromESCO || len(job.Needs) != 1 || !job.Needs[0].Essential {
		t.Errorf("the job did not arrive whole: %+v", job)
	}

	// A doctor arrives needing a person in the loop, whatever it was called.
	gp, _ := db.FindOccupations("general practitioner", 1)

	if len(gp) == 0 || gp[0].Risk != store.RiskCritical || !gp[0].Oversight {
		t.Errorf("a doctor was imported without the care a doctor needs: %+v", gp)
	}
}

/*
 * A job the catalogue already has is enriched, not duplicated — and what
 * shipped keeps its title.
 */
func TestAnImportEnrichesAJobItAlreadyHas(t *testing.T) {
	db := seeded(t)

	seedWelder, _ := db.FindOccupations("welder", 1)

	if len(seedWelder) == 0 {
		t.Skip("the seed has no welder to enrich")
	}

	src, _ := OpenSource(writeFiles(t, escoFiles))
	defer src.Close()

	if _, err := ImportESCO(context.Background(), db, src, nil); err != nil {
		t.Fatal(err)
	}

	all, _ := db.FindOccupations("welder", 10)

	count := 0

	for _, j := range all {
		if strings.EqualFold(j.Title, "welder") {
			count++
		}
	}

	if count != 1 {
		t.Errorf("%d jobs called welder after the import, want the one that shipped", count)
	}

	job, _ := db.Occupation(seedWelder[0].ID)

	if job.CameFrom != store.FromSeed || job.ISCO != "7212" {
		t.Errorf("the shipped welder was replaced rather than enriched: %+v", job)
	}
}

// Running it twice changes nothing the second time, which is also what makes
// a stopped import safe to start again.
func TestImportingTwiceAddsNothing(t *testing.T) {
	db := seeded(t)

	path := zipOf(t, escoFiles)

	for i := 0; i < 2; i++ {
		src, err := OpenSource(path)
		if err != nil {
			t.Fatal(err)
		}

		p, err := ImportESCO(context.Background(), db, src, nil)
		src.Close()

		if err != nil {
			t.Fatal(err)
		}

		if i == 1 && (p.So.Jobs != 0 || p.So.Capabilities != 0 || p.So.Links != 0 || p.So.Filled != 0) {
			t.Errorf("the second run changed things: %+v", p.So)
		}
	}
}

// Stopped, it says so and keeps what it had written; started again, it
// finishes.
func TestAStoppedImportCanBeStartedAgain(t *testing.T) {
	db := seeded(t)

	src, _ := OpenSource(writeFiles(t, escoFiles))
	defer src.Close()

	ctx, cancel := context.WithCancel(context.Background())

	_, err := ImportESCO(ctx, db, src, func(p Progress) {
		if p.Stage == "occupations" {
			cancel()
		}
	})

	if err == nil {
		t.Fatal("a cancelled import reported success")
	}

	if found, _ := db.FindOccupations("лоцман", 1); len(found) != 0 {
		t.Fatal("an import stopped before writing jobs wrote them anyway")
	}

	if _, err := ImportESCO(context.Background(), db, src, nil); err != nil {
		t.Fatal(err)
	}

	if found, _ := db.FindOccupations("лоцман", 1); len(found) == 0 {
		t.Error("starting again did not finish the job")
	}
}

// Something that is not ESCO is refused by name, rather than read as nonsense.
func TestSomethingThatIsNotESCOIsRefused(t *testing.T) {
	db := seeded(t)

	src, _ := OpenSource(writeFiles(t, map[string]string{
		"occupations_en.csv": "name,thing\nx,y\n",
	}))
	defer src.Close()

	_, err := ImportESCO(context.Background(), db, src, nil)

	if err == nil || !strings.Contains(err.Error(), "conceptUri") {
		t.Errorf("a file without ESCO's columns was not refused by name: %v", err)
	}
}

/*
 * O*NET enriches: tasks, knowledge, other titles and hot technologies land on
 * the job the catalogue already has by that name.
 */
func TestAnONETReleaseEnrichesJobs(t *testing.T) {
	db := seeded(t)

	existing, _ := db.FindOccupations("software architect", 1)

	if len(existing) == 0 {
		t.Skip("the seed has no software architect")
	}

	files := map[string]string{
		"db_29_1_text/Occupation Data.txt": "O*NET-SOC Code\tTitle\tDescription\n" +
			"15-1299.08\tSoftware Architect\tDesigns the structure of software systems.\n" +
			"53-5021.03\tShip Pilots\tCommand ships to steer them into and out of harbors.\n",
		"db_29_1_text/Alternate Titles.txt": "O*NET-SOC Code\tTitle\tAlternate Title\tShort Title\tSource(s)\n" +
			"15-1299.08\tSoftware Architect\tSolutions Architect\t\t01\n",
		"db_29_1_text/Task Statements.txt": "O*NET-SOC Code\tTitle\tTask ID\tTask\tTask Type\tIncumbents Responding\tDate\tDomain Source\n" +
			"15-1299.08\tSoftware Architect\t1\tDesign the high-level structure of systems.\tCore\t\t\t\n" +
			"15-1299.08\tSoftware Architect\t2\tAttend conferences.\tSupplemental\t\t\t\n",
		"db_29_1_text/Knowledge.txt": "O*NET-SOC Code\tTitle\tElement ID\tElement Name\tScale ID\tData Value\tN\n" +
			"15-1299.08\tSoftware Architect\t2.C.3.a\tComputers and Electronics\tIM\t4.62\t\n" +
			"15-1299.08\tSoftware Architect\t2.C.4.a\tBiology\tIM\t1.20\t\n",
		"db_29_1_text/Technology Skills.txt": "O*NET-SOC Code\tTitle\tExample\tCommodity Code\tCommodity Title\tHot Technology\tIn Demand\n" +
			"15-1299.08\tSoftware Architect\tPostgreSQL\t1\tDatabase management system software\tY\tY\n" +
			"15-1299.08\tSoftware Architect\tObscure Tool 3.1\t1\tSomething\tN\tN\n",
	}

	src, err := OpenSource(zipOf(t, files))
	if err != nil {
		t.Fatal(err)
	}

	defer src.Close()

	if _, err := ImportONET(context.Background(), db, src, nil); err != nil {
		t.Fatal(err)
	}

	job, _ := db.Occupation(existing[0].ID)

	if len(job.Does) == 0 || !strings.Contains(strings.Join(job.Does, " "), "high-level structure") {
		t.Errorf("the core task did not arrive: %v", job.Does)
	}

	if strings.Contains(strings.Join(job.Does, " "), "conferences") {
		t.Errorf("a supplemental task arrived as a responsibility: %v", job.Does)
	}

	// Added to what shipped, which it keeps; and only what O*NET rates as
	// important arrives.
	knows := strings.Join(job.Knows, "|")

	if !strings.Contains(knows, "Computers and Electronics") || strings.Contains(knows, "Biology") {
		t.Errorf("knowledge was not filtered by importance: %v", job.Knows)
	}

	hasPostgres := false

	for _, need := range job.Needs {
		if need.ID == "postgresql" {
			hasPostgres = true
		}

		if strings.Contains(need.ID, "obscure") {
			t.Errorf("a technology that is not hot became a capability: %s", need.ID)
		}
	}

	if !hasPostgres {
		t.Errorf("the hot technology was not linked to the capability already here: %+v", job.Needs)
	}

	if job.CameFrom != store.FromSeed {
		t.Errorf("the shipped job was taken over by the import: %s", job.CameFrom)
	}
}
