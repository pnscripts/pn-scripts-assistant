package tools

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pn-scripts-assistant/internal/brain/paths"
)

/*
 * Driving a real Chrome, the way its owner's work was done by hand.
 *
 * Publishing a game on itch.io or Game Jolt, answering a plugin reviewer,
 * bringing a social profile into line, reading Gmail: every one of those was
 * done in a browser, signed in, by clicking (ADR 0007). The tools that read a
 * page (fetch_url, read_a_page) see it as a stranger does and cannot act on
 * it, so this speaks the DevTools protocol to a Chrome of its own.
 *
 * Its own, not the owner's everyday Chrome: Chrome refuses remote debugging on
 * its default profile, and rightly. The profile lives beside the program's
 * other machine-wide things; he signs in there once, in the window it opens,
 * and the sign-ins stay. This program never types a password — a sign-in page
 * is something the playbooks say to stop at and hand to him.
 *
 * No library: the protocol is JSON over one WebSocket, and the few frames that
 * needs are written here rather than added as a fourth dependency.
 */

// DefaultDebugPort is where the assistant's Chrome listens, on loopback only.
const DefaultDebugPort = 9333

// DriveBrowser is the tool.
type DriveBrowser struct {
	// Port is the DevTools port; zero means DefaultDebugPort.
	Port int

	// Profile is the Chrome profile folder; empty means the program's own.
	Profile string

	// Chrome is the browser to start when none is listening; empty finds one.
	Chrome string

	// Headless starts it without a window, for tests and machines with no
	// display.
	Headless bool
}

func (DriveBrowser) Name() string { return "drive_browser" }

func (DriveBrowser) Description() string {
	return "Drive the assistant's own Chrome, signed in as the owner, over the DevTools protocol. " +
		"Actions: \"tabs\" lists open tabs; \"open\" {url} opens a new tab and returns its id; " +
		"\"navigate\" {tab,url}; \"eval\" {tab,expression} runs JavaScript in the page and returns the " +
		"result as JSON (read the DOM, click with element.click(), fill a field by setting .value and " +
		"dispatching input and change events); \"text\" {tab} returns the visible text; \"screenshot\" " +
		"{tab,path} saves a PNG; \"close\" {tab}. If a sign-in page appears, stop and ask the owner to " +
		"sign in in that window; never type a password."
}

func (DriveBrowser) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["tabs", "open", "navigate", "eval", "text", "screenshot", "close"]},
			"tab": {"type": "string", "description": "A tab id from tabs or open"},
			"url": {"type": "string"},
			"expression": {"type": "string", "description": "JavaScript to run in the page; a promise is awaited"},
			"path": {"type": "string", "description": "Where to save the screenshot, a .png"}
		},
		"required": ["action"]
	}`)
}

// Mutating: it acts in its owner's accounts.
func (DriveBrowser) Risk() Risk { return Mutating }

type browserArgs struct {
	Action     string `json:"action"`
	Tab        string `json:"tab"`
	URL        string `json:"url"`
	Expression string `json:"expression"`
	Path       string `json:"path"`
}

func (DriveBrowser) Summarize(raw json.RawMessage) string {
	var a browserArgs
	argsOf(raw, &a)

	switch a.Action {
	case "open", "navigate":
		return "Open " + a.URL + " in the assistant's Chrome"
	case "eval":
		return "Run in the page: " + truncateRunes(strings.Join(strings.Fields(a.Expression), " "), 160)
	case "screenshot":
		return "Photograph the page to " + a.Path
	case "text":
		return "Read the page in the assistant's Chrome"
	case "close":
		return "Close a tab in the assistant's Chrome"
	}

	return "List the tabs in the assistant's Chrome"
}

// Touches: a screenshot writes where its path says; nothing else writes files.
func (DriveBrowser) Touches(raw json.RawMessage) ([]string, []string) {
	var a browserArgs
	argsOf(raw, &a)

	if a.Action == "screenshot" {
		return []string{a.Path}, nil
	}

	return []string{}, nil
}

func (DriveBrowser) Cues() []string {
	return []string{"browser", "chrome", "tab", "click", "website", "page", "itch", "itch.io", "game jolt",
		"gamejolt", "gmail", "facebook", "linkedin", "youtube", "wordpress", "publish", "upload", "profile",
		"sign in", "form"}
}

func (d DriveBrowser) port() int {
	if d.Port > 0 {
		return d.Port
	}

	return DefaultDebugPort
}

func (d DriveBrowser) base() string { return "http://127.0.0.1:" + strconv.Itoa(d.port()) }

// MostPageText bounds what "text" hands back.
const MostPageText = 20000

func (d DriveBrowser) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a browserArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := d.ensure(ctx); err != nil {
		return "", err
	}

	switch a.Action {
	case "", "tabs":
		tabs, err := d.tabs(ctx)
		if err != nil {
			return "", err
		}

		out, _ := json.MarshalIndent(tabs, "", "  ")

		return string(out), nil

	case "open":
		if a.URL == "" {
			return "", errors.New("open needs a url")
		}

		var tab devTab

		if err := d.call(ctx, http.MethodPut, "/json/new?"+url.QueryEscape(a.URL), &tab); err != nil {
			return "", err
		}

		return fmt.Sprintf(`{"tab":%q,"url":%q}`, tab.ID, a.URL), nil

	case "close":
		if a.Tab == "" {
			return "", errors.New("close needs a tab")
		}

		if err := d.call(ctx, http.MethodGet, "/json/close/"+url.PathEscape(a.Tab), nil); err != nil {
			return "", err
		}

		return "closed " + a.Tab, nil
	}

	if a.Tab == "" {
		return "", fmt.Errorf("%s needs a tab: use tabs or open first", a.Action)
	}

	page, err := d.attach(ctx, a.Tab)
	if err != nil {
		return "", err
	}
	defer page.Close()

	switch a.Action {
	case "navigate":
		if a.URL == "" {
			return "", errors.New("navigate needs a url")
		}

		if _, err := page.Send(ctx, "Page.navigate", map[string]any{"url": a.URL}); err != nil {
			return "", err
		}

		return "navigating to " + a.URL, nil

	case "eval":
		return page.Evaluate(ctx, a.Expression)

	case "text":
		out, err := page.Evaluate(ctx, "document.body ? document.body.innerText : ''")
		if err != nil {
			return "", err
		}

		var text string
		if json.Unmarshal([]byte(out), &text) != nil {
			text = out
		}

		return truncateRunes(text, MostPageText), nil

	case "screenshot":
		if a.Path == "" {
			return "", errors.New("screenshot needs a path")
		}

		result, err := page.Send(ctx, "Page.captureScreenshot", map[string]any{"format": "png"})
		if err != nil {
			return "", err
		}

		var shot struct {
			Data string `json:"data"`
		}

		if err := json.Unmarshal(result, &shot); err != nil {
			return "", err
		}

		png, err := base64.StdEncoding.DecodeString(shot.Data)
		if err != nil {
			return "", err
		}

		if err := os.MkdirAll(filepath.Dir(a.Path), 0o755); err != nil {
			return "", err
		}

		if err := os.WriteFile(a.Path, png, 0o644); err != nil {
			return "", err
		}

		return fmt.Sprintf("saved %s (%d bytes)", a.Path, len(png)), nil
	}

	return "", fmt.Errorf("there is no action %q", a.Action)
}

// devTab is one target as Chrome lists it.
type devTab struct {
	ID    string `json:"id"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title"`
	URL   string `json:"url"`
	WS    string `json:"webSocketDebuggerUrl,omitempty"`
}

func (d DriveBrowser) tabs(ctx context.Context) ([]devTab, error) {
	var all []devTab

	if err := d.call(ctx, http.MethodGet, "/json/list", &all); err != nil {
		return nil, err
	}

	out := []devTab{}

	for _, t := range all {
		if t.Type == "" || t.Type == "page" {
			out = append(out, devTab{ID: t.ID, Title: t.Title, URL: t.URL})
		}
	}

	return out, nil
}

func (d DriveBrowser) call(ctx context.Context, method, path string, into any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, d.base()+path, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("the assistant's Chrome did not answer: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Chrome said %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if into == nil {
		return nil
	}

	return json.Unmarshal(body, into)
}

/*
 * ensure starts the assistant's Chrome when nothing is listening.
 *
 * Started in a session of its own, so it outlives the call and the program —
 * it is a browser somebody signs in to and keeps, not a tool run.
 */
func (d DriveBrowser) ensure(ctx context.Context) error {
	if d.call(ctx, http.MethodGet, "/json/version", nil) == nil {
		return nil
	}

	chrome := d.Chrome
	if chrome == "" {
		chrome = browserOnThisMachine()
	}

	if chrome == "" {
		return errors.New("there is no Chrome or Chromium on this machine to drive")
	}

	profile := d.Profile
	if profile == "" {
		profile = filepath.Join(paths.MachineFolder(), "chrome-profile")
	}

	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}

	args := []string{"--remote-debugging-port=" + strconv.Itoa(d.port()), "--remote-debugging-address=127.0.0.1",
		"--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check"}

	if d.Headless {
		args = append(args, "--headless=new", "--disable-gpu")
	}

	cmd := exec.Command(chrome, append(args, "about:blank")...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start Chrome: %w", err)
	}

	go cmd.Wait()

	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if d.call(ctx, http.MethodGet, "/json/version", nil) == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}

	return errors.New("Chrome was started and did not open its DevTools port within 20 seconds")
}

func browserOnThisMachine() string {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if at, err := exec.LookPath(name); err == nil {
			return at
		}
	}

	return ""
}

func (d DriveBrowser) attach(ctx context.Context, tab string) (*CDPPage, error) {
	var all []devTab

	if err := d.call(ctx, http.MethodGet, "/json/list", &all); err != nil {
		return nil, err
	}

	for _, t := range all {
		if t.ID == tab && t.WS != "" {
			return DialCDP(ctx, t.WS)
		}
	}

	return nil, fmt.Errorf("there is no tab %q; use tabs to list them", tab)
}

/*
 * CDPPage is one DevTools connection: requests by id, events skipped.
 */
type CDPPage struct {
	conn net.Conn
	r    *bufio.Reader
	next int
}

// DialCDP opens the WebSocket at a DevTools address.
func DialCDP(ctx context.Context, address string) (*CDPPage, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "ws" {
		return nil, fmt.Errorf("%q is not a DevTools address", address)
	}

	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}

	keyBytes := make([]byte, 16)
	rand.Read(keyBytes)
	key := base64.StdEncoding.EncodeToString(keyBytes)

	request := "GET " + u.RequestURI() + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUpgrade: websocket\r\n" +
		"Connection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n\r\n"

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err := conn.Write([]byte(request)); err != nil {
		conn.Close()

		return nil, err
	}

	r := bufio.NewReader(conn)

	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		conn.Close()

		return nil, err
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()

		return nil, fmt.Errorf("DevTools refused the connection: %s", resp.Status)
	}

	conn.SetDeadline(time.Time{})

	return &CDPPage{conn: conn, r: r}, nil
}

// Close ends the connection.
func (p *CDPPage) Close() error { return p.conn.Close() }

// Send calls one DevTools method and returns its result.
func (p *CDPPage) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	p.next++
	id := p.next

	msg, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})

	deadline := time.Now().Add(60 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	p.conn.SetDeadline(deadline)
	defer p.conn.SetDeadline(time.Time{})

	if err := writeFrame(p.conn, 0x1, msg); err != nil {
		return nil, err
	}

	for {
		data, err := p.readMessage()
		if err != nil {
			return nil, err
		}

		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if json.Unmarshal(data, &reply) != nil || reply.ID != id {
			continue // an event, or somebody else's answer
		}

		if reply.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, reply.Error.Message)
		}

		return reply.Result, nil
	}
}

// Evaluate runs an expression and returns its value as JSON.
func (p *CDPPage) Evaluate(ctx context.Context, expression string) (string, error) {
	result, err := p.Send(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true, "awaitPromise": true,
	})
	if err != nil {
		return "", err
	}

	var out struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}

	if err := json.Unmarshal(result, &out); err != nil {
		return "", err
	}

	if out.Exception != nil {
		return "", fmt.Errorf("the page threw: %s", orText(out.Exception.Exception.Description, out.Exception.Text))
	}

	if len(out.Result.Value) == 0 {
		return out.Result.Type, nil
	}

	return string(out.Result.Value), nil
}

func orText(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}

	return b
}

// readMessage reads one whole message, answering pings on the way.
func (p *CDPPage) readMessage() ([]byte, error) {
	var message []byte

	for {
		head := make([]byte, 2)
		if _, err := io.ReadFull(p.r, head); err != nil {
			return nil, err
		}

		fin, opcode := head[0]&0x80 != 0, head[0]&0x0f
		masked, size := head[1]&0x80 != 0, uint64(head[1]&0x7f)

		switch size {
		case 126:
			ext := make([]byte, 2)
			if _, err := io.ReadFull(p.r, ext); err != nil {
				return nil, err
			}

			size = uint64(binary.BigEndian.Uint16(ext))
		case 127:
			ext := make([]byte, 8)
			if _, err := io.ReadFull(p.r, ext); err != nil {
				return nil, err
			}

			size = binary.BigEndian.Uint64(ext)
		}

		if size > 256<<20 {
			return nil, errors.New("a DevTools message larger than 256 MB")
		}

		var mask []byte
		if masked {
			mask = make([]byte, 4)
			if _, err := io.ReadFull(p.r, mask); err != nil {
				return nil, err
			}
		}

		payload := make([]byte, size)
		if _, err := io.ReadFull(p.r, payload); err != nil {
			return nil, err
		}

		for i := range payload {
			if masked {
				payload[i] ^= mask[i%4]
			}
		}

		switch opcode {
		case 0x8:
			return nil, errors.New("DevTools closed the connection")
		case 0x9:
			writeFrame(p.conn, 0xA, payload)

			continue
		case 0xA:
			continue
		}

		message = append(message, payload...)

		if fin {
			return message, nil
		}
	}
}

// writeFrame writes one masked frame, as a client must.
func writeFrame(w io.Writer, opcode byte, payload []byte) error {
	frame := []byte{0x80 | opcode}

	switch n := len(payload); {
	case n < 126:
		frame = append(frame, 0x80|byte(n))
	case n < 1<<16:
		frame = append(frame, 0x80|126, byte(n>>8), byte(n))
	default:
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(n))
		frame = append(append(frame, 0x80|127), ext...)
	}

	mask := make([]byte, 4)
	rand.Read(mask)
	frame = append(frame, mask...)

	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}

	_, err := w.Write(append(frame, masked...))

	return err
}

func truncateRunes(s string, most int) string {
	r := []rune(s)
	if len(r) <= most {
		return s
	}

	return string(r[:most]) + "…"
}
