package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/bootstrap"
	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * The Organisation page in Bulgarian.
 *
 * Most of what it shows comes from the server: the chart that shipped, the
 * specialists and templates, and words like "department", "senior" or
 * "running" sent as values. The page says each in Bulgarian from i18n.js,
 * so every one of them must be there, and the shipped names must match what
 * the Go code calls them, or a rename here would leave the page saying the
 * old name in one language and the new one in the other.
 */
func TestTheOrganisationSaysWhatShippedInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	same := func(key, want string) {
		t.Helper()

		en, bg := strings2(t, js, key)
		if en != want {
			t.Errorf("%s says %q in English, and the program calls it %q", key, en, want)
		}

		if !cyrillic.MatchString(bg) {
			t.Errorf("%s in Bulgarian has no Bulgarian in it: %q", key, bg)
		}
	}

	for _, u := range org.BuiltIn() {
		same("org.unit."+u.Name, u.Title)
		same("org.unitPurpose."+u.Name, u.Purpose)

		for _, seat := range u.Seats {
			same("org.seat."+seat.Name, seat.Title)
		}
	}

	for name, title := range shippedWho() {
		same("org.who."+name, title)
	}

	same("org.proj.where", bootstrap.WhereQuestion)

	values := map[string][]string{
		"org.kind.":   {org.Company, org.Department, org.Team},
		"org.rank.":   {org.Junior, org.Mid, org.Senior, org.Lead},
		"org.state.":  {team.Active, team.Temporary, team.Suspended, team.Retired},
		"org.risk.":   {store.RiskLow, store.RiskMedium, store.RiskHigh, store.RiskCritical},
		"org.status.": {store.Established, store.Emerging, store.Specialised, store.Experimental},
		"org.work.":   {string(capability.Advisory), string(capability.Project), string(capability.Operating)},
		"org.recipe.": {provision.Engine, provision.Runtime, provision.Tool, provision.Part},
		"org.imp.":    {store.ImportRunning, store.ImportDone, store.ImportStopped, store.ImportFailed},
		"org.rk.": {string(orchestrator.Agent), string(orchestrator.Model), string(orchestrator.IDE),
			string(orchestrator.Engine), string(orchestrator.Runtime), string(orchestrator.Integration)},
		"org.rs.": {string(orchestrator.Available), string(orchestrator.Degraded), string(orchestrator.LowCapacity),
			string(orchestrator.LimitNear), string(orchestrator.LimitReached), string(orchestrator.AuthRequired),
			string(orchestrator.NotInstalled), string(orchestrator.NotLicensed), string(orchestrator.NotSupported),
			string(orchestrator.Offline), string(orchestrator.Failed), string(orchestrator.Unknown)},
		"org.ev.": {store.EvidenceCommand, store.EvidenceFile, store.EvidenceCheck, store.EvidenceBuild,
			store.EvidenceTest, store.EvidenceExport, store.EvidenceSmoke, store.EvidenceShot,
			store.EvidenceDocument, store.EvidenceSource, store.EvidenceQuestion, store.EvidenceVersion,
			store.EvidenceAgent, store.EvidenceApproval, store.EvidenceWarning, store.EvidenceNext,
			store.EvidenceDecision, store.EvidenceSwitch, store.EvidenceInstall},
		"org.usesShort.": {team.UsesWork, team.UsesQuick, team.UsesBest, team.UsesReason},
	}

	for prefix, words := range values {
		for _, word := range words {
			if _, bg := strings2(t, js, prefix+word); !cyrillic.MatchString(bg) {
				t.Errorf("%s%s in Bulgarian has no Bulgarian in it: %q", prefix, word, bg)
			}
		}
	}
}

// Every pass an import reports is one the page can name.
func TestEveryImportPassIsNamedOnThePage(t *testing.T) {
	page := readAsset(t, filepath.Join("js", "organisation.js"))
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	files, err := filepath.Glob(filepath.Join("..", "occupations", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("the importers have moved: %v", err)
	}

	stage := regexp.MustCompile(`say\("([^"]+)"`)
	mapped := regexp.MustCompile(`(\w+): '(org\.stage\.\w+)'`)

	keys := map[string]string{}
	for _, m := range mapped.FindAllStringSubmatch(page, -1) {
		keys[m[1]] = m[2]
	}

	found := 0

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		for _, m := range stage.FindAllStringSubmatch(string(raw), -1) {
			found++

			key, ok := keys[strings.ReplaceAll(m[1], " ", "_")]
			if !ok {
				t.Errorf("%s reports the pass %q, which organisation.js does not name", filepath.Base(file), m[1])

				continue
			}

			if en, _ := strings2(t, js, key); en != m[1] {
				t.Errorf("%s says %q in English, and the importer %q", key, en, m[1])
			}
		}
	}

	if found == 0 {
		t.Fatal("found no passes in the importers; has say() been renamed?")
	}
}

// A job's Bulgarian name is ESCO's label, never a search word of the
// built-in jobs.
func TestAJobsBulgarianNameComesFromESCO(t *testing.T) {
	cases := []struct {
		name string
		job  store.Occupation
		want string
	}{
		{
			name: "ESCO, with its Bulgarian label after the English names",
			job: store.Occupation{Title: "Ship pilot", CameFrom: store.FromESCO,
				Aliases: []string{"harbour pilot", "лоцман", "морски лоцман", "лоцман (укр.)"}},
			want: "Лоцман",
		},
		{
			name: "ESCO without a Bulgarian file",
			job:  store.Occupation{Title: "Ship pilot", CameFrom: store.FromESCO, Aliases: []string{"harbour pilot"}},
		},
		{
			name: "a built-in job, whose Bulgarian words are for searching",
			job: store.Occupation{Title: "Data analyst", CameFrom: store.FromSeed,
				Aliases: []string{"анализатор данни"}},
		},
		{
			name: "O*NET",
			job:  store.Occupation{Title: "Welders", CameFrom: store.FromONET, Aliases: []string{"заварчик"}},
		},
	}

	for _, tc := range cases {
		if got := bulgarianTitle(tc.job); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The usual question about a project is named, so the page asks it in its
// own language; any other question is the server's own sentence.
func TestTheWhereQuestionIsNamedForThePage(t *testing.T) {
	for _, tc := range []struct {
		question string
		want     any
	}{
		{bootstrap.WhereQuestion, "where"},
		{"I can't use /nowhere: it is not a folder. " + bootstrap.WhereQuestion, nil},
		{"", nil},
	} {
		body, _ := json.Marshal(bootstrap.Proposal{Question: tc.question})
		out := proposalOut(store.Proposal{ID: 1, Kind: store.ProposeProject, Body: string(body)})

		if out["question_key"] != tc.want {
			t.Errorf("%q: question_key %v, want %v", tc.question, out["question_key"], tc.want)
		}
	}
}

// What shipped is marked as shipped, so the page can say it in Bulgarian,
// and a name its owner changed is not.
func TestTheChartSaysWhichNamesShipped(t *testing.T) {
	ts, _, b := newServer(t)

	read := func() map[string]any {
		t.Helper()

		res, err := http.Get(ts.URL + "/api/organisation")
		if err != nil {
			t.Fatal(err)
		}

		defer res.Body.Close()

		out := map[string]any{}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}

		return out
	}

	unit := func(out map[string]any, name string) map[string]any {
		t.Helper()

		for _, u := range out["units"].([]any) {
			if u := u.(map[string]any); u["name"] == name {
				return u
			}
		}

		t.Fatalf("there is no unit %s", name)

		return nil
	}

	engineering := unit(read(), "engineering")
	if engineering["shipped"] != true || engineering["shipped_purpose"] != true {
		t.Errorf("the shipped Engineering is not marked as shipped: %v", engineering)
	}

	seats := engineering["seats"].([]any)
	if len(seats) == 0 || seats[0].(map[string]any)["shipped"] != true {
		t.Errorf("the shipped seats are not marked as shipped: %v", seats)
	}

	holders := 0

	for _, s := range seats {
		held, _ := s.(map[string]any)["held_by"].([]any)

		for _, h := range held {
			holders++

			if a := h.(map[string]any); a["shipped"] != true {
				t.Errorf("%v, who shipped with the program, is not marked as shipped", a["name"])
			}
		}
	}

	if holders == 0 {
		t.Error("nobody holds a seat in Engineering; the test checks nobody")
	}

	// Renamed by its owner: his words, not a translation of the old ones.
	changed := org.BuiltIn()[1]
	changed.Title = "Workshop"

	if err := org.Save(b.Root, changed); err != nil {
		t.Fatal(err)
	}

	if renamed := unit(read(), changed.Name); renamed["shipped"] == true || renamed["title"] != "Workshop" {
		t.Errorf("a renamed unit still counts as shipped: %v", renamed)
	}
}
