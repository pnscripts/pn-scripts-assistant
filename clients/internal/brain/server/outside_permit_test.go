package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/mcpserve"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Another program's permission questions, through the door (1.2.0 step
 * 5): Claude Code, started with --permission-prompt-tool, hands each
 * question to the assistant's MCP server; it is put to the assistant's
 * person as a waiting question, and only their answer at the desk goes
 * back.
 */

// permitCards is the permission questions that wait at the desk, once
// there are n of them.
type permitCard struct {
	ID        int64  `json:"id"`
	Tool      string `json:"tool"`
	Summary   string `json:"summary"`
	SummaryBG string `json:"summary_bg"`
	AskedBy   string `json:"asked_by"`
}

func permitCards(t *testing.T, url string, n int) []permitCard {
	t.Helper()

	var mine []permitCard

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		var cards []permitCard

		getJSON(t, url+"/api/approvals", &cards)

		mine = mine[:0]

		for _, c := range cards {
			if c.Tool == otherprograms.PermissionTool {
				mine = append(mine, c)
			}
		}

		if len(mine) == n {
			return mine
		}
	}

	t.Fatalf("%d permission questions wait, not %d", len(mine), n)

	return nil
}

// permitWaits sets how long the door holds a permission question.
func permitWaits(t *testing.T, wait time.Duration) {
	t.Helper()

	before := permitWait
	permitWait = wait

	t.Cleanup(func() { permitWait = before })
}

type doorSaid struct {
	code int
	body string
}

func askTheDoor(t *testing.T, url, body string) chan doorSaid {
	t.Helper()

	out := make(chan doorSaid, 1)

	go func() {
		code, said := post(t, url+"/api/outside/ask", body)
		out <- doorSaid{code, said}
	}()

	return out
}

func doorAnswered(t *testing.T, asked chan doorSaid) doorSaid {
	t.Helper()

	select {
	case said := <-asked:
		return said
	case <-time.After(10 * time.Second):
		t.Fatal("the door never answered")
	}

	return doorSaid{}
}

const permitBash = `{"tool":"permission_prompt","arguments":{"tool_name":"Bash","input":{"command":"rm -rf node_modules"},"tool_use_id":"toolu_01"},"client":{"name":"claude-code","version":"2.1.303"}}`

// Off on a new brain, whatever else is allowed; refused in words, with the
// refusal on the record and no card at the desk.
func TestPermissionQuestionsAreOffUntilAllowedAtTheDesk(t *testing.T) {
	ts, db, b := newServer(t)

	for _, other := range []otherprograms.Subject{"", otherprograms.Programs, otherprograms.Ask, otherprograms.Memory} {
		if other != "" {
			if err := b.AllowOutside(other, true); err != nil {
				t.Fatal(err)
			}
		}

		code, body := post(t, ts.URL+"/api/outside/ask", permitBash)
		if code != http.StatusForbidden || !strings.Contains(body, `"outside":"not allowed"`) || !strings.Contains(body, `"subject":"permissions"`) {
			t.Fatalf("before it was allowed (after allowing %q): %d %s", other, code, body)
		}
	}

	permitCards(t, ts.URL, 0)

	var refusals int

	for _, d := range theirs(t, db) {
		if d.Tool != otherprograms.PermissionTool {
			continue
		}

		refusals++

		if d.Outcome != "refused" || d.AskedBy != "claude-code 2.1.303" || d.Summary != "Another program asked whether it may do something" ||
			d.SummaryBG != "Друга програма попита дали може да направи нещо" ||
			!strings.Contains(d.Result, "may not put their permission questions to you") || strings.Contains(d.Asked+d.Result, "node_modules") {
			t.Errorf("the record of a refusal: %+v", d)
		}
	}

	if refusals != 4 {
		t.Errorf("refusals on the record: %d", refusals)
	}

	// The switch is one of those the page is given, off, and the last.
	var state struct {
		Subjects []struct {
			ID      string `json:"id"`
			Allowed bool   `json:"allowed"`
		} `json:"subjects"`
	}

	getJSON(t, ts.URL+"/api/outside", &state)

	if n := len(state.Subjects); n != 4 || state.Subjects[n-1].ID != "permissions" || state.Subjects[n-1].Allowed {
		t.Errorf("the switches the page is given: %+v", state.Subjects)
	}
}

/*
 * Allowed: the question waits at the desk with who asked and what for, the
 * door holds until the person answers there, and their answer is what the
 * program is told. Both answers, and the record of each.
 */
func TestAPermissionQuestionWaitsForItsAnswerAtTheDesk(t *testing.T) {
	ts, db, _ := newServer(t)
	permitWaits(t, time.Minute)

	if code, body := post(t, ts.URL+"/api/outside/allow", `{"subject":"permissions","allowed":true}`); code != http.StatusOK {
		t.Fatalf("allowing: %d %s", code, body)
	}

	for _, c := range []struct{ decision, behavior, ended, outcome, result string }{
		{"approve", "allow", "", "done", "Allowed on its card: the program was told it may."},
		{"deny", "deny", "refused", "refused", "Not allowed on its card: the program was told it may not."},
	} {
		asked := askTheDoor(t, ts.URL, permitBash)

		card := permitCards(t, ts.URL, 1)[0]
		if card.AskedBy != "claude-code 2.1.303" || card.Summary != "Wants to run this command: rm -rf node_modules" ||
			card.SummaryBG != "Иска да изпълни тази команда: rm -rf node_modules" {
			t.Fatalf("the card: %+v", card)
		}

		select {
		case said := <-asked:
			t.Fatalf("answered before anybody decided: %+v", said)
		case <-time.After(100 * time.Millisecond):
		}

		if code, body := post(t, ts.URL+"/api/approvals/"+itoa(card.ID)+"/"+c.decision, `{}`); code != http.StatusOK || strings.Contains(body, `"error"`) {
			t.Fatalf("%s: %d %s", c.decision, code, body)
		}

		said := doorAnswered(t, asked)

		var answer struct {
			Behavior string `json:"behavior"`
			Ended    string `json:"ended"`
		}

		if err := json.Unmarshal([]byte(said.body), &answer); err != nil || said.code != http.StatusOK || answer.Behavior != c.behavior || answer.Ended != c.ended {
			t.Fatalf("after %s the door said: %d %s", c.decision, said.code, said.body)
		}

		permitCards(t, ts.URL, 0)

		d := theirs(t, db)[0]
		if d.Tool != otherprograms.PermissionTool || d.Outcome != c.outcome || d.Result != c.result || d.AskedBy != "claude-code 2.1.303" ||
			d.Summary != card.Summary || !strings.Contains(d.Asked, `"command":"rm -rf node_modules"`) {
			t.Errorf("the record after %s: %+v", c.decision, d)
		}
	}
}

// Nobody answers: the door says no when its wait is over, the card goes,
// and the record says nobody answered.
func TestAPermissionQuestionNobodyAnswersIsANo(t *testing.T) {
	ts, db, b := newServer(t)
	permitWaits(t, 300*time.Millisecond)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	said := doorAnswered(t, askTheDoor(t, ts.URL, permitBash))
	if said.code != http.StatusOK || !strings.Contains(said.body, `"behavior":"deny"`) || !strings.Contains(said.body, `"ended":"unanswered"`) {
		t.Fatalf("unanswered: %d %s", said.code, said.body)
	}

	permitCards(t, ts.URL, 0)

	if d := theirs(t, db)[0]; d.Outcome != "refused" || !strings.HasPrefix(d.Result, "Not allowed: nobody answered in time") {
		t.Errorf("the record: %+v", d)
	}
}

// What cannot be read is not a question: refused, with no card and
// nothing on the record. And the arguments of this question belong to no
// other.
func TestAPermissionQuestionThatCannotBeReadIsNotOne(t *testing.T) {
	ts, db, b := newServer(t)
	permitWaits(t, 300*time.Millisecond)

	for _, s := range otherprograms.Subjects() {
		if err := b.AllowOutside(s, true); err != nil {
			t.Fatal(err)
		}
	}

	before := len(theirs(t, db))

	for _, arguments := range []string{
		`{}`,
		`{"input":{"command":"ls"}}`,
		`{"tool_name":42}`,
		`{"tool_name":""}`,
		`{"tool_name":" \n\u200b"}`,
		`{"tool_name":"Bash","input":"ls"}`,
		`{"tool_name":"Bash","input":["ls"]}`,
		`{"tool_name":"Bash","input":{"command":"ls"},"question":"Anything?"}`,
		`{"tool_name":"Bash","input":{"command":"ls"},"approved":true}`,
		`{"tool_name":"Bash","input":{"command":"ls"},"cut":{"command":0}}`,
		`{"tool_name":"Bash","input":{"command":"ls"},"cut":{"command":-5}}`,
		`{"tool_name":"Bash","input":{"command":"ls"},"cut":{"command":"long"}}`,
		`{"tool_name":"Bash","input":{"command":"ls"},"cut":[9000]}`,
		`{"tool_name":"` + strings.Repeat("x", 1000) + `"}`,
	} {
		body := fmt.Sprintf(`{"tool":"permission_prompt","arguments":%s,"client":{"name":"claude-code"}}`, arguments)

		if code, said := post(t, ts.URL+"/api/outside/ask", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", arguments, code, said)
		}
	}

	for _, body := range []string{
		`{"tool":"list_programs","arguments":{"tool_name":"Bash"},"client":{"name":"claude-code"}}`,
		`{"tool":"ask_assistant","arguments":{"question":"Anything?","input":{}},"client":{"name":"claude-code"}}`,
		`{"tool":"search_memory","arguments":{"query":"x","tool_use_id":"1"},"client":{"name":"claude-code"}}`,
		`{"tool":"search_memory","arguments":{"query":"x","cut":{"query":9}},"client":{"name":"claude-code"}}`,
	} {
		if code, said := post(t, ts.URL+"/api/outside/ask", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", body, code, said)
		}
	}

	permitCards(t, ts.URL, 0)

	if after := len(theirs(t, db)); after != before {
		t.Errorf("unreadable questions on the record: %d", after-before)
	}
}

// A page in a browser can neither put a permission question nor be told
// an answer, not even the assistant's own page.
func TestAWebPageCannotPutAPermissionQuestion(t *testing.T) {
	ts, db, b := newServer(t)
	permitWaits(t, 300*time.Millisecond)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	before := len(theirs(t, db))

	for _, headers := range [][]string{
		{"Origin", "https://example.org"},
		{"Sec-Fetch-Site", "cross-site"},
		{"Sec-Fetch-Site", "same-origin"},
		{"Origin", ts.URL},
	} {
		if code, said := post(t, ts.URL+"/api/outside/ask", permitBash, headers...); code != http.StatusForbidden {
			t.Errorf("%v: %d %s", headers, code, said)
		}
	}

	permitCards(t, ts.URL, 0)

	if len(theirs(t, db)) != before {
		t.Error("a page's permission question reached the record")
	}
}

// The questions a program wants to ask its user itself are not permission
// questions: refused at once with no card, and said on the record.
func TestAQuestionOfTheProgramsOwnIsNotPassedOn(t *testing.T) {
	ts, db, b := newServer(t)
	permitWaits(t, time.Minute)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	code, said := post(t, ts.URL+"/api/outside/ask",
		`{"tool":"permission_prompt","arguments":{"tool_name":"AskUserQuestion","input":{"questions":[{"question":"Which?"}]}},"client":{"name":"claude-code"}}`)
	if code != http.StatusOK || !strings.Contains(said, `"behavior":"deny"`) || !strings.Contains(said, `"ended":"not a permission"`) {
		t.Fatalf("a question of its own: %d %s", code, said)
	}

	permitCards(t, ts.URL, 0)

	if d := theirs(t, db)[0]; d.Outcome != "refused" || !strings.Contains(d.Result, "permission questions only") {
		t.Errorf("the record: %+v", d)
	}
}

// Only a few wait at once: one more is told so, and nothing is added to
// the desk. They count toward the minute's number like every question.
func TestOneMorePermissionQuestionThanMayWaitIsToldSo(t *testing.T) {
	ts, db, b := newServer(t)
	permitWaits(t, 2*time.Second)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	var all []chan doorSaid
	for i := 0; i < otherprograms.MostPermissionsWaiting; i++ {
		all = append(all, askTheDoor(t, ts.URL, permitBash))
	}

	permitCards(t, ts.URL, otherprograms.MostPermissionsWaiting)

	code, said := post(t, ts.URL+"/api/outside/ask", permitBash)
	if code != http.StatusServiceUnavailable || !strings.Contains(said, `"outside":"permits full"`) {
		t.Fatalf("one more: %d %s", code, said)
	}

	permitCards(t, ts.URL, otherprograms.MostPermissionsWaiting)

	// On the record, like every refusal.
	if d := theirs(t, db)[0]; d.Outcome != "refused" || d.Result != "Not answered: 3 permission questions were waiting already, and no more wait at once." ||
		d.AskedBy != "claude-code 2.1.303" {
		t.Errorf("the record of one more than may wait: %+v", d)
	}

	for _, each := range all {
		doorAnswered(t, each)
	}
}

// Turned off at the desk while one waits: the program is told no at once,
// and the change is on the record in its own words.
func TestTurningPermissionQuestionsOffAtTheDeskAnswersTheOnesThatWait(t *testing.T) {
	ts, db, _ := newServer(t)
	permitWaits(t, time.Minute)

	post(t, ts.URL+"/api/outside/allow", `{"subject":"permissions","allowed":true}`)

	asked := askTheDoor(t, ts.URL, permitBash)

	permitCards(t, ts.URL, 1)

	if code, body := post(t, ts.URL+"/api/outside/allow", `{"subject":"permissions","allowed":false}`); code != http.StatusOK {
		t.Fatalf("turning it off: %d %s", code, body)
	}

	if said := doorAnswered(t, asked); said.code != http.StatusOK || !strings.Contains(said.body, `"ended":"taken back"`) {
		t.Fatalf("after it was turned off: %d %s", said.code, said.body)
	}

	permitCards(t, ts.URL, 0)

	deeds, err := db.Deeds(store.DeedFilter{})
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, d := range deeds {
		if d.Tool == "other_programs_switch" {
			got = append(got, d.Summary+" / "+d.SummaryBG)
		}
	}

	want := []string{
		"Other programs may no longer put their permission questions to you / Другите програми вече не могат да ви задават своите въпроси за разрешение",
		"Other programs may now put their permission questions to you / Другите програми вече могат да ви задават своите въпроси за разрешение",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the record of the switch:\n%s", strings.Join(got, "\n"))
	}
}

/*
 * The whole way, as Claude Code goes it: its permission tool is the MCP
 * server's permission_prompt, which answers in the one form Claude Code
 * reads: a line of JSON, "allow" with the input given back as it came, or
 * "deny" with why. Every refusal on the way is a deny in words, not an
 * error.
 */
func TestThePermissionToolAnswersAsClaudeCodeReadsIt(t *testing.T) {
	ts, _, _ := newServer(t)
	permitWaits(t, time.Minute)

	run := func(arguments string) map[string]json.RawMessage {
		t.Helper()

		for _, each := range mcpserve.New(ts.URL, "test").Tools {
			if each.Name != mcpserve.PermissionTool {
				continue
			}

			text, err := each.Run(t.Context(), json.RawMessage(arguments), mcpserve.Caller{Name: "claude-code", Version: "2.1.303"})
			if err != nil {
				t.Fatalf("the permission tool failed instead of answering: %v", err)
			}

			var answer map[string]json.RawMessage
			if err := json.Unmarshal([]byte(text), &answer); err != nil {
				t.Fatalf("not one JSON object: %q", text)
			}

			return answer
		}

		t.Fatal("the MCP server has no permission tool")

		return nil
	}

	const bash = `{"tool_name":"Bash","input":{"command":"rm -rf node_modules","description":"Tidy"},"tool_use_id":"toolu_01","something_new":true}`

	// Off: a deny that says who can allow it and where.
	off := run(bash)
	if string(off["behavior"]) != `"deny"` || !strings.Contains(string(off["message"]), "has not allowed other programs to put their permission questions to them") ||
		!strings.Contains(string(off["message"]), "What other programs may ask") {
		t.Fatalf("with the switch off: %s", off)
	}

	post(t, ts.URL+"/api/outside/allow", `{"subject":"permissions","allowed":true}`)

	decided := func(decision string) map[string]json.RawMessage {
		t.Helper()

		done := make(chan map[string]json.RawMessage, 1)

		go func() { done <- run(bash) }()

		card := permitCards(t, ts.URL, 1)[0]

		if code, body := post(t, ts.URL+"/api/approvals/"+itoa(card.ID)+"/"+decision, `{}`); code != http.StatusOK {
			t.Fatalf("%s: %d %s", decision, code, body)
		}

		select {
		case answer := <-done:
			return answer
		case <-time.After(10 * time.Second):
			t.Fatal("the tool never answered")
		}

		return nil
	}

	yes := decided("approve")
	if string(yes["behavior"]) != `"allow"` || string(yes["updatedInput"]) != `{"command":"rm -rf node_modules","description":"Tidy"}` || len(yes) != 2 {
		t.Fatalf("after a yes: %s", yes)
	}

	no := decided("deny")
	if string(no["behavior"]) != `"deny"` || !strings.Contains(string(no["message"]), "The answer given on its card") || len(no) != 2 {
		t.Fatalf("after a no: %s", no)
	}

	// Nothing to ask about: a deny, and no card.
	for _, arguments := range []string{`{}`, `{"tool_name":" "}`, `not json`, `{"tool_name":"Bash","input":"ls"}`} {
		if got := run(arguments); string(got["behavior"]) != `"deny"` || len(got["message"]) < 20 {
			t.Errorf("%s: %s", arguments, got)
		}
	}

	permitCards(t, ts.URL, 0)

	// Nothing running at the address: a deny that says so.
	for _, each := range mcpserve.New("http://127.0.0.1:1", "test").Tools {
		if each.Name == mcpserve.PermissionTool {
			text, err := each.Run(t.Context(), json.RawMessage(bash), mcpserve.Caller{Name: "claude-code"})
			if err != nil || !strings.Contains(text, `"behavior":"deny"`) || !strings.Contains(text, "is not running") {
				t.Errorf("with no assistant running: %q %v", text, err)
			}
		}
	}
}

// The door answers a permission question before the MCP server gives up
// on it, and both well before Claude Code gives up on a silent tool (half
// an hour).
func TestTheDoorAnswersAPermissionQuestionBeforeTheMCPServerGivesUp(t *testing.T) {
	if permitWait >= mcpserve.PermitPatientFor || mcpserve.PermitPatientFor >= 30*time.Minute || permitWait+time.Minute > Thinking {
		t.Errorf("the door holds a permission question %s, the MCP server waits %s", permitWait, mcpserve.PermitPatientFor)
	}

	if mcpserve.PermissionTool != otherprograms.PermissionTool {
		t.Errorf("the MCP server calls it %q, the door %q", mcpserve.PermissionTool, otherprograms.PermissionTool)
	}
}

// A long input is shown cut and said to be cut, and given back whole with
// a yes.
func TestALongPermissionQuestionIsShownCutAndGivenBackWhole(t *testing.T) {
	ts, _, b := newServer(t)
	permitWaits(t, time.Minute)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	content := strings.Repeat("я", 9000)

	arguments, _ := json.Marshal(map[string]any{"tool_name": "Write", "input": map[string]string{"file_path": "/home/you/long.txt", "content": content}})

	done := make(chan string, 1)

	go func() {
		for _, each := range mcpserve.New(ts.URL, "test").Tools {
			if each.Name == mcpserve.PermissionTool {
				text, _ := each.Run(t.Context(), arguments, mcpserve.Caller{Name: "claude-code"})
				done <- text
			}
		}
	}()

	card := permitCards(t, ts.URL, 1)[0]
	if card.Summary != "Wants to write this file: /home/you/long.txt (also given, not shown here: content (length 9000))" {
		t.Fatalf("the card: %+v", card)
	}

	var kept struct {
		Arguments string `json:"arguments"`
	}

	var cards []json.RawMessage

	getJSON(t, ts.URL+"/api/approvals", &cards)

	if err := json.Unmarshal(cards[0], &kept); err != nil || !strings.Contains(kept.Arguments, `"cut":{"content":9000}`) || len(kept.Arguments) > 8000 {
		t.Fatalf("what is kept of a long input (%d bytes): %v", len(kept.Arguments), err)
	}

	post(t, ts.URL+"/api/approvals/"+itoa(card.ID)+"/approve", `{}`)

	select {
	case text := <-done:
		var answer struct {
			Behavior     string            `json:"behavior"`
			UpdatedInput map[string]string `json:"updatedInput"`
		}

		if err := json.Unmarshal([]byte(text), &answer); err != nil || answer.Behavior != "allow" || answer.UpdatedInput["content"] != content {
			t.Fatalf("the input was not given back whole: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the tool never answered")
	}
}

/*
 * The known limit, held so that the words stay true to it: a program on
 * this computer that posts to the card's route with no page's headers is
 * taken as the window is (as on every route of this address). So the
 * record of a permission question says "on its card", never "you".
 */
func TestTheRecordSaysOnItsCardBecauseAProgramCanReachTheCardsRoute(t *testing.T) {
	ts, db, b := newServer(t)
	permitWaits(t, time.Minute)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	asked := askTheDoor(t, ts.URL, permitBash)
	card := permitCards(t, ts.URL, 1)[0]

	// No Origin, no Sec-Fetch-Site: not a page.
	if code, body := post(t, ts.URL+"/api/approvals/"+itoa(card.ID)+"/approve", `{}`); code != http.StatusOK {
		t.Fatalf("the route: %d %s", code, body)
	}

	doorAnswered(t, asked)

	d := theirs(t, db)[0]
	if said := strings.ToLower(d.Request + " " + d.Result); strings.Contains(said, "you") || !strings.Contains(said, "on its card") {
		t.Errorf("the record claims who answered: %q / %q", d.Request, d.Result)
	}

	for _, name := range []string{"asked-by-other-programs.md", "asked-by-other-programs.bg.md"} {
		guide := readGuide(t, name)

		if !strings.Contains(guide, "Known limits") && !strings.Contains(guide, "Известни ограничения") {
			t.Errorf("%s does not point to the known limits", name)
		}
	}
}
