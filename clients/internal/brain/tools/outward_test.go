package tools

import (
	"encoding/json"
	"reflect"
	"testing"

	"pn-scripts-assistant/internal/brain/rules"
)

type mutatingFake struct{ fake }

func (mutatingFake) Risk() Risk { return Mutating }

func TestCallsAreReadForWhatTheyDoOutward(t *testing.T) {
	cases := []struct {
		name string
		tool Tool
		args string
		want []string
	}{
		{"an email", SendEmail{}, `{"to":"a@b.c","subject":"s","body":"b"}`, []string{rules.Send}},
		{"a draft", SendDraft{}, `{"id":3}`, []string{rules.Send}},
		{"a file", WriteFile{}, `{"path":"/tmp/x.txt","content":"x"}`, []string{rules.Files}},
		{"a commit", RunCommand{}, `{"argv":["git","commit","-m","x"]}`, []string{rules.Commit}},
		{"a push in a shell", RunCommand{}, `{"argv":["bash","-c","git push origin main"]}`, []string{rules.Commit}},
		{"a deletion, which changes files too", RunCommand{}, `{"argv":["rm","-rf","/tmp/old"]}`, []string{rules.Files, rules.Delete}},
		{"a listing", RunCommand{}, `{"argv":["ls","-l"]}`, nil},
		{"a browser that pays", DriveBrowser{}, `{"action":"click","expression":"Pay now"}`, []string{rules.Purchase}},
		{"a browser that posts", DriveBrowser{}, `{"action":"click","expression":"Публикувай"}`, []string{rules.Post}},
		{"an MCP tool that sends", mutatingFake{fake{name: "mcp__chat__send_message"}}, `{}`, []string{rules.Send}},
		{"an MCP tool that commits", mutatingFake{fake{name: "github.git_commit"}}, `{}`, []string{rules.Commit}},
		{"an MCP tool that writes", mutatingFake{fake{name: "fs_write_file"}}, `{}`, []string{rules.Files}},
		{"a tool that only reads", fake{name: "send_report_preview"}, `{}`, nil},
	}

	for _, c := range cases {
		if got := ActionsOf(c.tool, json.RawMessage(c.args)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
