package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Two servers that refuse what the real ones refuse.
 *
 * A fake that accepts anything proves nothing, and that is precisely how this
 * went unnoticed: every test passed against a provider written to be agreeable,
 * while Anthropic quietly dropped the evidence and OpenAI refused the request
 * outright. So these enforce the rules the documented APIs enforce, and
 * TestTheStrictServersWouldHaveCaughtTheOldShape proves they are strict enough
 * to have caught the bug they exist for.
 */

// refuse answers the way both services do when a conversation is malformed.
func refuse(w http.ResponseWriter, because string) {
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `{"error":{"message":%q}}`, because)
}

type anthropicBody struct {
	System   string `json:"system"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Name string `json:"name"`
	} `json:"tools"`
}

type sentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   string          `json:"content"`
}

// checkAnthropic returns why a conversation would be refused, or "".
func checkAnthropic(b anthropicBody) string {
	owed := map[string]bool{}

	for i, m := range b.Messages {
		var blocks []sentBlock

		if len(m.Content) > 0 && m.Content[0] == '[' {
			if err := json.Unmarshal(m.Content, &blocks); err != nil {
				return "content blocks that will not parse"
			}
		} else {
			var text string

			if err := json.Unmarshal(m.Content, &text); err != nil {
				return "content that is neither a string nor blocks"
			}

			if strings.TrimSpace(text) == "" {
				return fmt.Sprintf("message %d has empty content", i)
			}

			if text != strings.TrimRight(text, " \t\r\n") && i == len(b.Messages)-1 &&
				m.Role == "assistant" {
				return "final assistant content cannot end with trailing whitespace"
			}
		}

		answering := map[string]bool{}

		for _, blk := range blocks {
			switch blk.Type {
			case "tool_use":
				if len(b.Tools) == 0 {
					return "a tool_use with no tools declared"
				}

				if blk.ID == "" {
					return "a tool_use with no id"
				}

				if len(blk.Input) == 0 || blk.Input[0] != '{' {
					return fmt.Sprintf("tool_use input is not an object: %s", blk.Input)
				}

				owed[blk.ID] = true

			case "tool_result":
				if !owed[blk.ToolUseID] {
					return fmt.Sprintf("tool_result %q answers no tool_use in the turn before it", blk.ToolUseID)
				}

				answering[blk.ToolUseID] = true
			}
		}

		// Every call must be answered in the very next message.
		if len(answering) > 0 {
			for id := range answering {
				delete(owed, id)
			}
		}

		if len(owed) > 0 && len(answering) == 0 && i > 0 {
			// The message after the calls did not answer them.
			if b.Messages[i].Role != "assistant" {
				for id := range owed {
					return fmt.Sprintf("tool_use %q was never answered", id)
				}
			}
		}
	}

	return ""
}

func strictAnthropic(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b anthropicBody

		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			refuse(w, "unreadable body")

			return
		}

		if why := checkAnthropic(b); why != "" {
			refuse(w, why)

			return
		}

		// A tool_result anywhere means the tool has run, so answer in words.
		if strings.Contains(string(mustJSON(b.Messages)), "tool_result") {
			fmt.Fprint(w, `{"model":"claude-test","content":[{"type":"text","text":"There is one file, a.txt."}]}`)

			return
		}

		fmt.Fprintf(w, `{"model":"claude-test","content":[{"type":"tool_use","id":"toolu_1","name":"list_directory","input":{"path":%q}}]}`,
			r.Header.Get("X-Test-Dir"))
	}))
}

type openAIBody struct {
	Messages []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"messages"`
	Tools []json.RawMessage `json:"tools"`
}

// checkOpenAI returns why a conversation would be refused, or "".
func checkOpenAI(b openAIBody) string {
	declared := map[string]bool{}

	for i, m := range b.Messages {
		for _, c := range m.ToolCalls {
			if m.Role != "assistant" {
				return "tool_calls on a message that is not the assistant's"
			}

			if c.Type != "function" {
				return fmt.Sprintf("tool call of type %q", c.Type)
			}

			// The one that broke every tool: these services write arguments
			// as a string containing JSON, and expect them back that way.
			if len(c.Function.Arguments) == 0 || c.Function.Arguments[0] != '"' {
				return fmt.Sprintf("tool call arguments must be a string, got %s", c.Function.Arguments)
			}

			declared[c.ID] = true
		}

		if m.Role != "tool" {
			continue
		}

		if m.ToolCallID == "" {
			return fmt.Sprintf("message %d has role 'tool' with no tool_call_id", i)
		}

		if !declared[m.ToolCallID] {
			return "messages with role 'tool' must be a response to a preceding " +
				"message with 'tool_calls'"
		}
	}

	return ""
}

func strictOpenAI(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b openAIBody

		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			refuse(w, "unreadable body")

			return
		}

		if why := checkOpenAI(b); why != "" {
			refuse(w, why)

			return
		}

		for _, m := range b.Messages {
			if m.Role == "tool" {
				fmt.Fprint(w, `{"choices":[{"message":{"content":"There is one file, a.txt."}}]}`)

				return
			}
		}

		// Arguments go out as a string containing JSON, which is what these
		// services really send and what used to reach a tool still quoted.
		args := fmt.Sprintf(`{"path":%q}`, r.Header.Get("X-Test-Dir"))

		fmt.Fprintf(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_abc","type":"function","function":{"name":"list_directory","arguments":%q}}]}}]}`,
			args)
	}))
}

func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)

	return raw
}

// dirHeader puts the temporary directory where the fake models can find it, so
// the tool they ask for has something real to read.
type dirHeader struct {
	inner http.RoundTripper
	dir   string
}

func (d dirHeader) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("X-Test-Dir", d.dir)

	return d.inner.RoundTrip(r)
}

/*
 * A hosted model uses a tool and then answers from what came back.
 *
 * The turn this program could not do. Two rounds against a server that refuses
 * the same conversations the real one refuses: the model asks for a tool, the
 * loop runs it, and the result goes back with the message that asked for it.
 * Before, Anthropic was handed a result answering nothing and asked its
 * question again, and OpenAI refused the second round outright.
 */
func TestAHostedModelCanUseAToolAndThenAnswer(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name  string
		build func(base string) llm.Provider
	}{
		{"anthropic", func(base string) llm.Provider {
			a := llm.NewAnthropic("sk-ant-test", "claude-test")
			a.BaseURL = base
			a.HTTPClient = &http.Client{Transport: dirHeader{http.DefaultTransport, dir}}

			return a
		}},
		{"openai-compatible", func(base string) llm.Provider {
			o := llm.NewOpenAI("sk-test", "gpt-test")
			o.BaseURL = base + "/v1"
			o.HTTPClient = &http.Client{Transport: dirHeader{http.DefaultTransport, dir}}

			return o
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var server *httptest.Server

			if c.name == "anthropic" {
				server = strictAnthropic(t)
			} else {
				server = strictOpenAI(t)
			}

			defer server.Close()

			loop, db := newLoop(t, tools.ListDirectory{})
			conv, _ := db.NewConversation("t")

			res, err := loop.Run(context.Background(), conv, c.build(server.URL), nil)
			if err != nil {
				t.Fatalf("the conversation was refused: %v", err)
			}

			if len(res.ActionsTaken) != 1 {
				t.Fatalf("the tool did not run: %v", res.ActionsTaken)
			}

			// It ran with arguments it could actually read — the OpenAI shape
			// used to arrive still wrapped in its own quotes and fail here.
			if len(res.Steps) != 1 || res.Steps[0].Failed {
				t.Fatalf("the tool failed: %+v", res.Steps)
			}

			if !strings.Contains(res.Steps[0].Result, "a.txt") {
				t.Errorf("the tool read the wrong thing: %q", res.Steps[0].Result)
			}

			if !strings.Contains(res.Reply, "a.txt") {
				t.Errorf("the model did not answer from what came back: %q", res.Reply)
			}
		})
	}
}

/*
 * And the servers above are strict enough to have caught it.
 *
 * Without this, a fake that shrugged at the old shape would let the whole test
 * above pass whether or not anything was fixed.
 */
func TestTheStrictServersWouldHaveCaughtTheOldShape(t *testing.T) {
	// What the loop used to send: a result, and no message asking for it.
	orphaned := []llm.Message{
		{Role: llm.RoleUser, Content: "read it"},
		{Role: llm.RoleTool, ToolCallID: "c1", Content: "hello"},
	}

	var openai openAIBody

	openai.Messages = append(openai.Messages, struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}{Role: "tool", Content: json.RawMessage(`"hello"`), ToolCallID: "c1"})

	if why := checkOpenAI(openai); why == "" {
		t.Error("the OpenAI server accepted a tool result answering nothing")
	}

	// And Anthropic refuses a tool_result that answers no tool_use.
	var anth anthropicBody

	anth.Messages = append(anth.Messages, struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"c1","content":"hello"}]`)})

	if why := checkAnthropic(anth); why == "" {
		t.Error("the Anthropic server accepted a tool_result answering nothing")
	}

	_ = orphaned
}
