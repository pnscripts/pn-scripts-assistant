package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/mcpserve"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/store"
)

// readGuide is one of the guides the program ships, as it ships it.
func readGuide(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "seed", "pack", "guides", name))
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

/*
 * A question to the assistant itself and a search of its memory, through
 * the door other programs ask at (1.2.0 step 4): each behind its own
 * switch, bounded, on the record, and no more of them than a fixed number
 * a minute.
 */

// onThisComputer is the model on this computer, standing in: it answers
// with said (or fails), waits on hold when there is one, and turns words
// into something to search the memory by.
type onThisComputer struct {
	mu    sync.Mutex
	asked []llm.Request
	said  string
	fail  error
	hold  chan struct{}
}

func (*onThisComputer) Name() string                   { return llm.Local }
func (*onThisComputer) Available(context.Context) bool { return true }
func (*onThisComputer) EmbedModel() string             { return "fixed" }

func (*onThisComputer) Embed(_ context.Context, text string) ([]float32, error) {
	sum := sha256.Sum256([]byte(text))
	vec := make([]float32, 8)

	for i := range vec {
		vec[i] = float32(sum[i]) - 128
	}

	return vec, nil
}

func (m *onThisComputer) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	m.mu.Lock()
	m.asked = append(m.asked, req)
	hold := m.hold
	m.mu.Unlock()

	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		}
	}

	return llm.Response{Content: m.said}, m.fail
}

// remembered puts facts in the memory where a search for query finds them.
func remembered(t *testing.T, b *brain.Brain, m *onThisComputer, query string, facts ...string) {
	t.Helper()

	vec, _ := m.Embed(context.Background(), query)

	for _, fact := range facts {
		if _, err := b.DB.AddFact("note", fact, vec); err != nil {
			t.Fatal(err)
		}
	}
}

func waitingFor(t *testing.T, wait time.Duration) {
	t.Helper()

	was := outsideWait
	outsideWait = wait

	t.Cleanup(func() { outsideWait = was })
}

func TestTheMemoryIsNotSearchedUntilAllowedAtTheDesk(t *testing.T) {
	ts, db, b := newServer(t)

	model := &onThisComputer{}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	remembered(t, b, model, "accountant", "Alex's accountant is Maria", "The accountant is paid on the first of the month")

	search := `{"tool":"search_memory","arguments":{"query":"accountant"},"client":{"name":"claude-code","version":"2.1.0"}}`

	// Off on a new brain, and still off when the other two are allowed.
	for _, other := range []otherprograms.Subject{"", otherprograms.Programs, otherprograms.Ask} {
		if other != "" {
			if err := b.AllowOutside(other, true); err != nil {
				t.Fatal(err)
			}
		}

		code, body := post(t, ts.URL+"/api/outside/ask", search)
		if code != http.StatusForbidden || !strings.Contains(body, `"outside":"not allowed"`) || !strings.Contains(body, `"subject":"memory"`) ||
			strings.Contains(body, "Maria") {
			t.Fatalf("before it was allowed (after allowing %q): %d %s", other, code, body)
		}
	}

	// The refusals are on the record; what was searched for is not kept
	// with a refusal, nothing was done with it.
	deeds := theirs(t, db)
	if len(deeds) != 3 || deeds[0].Outcome != "refused" || deeds[0].Tool != "search_memory" || deeds[0].AskedBy != "claude-code 2.1.0" ||
		deeds[0].Summary != "Another program searched what I remember" || deeds[0].SummaryBG != "Друга програма потърси в това, което помня" ||
		!strings.Contains(deeds[0].Result, "may not search what I remember") || strings.Contains(deeds[0].Asked+deeds[0].Result, "accountant") {
		t.Fatalf("the record of the refusals: %+v", deeds)
	}

	code, body := post(t, ts.URL+"/api/outside/allow", `{"subject":"memory","allowed":true}`)
	if code != http.StatusOK {
		t.Fatalf("allowing: %d %s", code, body)
	}

	code, body = post(t, ts.URL+"/api/outside/ask", search)
	if code != http.StatusOK {
		t.Fatalf("after it was allowed: %d %s", code, body)
	}

	var told struct {
		Found []map[string]any `json:"found"`
	}

	if err := json.Unmarshal([]byte(body), &told); err != nil || len(told.Found) != 2 {
		t.Fatalf("what was found: %v %s", err, body)
	}

	// What kind of thing and what it says: no number, no score, no source.
	for _, f := range told.Found {
		for key := range f {
			if key != "category" && key != "content" {
				t.Errorf("another program is told %q of a remembered thing: %v", key, f)
			}
		}
	}

	deeds = theirs(t, db)
	if deeds[0].Outcome != "done" || deeds[0].Asked != "Search: accountant" || deeds[0].Result != "Things I remember that I told it: 2." ||
		deeds[0].AskedBy != "claude-code 2.1.0" || deeds[0].Changes {
		t.Errorf("the record of the search: %+v", deeds[0])
	}

	// As many as asked for.
	code, body = post(t, ts.URL+"/api/outside/ask", `{"tool":"search_memory","arguments":{"query":"accountant","limit":1},"client":{"name":"x"}}`)
	if json.Unmarshal([]byte(body), &told); code != http.StatusOK || len(told.Found) != 1 {
		t.Errorf("one asked for: %d %s", code, body)
	}

	// Taken back: refused again.
	post(t, ts.URL+"/api/outside/allow", `{"subject":"memory","allowed":false}`)

	if code, body = post(t, ts.URL+"/api/outside/ask", search); code != http.StatusForbidden || strings.Contains(body, "Maria") {
		t.Errorf("after it was taken back: %d %s", code, body)
	}
}

func TestASearchOrAQuestionThatCannotBeReadIsNotOne(t *testing.T) {
	ts, db, b := newServer(t)

	model := &onThisComputer{said: "never"}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	for _, s := range []otherprograms.Subject{otherprograms.Programs, otherprograms.Ask, otherprograms.Memory} {
		if err := b.AllowOutside(s, true); err != nil {
			t.Fatal(err)
		}
	}

	ticket := strings.Repeat("0", 32)

	for name, arguments := range map[string]string{
		"a search for nothing":          `{}`,
		"an empty search":               `{"query":""}`,
		"a search of invisible letters": `{"query":"​‮"}`,
		"a search too long":             `{"query":"` + strings.Repeat("я", otherprograms.MostSearch+1) + `"}`,
		"a search for none":             `{"query":"a","limit":0}`,
		"a search for too many":         `{"query":"a","limit":11}`,
		"a search with something extra": `{"query":"a","category":"note"}`,
		"a search with a question":      `{"query":"a","question":"b"}`,
		"a search with a source":        `{"query":"a","kind":"deb"}`,
		"a search that is not text":     `{"query":["a"]}`,
		"two searches in one":           `{"query":"a"}{"query":"b"}`,
	} {
		body := `{"tool":"search_memory","arguments":` + arguments + `,"client":{"name":"x"}}`
		if code, said := post(t, ts.URL+"/api/outside/ask", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, said)
		}
	}

	for name, arguments := range map[string]string{
		"no question":                     `{}`,
		"an empty question":               `{"question":" \n "}`,
		"a question of invisible letters": `{"question":"​"}`,
		"a question too long":             `{"question":"` + strings.Repeat("я", otherprograms.MostQuestion+1) + `"}`,
		"a question and a ticket":         `{"question":"a","answer_to":"` + ticket + `"}`,
		"a ticket of another shape":       `{"answer_to":"../../etc/passwd"}`,
		"a ticket in capitals":            `{"answer_to":"` + strings.Repeat("A", 32) + `"}`,
		"a question with a search":        `{"question":"a","query":"b"}`,
		"a question with something extra": `{"question":"a","provider":"anthropic"}`,
		"a question with a model named":   `{"question":"a","model":"x"}`,
		"a question with a conversation":  `{"question":"a","conversation_id":1}`,
	} {
		body := `{"tool":"ask_assistant","arguments":` + arguments + `,"client":{"name":"x"}}`
		if code, said := post(t, ts.URL+"/api/outside/ask", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, said)
		}
	}

	for name, body := range map[string]string{
		"a search given to the list of programs": `{"tool":"list_programs","arguments":{"query":"a"},"client":{"name":"x"}}`,
		"a question given to the status":         `{"tool":"assistant_status","arguments":{"question":"a"},"client":{"name":"x"}}`,
	} {
		if code, said := post(t, ts.URL+"/api/outside/ask", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, said)
		}
	}

	if deeds := theirs(t, db); len(deeds) != 0 {
		t.Errorf("something that is not a question was written down as one: %+v", deeds)
	}

	if len(model.asked) != 0 {
		t.Errorf("the model was asked %d times", len(model.asked))
	}

	// A ticket of the right shape that was never given.
	code, said := post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"answer_to":"`+ticket+`"},"client":{"name":"x"}}`)
	if code != http.StatusGone || !strings.Contains(said, "No answer is waiting under that ticket") {
		t.Errorf("a ticket never given: %d %s", code, said)
	}
}

// The memory cannot be searched without a model on this computer to
// search with: that is said, and is not the same as finding nothing.
func TestASearchThatCannotBeDoneIsNotAnsweredAsNothingFound(t *testing.T) {
	ts, db, b := newServer(t)

	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local)

	if err := b.AllowOutside(otherprograms.Memory, true); err != nil {
		t.Fatal(err)
	}

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"search_memory","arguments":{"query":"accountant"},"client":{"name":"x"}}`)
	if code != http.StatusServiceUnavailable || strings.Contains(body, `"found"`) || !strings.Contains(body, "cannot be searched now") {
		t.Errorf("with nothing to search with: %d %s", code, body)
	}

	// Nothing was told, so nothing is written as told.
	for _, d := range theirs(t, db) {
		if d.Tool == "search_memory" {
			t.Errorf("a search that was not done is on the record as done: %+v", d)
		}
	}
}

func TestAQuestionToTheAssistantThroughTheDoor(t *testing.T) {
	ts, db, b := newServer(t)

	model := &onThisComputer{said: "Sofia."}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	ask := `{"tool":"ask_assistant","arguments":{"question":"What is the capital of Bulgaria?"},"client":{"name":"claude-code","version":"2.1.0"}}`

	// Off on a new brain, and still off when the other two are allowed.
	for _, other := range []otherprograms.Subject{"", otherprograms.Programs, otherprograms.Memory} {
		if other != "" {
			if err := b.AllowOutside(other, true); err != nil {
				t.Fatal(err)
			}
		}

		code, body := post(t, ts.URL+"/api/outside/ask", ask)
		if code != http.StatusForbidden || !strings.Contains(body, `"subject":"ask"`) || strings.Contains(body, "Sofia") {
			t.Fatalf("before it was allowed (after allowing %q): %d %s", other, code, body)
		}
	}

	if len(model.asked) != 0 {
		t.Fatalf("the model was asked %d times while the switch was off", len(model.asked))
	}

	if err := b.AllowOutside(otherprograms.Memory, false); err != nil {
		t.Fatal(err)
	}

	post(t, ts.URL+"/api/outside/allow", `{"subject":"ask","allowed":true}`)

	code, body := post(t, ts.URL+"/api/outside/ask", ask)
	if code != http.StatusOK {
		t.Fatalf("after it was allowed: %d %s", code, body)
	}

	var told struct {
		Ready    bool   `json:"ready"`
		Answer   string `json:"answer"`
		AnswerTo string `json:"answer_to"`
	}

	if err := json.Unmarshal([]byte(body), &told); err != nil || !told.Ready || told.Answer != "Sofia." || told.AnswerTo != "" {
		t.Fatalf("the answer: %v %s", err, body)
	}

	deeds := theirs(t, db)
	if deeds[0].Tool != "ask_assistant" || deeds[0].Outcome != "done" || deeds[0].AskedBy != "claude-code 2.1.0" ||
		deeds[0].Asked != "Question: What is the capital of Bulgaria?" || deeds[0].Result != "Answer: Sofia." {
		t.Errorf("the record: %+v", deeds[0])
	}

	// No tools offered to the model, whatever the desk allows.
	if len(model.asked) != 1 || len(model.asked[0].Tools) != 0 {
		t.Errorf("the model was asked: %+v", model.asked)
	}

	// The page lists it among the last questions.
	var state outsideState

	getJSON(t, ts.URL+"/api/outside", &state)
	if len(state.Recent) == 0 || state.Recent[0].Tool != "ask_assistant" || state.Recent[0].Outcome != "done" {
		t.Errorf("the last questions: %+v", state.Recent)
	}
}

// A slow answer: the door answers with a ticket within its wait, the
// answer is fetched with it, and meanwhile nobody else's question is taken.
func TestASlowAnswerIsFetchedWithItsTicket(t *testing.T) {
	ts, db, b := newServer(t)
	waitingFor(t, 30*time.Millisecond)

	model := &onThisComputer{said: "Worth the wait.", hold: make(chan struct{})}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	if err := b.AllowOutside(otherprograms.Ask, true); err != nil {
		t.Fatal(err)
	}

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"question":"A slow one?"},"client":{"name":"claude-code"}}`)

	var told struct {
		Ready    *bool  `json:"ready"`
		Answer   string `json:"answer"`
		AnswerTo string `json:"answer_to"`
	}

	if err := json.Unmarshal([]byte(body), &told); err != nil || code != http.StatusOK || told.Ready == nil || *told.Ready ||
		!ticketLike(told.AnswerTo) || told.Answer != "" {
		t.Fatalf("not ready: %d %s", code, body)
	}

	ticket := told.AnswerTo
	fetch := `{"tool":"ask_assistant","arguments":{"answer_to":"` + ticket + `"},"client":{"name":"claude-code"}}`

	// Still not ready; and a second question is told to wait its turn.
	if code, body = post(t, ts.URL+"/api/outside/ask", fetch); code != http.StatusOK || !strings.Contains(body, `"ready":false`) {
		t.Fatalf("fetched too early: %d %s", code, body)
	}

	code, body = post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"question":"And me?"},"client":{"name":"cursor"}}`)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, `"outside":"busy"`) {
		t.Fatalf("a second question meanwhile: %d %s", code, body)
	}

	// Nothing is on the record as answered before it is.
	for _, d := range theirs(t, db) {
		if d.Tool == "ask_assistant" {
			t.Fatalf("on the record before the answer: %+v", d)
		}
	}

	close(model.hold)
	waitingFor(t, 5*time.Second)

	code, body = post(t, ts.URL+"/api/outside/ask", fetch)
	if code != http.StatusOK || !strings.Contains(body, `"ready":true`) || !strings.Contains(body, `"answer":"Worth the wait."`) {
		t.Fatalf("fetched: %d %s", code, body)
	}

	// One question on the record, however often its answer was fetched.
	asked := 0

	for _, d := range theirs(t, db) {
		if d.Tool == "ask_assistant" {
			asked++
		}
	}

	if asked != 1 {
		t.Errorf("the question is on the record %d times", asked)
	}
}

func TestAQuestionWithNoModelOnThisComputerIsNotSentElsewhere(t *testing.T) {
	ts, _, b := newServer(t)

	b.Router = llm.NewRouter(llm.ModeOpen, "anthropic")

	if err := b.AllowOutside(otherprograms.Ask, true); err != nil {
		t.Fatal(err)
	}

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"question":"Anything?"},"client":{"name":"x"}}`)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "no model on this computer") {
		t.Errorf("with no model here: %d %s", code, body)
	}
}

func TestAnAnswerTheModelDidNotGiveIsSaidWithoutItsFault(t *testing.T) {
	ts, db, b := newServer(t)

	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, &onThisComputer{fail: errors.New("dial tcp 10.1.2.3:11434: a private detail")})

	if err := b.AllowOutside(otherprograms.Ask, true); err != nil {
		t.Fatal(err)
	}

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"question":"Anything?"},"client":{"name":"x"}}`)
	if code != http.StatusBadGateway || strings.Contains(body, "private detail") || strings.Contains(body, "10.1.2.3") ||
		!strings.Contains(body, "did not give an answer") {
		t.Errorf("a model that failed: %d %s", code, body)
	}

	if deeds := theirs(t, db); len(deeds) != 1 || deeds[0].Outcome != "failed" || strings.Contains(deeds[0].Result, "private detail") {
		t.Errorf("the record: %+v", deeds)
	}
}

/*
 * A program that asks in a loop: the questions over the minute's number
 * are refused whatever is allowed, and the record gets one line for them,
 * not one each.
 */
func TestAProgramThatAsksInALoopDoesNotFillTheRecord(t *testing.T) {
	ts, db, b := newServer(t)
	twoPrograms(t, db)

	if err := b.AllowOutside(otherprograms.Programs, true); err != nil {
		t.Fatal(err)
	}

	status := `{"tool":"assistant_status","client":{"name":"loop","version":"1"}}`
	list := `{"tool":"list_programs","client":{"name":"loop","version":"1"}}`

	for i := 0; i < otherprograms.MostAMinute; i++ {
		body := status
		if i%2 == 1 {
			body = list
		}

		if code, said := post(t, ts.URL+"/api/outside/ask", body); code != http.StatusOK {
			t.Fatalf("question %d of the minute: %d %s", i+1, code, said)
		}
	}

	for i := 0; i < 40; i++ {
		body := status
		if i%2 == 1 {
			body = list
		}

		code, said := post(t, ts.URL+"/api/outside/ask", body)
		if code != http.StatusTooManyRequests || !strings.Contains(said, `"outside":"too many"`) || strings.Contains(strings.ToLower(said), "gimp") {
			t.Fatalf("question %d over the number: %d %s", i+1, code, said)
		}
	}

	deeds := theirs(t, db)
	if len(deeds) != otherprograms.MostAMinute+1 {
		t.Fatalf("%d lines on the record for %d questions answered and 40 refused", len(deeds), otherprograms.MostAMinute)
	}

	if deeds[0].Outcome != "refused" || deeds[0].AskedBy != "loop 1" || !strings.Contains(deeds[0].Result, fmt.Sprintf("more than %d questions in a minute", otherprograms.MostAMinute)) {
		t.Errorf("the line for the flood: %+v", deeds[0])
	}

	// What cannot be read does not use up the minute, and is refused as
	// unreadable whatever the count.
	if code, _ := post(t, ts.URL+"/api/outside/ask", `{"tool":"read_file","client":{"name":"loop"}}`); code != http.StatusBadRequest {
		t.Errorf("something that is not a question, during the flood: %d", code)
	}

	// The MCP server says it in words a model can pass on.
	for _, each := range mcpserve.New(ts.URL, "test").Tools {
		if each.Name != "list_programs" {
			continue
		}

		_, err := each.Run(t.Context(), json.RawMessage(`{}`), mcpserve.Caller{Name: "loop", Version: "1"})
		if err == nil || !strings.Contains(err.Error(), "too many questions in the last minute") || !strings.Contains(err.Error(), "Wait a minute") {
			t.Errorf("as the MCP server says it: %v", err)
		}
	}
}

// The MCP server's two new tools through the real door, end to end.
func TestTheMCPServerAsksAndSearchesThroughTheDoor(t *testing.T) {
	ts, db, b := newServer(t)
	waitingFor(t, 5*time.Second)

	model := &onThisComputer{said: "Maria, on weekdays.\nShe prefers e-mail."}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	remembered(t, b, model, "Who is the accountant?", "Alex's accountant is Maria")

	run := func(tool, arguments string) (string, error) {
		for _, each := range mcpserve.New(ts.URL, "test").Tools {
			if each.Name == tool {
				return each.Run(t.Context(), json.RawMessage(arguments), mcpserve.Caller{Name: "claude-code", Version: "2.1.0"})
			}
		}

		t.Fatalf("no tool %s", tool)

		return "", nil
	}

	// Both off: each refusal names its own switch, in words for a person.
	if _, err := run("ask_assistant", `{"question":"Who is the accountant?"}`); err == nil ||
		!strings.Contains(err.Error(), "has not allowed other programs to ask it questions") || !strings.Contains(err.Error(), "What other programs may ask") {
		t.Fatalf("a question while its switch is off: %v", err)
	}

	if _, err := run("search_memory", `{"query":"Who is the accountant?"}`); err == nil ||
		!strings.Contains(err.Error(), "has not allowed other programs to search what it remembers") || strings.Contains(err.Error(), "Maria") {
		t.Fatalf("a search while its switch is off: %v", err)
	}

	post(t, ts.URL+"/api/outside/allow", `{"subject":"ask","allowed":true}`)

	// Asking allowed, the memory not: answered, and the model was told
	// nothing the memory holds.
	text, err := run("ask_assistant", `{"question":"Who is the accountant?"}`)
	if err != nil || !strings.HasSuffix(text, ":\nMaria, on weekdays.\nShe prefers e-mail.") {
		t.Fatalf("the answer: %q %v", text, err)
	}

	for _, m := range model.asked[0].Messages {
		if strings.Contains(m.Content, "Alex's accountant") {
			t.Errorf("the memory was put before the model with its switch off: %q", m.Content)
		}
	}

	if _, err := run("search_memory", `{"query":"Who is the accountant?"}`); err == nil {
		t.Fatal("allowing questions allowed searching the memory")
	}

	post(t, ts.URL+"/api/outside/allow", `{"subject":"memory","allowed":true}`)

	text, err = run("search_memory", `{"query":"Who is the accountant?"}`)
	if err != nil || !strings.Contains(text, "- Alex's accountant is Maria (note)") || !strings.Contains(text, "Personal to its user") {
		t.Fatalf("the search: %q %v", text, err)
	}

	// And now a question is answered with the memory before the model.
	if _, err = run("ask_assistant", `{"question":"Who is the accountant?"}`); err != nil {
		t.Fatal(err)
	}

	with := false

	for _, m := range model.asked[len(model.asked)-1].Messages {
		with = with || strings.Contains(m.Content, "Alex's accountant is Maria")
	}

	if !with {
		t.Error("the memory was not put before the model with both switches on")
	}

	// Everything asked is on the record with who asked, newest first.
	var got []string
	for _, d := range theirs(t, db) {
		got = append(got, d.Tool+" "+d.Outcome+" "+d.AskedBy)
	}

	want := []string{
		"ask_assistant done claude-code 2.1.0",
		"search_memory done claude-code 2.1.0",
		"search_memory refused claude-code 2.1.0",
		"ask_assistant done claude-code 2.1.0",
		"search_memory refused claude-code 2.1.0",
		"ask_assistant refused claude-code 2.1.0",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the record:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The wait the door holds a question open and the wait the MCP server
// gives it stay on the right side of each other and of a client's minute.
func TestTheDoorAnswersBeforeTheMCPServerGivesUp(t *testing.T) {
	if outsideWait >= mcpserve.PatientFor || mcpserve.PatientFor >= time.Minute {
		t.Errorf("the door holds a question %s, the MCP server waits %s, a client gives up after a minute", outsideWait, mcpserve.PatientFor)
	}
}

// Every switch is turned at the desk and each turn is on the record in
// its own words.
func TestEachNewSwitchIsTurnedAndRecorded(t *testing.T) {
	ts, db, _ := newServer(t)

	for _, subject := range []string{"ask", "memory"} {
		for _, allowed := range []bool{true, false} {
			code, body := post(t, ts.URL+"/api/outside/allow", fmt.Sprintf(`{"subject":%q,"allowed":%v}`, subject, allowed))
			if code != http.StatusOK {
				t.Fatalf("%s to %v: %d %s", subject, allowed, code, body)
			}
		}
	}

	deeds, err := db.Deeds(store.DeedFilter{})
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, d := range deeds {
		got = append(got, d.Summary+" / "+d.SummaryBG)
	}

	want := []string{
		"Other programs may no longer search what I remember / Другите програми вече не могат да търсят в това, което помня",
		"Other programs may now search what I remember / Другите програми вече могат да търсят в това, което помня",
		"Other programs may no longer ask me questions / Другите програми вече не могат да ми задават въпроси",
		"Other programs may now ask me questions / Другите програми вече могат да ми задават въпроси",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the record:\n%s", strings.Join(got, "\n"))
	}
}

// Found by the independent review of this step, each with its test.

// A ticket fetches nothing once questions are switched off, and a question
// in hand is taken back with the switch.
func TestATicketFetchesNothingOnceQuestionsAreOff(t *testing.T) {
	ts, _, b := newServer(t)
	waitingFor(t, 5*time.Second)

	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, &onThisComputer{said: "Kept for a moment."})

	if err := b.AllowOutside(otherprograms.Ask, true); err != nil {
		t.Fatal(err)
	}

	ticket, err := b.AskForOutside("claude-code", "Anything?")
	if err != nil {
		t.Fatal(err)
	}

	fetch := `{"tool":"ask_assistant","arguments":{"answer_to":"` + ticket + `"},"client":{"name":"claude-code"}}`

	if code, body := post(t, ts.URL+"/api/outside/ask", fetch); code != http.StatusOK || !strings.Contains(body, "Kept for a moment.") {
		t.Fatalf("while allowed: %d %s", code, body)
	}

	post(t, ts.URL+"/api/outside/allow", `{"subject":"ask","allowed":false}`)

	if code, body := post(t, ts.URL+"/api/outside/ask", fetch); code != http.StatusForbidden || strings.Contains(body, "Kept for a moment.") {
		t.Errorf("after questions were switched off: %d %s", code, body)
	}

	// Allowed again: the old ticket stays dead.
	post(t, ts.URL+"/api/outside/allow", `{"subject":"ask","allowed":true}`)

	if code, body := post(t, ts.URL+"/api/outside/ask", fetch); code != http.StatusGone || strings.Contains(body, "Kept for a moment.") {
		t.Errorf("the old ticket after questions were allowed again: %d %s", code, body)
	}
}

// The person's own question first, through the door: the program is told
// to ask again, in words, and the record says why there was no answer.
func TestThroughTheDoorAProgramsQuestionGivesWayToThePerson(t *testing.T) {
	ts, db, b := newServer(t)
	waitingFor(t, 5*time.Second)

	model := &onThisComputer{said: "never said", hold: make(chan struct{})}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	if err := b.AllowOutside(otherprograms.Ask, true); err != nil {
		t.Fatal(err)
	}

	ticket, err := b.AskForOutside("claude-code", "A slow one?")
	if err != nil {
		t.Fatal(err)
	}

	progress.StartedATurn()
	defer progress.FinishedATurn()

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"answer_to":"`+ticket+`"},"client":{"name":"claude-code"}}`)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, `"outside":"gave way"`) {
		t.Fatalf("after the person asked: %d %s", code, body)
	}

	if deeds := theirs(t, db); len(deeds) != 1 || deeds[0].Outcome != "failed" || !strings.Contains(deeds[0].Result, "my user asked me something") {
		t.Errorf("the record: %+v", deeds)
	}

	// The MCP server says it in words a model can act on.
	for _, each := range mcpserve.New(ts.URL, "test").Tools {
		if each.Name != "ask_assistant" {
			continue
		}

		_, err := each.Run(t.Context(), json.RawMessage(`{"answer_to":"`+ticket+`"}`), mcpserve.Caller{Name: "claude-code"})
		if err == nil || !strings.Contains(err.Error(), "its user comes first") || !strings.Contains(err.Error(), "Ask the question again") {
			t.Errorf("as the MCP server says it: %v", err)
		}
	}
}

// What a program searched for is kept on the record behind a label, so
// words that look like the store's own sealed text cannot make the record
// unreadable (ASSISTANT-SEALED-PREFIX-1118).
func TestASearchThatLooksSealedDoesNotBreakTheRecord(t *testing.T) {
	ts, db, b := newServer(t)

	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, &onThisComputer{})

	if err := b.AllowOutside(otherprograms.Memory, true); err != nil {
		t.Fatal(err)
	}

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"search_memory","arguments":{"query":"pnsealed1:bm90IHNlYWxlZA=="},"client":{"name":"pnsealed1:x"}}`)
	if code != http.StatusOK {
		t.Fatalf("the search: %d %s", code, body)
	}

	deeds, err := db.Deeds(store.DeedFilter{})
	if err != nil || len(deeds) != 2 || deeds[0].Asked != "Search: pnsealed1:bm90IHNlYWxlZA==" || deeds[0].AskedBy != "x" {
		t.Errorf("the record after it: %v %+v", err, deeds)
	}
}

// The minute's number holds for the new questions too, and fetching an
// answer already made is not another question: one program's flood does
// not lose another its answer.
func TestTheFloodStopsEveryQuestionButNotAFetch(t *testing.T) {
	ts, db, b := newServer(t)
	waitingFor(t, 5*time.Second)

	model := &onThisComputer{said: "Made before the flood."}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	for _, s := range []otherprograms.Subject{otherprograms.Ask, otherprograms.Memory} {
		if err := b.AllowOutside(s, true); err != nil {
			t.Fatal(err)
		}
	}

	remembered(t, b, model, "accountant", "Alex's accountant is Maria")

	ticket, err := b.AskForOutside("claude-code", "Anything?")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < otherprograms.MostAMinute; i++ {
		post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status","client":{"name":"loop"}}`)
	}

	asked := len(model.asked)

	for name, body := range map[string]string{
		"a search":   `{"tool":"search_memory","arguments":{"query":"accountant"},"client":{"name":"loop"}}`,
		"a question": `{"tool":"ask_assistant","arguments":{"question":"Again?"},"client":{"name":"loop"}}`,
	} {
		code, said := post(t, ts.URL+"/api/outside/ask", body)
		if code != http.StatusTooManyRequests || strings.Contains(said, "Maria") {
			t.Errorf("%s during the flood: %d %s", name, code, said)
		}
	}

	if len(model.asked) != asked {
		t.Error("the model was asked during the flood")
	}

	before := len(theirs(t, db))

	for i := 0; i < 3; i++ {
		code, said := post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"answer_to":"`+ticket+`"},"client":{"name":"claude-code"}}`)
		if code != http.StatusOK || !strings.Contains(said, "Made before the flood.") {
			t.Fatalf("fetching during the flood: %d %s", code, said)
		}
	}

	// Fetching wrote nothing more on the record.
	if after := len(theirs(t, db)); after != before {
		t.Errorf("fetching wrote %d lines on the record", after-before)
	}
}

// A page in a browser can neither ask the assistant a question nor search
// its memory as "another program", whatever is allowed.
func TestAWebPageCanNeitherAskAQuestionNorSearchTheMemory(t *testing.T) {
	ts, db, b := newServer(t)

	model := &onThisComputer{said: "never"}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	for _, s := range []otherprograms.Subject{otherprograms.Ask, otherprograms.Memory} {
		if err := b.AllowOutside(s, true); err != nil {
			t.Fatal(err)
		}
	}

	remembered(t, b, model, "accountant", "Alex's accountant is Maria")

	before := len(theirs(t, db))

	for _, body := range []string{
		`{"tool":"search_memory","arguments":{"query":"accountant"},"client":{"name":"a page"}}`,
		`{"tool":"ask_assistant","arguments":{"question":"Anything?"},"client":{"name":"a page"}}`,
	} {
		for _, headers := range [][]string{
			{"Origin", "https://example.org"},
			{"Sec-Fetch-Site", "cross-site"},
			{"Sec-Fetch-Site", "same-origin"},
			{"Origin", ts.URL},
		} {
			code, said := post(t, ts.URL+"/api/outside/ask", body, headers...)
			if code != http.StatusForbidden || strings.Contains(said, "Maria") || strings.Contains(said, "never") {
				t.Errorf("%v: %d %s", headers, code, said)
			}
		}
	}

	if len(model.asked) != 0 || len(theirs(t, db)) != before {
		t.Errorf("a page's request reached the model (%d) or the record", len(model.asked))
	}
}

// A long answer is handed over whole and kept on the record cut short;
// the guide says so.
func TestALongAnswerIsOnTheRecordCutShort(t *testing.T) {
	ts, db, b := newServer(t)
	waitingFor(t, 5*time.Second)

	long := strings.Repeat("A sentence of the answer. ", 100)
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, &onThisComputer{said: long})

	if err := b.AllowOutside(otherprograms.Ask, true); err != nil {
		t.Fatal(err)
	}

	code, body := post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"question":"Tell me a lot."},"client":{"name":"x"}}`)
	if code != http.StatusOK || !strings.Contains(body, strings.TrimSpace(long)) {
		t.Fatalf("the long answer: %d (%d bytes)", code, len(body))
	}

	deeds := theirs(t, db)
	if n := len([]rune(deeds[0].Result)); n < 500 || n > 620 || !strings.HasPrefix(deeds[0].Result, "Answer: A sentence") {
		t.Errorf("the record keeps %d characters of the answer", n)
	}

	guide := readGuide(t, "asked-by-other-programs.md")
	if !strings.Contains(guide, "the first 600 characters") {
		t.Error("the guide does not say how much of an answer the record keeps")
	}
}

/*
 * Found by the second pass of the review: fetching is not counted by the
 * minute's number, so a fetch refused because questions are off must not
 * write a line either, or a program that fetches in a loop fills the
 * record at any speed.
 */
func TestFetchingInALoopWithQuestionsOffWritesNothing(t *testing.T) {
	ts, db, _ := newServer(t)

	fetch := `{"tool":"ask_assistant","arguments":{"answer_to":"` + strings.Repeat("0", 32) + `"},"client":{"name":"loop"}}`

	for i := 0; i < 100; i++ {
		if code, body := post(t, ts.URL+"/api/outside/ask", fetch); code != http.StatusForbidden || !strings.Contains(body, `"subject":"ask"`) {
			t.Fatalf("fetch %d with questions off: %d %s", i+1, code, body)
		}
	}

	if deeds := theirs(t, db); len(deeds) != 0 {
		t.Errorf("100 fetches with questions off wrote %d lines on the record", len(deeds))
	}

	// A question itself, refused, is written as before.
	post(t, ts.URL+"/api/outside/ask", `{"tool":"ask_assistant","arguments":{"question":"Anything?"},"client":{"name":"loop"}}`)

	if deeds := theirs(t, db); len(deeds) != 1 || deeds[0].Outcome != "refused" {
		t.Errorf("a refused question on the record: %+v", deeds)
	}
}
