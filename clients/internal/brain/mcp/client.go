/*
 * Package mcp connects this program to Model Context Protocol servers — as a
 * client, and only through its own gate.
 *
 * An MCP server is somebody else's program offering tools, readable resources
 * and prompts. This package speaks the protocol to it; the gateway beside it
 * decides who may use what. Nothing a server offers reaches a model directly:
 * each of its tools is wrapped as one of this program's tools, with its risk,
 * its approval, its project boundary and its privacy rule, and everything a
 * server says back is handed on as information, never as instructions.
 *
 * Both eras of the protocol are spoken. Servers from 2026-07-28 on are
 * stateless — every request carries its version and who is asking — and
 * answer server/discover; older ones expect an initialize handshake first.
 * The client asks the modern question and falls back when it is not
 * understood, as the specification says to, and remembers the answer for as
 * long as the server runs.
 */
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Protocol versions, newest first. Modern ones carry everything per request.
const (
	Modern = "2026-07-28"
	Legacy = "2025-11-25"
)

var (
	modernVersions = []string{"2026-07-28"}
	legacyVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}
)

// ClientName is how this program introduces itself to servers.
const ClientName = "pn-scripts-assistant"

// ClientVersion is the version it gives.
var ClientVersion = "1"

// Error codes the protocol reserves, which say "this server is modern".
const (
	codeUnsupportedVersion = -32022
	codeMissingCapability  = -32021
	codeHeaderMismatch     = -32020
	codeMethodNotFound     = -32601
)

// RPCError is an error a server answered with.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("the server answered %d: %s", e.Code, e.Message)
}

func modernError(err error) (*RPCError, bool) {
	var rpc *RPCError

	if errors.As(err, &rpc) {
		switch rpc.Code {
		case codeUnsupportedVersion, codeMissingCapability, codeHeaderMismatch:
			return rpc, true
		}
	}

	return nil, false
}

// message is one JSON-RPC message either way.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Transport carries one request to a server and its answer back.
type Transport interface {
	// Call sends a request and waits for its response.
	Call(ctx context.Context, req message, era string, version string) (message, error)

	// Notify sends a notification, which has no answer.
	Notify(ctx context.Context, req message, version string) error

	Close() error
}

// Info is what a server says about itself.
type Info struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// Tool is one tool a server offers.
type Tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Annotations *struct {
		ReadOnlyHint    *bool `json:"readOnlyHint,omitempty"`
		DestructiveHint *bool `json:"destructiveHint,omitempty"`
		OpenWorldHint   *bool `json:"openWorldHint,omitempty"`
	} `json:"annotations,omitempty"`
}

// Resource is one thing a server offers to be read.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// Prompt is one prompt a server offers.
type Prompt struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Arguments   []struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Required    bool   `json:"required,omitempty"`
	} `json:"arguments,omitempty"`
}

// Content is one piece of what a server sent back.
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
	Resource *struct {
		URI      string `json:"uri"`
		MimeType string `json:"mimeType,omitempty"`
		Text     string `json:"text,omitempty"`
		Blob     string `json:"blob,omitempty"`
	} `json:"resource,omitempty"`
}

// CallResult is what a tool call came back with.
type CallResult struct {
	Content           []Content       `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
	ResultType        string          `json:"resultType,omitempty"`
}

// Client is one connection to one server.
type Client struct {
	transport Transport

	// Timeout bounds every request.
	Timeout time.Duration

	mu           sync.Mutex
	era          string
	version      string
	server       Info
	capabilities json.RawMessage
	instructions string

	next atomic.Int64

	// schemas are each tool's input schema, for the parameters a tool asks
	// to be carried in headers as well as the body.
	schemas map[string]json.RawMessage
}

// NewClient speaks to a server over a transport. Connect before anything else.
func NewClient(t Transport) *Client {
	return &Client{transport: t, Timeout: 60 * time.Second}
}

// Era is "modern" or "legacy", once connected.
func (c *Client) Era() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.era
}

// Version is the protocol version in use.
func (c *Client) Version() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.version
}

// Server is what the server says it is. Self-reported, so for showing only.
func (c *Client) Server() Info {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.server
}

// Instructions is what the server says about using it — untrusted text.
func (c *Client) Instructions() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.instructions
}

// HowLongAProbeWaits is how long an old server gets to answer a question it
// does not know before it is taken to be old: some never answer at all.
const HowLongAProbeWaits = 8 * time.Second

/*
 * Connect works out which era the server is and gets ready to talk to it.
 *
 * server/discover first, with the modern version: an answer is a modern
 * server; a modern error means modern but another version, and the client
 * uses one it offers; anything else — another error, or silence — means an
 * older server, which is then given the initialize handshake it expects.
 */
func (c *Client) Connect(ctx context.Context) error {
	probe, cancel := context.WithTimeout(ctx, HowLongAProbeWaits)

	raw, err := c.request(probe, "server/discover", map[string]any{}, "modern", Modern)

	cancel()

	if err == nil {
		var found struct {
			SupportedVersions []string        `json:"supportedVersions"`
			Capabilities      json.RawMessage `json:"capabilities"`
			Instructions      string          `json:"instructions"`
			Meta              struct {
				ServerInfo Info `json:"io.modelcontextprotocol/serverInfo"`
			} `json:"_meta"`
		}

		if json.Unmarshal(raw, &found) == nil && len(found.SupportedVersions) > 0 {
			version, ok := pick(found.SupportedVersions, modernVersions)

			if ok {
				c.mu.Lock()
				c.era, c.version = "modern", version
				c.capabilities, c.instructions, c.server = found.Capabilities, found.Instructions, found.Meta.ServerInfo
				c.mu.Unlock()

				return nil
			}

			if legacy, ok := pick(found.SupportedVersions, legacyVersions); ok {
				return c.initialize(ctx, legacy)
			}

			return fmt.Errorf("the server speaks %s, and this program speaks none of them",
				strings.Join(found.SupportedVersions, ", "))
		}
	}

	if rpc, modern := modernError(err); modern {
		var data struct {
			Supported []string `json:"supported"`
		}

		json.Unmarshal(rpc.Data, &data)

		if version, ok := pick(data.Supported, modernVersions); ok {
			c.mu.Lock()
			c.era, c.version = "modern", version
			c.mu.Unlock()

			return nil
		}

		if legacy, ok := pick(data.Supported, legacyVersions); ok {
			return c.initialize(ctx, legacy)
		}

		return fmt.Errorf("the server supports %v, and this program none of them", data.Supported)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return c.initialize(ctx, Legacy)
}

func pick(offered, ours []string) (string, bool) {
	for _, mine := range ours {
		for _, theirs := range offered {
			if mine == theirs {
				return mine, true
			}
		}
	}

	return "", false
}

// initialize is the older handshake: say what we are, hear what it is, and
// say we are ready.
func (c *Client) initialize(ctx context.Context, version string) error {
	params := map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": ClientName, "version": ClientVersion},
	}

	raw, err := c.request(ctx, "initialize", params, "legacy", version)
	if err != nil {
		return fmt.Errorf("the server would not start a session: %w", err)
	}

	var answer struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Capabilities    json.RawMessage `json:"capabilities"`
		ServerInfo      Info            `json:"serverInfo"`
		Instructions    string          `json:"instructions"`
	}

	if err := json.Unmarshal(raw, &answer); err != nil {
		return fmt.Errorf("the server's greeting could not be read: %w", err)
	}

	agreed := answer.ProtocolVersion
	if _, ok := pick([]string{agreed}, legacyVersions); !ok {
		return fmt.Errorf("the server wants protocol %s, which this program does not speak", agreed)
	}

	c.mu.Lock()
	c.era, c.version = "legacy", agreed
	c.capabilities, c.instructions, c.server = answer.Capabilities, answer.Instructions, answer.ServerInfo
	c.mu.Unlock()

	return c.transport.Notify(ctx, message{JSONRPC: "2.0", Method: "notifications/initialized"}, agreed)
}

// meta is what a modern request carries about itself.
func meta(version string) map[string]any {
	return map[string]any{
		"io.modelcontextprotocol/protocolVersion":    version,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		"io.modelcontextprotocol/clientInfo":         map[string]string{"name": ClientName, "version": ClientVersion},
	}
}

// request sends one request in one era and returns its result.
func (c *Client) request(ctx context.Context, method string, params map[string]any, era, version string) (json.RawMessage, error) {
	if params == nil {
		params = map[string]any{}
	}

	if era == "modern" {
		params["_meta"] = meta(version)
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	id := c.next.Add(1)

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	answer, err := c.transport.Call(ctx, message{JSONRPC: "2.0", ID: &id, Method: method, Params: body}, era, version)
	if err != nil {
		return nil, err
	}

	if answer.Error != nil {
		return nil, answer.Error
	}

	/*
	 * A server asking for more before it can answer — the multi round-trip
	 * pattern — wants input this program does not give servers: no sampling,
	 * no elicitation, no roots. Said, rather than retried forever.
	 */
	var shape struct {
		ResultType string `json:"resultType"`
	}

	if json.Unmarshal(answer.Result, &shape) == nil && shape.ResultType == "input_required" {
		return nil, fmt.Errorf("the server asked for more input before answering, which this program does not provide")
	}

	return answer.Result, nil
}

// call is a request in whatever era the server turned out to be.
func (c *Client) call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	era, version := c.era, c.version
	c.mu.Unlock()

	if era == "" {
		return nil, fmt.Errorf("not connected")
	}

	return c.request(ctx, method, params, era, version)
}

// Tools is every tool the server offers, following its pages.
func (c *Client) Tools(ctx context.Context) ([]Tool, error) {
	var all []Tool

	cursor := ""

	for page := 0; page < 50; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}

		raw, err := c.call(ctx, "tools/list", params)
		if err != nil {
			return all, err
		}

		var got struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}

		if err := json.Unmarshal(raw, &got); err != nil {
			return all, fmt.Errorf("the list of tools could not be read: %w", err)
		}

		all = append(all, got.Tools...)

		c.mu.Lock()
		if c.schemas == nil {
			c.schemas = map[string]json.RawMessage{}
		}

		for _, t := range got.Tools {
			c.schemas[t.Name] = t.InputSchema
		}
		c.mu.Unlock()

		if got.NextCursor == "" {
			break
		}

		cursor = got.NextCursor
	}

	return all, nil
}

// CallTool calls one tool.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (CallResult, error) {
	var args any = map[string]any{}

	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return CallResult{}, fmt.Errorf("those arguments could not be read: %w", err)
		}
	}

	c.mu.Lock()
	schema := c.schemas[name]
	c.mu.Unlock()

	if headers := paramHeaders(schema, args); len(headers) > 0 {
		ctx = WithHeaders(ctx, headers)
	}

	raw, err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return CallResult{}, err
	}

	var result CallResult

	if err := json.Unmarshal(raw, &result); err != nil {
		return CallResult{}, fmt.Errorf("the answer could not be read: %w", err)
	}

	return result, nil
}

/*
 * paramHeaders is the arguments a tool's schema asks to be mirrored into
 * headers (x-mcp-header), which a modern HTTP server may check against the
 * body. Top-level properties only, and only strings, numbers and booleans.
 */
func paramHeaders(schema json.RawMessage, args any) http.Header {
	var s struct {
		Properties map[string]struct {
			Header string `json:"x-mcp-header"`
		} `json:"properties"`
	}

	if len(schema) == 0 || json.Unmarshal(schema, &s) != nil {
		return nil
	}

	values, _ := args.(map[string]any)

	out := http.Header{}

	for name, prop := range s.Properties {
		if prop.Header == "" {
			continue
		}

		switch v := values[name].(type) {
		case string:
			out.Set("Mcp-Param-"+prop.Header, v)
		case float64:
			out.Set("Mcp-Param-"+prop.Header, strconv.FormatFloat(v, 'f', -1, 64))
		case bool:
			out.Set("Mcp-Param-"+prop.Header, strconv.FormatBool(v))
		}
	}

	return out
}

// Resources is everything the server offers to be read.
func (c *Client) Resources(ctx context.Context) ([]Resource, error) {
	raw, err := c.call(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}

	var got struct {
		Resources []Resource `json:"resources"`
	}

	return got.Resources, json.Unmarshal(raw, &got)
}

// ReadResource reads one, as text: binary contents are described, not sent.
func (c *Client) ReadResource(ctx context.Context, uri string) (string, error) {
	raw, err := c.call(ctx, "resources/read", map[string]any{"uri": uri})
	if err != nil {
		return "", err
	}

	var got struct {
		Contents []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Blob     string `json:"blob"`
		} `json:"contents"`
	}

	if err := json.Unmarshal(raw, &got); err != nil {
		return "", err
	}

	var b strings.Builder

	for _, part := range got.Contents {
		if part.Text != "" {
			b.WriteString(part.Text + "\n")
		} else if part.Blob != "" {
			fmt.Fprintf(&b, "[%s: %s, %d bytes of binary content, not shown]\n", part.URI, part.MimeType, len(part.Blob)*3/4)
		}
	}

	return strings.TrimSpace(b.String()), nil
}

// Prompts is every prompt the server offers.
func (c *Client) Prompts(ctx context.Context) ([]Prompt, error) {
	raw, err := c.call(ctx, "prompts/list", nil)
	if err != nil {
		return nil, err
	}

	var got struct {
		Prompts []Prompt `json:"prompts"`
	}

	return got.Prompts, json.Unmarshal(raw, &got)
}

// GetPrompt is one prompt's messages, as text.
func (c *Client) GetPrompt(ctx context.Context, name string, arguments map[string]string) (string, error) {
	raw, err := c.call(ctx, "prompts/get", map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return "", err
	}

	var got struct {
		Messages []struct {
			Role    string  `json:"role"`
			Content Content `json:"content"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(raw, &got); err != nil {
		return "", err
	}

	var b strings.Builder

	for _, m := range got.Messages {
		fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content.Text)
	}

	return strings.TrimSpace(b.String()), nil
}

/*
 * Healthy asks the server whether it is there. The modern way is to ask it
 * what it is again; the old way is ping, which modern servers no longer have.
 */
func (c *Client) Healthy(ctx context.Context) error {
	if c.Era() == "modern" {
		_, err := c.call(ctx, "server/discover", nil)

		return err
	}

	_, err := c.call(ctx, "ping", nil)

	return err
}

// Close ends the connection.
func (c *Client) Close() error { return c.transport.Close() }
