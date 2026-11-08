package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/mail"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * A draft's last look travels in the call: the question shows the polished
 * words, the pin keeps them, and the draft itself keeps its own words until
 * it is really sent — a draft that is refused, declined or cannot be sent
 * is not rewritten behind its owner's back.
 */
func TestADraftIsPolishedInTheCallNotInTheDraft(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	original := "Hi Boris,\n\nI hope this email finds you well. The invoice is attached."

	dr, err := db.AddDraft("boris@example.org", "Invoice", original, "", "")
	if err != nil {
		t.Fatal(err)
	}

	tool := SendDraft{Desk: MailDesk{Drafts: db, Account: func() mail.Account { return mail.Account{} }}}

	args, r := tool.Polish(json.RawMessage(`{"id":` + itoa64(dr.ID) + `}`))
	if !r.Changed {
		t.Fatal("nothing was polished")
	}

	if q := tool.Summarize(args); !strings.Contains(q, "Hi Boris,\n\nThe invoice is attached.") || strings.Contains(q, "finds you well") {
		t.Errorf("the question does not show the polished words:\n%s", q)
	}

	pinned := tool.Pin(args)

	var a draftArgs
	_ = json.Unmarshal(pinned, &a)

	if a.Seen == "" || a.Polished != "Hi Boris,\n\nThe invoice is attached." {
		t.Errorf("the pin lost the polished words: %s", pinned)
	}

	// Not sent (no account): the draft keeps its own words.
	if _, err := tool.Execute(context.Background(), pinned); err == nil {
		t.Fatal("sent without an account")
	}

	if kept, _ := db.Draft(dr.ID); kept.Body != original {
		t.Errorf("the draft was rewritten although nothing was sent: %q", kept.Body)
	}
}

func itoa64(n int64) string {
	b, _ := json.Marshal(n)

	return string(b)
}
