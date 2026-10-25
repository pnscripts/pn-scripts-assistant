package server

import (
	"net/http"
	"strconv"
	"testing"

	"pn-scripts-assistant/internal/brain/store"
)

func TestDraftsAreChangedDiscardedAndRecordedWhenSent(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no Thunderbird, and never the real one

	ts, db, _ := newServer(t)

	var digest struct {
		None bool `json:"none"`
	}

	getJSON(t, ts.URL+"/api/mail/digest", &digest)

	if !digest.None {
		t.Errorf("with no mailbox the digest did not say so: %+v", digest)
	}

	a, _ := db.AddDraft("anna@example.com", "Re: Offer", "Thanks.", "Anna — Offer", "")
	b, _ := db.AddDraft("bob@example.com", "Hello", "Hi.", "", "")

	var changed store.Draft

	if code := sendJSON(t, ts.URL+"/api/mail/drafts/"+strconv.FormatInt(a.ID, 10),
		map[string]any{"to": "anna@example.com", "subject": "Re: Offer", "body": "Thank you, Anna."}, &changed); code != http.StatusOK || changed.Body != "Thank you, Anna." {
		t.Fatalf("%d %+v", code, changed)
	}

	if code := sendJSON(t, ts.URL+"/api/mail/drafts/"+strconv.FormatInt(b.ID, 10)+"/discard", nil, nil); code != http.StatusOK {
		t.Errorf("discarding answered %d", code)
	}

	// No account: the press is refused, recorded, and the draft still waits.
	if code := sendJSON(t, ts.URL+"/api/mail/send/"+strconv.FormatInt(a.ID, 10), nil, nil); code != http.StatusBadRequest {
		t.Errorf("sending with no account answered %d", code)
	}

	var list struct {
		Waiting []store.Draft `json:"waiting"`
		Done    []store.Draft `json:"done"`
		CanSend bool          `json:"can_send"`
	}

	getJSON(t, ts.URL+"/api/mail/drafts", &list)

	if len(list.Waiting) != 1 || list.Waiting[0].ID != a.ID || len(list.Done) != 1 || list.CanSend {
		t.Errorf("%+v", list)
	}

	deeds, _ := db.Deeds(store.DeedFilter{Limit: 10})
	if len(deeds) != 1 || deeds[0].Tool != "send_draft" || deeds[0].Outcome != "failed" || !deeds[0].Changes {
		t.Errorf("the press was not recorded: %+v", deeds)
	}
}
