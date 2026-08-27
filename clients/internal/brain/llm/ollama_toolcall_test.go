package llm

import "testing"

// A tool call written as prose must still be a tool call.
//
// The payload in the first case is the one qwen2.5-coder:7b actually returned
// through Ollama when asked to list a directory: tool_calls empty, and the call
// itself sitting in the message content as text. Reading only the structured
// field threw it away, and the brain then told its owner it could not reach
// their files while holding a tool that reads any file on the machine.
func TestToolCallWrittenAsText(t *testing.T) {
	offered := []ToolSpec{{Name: "list_directory"}, {Name: "read_file"}}

	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "bare object, as observed",
			content: `{"name": "list_directory", "arguments": {"path": "/tmp"}}`,
			want:    "list_directory",
		},
		{
			name:    "wrapped in the tags some templates use",
			content: `<tool_call>{"name": "read_file", "arguments": {"path": "/etc/hosts"}}</tool_call>`,
			want:    "read_file",
		},
		{
			name:    "fenced as code",
			content: "```json\n{\"name\": \"read_file\", \"arguments\": {\"path\": \"/etc/hosts\"}}\n```",
			want:    "read_file",
		},
		{
			name:    "arguments under the other spelling",
			content: `{"name": "list_directory", "parameters": {"path": "/tmp"}}`,
			want:    "list_directory",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			call, ok := toolCallInText(c.content, offered)

			if !ok {
				t.Fatalf("no call recovered from %q", c.content)
			}

			if call.Name != c.want {
				t.Errorf("recovered %q, want %q", call.Name, c.want)
			}

			if len(call.Arguments) == 0 {
				t.Error("the call arrived with no arguments")
			}
		})
	}
}

// Ordinary replies must not be mistaken for calls.
//
// This is the reason the reading is strict. A brain that runs any JSON object
// its model happens to write is far worse than one that occasionally misses a
// call, because the failure is silent and it acts.
func TestProseIsNotMistakenForAToolCall(t *testing.T) {
	offered := []ToolSpec{{Name: "list_directory"}}

	cases := []struct {
		name    string
		content string
	}{
		{"a plain answer", "I had a look and there are eleven files in there."},
		{"a tool nobody offered", `{"name": "delete_everything", "arguments": {}}`},
		{
			name:    "an answer that happens to explain JSON",
			content: `A call looks like {"name": "list_directory", "arguments": {"path": "/tmp"}} in the docs.`,
		},
		{"an object that is not a call", `{"files": 11, "path": "/tmp"}`},
		{"nothing at all", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := toolCallInText(c.content, offered); ok {
				t.Errorf("treated as a tool call: %q", c.content)
			}
		})
	}
}
