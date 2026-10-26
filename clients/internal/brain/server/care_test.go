package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

// The Customers page: the profile and the answer book are kept, a reply is
// written from them and checked, and keeping it files a draft that waits.
func TestTheCustomersPage(t *testing.T) {
	ts, db, b := newServer(t)

	b.Router = nil

	if code := sendJSON(t, ts.URL+"/api/care/reply", map[string]any{"message": "Do you deliver?"}, nil); code != http.StatusServiceUnavailable {
		t.Errorf("a reply with no model answered %d", code)
	}

	folder := t.TempDir()
	_ = os.WriteFile(filepath.Join(folder, "prices.md"), []byte("Delivery to Plovdiv costs 6 leva."), 0o600)

	if code := sendJSON(t, ts.URL+"/api/care/profile", map[string]any{"name": "Green Shop", "tone": "loud"}, nil); code != http.StatusBadRequest {
		t.Errorf("an unknown tone answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/care/profile", map[string]any{"name": "Green Shop", "signature": "Maria", "tone": "formal", "folder": folder}, nil); code != http.StatusOK {
		t.Fatalf("the profile answered %d", code)
	}

	var added struct {
		ID int64 `json:"id"`
	}

	if code := sendJSON(t, ts.URL+"/api/care/answers", map[string]any{"question": "Opening hours?", "answer": "9 to 6."}, &added); code != http.StatusOK || added.ID == 0 {
		t.Fatalf("%d %+v", code, added)
	}

	if code := sendJSON(t, ts.URL+"/api/care/answers", map[string]any{"id": 99, "question": "q", "answer": "a"}, nil); code != http.StatusNotFound {
		t.Errorf("changing a missing answer answered %d", code)
	}

	var page struct {
		Profile struct{ Name, Tone string } `json:"profile"`
		Answers []map[string]any            `json:"answers"`
		Files   int                         `json:"files"`
	}

	getJSON(t, ts.URL+"/api/care", &page)

	if page.Profile.Name != "Green Shop" || page.Profile.Tone != "formal" || len(page.Answers) != 1 || page.Files != 1 {
		t.Fatalf("%+v", page)
	}

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", scribe{reply: "Dear customer, delivery to Plovdiv costs 6 leva [1]. " +
		"We will also give you a discount. [[ASK: Saturday delivery]]"})
	b.Cfg.DefaultProvider = "ollama"

	var got struct {
		Reply struct {
			Text     string   `json:"text"`
			Gaps     []string `json:"gaps"`
			Promises []string `json:"promises"`
			BasedOn  []any    `json:"based_on"`
		} `json:"reply"`
		Ready bool `json:"ready"`
	}

	if code := sendJSON(t, ts.URL+"/api/care/reply", map[string]any{"message": "How much is delivery to Plovdiv?"}, &got); code != http.StatusOK {
		t.Fatalf("the reply answered %d", code)
	}

	if got.Ready || len(got.Reply.Gaps) != 1 || len(got.Reply.Promises) != 1 || len(got.Reply.BasedOn) != 1 ||
		!strings.HasSuffix(got.Reply.Text, "Maria") || strings.Contains(got.Reply.Text, "[1]") {
		t.Errorf("%+v", got)
	}

	if code := sendJSON(t, ts.URL+"/api/care/keep", map[string]any{"body": got.Reply.Text}, nil); code != http.StatusBadRequest {
		t.Errorf("a draft with nobody to send it to answered %d", code)
	}

	var kept struct {
		Draft struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"draft"`
		Gaps []string `json:"gaps"`
	}

	if code := sendJSON(t, ts.URL+"/api/care/keep", map[string]any{"to": "ivan@example.com", "subject": "Delivery", "body": got.Reply.Text}, &kept); code != http.StatusOK ||
		kept.Draft.Status != "waiting" || len(kept.Gaps) != 1 {
		t.Fatalf("%d %+v", code, kept)
	}

	// It cannot be sent with the gap in it.
	if code := sendJSON(t, ts.URL+"/api/mail/send/"+strconv.FormatInt(kept.Draft.ID, 10), map[string]any{}, nil); code == http.StatusOK {
		t.Error("a draft with a gap was sent")
	}

	if dr, _ := db.Draft(kept.Draft.ID); dr.Status != "waiting" {
		t.Errorf("%+v", dr)
	}

	if code := sendJSON(t, ts.URL+"/api/care/answers/"+strconv.FormatInt(added.ID, 10)+"/remove", map[string]any{}, nil); code != http.StatusOK {
		t.Errorf("remove answered %d", code)
	}
}
