package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const mcpMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"a test","version":"1"}}`

// "pn-scripts-assistant mcp" answers another program on its standard
// streams: messages on the output and nothing else there.
func TestTheMCPCommandAnswersOnItsStreams(t *testing.T) {
	running := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			fmt.Fprint(w, `{"product":"PN Scripts Assistant","version":"1.1.0","platform":"linux/amd64"}`)
		case "/api/terms":
			fmt.Fprint(w, `{"accepted":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer running.Close()

	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{` + mcpMeta + `}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"assistant_status","arguments":{},` + mcpMeta + `}}` + "\n")

	var out, said bytes.Buffer

	if err := serveMCP(context.Background(), []string{"--api", running.URL}, in, &out, &said); err != nil {
		t.Fatalf("%v (%s)", err, said.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("two requests, %d lines: %q", len(lines), out.String())
	}

	var all string

	for _, line := range lines {
		var message struct {
			JSONRPC string         `json:"jsonrpc"`
			Result  map[string]any `json:"result"`
		}

		if err := json.Unmarshal([]byte(line), &message); err != nil || message.JSONRPC != "2.0" || message.Result == nil {
			t.Errorf("not an answer: %q", line)
		}

		all += line
	}

	if !strings.Contains(all, `"name":"pn-scripts-assistant"`) || !strings.Contains(all, "PN Scripts Assistant 1.1.0 is running on this computer") {
		t.Errorf("what it answered: %s", all)
	}
}

// An address that is not this computer is refused before anything is read
// or written, and the reason goes where a person would look.
func TestTheMCPCommandOnlyAsksThisComputer(t *testing.T) {
	for _, args := range [][]string{{"--api", "http://192.168.1.20:8790"}, {"--api", "https://example.org"}, {"--no-such-flag"}, {"extra"}} {
		var out, said bytes.Buffer

		err := serveMCP(context.Background(), args, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &out, &said)
		if err == nil {
			t.Errorf("%v was accepted", args)
		}

		if out.Len() != 0 {
			t.Errorf("%v: the output carries something that is not a message: %q", args, out.String())
		}
	}
}

func TestTheHelpNamesTheMCPCommand(t *testing.T) {
	var help bytes.Buffer

	usageTo(&help)

	if !strings.Contains(help.String(), "pn-scripts-assistant mcp ") {
		t.Errorf("the help does not say how other programs reach it:\n%s", help.String())
	}
}
