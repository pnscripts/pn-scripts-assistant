package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/mcp"
)

/*
 * A yes is for what was on the card.
 *
 * Found by the 1.1.0 security review (F10): Import and Approve named the
 * entry and nothing else, so an entry rewritten between the page drawing
 * its card and the click (by the other program, or by anything that can
 * write its settings) was imported or approved as it then was, unseen. The
 * click now carries the fingerprint of what was shown, and a changed entry
 * answers 409 and changes nothing.
 */
func TestAnEntryChangedSinceItWasShownIsNotImportedOrApproved(t *testing.T) {
	home := otherProgramsHome(t)
	settings := filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")

	ts, _, b := newServer(t)

	type listing struct {
		Found        []brain.MCPFound `json:"found"`
		Integrations []mcp.View       `json:"integrations"`
	}

	memory := func() brain.MCPFound {
		t.Helper()

		var l listing

		getJSON(t, ts.URL+"/api/integrations", &l)

		for _, f := range l.Found {
			if f.Name == "memory" {
				return f
			}
		}

		t.Fatal("memory is not listed")

		return brain.MCPFound{}
	}

	view := func(id string) mcp.View {
		t.Helper()

		var l listing

		getJSON(t, ts.URL+"/api/integrations", &l)

		for _, v := range l.Integrations {
			if v.ID == id {
				return v
			}
		}

		t.Fatalf("%s is not listed", id)

		return mcp.View{}
	}

	shown := memory()
	if shown.Print == "" {
		t.Fatal("the listing carries no fingerprint")
	}

	// Without saying what was shown, nothing is imported.
	if code, body := postBody(t, ts.URL+"/api/integrations/import", `{"key":`+quote(shown.Key)+`}`); code != 400 {
		t.Fatalf("imported without a fingerprint: %d %v", code, body)
	}

	// Rewritten after the card was drawn.
	if err := os.WriteFile(settings, []byte(`{"mcpServers": {"memory": {"command": "npx", "args": ["-y", "something-else"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, body := postBody(t, ts.URL+"/api/integrations/import", `{"key":`+quote(shown.Key)+`,"print":`+quote(shown.Print)+`}`)
	if code != 409 || !strings.Contains(body["error"].(string), "changed") {
		t.Fatalf("a changed entry: %d %v, want 409", code, body)
	}

	for _, s := range b.Integrations.Book.Servers() {
		if s.From != nil {
			t.Fatalf("something was imported all the same: %+v", s)
		}
	}

	// Looked at again, it is imported as it is now.
	now := memory()
	if now.Print == shown.Print {
		t.Fatal("the changed entry has the old fingerprint")
	}

	code, body = postBody(t, ts.URL+"/api/integrations/import", `{"key":`+quote(now.Key)+`,"print":`+quote(now.Print)+`}`)
	if code != 200 {
		t.Fatalf("import: %d %v", code, body)
	}

	id := body["imported"].(string)

	// Approve: the card shown, then the integration changes under it.
	card := view(id)
	if card.Shown == "" || card.Approved {
		t.Fatalf("the card carries no fingerprint, or is approved: %+v", card)
	}

	if code, body := postBody(t, ts.URL+"/api/integrations/"+id+"/approve", `{"agents":["assistant"]}`); code != 400 {
		t.Fatalf("approved without a fingerprint: %d %v", code, body)
	}

	if _, err := b.Integrations.Book.Change(id, func(s *mcp.Server) { s.Command = []string{"npx", "-y", "a-third-thing"} }); err != nil {
		t.Fatal(err)
	}

	code, body = postBody(t, ts.URL+"/api/integrations/"+id+"/approve", `{"agents":["assistant"],"shown":`+quote(card.Shown)+`}`)
	if code != 409 || !strings.Contains(body["error"].(string), "changed") {
		t.Fatalf("a changed integration: %d %v, want 409", code, body)
	}

	if s, _ := b.Integrations.Book.Get(id); s.Approved {
		t.Fatal("approved all the same")
	}

	card = view(id)

	code, body = postBody(t, ts.URL+"/api/integrations/"+id+"/approve", `{"agents":["assistant"],"shown":`+quote(card.Shown)+`}`)
	if code != 200 {
		t.Fatalf("approve: %d %v", code, body)
	}

	if s, _ := b.Integrations.Book.Get(id); !s.Approved {
		t.Fatal("not approved")
	}

	// One from the catalogue, never added: approved from its card too.
	fromCatalogue := view("memory")
	if fromCatalogue.Shown == "" {
		t.Fatal("a catalogue card carries no fingerprint")
	}

	if code, body := postBody(t, ts.URL+"/api/integrations/memory/approve", `{"shown":`+quote(fromCatalogue.Shown)+`}`); code != 200 {
		t.Fatalf("approving a catalogue integration: %d %v", code, body)
	}
}
