package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/mcpserve"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The door other programs ask through (POST /api/outside/ask), the switch
 * at the desk for what they may ask (/api/outside), and the record of who
 * asked.
 */

// post sends a body as it is, with headers, and returns the status and
// the answer as it came.
func post(t *testing.T, url, body string, headers ...string) (int, string) {
	t.Helper()

	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, string(raw)
}

func twoPrograms(t *testing.T, db *store.DB) {
	t.Helper()

	now := time.Now().UTC()

	save, _ := programs.Reconcile(programs.Deb, nil, []programs.Program{
		{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10", Path: "/usr/bin/gimp-private-path"},
	}, now)
	if err := db.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	apps, _ := programs.Reconcile(programs.App, nil, []programs.Program{{Kind: programs.App, ID: "gimp.desktop", Name: "GIMP", Exec: "gimp --private-flag"}}, now)
	if err := db.SavePrograms(apps); err != nil {
		t.Fatal(err)
	}
}

type outsideState struct {
	Subjects []struct {
		ID      string  `json:"id"`
		Allowed bool    `json:"allowed"`
		Since   *string `json:"since"`
	} `json:"subjects"`
	Recent []struct {
		By      string `json:"by"`
		Tool    string `json:"tool"`
		Outcome string `json:"outcome"`
		At      string `json:"at"`
	} `json:"recent"`
}

func theirs(t *testing.T, db *store.DB) []store.Deed {
	t.Helper()

	deeds, err := db.Deeds(store.DeedFilter{Outside: true})
	if err != nil {
		t.Fatal(err)
	}

	return deeds
}

func TestWhichProgramsAreInstalledIsNotToldUntilAllowedAtTheDesk(t *testing.T) {
	ts, db, _ := newServer(t)
	twoPrograms(t, db)

	ask := `{"tool":"list_programs","arguments":{},"client":{"name":"claude-code","version":"2.1.0"}}`

	// Off on a new brain: refused, and nothing about the computer in the refusal.
	code, body := post(t, ts.URL+"/api/outside/ask", ask)
	if code != http.StatusForbidden || !strings.Contains(body, `"outside":"not allowed"`) || !strings.Contains(body, `"subject":"programs"`) ||
		strings.Contains(strings.ToLower(body), "gimp") {
		t.Fatalf("before it was allowed: %d %s", code, body)
	}

	// The refusal is on the record, with who asked, in both languages.
	deeds := theirs(t, db)
	if len(deeds) != 1 || deeds[0].AskedBy != "claude-code 2.1.0" || deeds[0].Outcome != "refused" || deeds[0].Tool != "list_programs" ||
		deeds[0].Summary != "Another program asked which programs are installed" ||
		deeds[0].SummaryBG != "Друга програма попита кои програми са инсталирани" ||
		!strings.Contains(deeds[0].Result, "may not ask which programs are installed") || deeds[0].Changes || deeds[0].Leaves != "" {
		t.Fatalf("the record of the refusal: %+v", deeds)
	}

	var state outsideState

	getJSON(t, ts.URL+"/api/outside", &state)
	if len(state.Subjects) != 1 || state.Subjects[0].ID != "programs" || state.Subjects[0].Allowed || state.Subjects[0].Since != nil {
		t.Fatalf("the switch on a new brain: %+v", state.Subjects)
	}

	if len(state.Recent) != 1 || state.Recent[0].By != "claude-code 2.1.0" || state.Recent[0].Outcome != "refused" || state.Recent[0].At == "" {
		t.Errorf("the last questions: %+v", state.Recent)
	}

	// Allowed at the desk.
	code, body = post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":true}`)
	if code != http.StatusOK {
		t.Fatalf("allowing: %d %s", code, body)
	}

	json.Unmarshal([]byte(body), &state)
	if len(state.Subjects) != 1 || !state.Subjects[0].Allowed || state.Subjects[0].Since == nil {
		t.Fatalf("the switch after allowing: %s", body)
	}

	code, body = post(t, ts.URL+"/api/outside/ask", ask)
	if code != http.StatusOK {
		t.Fatalf("after it was allowed: %d %s", code, body)
	}

	var told struct {
		Programs []map[string]any `json:"programs"`
	}

	if err := json.Unmarshal([]byte(body), &told); err != nil || len(told.Programs) != 2 {
		t.Fatalf("the programs: %v %s", err, body)
	}

	// Name, version, source and whether it is here: nothing else of a
	// program goes to another program (no path, no command line).
	for _, p := range told.Programs {
		for key := range p {
			switch key {
			case "kind", "id", "name", "version", "present", "removed_at":
			default:
				t.Errorf("another program is told a program's %s: %v", key, p)
			}
		}
	}

	if strings.Contains(body, "private") {
		t.Errorf("a path or a command line was told: %s", body)
	}

	// One source alone, and a source there is no such thing as.
	code, body = post(t, ts.URL+"/api/outside/ask", `{"tool":"list_programs","arguments":{"kind":"app"},"client":{"name":"claude-code"}}`)
	if code != http.StatusOK || !strings.Contains(body, "GIMP") || strings.Contains(body, `"deb"`) {
		t.Errorf("one source: %d %s", code, body)
	}

	if code, body = post(t, ts.URL+"/api/outside/ask", `{"tool":"list_programs","arguments":{"kind":"rpm"},"client":{"name":"claude-code"}}`); code != http.StatusBadRequest ||
		!strings.Contains(body, "kind is app, deb") {
		t.Errorf("an unknown source: %d %s", code, body)
	}

	deeds = theirs(t, db)
	if len(deeds) != 3 || deeds[0].Outcome != "done" || deeds[0].AskedBy != "claude-code" || deeds[0].Asked != "kind: app" ||
		deeds[1].Outcome != "done" || deeds[1].AskedBy != "claude-code 2.1.0" || deeds[1].Result != "Programs I told it about: 2." {
		t.Errorf("the record after allowing: %+v", deeds)
	}

	// Taken back: refused again at once.
	if code, _ = post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":false}`); code != http.StatusOK {
		t.Fatalf("taking it back: %d", code)
	}

	if code, _ = post(t, ts.URL+"/api/outside/ask", ask); code != http.StatusForbidden {
		t.Errorf("after it was taken back: %d", code)
	}
}

func TestWhetherItIsRunningIsAnsweredAndWrittenDown(t *testing.T) {
	ts, db, _ := newServer(t)

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status","client":{"name":"Cursor","version":"3.0"}}`)
	if code != http.StatusOK || !strings.Contains(body, `"noted":true`) {
		t.Fatalf("%d %s", code, body)
	}

	deeds := theirs(t, db)
	if len(deeds) != 1 || deeds[0].AskedBy != "Cursor 3.0" || deeds[0].Tool != "assistant_status" || deeds[0].Outcome != "done" ||
		deeds[0].SummaryBG != "Друга програма попита дали работя" {
		t.Errorf("%+v", deeds)
	}
}

// The name is the program's own word, so it is made safe before it is
// kept, and a program that gives none is still written down.
func TestTheNameAProgramGaveIsKeptSafeToShow(t *testing.T) {
	ts, db, _ := newServer(t)

	for _, client := range []string{
		`{"name":"helper\nApproved by you‮","version":"1"}`,
		`{"name":"","version":""}`,
		`{}`,
	} {
		if code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status","client":`+client+`}`); code != http.StatusOK {
			t.Fatalf("%s: %d %s", client, code, body)
		}
	}

	// And with no client at all.
	if code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status"}`); code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}

	deeds := theirs(t, db)
	if len(deeds) != 4 {
		t.Fatalf("%+v", deeds)
	}

	if deeds[3].AskedBy != "helper Approved by you 1" {
		t.Errorf("a name with a line break and a direction mark: %q", deeds[3].AskedBy)
	}

	for _, d := range deeds[:3] {
		if d.AskedBy != otherprograms.Unnamed {
			t.Errorf("a program that gave no name: %q", d.AskedBy)
		}
	}
}

func TestTheDoorTakesOnlyTheQuestionsThereAre(t *testing.T) {
	ts, db, b := newServer(t)

	if err := b.AllowOutside(otherprograms.Programs, true); err != nil {
		t.Fatal(err)
	}

	for name, body := range map[string]string{
		"a tool of the assistant's own": `{"tool":"read_file","arguments":{"path":"/etc/passwd"},"client":{"name":"x"}}`,
		"no tool":                       `{"client":{"name":"x"}}`,
		"something extra":               `{"tool":"assistant_status","client":{"name":"x"},"as":"owner"}`,
		"an argument there is not":      `{"tool":"list_programs","arguments":{"kind":"deb","path":"/"},"client":{"name":"x"}}`,
		"arguments for the status":      `{"tool":"assistant_status","arguments":{"kind":"deb"},"client":{"name":"x"}}`,
		"not JSON":                      `tool=assistant_status`,
		"two requests in one":           `{"tool":"assistant_status"}{"tool":"list_programs"}`,
		"far too long":                  `{"tool":"assistant_status","client":{"name":"` + strings.Repeat("a", 100_000) + `"}}`,
	} {
		if code, said := post(t, ts.URL+"/api/outside/ask", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, said)
		}
	}

	if deeds := theirs(t, db); len(deeds) != 0 {
		t.Errorf("something that is not a question was written down as one: %+v", deeds)
	}

	// Asking is never a GET: it writes the record.
	resp, err := http.Get(ts.URL + "/api/outside/ask")
	if err != nil {
		t.Fatal(err)
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET at the door: %d", resp.StatusCode)
	}

	if deeds := theirs(t, db); len(deeds) != 0 {
		t.Errorf("a GET was written down as a question: %+v", deeds)
	}

	for name, body := range map[string]string{
		"a switch there is not": `{"subject":"memory","allowed":true}`,
		"no switch":             `{"allowed":true}`,
		"no answer":             `{"subject":"programs"}`,
		"something extra":       `{"subject":"programs","allowed":true,"for":"everybody"}`,
	} {
		if code, said := post(t, ts.URL+"/api/outside/allow", body); code != http.StatusBadRequest {
			t.Errorf("the switch, %s: %d %s", name, code, said)
		}
	}
}

// A web page open in a browser on this computer can reach 127.0.0.1 too:
// it can neither ask as "another program" nor flip the switch.
func TestAWebPageCanNeitherAskNorFlipTheSwitch(t *testing.T) {
	ts, db, b := newServer(t)
	twoPrograms(t, db)

	for _, path := range []string{"/api/outside/ask", "/api/outside/allow"} {
		body := `{"tool":"list_programs","client":{"name":"page"}}`
		if strings.HasSuffix(path, "allow") {
			body = `{"subject":"programs","allowed":true}`
		}

		for _, headers := range [][]string{
			{"Origin", "http://evil.test"},
			{"Sec-Fetch-Site", "cross-site"},
			{"Sec-Fetch-Site", "same-site"},
			{"Origin", "null"},
		} {
			if code, said := post(t, ts.URL+path, body, headers...); code != http.StatusForbidden {
				t.Errorf("%s from another site (%v): %d %s", path, headers, code, said)
			}
		}
	}

	if b.OutsideMay(otherprograms.Programs) {
		t.Error("a web page flipped the switch")
	}

	// The door is for programs, which say nothing about pages: even the
	// assistant's own page cannot ask as "another program".
	own := strings.TrimPrefix(ts.URL, "http://")

	for _, headers := range [][]string{
		{"Origin", "http://" + own},
		{"Sec-Fetch-Site", "same-origin"},
	} {
		if code, said := post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status","client":{"name":"page"}}`, headers...); code != http.StatusForbidden {
			t.Errorf("asking from the assistant's own page (%v): %d %s", headers, code, said)
		}
	}

	// Its own page is where the switch is flipped.
	if code, said := post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":true}`,
		"Origin", "http://"+own, "Sec-Fetch-Site", "same-origin"); code != http.StatusOK || !b.OutsideMay(otherprograms.Programs) {
		t.Errorf("the switch from its own page: %d %s", code, said)
	}

	b.AllowOutside(otherprograms.Programs, false)

	if deeds := theirs(t, db); len(deeds) != 0 {
		t.Errorf("a web page asked as another program: %+v", deeds)
	}
}

func TestTheDoorIsShutBeforeTheTerms(t *testing.T) {
	ts, b := notAgreed(t)

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status","client":{"name":"x"}}`)
	if code != http.StatusForbidden || !strings.Contains(body, `"terms":"not accepted"`) {
		t.Errorf("asking before the terms: %d %s", code, body)
	}

	if code, _ = post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":true}`); code != http.StatusForbidden {
		t.Errorf("the switch before the terms: %d", code)
	}

	if b.OutsideMay(otherprograms.Programs) {
		t.Error("allowed before the terms")
	}
}

// A record that cannot be written is a question that is not answered.
func TestNoRecordNoAnswer(t *testing.T) {
	ts, db, b := newServer(t)
	twoPrograms(t, db)

	if err := b.AllowOutside(otherprograms.Programs, true); err != nil {
		t.Fatal(err)
	}

	db.Close()

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"list_programs","client":{"name":"x"}}`)
	if code == http.StatusOK || strings.Contains(strings.ToLower(body), "gimp") {
		t.Errorf("answered without a record: %d %s", code, body)
	}
}

/*
 * The MCP server's own tools through the real door: refused in words a
 * model can pass on while the switch is off, answered once it is on, and
 * the name the program gave is what the record shows.
 */
func TestTheMCPServerAsksThroughTheDoorAndIsWrittenDown(t *testing.T) {
	ts, db, _ := newServer(t)
	twoPrograms(t, db)

	run := func(tool, arguments string) (string, error) {
		for _, each := range mcpserve.New(ts.URL, "test").Tools {
			if each.Name == tool {
				return each.Run(t.Context(), json.RawMessage(arguments), mcpserve.Caller{Name: "claude-code", Version: "2.1.0"})
			}
		}

		t.Fatalf("no tool %s", tool)

		return "", nil
	}

	text, err := run("list_programs", `{}`)
	if err == nil || text != "" || !strings.Contains(err.Error(), "has not allowed other programs to ask which programs are installed") ||
		!strings.Contains(err.Error(), "What other programs may ask") || strings.Contains(strings.ToLower(err.Error()), "gimp") {
		t.Fatalf("while the switch is off: %q %v", text, err)
	}

	post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":true}`)

	text, err = run("list_programs", `{"search":"gimp"}`)
	if err != nil || !strings.Contains(text, "- GIMP (app, gimp.desktop)") || !strings.Contains(text, "- gimp 2.10 (deb)") {
		t.Fatalf("once it is on: %q %v", text, err)
	}

	if text, err = run("assistant_status", `{}`); err != nil || !strings.Contains(text, "is running on this computer") {
		t.Fatalf("status: %q %v", text, err)
	}

	deeds := theirs(t, db)
	if len(deeds) != 3 {
		t.Fatalf("the record: %+v", deeds)
	}

	for i, want := range []struct{ tool, outcome string }{{"assistant_status", "done"}, {"list_programs", "done"}, {"list_programs", "refused"}} {
		if deeds[i].Tool != want.tool || deeds[i].Outcome != want.outcome || deeds[i].AskedBy != "claude-code 2.1.0" {
			t.Errorf("record %d: %+v", i, deeds[i])
		}
	}

	// The record page is served who asked.
	var served struct {
		Deeds []struct {
			Tool    string `json:"tool"`
			AskedBy string `json:"asked_by"`
		} `json:"deeds"`
	}

	// Three questions with who asked, and the change of the switch between them.
	getJSON(t, ts.URL+"/api/deeds", &served)
	if len(served.Deeds) != 4 || served.Deeds[0].AskedBy != "claude-code 2.1.0" ||
		served.Deeds[2].Tool != "other_programs_switch" || served.Deeds[2].AskedBy != "" {
		t.Errorf("the record as the page is served it: %+v", served)
	}
}

// The questions the door knows are the tools the MCP server offers: one
// added to either without the other fails here.
func TestTheDoorAndTheMCPServerOfferTheSameQuestions(t *testing.T) {
	var offered []string
	for _, each := range mcpserve.New("http://127.0.0.1:1", "test").Tools {
		offered = append(offered, each.Name)
	}

	if a, b := strings.Join(offered, " "), strings.Join(otherprograms.Tools(), " "); a != b {
		t.Errorf("the MCP server offers %q, the door knows %q", a, b)
	}
}

// A waiting question another program asked for says so on its card.
func TestAWaitingQuestionSaysWhichProgramAskedForIt(t *testing.T) {
	ts, db, _ := newServer(t)

	id, _ := db.RecordInvocation(0, "write_file", `{}`, "Always asked (another program asked for it): Write notes.txt", "mutating")
	if err := db.InvocationAskedBy(id, "claude-code 2.1.0"); err != nil {
		t.Fatal(err)
	}

	db.RecordInvocation(0, "write_file", `{}`, "Write other.txt", "mutating")

	var cards []struct {
		ID      int64  `json:"id"`
		AskedBy string `json:"asked_by"`
	}

	getJSON(t, ts.URL+"/api/approvals", &cards)
	if len(cards) != 2 || cards[0].AskedBy != "claude-code 2.1.0" || cards[1].AskedBy != "" {
		t.Fatalf("the cards: %+v", cards)
	}

	// Rejected: the record of the no says who had asked.
	if code, body := post(t, ts.URL+"/api/approvals/"+itoa(id)+"/deny", `{}`); code != http.StatusOK {
		t.Fatalf("rejecting: %d %s", code, body)
	}

	deeds := theirs(t, db)
	if len(deeds) != 1 || deeds[0].AskedBy != "claude-code 2.1.0" || deeds[0].Outcome != "refused" {
		t.Errorf("the record of the rejection: %+v", deeds)
	}
}

/*
 * Every change of the switch is on the record, whoever made it: the
 * assistant's own page, or any program on this computer that posts to the
 * same address (nothing can tell the two apart, which is why it is
 * written down rather than promised impossible).
 */
func TestEveryChangeOfTheSwitchIsOnTheRecord(t *testing.T) {
	ts, db, _ := newServer(t)

	post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":true}`)
	post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":true}`)
	post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":false}`)

	deeds, err := db.Deeds(store.DeedFilter{})
	if err != nil {
		t.Fatal(err)
	}

	// Allowed twice is one change.
	if len(deeds) != 2 {
		t.Fatalf("the record of the switch: %+v", deeds)
	}

	off, on := deeds[0], deeds[1]

	if on.Tool != "other_programs_switch" || !on.Changes || on.Outcome != "done" || on.AskedBy != "" ||
		on.Summary != "Other programs may now ask which programs are installed" ||
		on.SummaryBG != "Другите програми вече може да питат кои програми са инсталирани" ||
		on.Specialist != "changed at this computer" {
		t.Errorf("allowing, on the record: %+v", on)
	}

	if off.Summary != "Other programs may no longer ask which programs are installed" ||
		off.SummaryBG != "Другите програми вече не може да питат кои програми са инсталирани" {
		t.Errorf("taking it back, on the record: %+v", off)
	}

	// The page names it in both languages.
	names := readAsset(t, "js/i18n.js")
	if !strings.Contains(names, "other_programs_switch: ['What other programs may ask', 'Какво могат да питат другите програми']") {
		t.Error("the record's name for the switch is not in both languages")
	}
}

// The word for a program with no name is one word in both places.
func TestNoNameIsOneWord(t *testing.T) {
	if otherprograms.Unnamed != store.NoName {
		t.Errorf("%q and %q", otherprograms.Unnamed, store.NoName)
	}
}

// A name that looks like the store's sealed text neither breaks the record
// nor the card with the switch.
func TestANameThatLooksSealedIsJustAName(t *testing.T) {
	ts, db, _ := newServer(t)

	for _, name := range []string{"pnsealed1:x", "pnsealed1:"} {
		if code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status","client":{"name":"`+name+`"}}`); code != http.StatusOK {
			t.Fatalf("%s: %d %s", name, code, body)
		}
	}

	var state outsideState

	getJSON(t, ts.URL+"/api/outside", &state)
	if len(state.Subjects) != 1 || len(state.Recent) != 2 || state.Recent[0].By != otherprograms.Unnamed || state.Recent[1].By != "x" {
		t.Errorf("the card after such a name: %+v", state)
	}

	if deeds := theirs(t, db); len(deeds) != 2 {
		t.Errorf("the record after such a name: %+v", deeds)
	}
}
