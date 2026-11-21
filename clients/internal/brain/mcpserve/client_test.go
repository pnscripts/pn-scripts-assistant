package mcpserve_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/mcp"
	"pn-scripts-assistant/internal/brain/vouch"
)

/*
 * The server, started as a program and talked to by a real client.
 *
 * The other tests hand Serve its lines directly. Here it is a process with
 * real pipes, and the client is the one this program itself uses for other
 * people's servers (package mcp), which asks the current question first and
 * falls back to the older handshake: both ends were written from the
 * specification apart, and here they have to agree.
 */

// clientKey is the key the stand-in assistant of these tests seals its
// answers with, and keyFile the file the stand-in program reads it from
// (the real program reads its account's: vouch.Here).
var clientKey = bytes.Repeat([]byte{5}, 32)

func sealing(h http.Handler) http.Handler {
	return vouch.Sealing(func(*http.Request) []byte { return clientKey }, h)
}

func keyFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), vouch.FileName)

	if err := os.WriteFile(path, []byte(hex.EncodeToString(clientKey)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func standIn(t *testing.T) string {
	t.Helper()

	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the test builds its stand-in with go: %v", err)
	}

	name := "pn-scripts-assistant-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	bin := filepath.Join(t.TempDir(), name)

	cmd := exec.Command(gobin, "build", "-o", bin, "./testdata/standin")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the stand-in: %v\n%s", err, out)
	}

	return bin
}

// asked is who a stand-in assistant was asked by through its door:
// "<tool> by <name>/<version>".
type asked struct {
	mu    sync.Mutex
	asked []string
}

func (a *asked) all() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]string(nil), a.asked...)
}

// running is a stand-in assistant, and who asked it.
func running(t *testing.T) (*httptest.Server, *asked) {
	t.Helper()

	kept := &asked{}

	ts := httptest.NewServer(sealing(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/version":
			fmt.Fprint(w, `{"product":"PN Scripts Assistant","version":"1.1.0","platform":"linux/amd64"}`)
		case "GET /api/terms":
			fmt.Fprint(w, `{"accepted":true}`)
		case "POST /api/outside/ask":
			var q struct {
				Tool   string `json:"tool"`
				Client struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"client"`
			}

			json.NewDecoder(r.Body).Decode(&q)

			kept.mu.Lock()
			kept.asked = append(kept.asked, q.Tool+" by "+q.Client.Name+"/"+q.Client.Version)
			kept.mu.Unlock()

			if q.Tool == "assistant_status" {
				fmt.Fprint(w, `{"noted":true}`)

				return
			}

			fmt.Fprint(w, `{"programs":[{"kind":"deb","id":"gimp","name":"gimp","version":"2.10.36","present":true}]}`)
		default:
			http.NotFound(w, r)
		}
	})))

	t.Cleanup(ts.Close)

	return ts, kept
}

func TestARealClientFindsItCurrentAndUsesItsTools(t *testing.T) {
	bin := standIn(t)
	ts, kept := running(t)

	stdio, err := mcp.StartStdio([]string{bin}, []string{"PN_SCRIPTS_ASSISTANT_API_URL=" + ts.URL, "STANDIN_KEY_FILE=" + keyFile(t)}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	client := mcp.NewClient(stdio)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connecting: %v\nit said: %s", err, stdio.Said())
	}

	if client.Era() != "modern" || client.Version() != mcp.Modern {
		t.Errorf("found %s %s, want the current protocol", client.Era(), client.Version())
	}

	if got := client.Server(); got.Name != "pn-scripts-assistant" || got.Version != "stand-in" {
		t.Errorf("who it says it is: %+v", got)
	}

	if !strings.Contains(client.Instructions(), "PN Scripts Assistant") {
		t.Errorf("instructions: %q", client.Instructions())
	}

	tools, err := client.Tools(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var names []string

	for _, tool := range tools {
		names = append(names, tool.Name)

		// The one tool a yes through which lets another program act says
		// nothing of only reading; every other says it.
		if tool.Name == "permission_prompt" {
			if tool.Annotations != nil && tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint {
				t.Errorf("%s says it only reads", tool.Name)
			}

			continue
		}

		// What hands the assistant something says it does not only read,
		// and only the one that runs a command says it may destroy.
		if handsOver := map[string]bool{"remember_fact": true, "add_reminder": true, "add_todo": true, "tell_user": true,
			"say_to_user": true, "run_action": true}; handsOver[tool.Name] {
			a := tool.Annotations

			if a == nil || a.ReadOnlyHint == nil || *a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint != (tool.Name == "run_action") {
				t.Errorf("%s hands something over and does not say so as a client reads it: %+v", tool.Name, a)
			}

			continue
		}

		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
			t.Errorf("%s does not say it only reads", tool.Name)
		}
	}

	if strings.Join(names, ",") != "assistant_status,list_programs,ask_assistant,search_memory,permission_prompt,"+
		"remember_fact,add_reminder,add_todo,tell_user,say_to_user,run_action" {
		t.Errorf("tools: %v", names)
	}

	result, err := client.CallTool(ctx, "list_programs", json.RawMessage(`{"search":"gimp"}`))
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError || len(result.Content) != 1 || !strings.HasSuffix(result.Content[0].Text, "\n- gimp 2.10.36 (deb)") {
		t.Errorf("the call: %+v", result)
	}

	// The assistant was told who asked: the name this client gives with
	// every request of the current protocol.
	if got, want := strings.Join(kept.all(), ", "), "list_programs by "+mcp.ClientName+"/"+mcp.ClientVersion; got != want {
		t.Errorf("who the assistant was told asked: %q, want %q", got, want)
	}

	// A tool that could not do it comes back as an answer marked failed.
	result, err = client.CallTool(ctx, "list_programs", json.RawMessage(`{"limit":5000}`))
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].Text, "limit is from 1 to 200") {
		t.Errorf("a refused call: %+v %v", result, err)
	}

	if err := client.Healthy(ctx); err != nil {
		t.Errorf("after a refused call: %v", err)
	}
}

// An older client's whole conversation, written to the program by hand the
// way Claude Desktop and Cursor write it, and the program ending when its
// input closes.
func TestAnOlderClientsConversationOverRealPipes(t *testing.T) {
	bin := standIn(t)
	ts, kept := running(t)

	cmd := exec.Command(bin)
	cmd.Env = []string{"PN_SCRIPTS_ASSISTANT_API_URL=" + ts.URL, "STANDIN_KEY_FILE=" + keyFile(t)}

	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	lines := bufio.NewReader(out)

	ask := func(line string) map[string]any {
		t.Helper()

		if _, err := io.WriteString(in, line+"\n"); err != nil {
			t.Fatal(err)
		}

		got, err := lines.ReadString('\n')
		if err != nil {
			t.Fatalf("no answer to %s: %v", line, err)
		}

		var answer struct {
			Result map[string]any `json:"result"`
		}

		if err := json.Unmarshal([]byte(got), &answer); err != nil || answer.Result == nil {
			t.Fatalf("answer to %s: %q", line, got)
		}

		return answer.Result
	}

	hello := ask(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{"roots":{}},"clientInfo":{"name":"claude-ai","version":"0.1.0"}}}`)
	if hello["protocolVersion"] != "2025-06-18" {
		t.Errorf("the handshake: %v", hello)
	}

	if _, err := io.WriteString(in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"); err != nil {
		t.Fatal(err)
	}

	if list := ask(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`); len(list["tools"].([]any)) != 11 {
		t.Errorf("the list has %d tools, not 11", len(list["tools"].([]any)))
	}

	status := ask(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"assistant_status","arguments":{}}}`)
	if text := status["content"].([]any)[0].(map[string]any)["text"].(string); !strings.Contains(text, "PN Scripts Assistant 1.1.0 is running") {
		t.Errorf("status: %v", status)
	}

	// The assistant was told who asked: the name given once, in the
	// handshake, still goes with a call made later.
	if got := strings.Join(kept.all(), ", "); got != "assistant_status by claude-ai/0.1.0" {
		t.Errorf("who the assistant was told asked: %q", got)
	}

	in.Close()

	ended := make(chan error, 1)

	go func() { ended <- cmd.Wait() }()

	select {
	case err := <-ended:
		if err != nil {
			t.Errorf("it ended with %v", err)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("it kept running after its input closed")
	}

	if rest, _ := io.ReadAll(lines); len(rest) != 0 {
		t.Errorf("it wrote more than its answers: %q", rest)
	}
}
