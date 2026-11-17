/*
 * Package mcpserve is this program as a Model Context Protocol server: the
 * side other programs talk to.
 *
 * Claude Code, Claude Desktop, Cursor and anything else that speaks the
 * protocol start "pn-scripts-assistant mcp" and talk to it over its
 * standard streams, one JSON-RPC message a line. This file is the protocol
 * and nothing else: it knows no tool by name and reaches nothing. What it
 * offers is a short fixed list given to it (assistant.go), never the
 * program's own tools.
 *
 * Both eras of the protocol are served, as package mcp speaks both to
 * others. A request that carries its version (2026-07-28 on) is answered on
 * its own, with no memory of the one before; a client that says initialize
 * first is served the older way for as long as the process lives.
 *
 * Standard output carries messages and nothing else, ever: a stray line
 * there is a broken conversation for the program on the other end.
 */
package mcpserve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Protocol versions served, newest first. Modern is the first that carries
// everything on each request; the others shake hands first.
const (
	Modern = "2026-07-28"
	Legacy = "2025-11-25"
)

var legacyVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Versions is every version served, newest first.
func Versions() []string { return append([]string{Modern}, legacyVersions...) }

// Error codes: JSON-RPC's own, and the one the protocol keeps for a version
// that is not spoken.
const (
	codeParse              = -32700
	codeInvalidRequest     = -32600
	codeMethodNotFound     = -32601
	codeInvalidParams      = -32602
	codeInternal           = -32603
	codeUnsupportedVersion = -32022
)

// The keys a current request carries about itself.
const (
	metaVersion      = "io.modelcontextprotocol/protocolVersion"
	metaCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaClient       = "io.modelcontextprotocol/clientInfo"
	metaServer       = "io.modelcontextprotocol/serverInfo"
)

// Caller is the program asking, as it names itself. Nothing checks it: it
// is for showing, never for deciding.
type Caller struct {
	Name    string
	Version string
}

// Tool is one thing a caller may ask for.
type Tool struct {
	Name        string
	Title       string
	Description string

	// InputSchema is the JSON Schema of its arguments; an object taking
	// nothing when empty.
	InputSchema json.RawMessage

	// ReadOnly says the tool changes nothing, which a client may show.
	ReadOnly bool

	// Run does it. What it returns is the answer; an error is an answer
	// too, said to the caller as one that failed.
	Run func(ctx context.Context, arguments json.RawMessage, who Caller) (string, error)
}

// Server is the protocol around a fixed list of tools.
type Server struct {
	Name    string
	Version string

	// Instructions is what a calling model is told about using it.
	Instructions string

	Tools []Tool

	// Log is where faults are written (standard error); nowhere when nil.
	Log io.Writer

	// MostBytes is the longest line read; DefaultMostBytes when zero.
	MostBytes int

	// Grace is how long requests in hand may take to finish once the input
	// has ended; DefaultGrace when zero.
	Grace time.Duration

	// AtOnce is how many requests are worked on at a time; DefaultAtOnce
	// when zero.
	AtOnce int
}

const (
	// DefaultMostBytes is far more than any request to these tools needs.
	DefaultMostBytes = 1 << 20

	// DefaultGrace is enough for an answer already on its way.
	DefaultGrace = 10 * time.Second

	// DefaultAtOnce keeps a client that sends a thousand requests from
	// starting a thousand calls.
	DefaultAtOnce = 8
)

/*
 * KeepFor is how long a current client may keep the description of this
 * server and its list of tools before asking again (ttlMs). Neither changes
 * while the process lives, and both are the same for everyone who asks
 * (cacheScope "public": nothing of the user's is in them). They change only
 * when the program is updated, so what a client kept from the version
 * before is at most this old.
 */
const KeepFor = 5 * time.Minute

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// session is one run of Serve: what an older client agreed at its
// handshake, and the requests in hand.
type session struct {
	s   *Server
	ctx context.Context

	writing sync.Mutex
	out     io.Writer
	broken  bool

	mu      sync.Mutex
	shaken  bool
	version string
	client  Caller
	inHand  map[string]*call
}

// call is one request being worked on.
type call struct {
	cancel  context.CancelFunc
	givenUp bool
}

/*
 * Serve reads requests from in and writes answers to out until in ends.
 *
 * Each request is worked on by itself, so a slow one holds nothing up, and
 * no more than AtOnce at a time. When the input ends, whatever is in hand
 * has Grace to finish, is then stopped, and Serve returns. An error is
 * returned only for a failure of the streams themselves.
 */
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()

	se := &session{s: s, ctx: ctx, out: out, inHand: map[string]*call{}}

	most := s.MostBytes
	if most <= 0 {
		most = DefaultMostBytes
	}

	atOnce := s.AtOnce
	if atOnce <= 0 {
		atOnce = DefaultAtOnce
	}

	var (
		working sync.WaitGroup
		slots   = make(chan struct{}, atOnce)
		reader  = bufio.NewReaderSize(in, 64*1024)
		failed  error
	)

	for {
		line, tooLong, err := readLine(reader, most)

		if tooLong {
			se.write(response{ID: null, Error: &rpcError{Code: codeInvalidRequest,
				Message: fmt.Sprintf("That line is too long: a request here is at most %d bytes.", most)}})
		} else if len(bytes.TrimSpace(line)) > 0 {
			if req, ok := se.parse(line); ok {
				switch {
				case se.immediate(req):
					// Answered, or needing no answer.
				case req.Method == "initialize":
					/*
					 * The handshake is answered here, in the order it was
					 * read: a client that writes its next request without
					 * waiting must find the handshake already made.
					 */
					if result, failure := se.shake(req.Params); failure != nil {
						se.write(response{ID: req.ID, Error: failure})
					} else {
						se.write(response{ID: req.ID, Result: result})
					}
				case !se.begin(req):
					se.write(response{ID: req.ID, Error: &rpcError{Code: codeInternal,
						Message: "Too many requests are waiting for an answer: wait for some, then ask again."}})
				default:
					working.Add(1)

					/*
					 * The wait for a free place is in the request's own
					 * goroutine, never here: this loop must keep reading,
					 * or a client could not give up on what is in hand.
					 */
					go func() {
						defer working.Done()

						select {
						case slots <- struct{}{}:
							defer func() { <-slots }()

							se.work(req)
						case <-ctx.Done():
						}
					}()
				}
			}
		}

		if err != nil {
			if !errors.Is(err, io.EOF) {
				failed = err
			}

			break
		}
	}

	/*
	 * The input has ended: the client has gone, or it was a script that
	 * wrote its requests and closed. What is in hand is given a short while
	 * to finish and be answered, then stopped.
	 */
	grace := s.Grace
	if grace <= 0 {
		grace = DefaultGrace
	}

	finished := make(chan struct{})

	go func() {
		working.Wait()
		close(finished)
	}()

	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case <-finished:
	case <-timer.C:
	case <-ctx.Done():
	}

	stop()
	<-finished

	return failed
}

var null = json.RawMessage("null")

// readLine reads one line of at most most bytes. A longer one is read to
// its end and thrown away, so the line after it starts where a line starts.
func readLine(r *bufio.Reader, most int) (line []byte, tooLong bool, err error) {
	for {
		part, err := r.ReadSlice('\n')

		if !tooLong {
			line = append(line, part...)

			if len(line) > most {
				line, tooLong = nil, true
			}
		}

		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}

		return line, tooLong, err
	}
}

// parse reads a line as one request or notification. What cannot be one is
// answered here.
func (se *session) parse(line []byte) (request, bool) {
	var req request

	trimmed := bytes.TrimSpace(line)

	if trimmed[0] == '[' && json.Valid(trimmed) {
		se.write(response{ID: null, Error: &rpcError{Code: codeInvalidRequest,
			Message: "Send one message a line: batches are not part of the protocol any more."}})

		return req, false
	}

	if err := json.Unmarshal(trimmed, &req); err != nil {
		se.write(response{ID: null, Error: &rpcError{Code: codeParse, Message: "That line is not JSON."}})

		return req, false
	}

	return req, true
}

// usable reports whether an id is one the protocol allows: text or a whole
// number, never null.
func usable(id json.RawMessage) bool {
	id = bytes.TrimSpace(id)
	if len(id) == 0 {
		return false
	}

	if id[0] == '"' {
		var s string

		return json.Unmarshal(id, &s) == nil
	}

	if id[0] == '-' || (id[0] >= '0' && id[0] <= '9') {
		var n json.Number

		return json.Unmarshal(id, &n) == nil
	}

	return false
}

/*
 * immediate deals with everything that is not a request to work on:
 * notifications (never answered), a client's own answers (it is asked
 * nothing, so there are none to expect), and requests so malformed they
 * have no usable id.
 */
func (se *session) immediate(req request) bool {
	hasID := len(bytes.TrimSpace(req.ID)) > 0

	if !hasID {
		if req.Method == "notifications/cancelled" {
			se.giveUp(req.Params)
		}

		return true
	}

	if req.Method == "" && (len(req.Result) > 0 || len(req.Error) > 0) {
		return true
	}

	if !usable(req.ID) {
		se.write(response{ID: null, Error: &rpcError{Code: codeInvalidRequest,
			Message: "A request's id is text or a whole number."}})

		return true
	}

	if req.Method == "" {
		se.write(response{ID: req.ID, Error: &rpcError{Code: codeInvalidRequest, Message: "A request names a method."}})

		return true
	}

	return false
}

// MostWaiting is how many requests may be in hand or waiting their turn.
const MostWaiting = 256

// begin notes a request as in hand, so the client can give up on it. It
// reports false when there are already as many as will be kept.
func (se *session) begin(req request) bool {
	se.mu.Lock()
	defer se.mu.Unlock()

	if len(se.inHand) >= MostWaiting {
		return false
	}

	se.inHand[string(bytes.TrimSpace(req.ID))] = &call{}

	return true
}

// giveUp stops a request the client no longer wants; it is never answered.
func (se *session) giveUp(params json.RawMessage) {
	var which struct {
		RequestID json.RawMessage `json:"requestId"`
	}

	if json.Unmarshal(params, &which) != nil {
		return
	}

	se.mu.Lock()
	defer se.mu.Unlock()

	if c := se.inHand[string(bytes.TrimSpace(which.RequestID))]; c != nil {
		c.givenUp = true

		if c.cancel != nil {
			c.cancel()
		}
	}
}

// work answers one request, unless the client gave up on it meanwhile.
func (se *session) work(req request) {
	id := string(bytes.TrimSpace(req.ID))

	ctx, cancel := context.WithCancel(se.ctx)
	defer cancel()

	se.mu.Lock()
	c := se.inHand[id]

	if c == nil || c.givenUp {
		delete(se.inHand, id)
		se.mu.Unlock()

		return
	}

	c.cancel = cancel
	se.mu.Unlock()

	result, failure := se.answer(ctx, req)

	se.mu.Lock()
	givenUp := c.givenUp

	if se.inHand[id] == c {
		delete(se.inHand, id)
	}
	se.mu.Unlock()

	// A client that has gone, or gave up on this, is told nothing more.
	if givenUp || se.ctx.Err() != nil {
		return
	}

	if failure != nil {
		se.write(response{ID: req.ID, Error: failure})

		return
	}

	se.write(response{ID: req.ID, Result: result})
}

// answer is the result of one request, or why there is none. A fault in a
// tool is caught here: it ends that request, not the conversation.
func (se *session) answer(ctx context.Context, req request) (result map[string]any, failure *rpcError) {
	defer func() {
		if r := recover(); r != nil {
			se.log("a fault while answering %s: %v", req.Method, r)

			result, failure = nil, &rpcError{Code: codeInternal, Message: "Something went wrong inside the assistant's server while answering."}
		}
	}()

	modern, who, failure := se.era(req)
	if failure != nil {
		return nil, failure
	}

	keeps := false

	switch req.Method {
	case "server/discover":
		keeps = true
		result = map[string]any{
			"supportedVersions": Versions(),
			"capabilities":      capabilities(),
			"instructions":      se.s.Instructions,
		}
	case "ping":
		result = map[string]any{}
	case "tools/list":
		keeps = true
		result = map[string]any{"tools": se.s.list()}
	case "tools/call":
		result, failure = se.s.call(ctx, req.Params, who)
		if failure != nil {
			return nil, failure
		}
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "This server has no method " + printable(req.Method, 80) + ": it offers tools only."}
	}

	if modern {
		result["resultType"] = "complete"
		result["_meta"] = map[string]any{metaServer: se.s.info()}

		// The current protocol requires both on what a client may keep.
		if keeps {
			result["ttlMs"] = KeepFor.Milliseconds()
			result["cacheScope"] = "public"
		}
	}

	return result, nil
}

func capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{}}
}

func (s *Server) info() map[string]string {
	return map[string]string{"name": s.Name, "version": s.Version}
}

// shake is the older handshake: the client says what it speaks, and is
// told what will be spoken: its version when it is one served here, the
// newest older one otherwise.
func (se *session) shake(params json.RawMessage) (map[string]any, *rpcError) {
	var hello struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}

	if err := json.Unmarshal(orEmpty(params), &hello); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "initialize takes protocolVersion, capabilities and clientInfo."}
	}

	version := Legacy

	for _, ours := range legacyVersions {
		if ours == hello.ProtocolVersion {
			version = ours
		}
	}

	se.mu.Lock()
	se.shaken, se.version = true, version
	se.client = Caller{Name: printable(hello.ClientInfo.Name, 60), Version: printable(hello.ClientInfo.Version, 30)}
	se.mu.Unlock()

	return map[string]any{
		"protocolVersion": version,
		"capabilities":    capabilities(),
		"serverInfo":      se.s.info(),
		"instructions":    se.s.Instructions,
	}, nil
}

/*
 * era works out which protocol a request is in, and who is asking.
 *
 * One that says its version is current and stands alone: the version must
 * be the one served, and the client's capabilities must be there, as the
 * specification requires of every such request. One that says nothing is
 * from an older client, which must have said initialize first.
 */
func (se *session) era(req request) (modern bool, who Caller, failure *rpcError) {
	var params struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}

	if err := json.Unmarshal(orEmpty(req.Params), &params); err != nil {
		return false, who, &rpcError{Code: codeInvalidParams, Message: "A request's params are an object."}
	}

	raw, says := params.Meta[metaVersion]

	if !says {
		se.mu.Lock()
		shaken, client := se.shaken, se.client
		se.mu.Unlock()

		if !shaken {
			return false, who, &rpcError{Code: codeInvalidParams, Message: "Say which protocol version this request is in (_meta, " +
				metaVersion + ": " + Modern + "), or start with initialize (" + strings.Join(legacyVersions, ", ") + ")."}
		}

		return false, client, nil
	}

	var version string

	if json.Unmarshal(raw, &version) != nil || version != Modern {
		return true, who, &rpcError{Code: codeUnsupportedVersion, Message: "Unsupported protocol version",
			Data: map[string]any{"supported": Versions(), "requested": printable(version, 40)}}
	}

	if _, has := params.Meta[metaCapabilities]; !has {
		return true, who, &rpcError{Code: codeInvalidParams, Message: "The request's _meta is missing " + metaCapabilities + "."}
	}

	var client struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	if raw, has := params.Meta[metaClient]; has {
		_ = json.Unmarshal(raw, &client)
	}

	return true, Caller{Name: printable(client.Name, 60), Version: printable(client.Version, 30)}, nil
}

// emptySchema is the arguments of a tool that takes none.
var emptySchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

func (s *Server) list() []map[string]any {
	out := make([]map[string]any, 0, len(s.Tools))

	for _, t := range s.Tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = emptySchema
		}

		tool := map[string]any{"name": t.Name, "description": t.Description, "inputSchema": schema}

		if t.Title != "" {
			tool["title"] = t.Title
		}

		if t.ReadOnly {
			tool["annotations"] = map[string]any{"readOnlyHint": true, "openWorldHint": false}
		}

		out = append(out, tool)
	}

	return out
}

// call runs one tool. A tool that could not do it is still an answer, one
// the asking model can read and act on; only a call that could not be made
// at all is an error of the protocol.
func (s *Server) call(ctx context.Context, params json.RawMessage, who Caller) (map[string]any, *rpcError) {
	var asked struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(orEmpty(params), &asked); err != nil || asked.Name == "" {
		return nil, &rpcError{Code: codeInvalidParams, Message: "tools/call takes the tool's name and its arguments."}
	}

	arguments := bytes.TrimSpace(asked.Arguments)
	if len(arguments) == 0 || bytes.Equal(arguments, null) {
		arguments = []byte("{}")
	}

	if arguments[0] != '{' {
		return nil, &rpcError{Code: codeInvalidParams, Message: "A tool's arguments are an object."}
	}

	for _, t := range s.Tools {
		if t.Name != asked.Name {
			continue
		}

		text, err := t.Run(ctx, arguments, who)
		if err != nil {
			return map[string]any{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}

		return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": false}, nil
	}

	return nil, &rpcError{Code: codeInvalidParams, Message: "This server has no tool " + printable(asked.Name, 80) + "."}
}

// write sends one message as one line. A message is marshalled whole, so a
// line break inside any text is written as an escape, never as a break.
func (se *session) write(r response) {
	r.JSONRPC = "2.0"

	if len(r.ID) == 0 {
		r.ID = null
	}

	if r.Error == nil && r.Result == nil {
		r.Result = map[string]any{}
	}

	line, err := json.Marshal(r)
	if err != nil {
		se.log("an answer could not be written: %v", err)

		return
	}

	se.writing.Lock()
	defer se.writing.Unlock()

	if se.broken {
		return
	}

	if _, err := se.out.Write(append(line, '\n')); err != nil {
		se.broken = true

		se.log("the output closed: %v", err)
	}
}

func (se *session) log(format string, args ...any) {
	if se.s.Log != nil {
		fmt.Fprintf(se.s.Log, "pn-scripts-assistant mcp: "+format+"\n", args...)
	}
}

func orEmpty(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), null) {
		return json.RawMessage("{}")
	}

	return raw
}

// printable is text another program gave, made safe to show: one line, no
// control characters, at most so many characters.
func printable(s string, most int) string {
	var b strings.Builder

	n := 0

	for _, r := range s {
		if n >= most {
			break
		}

		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			if r == '\n' || r == '\t' || r == '\r' {
				b.WriteByte(' ')
				n++
			}

			continue
		}

		b.WriteRune(r)
		n++
	}

	return strings.TrimSpace(b.String())
}
