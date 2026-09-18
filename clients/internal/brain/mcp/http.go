package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

/*
 * A server somewhere else, spoken to over Streamable HTTP.
 *
 * Every request is a POST of one message. The answer is either the response
 * itself or a stream of events with the response among them. Modern servers
 * are told on every request which version and which method, in headers as
 * well as the body; older ones hand out a session on initialize, which is
 * sent back on everything after.
 *
 * Redirects are not followed. A server that answers from somewhere else is
 * not the server its owner approved, and following it would hand whatever
 * carries its credentials to wherever it pointed.
 */
type HTTP struct {
	URL string

	// Header is what goes with every request — the credential its owner gave
	// this server, and nothing else of this program's.
	Header http.Header

	Client *http.Client

	mu      sync.Mutex
	session string
}

// NewHTTP speaks to a server at a URL.
func NewHTTP(url string, header http.Header) *HTTP {
	return &HTTP{
		URL:    url,
		Header: header,
		Client: &http.Client{
			Timeout: 2 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("the server tried to send this request somewhere else, which is not followed")
			},
		},
	}
}

type headersKey struct{}

// WithHeaders carries per-call headers — a tool's x-mcp-header parameters —
// down to the transport.
func WithHeaders(ctx context.Context, h http.Header) context.Context {
	return context.WithValue(ctx, headersKey{}, h)
}

// headerValue is a value as a header may carry it, base64 in the protocol's
// sentinel when it may not be carried plainly.
func headerValue(value string) string {
	plain := value == strings.TrimSpace(value)

	for _, r := range value {
		if r < 0x20 && r != '\t' || r > 0x7e {
			plain = false

			break
		}
	}

	if strings.HasPrefix(value, "=?base64?") && strings.HasSuffix(value, "?=") {
		plain = false
	}

	if plain {
		return value
	}

	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

func (h *HTTP) post(ctx context.Context, m message, version string) (*http.Response, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	for name, values := range h.Header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	if version != "" {
		req.Header.Set("MCP-Protocol-Version", version)
	}

	req.Header.Set("Mcp-Method", m.Method)

	if name := nameOf(m); name != "" {
		req.Header.Set("Mcp-Name", headerValue(name))
	}

	if extra, ok := ctx.Value(headersKey{}).(http.Header); ok {
		for name, values := range extra {
			for _, v := range values {
				req.Header.Add(name, headerValue(v))
			}
		}
	}

	h.mu.Lock()
	if h.session != "" {
		req.Header.Set("Mcp-Session-Id", h.session)
	}
	h.mu.Unlock()

	return h.Client.Do(req)
}

// nameOf is what Mcp-Name carries: the tool's or prompt's name, or the
// resource's address.
func nameOf(m message) string {
	switch m.Method {
	case "tools/call", "prompts/get", "resources/read":
	default:
		return ""
	}

	var p struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	}

	json.Unmarshal(m.Params, &p)

	if p.URI != "" {
		return p.URI
	}

	return p.Name
}

// Call posts a request and reads its answer, however it comes.
func (h *HTTP) Call(ctx context.Context, m message, era, version string) (message, error) {
	resp, err := h.post(ctx, m, version)
	if err != nil {
		return message{}, fmt.Errorf("could not reach the server: %w", err)
	}

	defer resp.Body.Close()

	if era == "legacy" && m.Method == "initialize" {
		if session := resp.Header.Get("Mcp-Session-Id"); session != "" {
			h.mu.Lock()
			h.session = session
			h.mu.Unlock()
		}
	}

	/*
	 * A 400 is read before it is judged: modern servers answer a version
	 * they do not speak with a 400 whose body says which they do, and that
	 * is how a client knows to retry rather than give up.
	 */
	if resp.StatusCode == http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

		var answer message

		if json.Unmarshal(raw, &answer) == nil && answer.Error != nil {
			return answer, nil
		}

		return message{}, fmt.Errorf("the server refused the request (400): %s", strings.TrimSpace(string(raw[:min(len(raw), 200)])))
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return message{}, fmt.Errorf("the server refused the credentials (%d)", resp.StatusCode)
	}

	if resp.StatusCode/100 != 2 {
		return message{}, fmt.Errorf("the server answered %s", resp.Status)
	}

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readEvents(resp.Body, *m.ID)
	}

	var answer message

	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&answer); err != nil {
		return message{}, fmt.Errorf("the answer could not be read: %w", err)
	}

	return answer, nil
}

// readEvents reads a stream until the response to one request is in it.
func readEvents(body io.Reader, id int64) (message, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 32<<20)

	var data strings.Builder

	take := func() (message, bool) {
		defer data.Reset()

		var m message

		if json.Unmarshal([]byte(data.String()), &m) != nil {
			return message{}, false
		}

		return m, m.ID != nil && *m.ID == id && m.Method == ""
	}

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case line == "":
			if data.Len() > 0 {
				if m, ok := take(); ok {
					return m, nil
				}
			}
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteString("\n")
			}

			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}

	if data.Len() > 0 {
		if m, ok := take(); ok {
			return m, nil
		}
	}

	return message{}, errors.New("the server's stream ended without an answer")
}

// Notify posts a notification; it has no answer.
func (h *HTTP) Notify(ctx context.Context, m message, version string) error {
	resp, err := h.post(ctx, m, version)
	if err != nil {
		return err
	}

	resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("the server answered %s", resp.Status)
	}

	return nil
}

// Close ends an old-style session, if there was one. There is nothing to end
// for a modern server.
func (h *HTTP) Close() error {
	h.mu.Lock()
	session := h.session
	h.mu.Unlock()

	if session == "" {
		return nil
	}

	req, err := http.NewRequest(http.MethodDelete, h.URL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Mcp-Session-Id", session)

	if resp, err := h.Client.Do(req); err == nil {
		resp.Body.Close()
	}

	return nil
}
