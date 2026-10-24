package tools

import (
	"encoding/json"
	"testing"
)

// Where a call sends something is read from its own arguments.
func TestReachesIsReadFromTheCall(t *testing.T) {
	cases := []struct {
		tool Tool
		args string
		want string
	}{
		{ReadAPage{}, `{"url":"https://www.example.org/a/b"}`, "example.org"},
		{FetchURL{}, `{"url":"http://docs.example.com/x"}`, "docs.example.com"},
		{SendEmail{}, `{"to":"Ana <ana@firm.example>, b@x.example","subject":"s","body":"b"}`, "mail to firm.example"},
		{WebSearch{}, `{"query":"vat"}`, "web search"},
		{ReadFile{}, `{"path":"/etc/hosts"}`, ""},
		{WriteFile{}, `{"path":"/tmp/x","content":"y"}`, ""},
	}

	for _, c := range cases {
		if got := Reaches(c.tool, json.RawMessage(c.args)); got != c.want {
			t.Errorf("%s: %q, expected %q", c.tool.Name(), got, c.want)
		}
	}
}

// The Control page's question, asked of a capability with no call yet.
func TestLeavesSaysWhichCapabilitiesCanSendAnything(t *testing.T) {
	for _, t2 := range []Tool{WebSearch{}, ReadAPage{}, SendEmail{}} {
		if leaves, what := Leaves(t2); !leaves || what == "" {
			t.Errorf("%s says it stays here", t2.Name())
		}
	}

	for _, t2 := range []Tool{ReadFile{}, WriteFile{}, ListDirectory{}} {
		if leaves, _ := Leaves(t2); leaves {
			t.Errorf("%s says it can send something", t2.Name())
		}
	}
}
