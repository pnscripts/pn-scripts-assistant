package mcp

import "testing"

// What an approval card shows has one fingerprint: anything on the card
// that changes, changes it; who may use it and whether it runs do not.
func TestWhatACardShowsHasAFingerprint(t *testing.T) {
	base := Server{ID: "demo", Title: "Demo", Transport: "stdio", Command: []string{"npx", "-y", "server-x"},
		Env: map[string]string{"A": "1", "B": "2"}, Secrets: []string{"API_KEY"}}

	same := base
	same.Env = map[string]string{"B": "2", "A": "1"}
	same.Approved, same.Active, same.Agents, same.ReadOnly = true, true, []string{"assistant"}, []string{"echo"}

	if base.Shown() == "" || base.Shown() != same.Shown() {
		t.Errorf("the same card has two fingerprints: %q, %q", base.Shown(), same.Shown())
	}

	for what, change := range map[string]func(*Server){
		"the command":      func(s *Server) { s.Command = []string{"npx", "-y", "server-y"} },
		"a word moved":     func(s *Server) { s.Command = []string{"npx", "-yserver-x"} },
		"a setting":        func(s *Server) { s.Env = map[string]string{"A": "1", "B": "3"} },
		"a setting's name": func(s *Server) { s.Env = map[string]string{"A": "1", "C": "2"} },
		"a secret's name":  func(s *Server) { s.Secrets = []string{"OTHER_KEY"} },
		"the address":      func(s *Server) { s.Transport, s.URL, s.Command = "http", "https://mcp.example.com/mcp", nil },
		"the header":       func(s *Server) { s.Header = "X-Api-Key" },
		"reaching out":     func(s *Server) { s.Network = true },
		"where it is from": func(s *Server) { s.From = &Origin{Client: "cursor", File: "/c.json", Name: "demo", Print: "abc"} },
	} {
		changed := base
		change(&changed)

		if changed.Shown() == base.Shown() {
			t.Errorf("%s changed and the fingerprint did not", what)
		}
	}
}
