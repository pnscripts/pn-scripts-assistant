package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/mcpserve"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/pair"
	"pn-scripts-assistant/internal/brain/paths"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/vouch"
)

/*
 * The seal on the assistant's answers, through the real server and the
 * MCP server's real tools (the review of 1.2.0, note 3; package vouch).
 */

// answersKey is the key every test server of this package seals with
// (TestMain), and testAnswersKey reads it as the MCP server reads the
// account's file.
var answersKey = bytes.Repeat([]byte{8}, 32)

func testAnswersKey() ([]byte, error) { return answersKey, nil }

// asking sends one request with a number, as the MCP server does, and
// says whether the answer's seal holds under key.
func asking(t *testing.T, method, url, path, body string, key []byte) (int, string, bool) {
	t.Helper()

	number, err := vouch.Number()
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(method, url+path, strings.NewReader(body))
	req.Header.Set(vouch.AskHeader, number)

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := bare.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)

	var sent []byte
	if body != "" {
		sent = []byte(body)
	}

	return res.StatusCode, string(raw), vouch.Holds(key, number, method, path, sent, res.StatusCode, raw, res.Header.Get(vouch.SealHeader))
}

// Everything the MCP server reads is sealed: answers and the refusals of
// the door alike (and of the terms: the next test). Nothing else is, and
// nobody who did not ask sees a seal.
func TestEveryAnswerTheMCPServerReadsIsSealed(t *testing.T) {
	ts, _, _ := newServer(t)

	for _, c := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/v1/version", "", http.StatusOK},
		{"GET", "/api/terms", "", http.StatusOK},
		{"POST", "/api/outside/ask", `{"tool":"assistant_status","client":{"name":"a test"}}`, http.StatusOK},
		{"POST", "/api/outside/ask", `{"tool":"list_programs","client":{"name":"a test"}}`, http.StatusForbidden},
		{"POST", "/api/outside/ask", `{"tool":"read_file"}`, http.StatusBadRequest},
	} {
		status, said, holds := asking(t, c.method, ts.URL, c.path, c.body, answersKey)
		if status != c.status || !holds {
			t.Errorf("%s %s: %d (want %d), sealed %v: %s", c.method, c.path, status, c.status, holds, said)
		}

		if _, _, other := asking(t, c.method, ts.URL, c.path, c.body, bytes.Repeat([]byte{1}, 32)); other {
			t.Errorf("%s %s: the seal holds under another key", c.method, c.path)
		}
	}

	/*
	 * Only those three routes. A sealed answer is held in memory until it
	 * is whole, so a seal on every route would hold the stream of what the
	 * assistant is doing, or a large file, for any program that asked;
	 * and nothing else needs one.
	 */
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/outside/allow", `{"subject":"programs","allowed":true}`},
		{"GET", "/api/outside", ""},
		{"GET", "/api/programs", ""},
		{"GET", "/api/no-such-route", ""},
		{"GET", "/health", ""},
		{"POST", "/api/terms", `{}`},
		{"POST", "/api/v1/version", ""},
	} {
		if _, said, holds := asking(t, c.method, ts.URL, c.path, c.body, answersKey); holds {
			t.Errorf("%s %s was sealed: %s", c.method, c.path, said)
		}
	}

	// The stream is answered as a stream, at once, to one who asks for a
	// seal like to anybody: nothing of it is held back.
	number, _ := vouch.Number()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/pulse", nil)
	req.Header.Set(vouch.AskHeader, number)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	streamed, err := bare.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatalf("the stream, asked with a number, did not begin: %v", err)
	}

	if streamed.Header.Get(vouch.SealHeader) != "" || streamed.StatusCode != http.StatusOK {
		t.Errorf("the stream: %d, sealed %v", streamed.StatusCode, streamed.Header.Get(vouch.SealHeader) != "")
	}

	streamed.Body.Close()

	// Not asked for: no seal, on any answer.
	res, err := bare.Get(ts.URL + "/api/v1/version")
	if err != nil {
		t.Fatal(err)
	}

	res.Body.Close()

	if res.Header.Get(vouch.SealHeader) != "" {
		t.Error("an answer was sealed for somebody who did not ask")
	}
}

// Before the terms are accepted the refusal is sealed too, so the MCP
// server can say "its terms are not accepted" and not "something answered".
func TestTheRefusalBeforeTheTermsIsSealed(t *testing.T) {
	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := brain.New(db, config.Default(), root, filepath.Join(root, "brain.sqlite"), logger)

	ts := httptest.NewServer(New(b, logger).Handler())
	defer ts.Close()

	status, said, holds := asking(t, "POST", ts.URL, "/api/outside/ask", `{"tool":"assistant_status","client":{"name":"a test"}}`, answersKey)
	if status != http.StatusForbidden || !strings.Contains(said, `"terms":"not accepted"`) || !holds {
		t.Fatalf("before the terms: %d, sealed %v: %s", status, holds, said)
	}

	for _, each := range mcpserve.New(ts.URL, "test", testAnswersKey).Tools {
		if each.Name != "list_programs" {
			continue
		}

		if _, err := each.Run(t.Context(), json.RawMessage(`{}`), mcpserve.Caller{Name: "a test"}); err == nil ||
			!strings.Contains(err.Error(), "terms of use have not been accepted") {
			t.Errorf("as the MCP server says it: %v", err)
		}
	}
}

// A device elsewhere is given no seal, with or without a token: the key
// is for programs on this computer.
func TestNoSealForADeviceElsewhere(t *testing.T) {
	s := plain(t, config.ReachNetwork)
	s.Handler()

	number, _ := vouch.Number()

	// A paired device, so that its request is answered at all.
	offer, _, _ := s.paired.Offer(pair.Full)
	_, token, _ := s.paired.Accept(offer, "laptop", "192.168.1.9")

	for _, from := range []string{"192.168.1.9:4000", "10.0.0.7:4000"} {
		w := ask(s, "GET", "/api/v1/version", from, func(r *http.Request) {
			r.Header.Set(vouch.AskHeader, number)
			r.Header.Set("Authorization", "Bearer "+token)
		})

		if w.Code != http.StatusOK || w.Header().Get(vouch.SealHeader) != "" {
			t.Errorf("a device at %s: %d, given a seal: %v", from, w.Code, w.Header().Get(vouch.SealHeader) != "")
		}
	}

	if w := ask(s, "GET", "/api/v1/version", "127.0.0.1:5000", func(r *http.Request) { r.Header.Set(vouch.AskHeader, number) }); w.Header().Get(vouch.SealHeader) == "" {
		t.Error("a program on this computer was given no seal")
	}
}

/*
 * A server the program gave no key (it could not keep one) seals nothing,
 * and the MCP server then believes none of its answers; given the key
 * (Vouch), it is believed. A permission question is the one that matters:
 * with no seal it is a no that says why, and no card is put to anybody.
 */
func TestWithoutTheKeyTheMCPServerBelievesNothingAndWithItThePersonsYes(t *testing.T) {
	fixedAnswersKey = nil
	defer func() { fixedAnswersKey = answersKey }()

	ts, _, b := newServer(t)
	permitWaits(t, time.Minute)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	permit := func() string {
		for _, each := range mcpserve.New(ts.URL, "test", testAnswersKey).Tools {
			if each.Name == mcpserve.PermissionTool {
				text, err := each.Run(t.Context(), json.RawMessage(`{"tool_name":"Bash","input":{"command":"ls"}}`), mcpserve.Caller{Name: "claude-code", Version: "2.1.303"})
				if err != nil {
					t.Errorf("the permission tool failed instead of answering: %v", err)
				}

				return text
			}
		}

		return ""
	}

	// A real question, answered yes on its card.
	done := make(chan string, 1)
	go func() { done <- permit() }()

	card := permitCards(t, ts.URL, 1)[0]

	if code, body := post(t, ts.URL+"/api/approvals/"+itoa(card.ID)+"/approve", `{}`); code != http.StatusOK {
		t.Fatalf("approving: %d %s", code, body)
	}

	// Even the person's own yes is not passed on from a server that did
	// not seal it: the MCP server cannot know it was the assistant's.
	if said := <-done; !strings.Contains(said, `"behavior":"deny"`) || !strings.Contains(said, "not in a way that shows it is PN Scripts Assistant") {
		t.Fatalf("an unsealed yes was passed on: %s", said)
	}
}

func TestGivenItsKeyTheServerIsBelieved(t *testing.T) {
	fixedAnswersKey = nil
	defer func() { fixedAnswersKey = answersKey }()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	agreedTo(t, root)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(brain.New(db, config.Default(), root, filepath.Join(root, "brain.sqlite"), logger), logger)

	// As the program does when it starts: the key from its user's file.
	path := filepath.Join(t.TempDir(), "settings", vouch.FileName)

	key, err := vouch.Keep(path)
	if err != nil {
		t.Fatal(err)
	}

	srv.Vouch(key)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	status := func(read func() ([]byte, error)) (string, error) {
		return mcpserve.New(ts.URL, "test", read).Tools[0].Run(t.Context(), json.RawMessage(`{}`), mcpserve.Caller{Name: "a test"})
	}

	if text, err := status(func() ([]byte, error) { return vouch.Read(path) }); err != nil || !strings.Contains(text, "is running on this computer") {
		t.Errorf("with the key from the file: %q %v", text, err)
	}

	if text, err := status(testAnswersKey); err == nil {
		t.Errorf("with another key: %q", text)
	}
}

// The key's folder is the program's own in the account's settings folder
// (paths.Name), where its other file for this computer is.
func TestTheKeyIsKeptInTheProgramsSettingsFolder(t *testing.T) {
	place, err := vouch.Place()
	if err != nil {
		t.Skip("this account has no settings folder")
	}

	if filepath.Base(filepath.Dir(place)) != paths.Name || filepath.Base(place) != vouch.FileName {
		t.Errorf("the key is kept at %s, the program's folder is %s", place, paths.Name)
	}
}
