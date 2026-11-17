package mcpserve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// demo is a server with three tools: one that answers, one that fails, and
// one that waits until it is told to stop.
// Each start of the waiting one is reported with the arguments it was given.
func demo() (*Server, chan string) {
	started := make(chan string, 4)

	return &Server{
		Name: "pn-scripts-assistant", Version: "9.9.9", Instructions: "Ask it.",
		Tools: []Tool{
			{
				Name: "echo", Title: "Echo", Description: "Says it back.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
				Run: func(_ context.Context, args json.RawMessage, who Caller) (string, error) {
					var in struct {
						Text string `json:"text"`
					}

					if err := json.Unmarshal(args, &in); err != nil {
						return "", err
					}

					return "said: " + in.Text + "\nasked by: " + who.Name, nil
				},
			},
			{
				Name: "fails", Description: "Never works.",
				Run: func(context.Context, json.RawMessage, Caller) (string, error) {
					return "", errors.New("it is not running")
				},
			},
			{
				Name: "waits", Description: "Waits until stopped.",
				Run: func(ctx context.Context, args json.RawMessage, _ Caller) (string, error) {
					started <- string(args)
					<-ctx.Done()

					return "", ctx.Err()
				},
			},
			{
				Name: "breaks", Description: "Panics.",
				Run: func(context.Context, json.RawMessage, Caller) (string, error) {
					panic("a fault in the tool")
				},
			},
		},
	}, started
}

type answer struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  map[string]any  `json:"result"`
	Error   *struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	} `json:"error"`
}

// talk gives the server every line, closes its input and returns what it
// wrote, by request id. Every line it wrote must be one JSON message.
func talk(t *testing.T, s *Server, lines ...string) map[string]answer {
	t.Helper()

	var out bytes.Buffer

	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	return read(t, out.String())
}

func read(t *testing.T, written string) map[string]answer {
	t.Helper()

	answers := map[string]answer{}

	for _, line := range strings.Split(strings.TrimSuffix(written, "\n"), "\n") {
		if line == "" {
			continue
		}

		var a answer

		if err := json.Unmarshal([]byte(line), &a); err != nil {
			t.Fatalf("a line that is not a message: %q (%v)", line, err)
		}

		if a.JSONRPC != "2.0" {
			t.Errorf("not JSON-RPC 2.0: %s", line)
		}

		if (a.Result == nil) == (a.Error == nil) {
			t.Errorf("neither a result nor an error, or both: %s", line)
		}

		if _, twice := answers[string(a.ID)]; twice {
			t.Errorf("answered twice: %s", line)
		}

		answers[string(a.ID)] = a
	}

	return answers
}

const modernMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"Claude Code","version":"2.1"}}`

// A client of the older protocol says hello first, and is then served
// without anything more on each request.
func TestAnOlderClientShakesHandsAndIsServed(t *testing.T) {
	s, _ := demo()

	got := talk(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"Cursor","version":"3"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hello"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"ping"}`,
	)

	if len(got) != 4 {
		t.Fatalf("four requests, %d answers: %+v", len(got), got)
	}

	hello := got["1"].Result
	if hello["protocolVersion"] != "2025-06-18" {
		t.Errorf("the version the client asked for is one it speaks: %v", hello)
	}

	if info, _ := hello["serverInfo"].(map[string]any); info["name"] != "pn-scripts-assistant" || info["version"] != "9.9.9" {
		t.Errorf("who it is: %v", hello["serverInfo"])
	}

	if caps, _ := hello["capabilities"].(map[string]any); caps["tools"] == nil || len(caps) != 1 {
		t.Errorf("it offers tools and nothing else: %v", hello["capabilities"])
	}

	if hello["instructions"] != "Ask it." {
		t.Errorf("instructions: %v", hello["instructions"])
	}

	list, _ := got["2"].Result["tools"].([]any)
	if len(list) != 4 {
		t.Fatalf("tools: %v", got["2"].Result)
	}

	for _, each := range list {
		tool := each.(map[string]any)

		schema, _ := tool["inputSchema"].(map[string]any)
		if schema["type"] != "object" {
			t.Errorf("%v has no object schema: %v", tool["name"], tool["inputSchema"])
		}
	}

	if _, has := got["2"].Result["resultType"]; has {
		t.Errorf("an older client is not sent resultType: %v", got["2"].Result)
	}

	called := got["3"].Result
	content, _ := called["content"].([]any)

	if len(content) != 1 || content[0].(map[string]any)["type"] != "text" ||
		content[0].(map[string]any)["text"] != "said: hello\nasked by: Cursor" || called["isError"] != false {
		t.Errorf("the call: %v", called)
	}

	if got["4"].Result == nil || len(got["4"].Result) != 0 {
		t.Errorf("ping: %+v", got["4"])
	}
}

// An older client that asks for a version nobody here speaks is offered the
// newest older one, as the handshake says.
func TestAnUnknownOlderVersionIsAnsweredWithOurs(t *testing.T) {
	s, _ := demo()

	got := talk(t, s, `{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":"1999-01-01","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}`)

	if got[`"a"`].Result["protocolVersion"] != "2025-11-25" {
		t.Errorf("%+v", got)
	}
}

// A client of the current protocol says its version on every request and is
// served with no handshake at all.
func TestACurrentClientIsServedWithoutAHandshake(t *testing.T) {
	s, _ := demo()

	got := talk(t, s,
		`{"jsonrpc":"2.0","id":"discover-1","method":"server/discover","params":{`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hi"},`+modernMeta+`}}`,
	)

	found := got[`"discover-1"`].Result
	if found == nil {
		t.Fatalf("a text id is answered as text: %+v", got)
	}

	if found["resultType"] != "complete" {
		t.Errorf("resultType: %v", found)
	}

	versions, _ := found["supportedVersions"].([]any)
	if len(versions) < 2 || versions[0] != "2026-07-28" {
		t.Errorf("versions, newest first: %v", versions)
	}

	meta, _ := found["_meta"].(map[string]any)
	if info, _ := meta["io.modelcontextprotocol/serverInfo"].(map[string]any); info["name"] != "pn-scripts-assistant" {
		t.Errorf("who it is: %v", found["_meta"])
	}

	if caps, _ := found["capabilities"].(map[string]any); caps["tools"] == nil {
		t.Errorf("capabilities: %v", found["capabilities"])
	}

	if got["2"].Result["resultType"] != "complete" || len(got["2"].Result["tools"].([]any)) != 4 {
		t.Errorf("the list: %v", got["2"].Result)
	}

	called := got["3"].Result
	if called["resultType"] != "complete" || called["isError"] != false ||
		called["content"].([]any)[0].(map[string]any)["text"] != "said: hi\nasked by: Claude Code" {
		t.Errorf("the call: %v", called)
	}
}

// A version it does not speak is refused with the ones it does, the way a
// current client expects, so it can ask again.
func TestAVersionItDoesNotSpeakIsRefusedWithTheOnesItDoes(t *testing.T) {
	s, _ := demo()

	got := talk(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2031-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}`)

	e := got["1"].Error
	if e == nil || e.Code != -32022 || e.Data["requested"] != "2031-01-01" {
		t.Fatalf("%+v", got["1"])
	}

	if supported, _ := e.Data["supported"].([]any); len(supported) < 2 || supported[0] != "2026-07-28" {
		t.Errorf("supported: %v", e.Data)
	}
}

// A request that says neither a version nor hello first cannot be served in
// either protocol, and is told what to send.
func TestARequestWithNoVersionAndNoHandshakeIsRefused(t *testing.T) {
	s, _ := demo()

	got := talk(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
	)

	for _, id := range []string{"1", "2", "3"} {
		if e := got[id].Error; e == nil || e.Code != -32602 {
			t.Errorf("request %s: %+v", id, got[id])
		}
	}

	if !strings.Contains(got["1"].Error.Message, "2026-07-28") || !strings.Contains(got["1"].Error.Message, "initialize") {
		t.Errorf("it does not say what to send: %s", got["1"].Error.Message)
	}

	if !strings.Contains(got["3"].Error.Message, "clientCapabilities") {
		t.Errorf("the missing field is not named: %s", got["3"].Error.Message)
	}
}

// What is not a message is answered as an error and does not end the
// conversation; what needs no answer gets none.
func TestWhatIsNotARequestDoesNotEndTheConversation(t *testing.T) {
	s, _ := demo()

	got := talk(t, s,
		`{"jsonrpc":"2.0","id":8}`,
		`{"jsonrpc":"2.0","method":"notifications/whatever","params":{}}`,
		`{"jsonrpc":"2.0","method":"ping"}`,
		`{"jsonrpc":"2.0","id":9,"result":{}}`,
		``,
		`   `,
		`{"jsonrpc":"2.0","id":10,"method":"resources/list","params":{`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":11,"method":"ping","params":{`+modernMeta+`}}`,
	)

	if e := got["10"].Error; e == nil || e.Code != -32601 {
		t.Errorf("a method it does not have: %+v", got["10"])
	}

	if got["11"].Result == nil {
		t.Errorf("the ping after all of it: %+v", got["11"])
	}

	if _, answered := got["9"]; answered {
		t.Errorf("a response sent to a server was answered: %+v", got["9"])
	}

	if e := got["8"].Error; e == nil || e.Code != -32600 {
		t.Errorf("a request with no method: %+v", got["8"])
	}

	// Lines whose id cannot be used are answered with a null id, each on
	// its own line.
	var out bytes.Buffer

	unusable := "this is not json\n" +
		`[{"jsonrpc":"2.0","id":7,"method":"ping"}]` + "\n" +
		`{"jsonrpc":"2.0","id":null,"method":"ping"}` + "\n" +
		`{"jsonrpc":"2.0","id":{"a":1},"method":"ping"}` + "\n"

	if err := s.Serve(context.Background(), strings.NewReader(unusable), &out); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("four unusable lines, %d answers: %q", len(lines), out.String())
	}

	for i, want := range []int{-32700, -32600, -32600, -32600} {
		var a answer

		if err := json.Unmarshal([]byte(lines[i]), &a); err != nil || a.Error == nil || a.Error.Code != want || string(a.ID) != "null" {
			t.Errorf("line %d: %s, want %d with a null id", i, lines[i], want)
		}
	}
}

func TestToolsThatAreNotThereFailOrBreak(t *testing.T) {
	s, _ := demo()

	got := talk(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"no_such_tool","arguments":{},`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fails","arguments":{},`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"breaks",`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo","arguments":[1,2],`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"echo",`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":6,"method":"ping","params":{`+modernMeta+`}}`,
	)

	if e := got["1"].Error; e == nil || e.Code != -32602 || !strings.Contains(e.Message, "no_such_tool") {
		t.Errorf("a tool that is not there: %+v", got["1"])
	}

	// A tool that could not do it is an answer the asking model can read,
	// not a protocol error.
	failed := got["2"].Result
	if failed["isError"] != true || failed["content"].([]any)[0].(map[string]any)["text"] != "it is not running" {
		t.Errorf("a failed tool: %+v", got["2"])
	}

	if e := got["3"].Error; e == nil || e.Code != -32603 || strings.Contains(e.Message, "a fault in the tool") {
		t.Errorf("a broken tool says only that it broke: %+v", got["3"])
	}

	if e := got["4"].Error; e == nil || e.Code != -32602 {
		t.Errorf("arguments that are not an object: %+v", got["4"])
	}

	// No arguments at all is an empty object.
	if got["5"].Result == nil || got["5"].Result["isError"] != false {
		t.Errorf("no arguments: %+v", got["5"])
	}

	if got["6"].Result == nil {
		t.Errorf("it stopped answering after a tool broke: %+v", got)
	}
}

// A name the client gives is shown to people later: kept short and free of
// anything that is not printable.
func TestTheCallersNameIsKeptShortAndPrintable(t *testing.T) {
	s, _ := demo()

	long := strings.Repeat("x", 300)

	got := talk(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"text":""},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"Evil\u001b[31m\nApproved by the owner `+long+`","version":"1"}}}}`,
	)

	text := got["1"].Result["content"].([]any)[0].(map[string]any)["text"].(string)
	name := strings.TrimPrefix(text, "said: \nasked by: ")

	if strings.ContainsAny(name, "\n\x1b") || len([]rune(name)) > 60 {
		t.Errorf("the name as kept: %q", name)
	}

	if !strings.HasPrefix(name, "Evil") {
		t.Errorf("the name lost its start: %q", name)
	}
}

// A line longer than any request should be is refused, and the next line is
// read as a line of its own.
func TestALineTooLongIsRefusedAndTheNextIsRead(t *testing.T) {
	s, _ := demo()
	s.MostBytes = 4096

	got := talk(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"text":"`+strings.Repeat("a", 3*4096)+`"},`+modernMeta+`}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping","params":{`+modernMeta+`}}`,
	)

	if e := got["null"].Error; e == nil || e.Code != -32600 || !strings.Contains(e.Message, "long") {
		t.Errorf("the long line: %+v", got)
	}

	if got["2"].Result == nil || len(got) != 2 {
		t.Errorf("the line after it: %+v", got)
	}
}

// pipes is a server being talked to line by line, the way a client does.
type pipes struct {
	t    *testing.T
	in   *io.PipeWriter
	out  chan string
	done chan error
}

func live(t *testing.T, s *Server) *pipes {
	t.Helper()

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	p := &pipes{t: t, in: inW, out: make(chan string, 64), done: make(chan error, 1)}

	go func() {
		err := s.Serve(context.Background(), inR, outW)
		outW.Close()
		p.done <- err
	}()

	go func() {
		var kept []byte

		buf := make([]byte, 4096)

		for {
			n, err := outR.Read(buf)
			kept = append(kept, buf[:n]...)

			for {
				i := bytes.IndexByte(kept, '\n')
				if i < 0 {
					break
				}

				p.out <- string(kept[:i])
				kept = kept[i+1:]
			}

			if err != nil {
				close(p.out)

				return
			}
		}
	}()

	t.Cleanup(func() { inW.Close() })

	return p
}

func (p *pipes) say(line string) {
	p.t.Helper()

	if _, err := io.WriteString(p.in, line+"\n"); err != nil {
		p.t.Fatal(err)
	}
}

func (p *pipes) hear(within time.Duration) (string, bool) {
	select {
	case line, open := <-p.out:
		return line, open
	case <-time.After(within):
		return "", false
	}
}

// A request that is taking long does not hold up the next one, and one the
// client gives up on is stopped and never answered.
func TestASlowCallHoldsNothingUpAndAGivenUpOneIsNeverAnswered(t *testing.T) {
	s, started := demo()
	p := live(t, s)

	p.say(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"waits",` + modernMeta + `}}`)

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the slow tool never started")
	}

	p.say(`{"jsonrpc":"2.0","id":2,"method":"ping","params":{` + modernMeta + `}}`)

	line, ok := p.hear(5 * time.Second)
	if !ok || !strings.Contains(line, `"id":2`) {
		t.Fatalf("the ping waited for the slow call: %q", line)
	}

	p.say(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1,"reason":"gave up"}}`)
	p.say(`{"jsonrpc":"2.0","id":3,"method":"ping","params":{` + modernMeta + `}}`)

	line, ok = p.hear(5 * time.Second)
	if !ok || !strings.Contains(line, `"id":3`) {
		t.Fatalf("after giving up on 1 the next line was %q", line)
	}

	p.in.Close()

	select {
	case err := <-p.done:
		if err != nil {
			t.Errorf("serve ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("it did not end when its input closed")
	}

	if line, open := p.hear(time.Second); open {
		t.Errorf("the call given up on was answered: %q", line)
	}
}

// When the client goes away, work in hand is stopped and the server ends;
// it does not wait for a tool that would never finish.
func TestItEndsWhenItsInputClosesEvenMidCall(t *testing.T) {
	s, started := demo()
	s.Grace = 100 * time.Millisecond
	p := live(t, s)

	p.say(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"waits",` + modernMeta + `}}`)
	<-started
	p.in.Close()

	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		t.Fatal("it kept running after its input closed")
	}
}

// Many requests at once are all answered, each once, each on a line of its
// own (run with -race).
func TestManyRequestsAtOnceAreEachAnsweredOnce(t *testing.T) {
	s, _ := demo()

	var lines []string

	for i := 1; i <= 200; i++ {
		lines = append(lines, `{"jsonrpc":"2.0","id":`+itoa(i)+`,"method":"tools/call","params":{"name":"echo","arguments":{"text":"line one\nline two"},`+modernMeta+`}}`)
	}

	got := talk(t, s, lines...)

	if len(got) != 200 {
		t.Fatalf("%d answers to 200 requests", len(got))
	}
}

// More requests than it will keep waiting are refused at once, and the
// ones kept are still answered.
func TestMoreRequestsThanItKeepsAreRefusedNotQueuedForever(t *testing.T) {
	s, started := demo()
	s.AtOnce = 1
	s.Grace = 100 * time.Millisecond
	p := live(t, s)

	for i := 1; i <= MostWaiting+1; i++ {
		p.say(`{"jsonrpc":"2.0","id":` + itoa(i) + `,"method":"tools/call","params":{"name":"waits","arguments":{"n":` + itoa(i) + `},` + modernMeta + `}}`)
	}

	// Whichever got the one place: requests are not promised an order.
	var first struct {
		N int `json:"n"`
	}

	if err := json.Unmarshal([]byte(<-started), &first); err != nil || first.N == 0 {
		t.Fatalf("which request started: %v %+v", err, first)
	}

	line, ok := p.hear(5 * time.Second)
	if !ok || !strings.Contains(line, `"id":`+itoa(MostWaiting+1)+`,`) || !strings.Contains(line, "-32603") {
		t.Fatalf("the request over the limit: %q", line)
	}

	// The loop is still reading: giving up on the one in hand is heard,
	// and another gets its turn.
	p.say(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":` + itoa(first.N) + `}}`)

	select {
	case next := <-started:
		if next == `{"n":`+itoa(first.N)+`}` {
			t.Errorf("the one given up on started again: %s", next)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the next waiting request never got its turn")
	}
}

// What it writes is only messages: nothing a tool or the log says reaches
// the output.
func TestTheLogNeverReachesTheOutput(t *testing.T) {
	s, _ := demo()

	var log bytes.Buffer

	var mu sync.Mutex

	s.Log = writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()

		return log.Write(p)
	})

	got := talk(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"breaks",`+modernMeta+`}}`)

	if got["3"].Error == nil {
		t.Fatalf("%+v", got)
	}

	mu.Lock()
	defer mu.Unlock()

	if !strings.Contains(log.String(), "a fault in the tool") {
		t.Errorf("what broke is not in the log: %q", log.String())
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func itoa(i int) string {
	b, _ := json.Marshal(i)

	return string(b)
}
